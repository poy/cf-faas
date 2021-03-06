package handlers_test

import (
	"context"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/poy/cf-faas"
	"github.com/poy/cf-faas/internal/handlers"
	"github.com/poy/cf-faas/internal/internalapi"
	"github.com/poy/onpar"
	. "github.com/poy/onpar/expect"
	. "github.com/poy/onpar/matchers"
)

type TE struct {
	*testing.T
	h        http.Handler
	recorder *httptest.ResponseRecorder

	spyWorkSubmitter *spyWorkSubmitter
	spyRelayer       *spyRelayer
}

func TestHTTPEvent(t *testing.T) {
	t.Parallel()
	o := onpar.New()
	defer o.Run(t)

	o.BeforeEach(func(t *testing.T) TE {
		spyRelayer := newSpyRelayer()
		spyWorkSubmitter := newSpyWorkSubmitter()
		return TE{
			T:                t,
			recorder:         httptest.NewRecorder(),
			spyRelayer:       spyRelayer,
			spyWorkSubmitter: spyWorkSubmitter,
			h: handlers.NewHTTPEvent(
				"some-command",
				"some-app",
				spyRelayer,
				spyWorkSubmitter,
				log.New(ioutil.Discard, "", 0),
			),
		}
	})

	o.Spec("submits the relayer's addr for work", func(t TE) {
		u, err := url.Parse("http://some.addr")
		Expect(t, err).To(BeNil())

		t.spyRelayer.u = u
		req, err := http.NewRequest("GET", "http://some.url", nil)
		Expect(t, err).To(BeNil())

		t.h.ServeHTTP(t.recorder, req)
		Expect(t, t.spyWorkSubmitter.w).To(Equal(internalapi.Work{
			Href:    u.String(),
			Command: "some-command",
			AppName: "some-app",
		}))
	})

	o.Spec("relayer should be given the request", func(t TE) {
		req, err := http.NewRequest("GET", "http://some.url", nil)
		Expect(t, err).To(BeNil())

		t.h.ServeHTTP(t.recorder, req)

		Expect(t, t.spyRelayer.r.URL).To(Equal(req.URL))
	})

	o.Spec("it should return the faas.Response", func(t TE) {
		t.spyRelayer.resp = faas.Response{
			StatusCode: 234,
			Header: http.Header{
				"A": []string{"x", "y"},
				"B": []string{"z"},
			},
			Body: []byte("some-data"),
		}

		req, err := http.NewRequest("GET", "http://some.url", nil)
		Expect(t, err).To(BeNil())

		t.h.ServeHTTP(t.recorder, req)
		Expect(t, t.recorder.Code).To(Equal(234))
		Expect(t, t.recorder.Header()).To(Equal(t.spyRelayer.resp.Header))
		Expect(t, t.recorder.Body.String()).To(Equal("some-data"))
	})

	o.Spec("it should return a 500 if the task fails", func(t TE) {
		t.spyRelayer.respErr = errors.New("some-error")

		req, err := http.NewRequest("GET", "http://some.url", nil)
		Expect(t, err).To(BeNil())

		t.h.ServeHTTP(t.recorder, req)
		Expect(t, t.recorder.Code).To(Equal(http.StatusInternalServerError))
	})

	o.Spec("it should return a 500 if the relayer fails", func(t TE) {
		t.spyRelayer.err = errors.New("some-error")

		req, err := http.NewRequest("GET", "http://some.url", nil)
		Expect(t, err).To(BeNil())

		t.h.ServeHTTP(t.recorder, req)
		Expect(t, t.recorder.Code).To(Equal(http.StatusInternalServerError))
	})

	o.Spec("it should use context from request for submitting work", func(t TE) {
		ctx, cancel := context.WithCancel(context.Background())
		req, err := http.NewRequest("GET", "http://some.url", nil)
		Expect(t, err).To(BeNil())
		req = req.WithContext(ctx)

		t.h.ServeHTTP(t.recorder, req)
		cancel()

		Expect(t, t.spyWorkSubmitter.ctx).To(Not(BeNil()))
		Expect(t, t.spyWorkSubmitter.ctx.Err()).To(Not(BeNil()))
	})

	o.Spec("it should use context from request for the relayer", func(t TE) {
		ctx, cancel := context.WithCancel(context.Background())
		req, err := http.NewRequest("GET", "http://some.url", nil)
		Expect(t, err).To(BeNil())
		req = req.WithContext(ctx)

		t.h.ServeHTTP(t.recorder, req)
		cancel()

		// Ensure it added a timeout
		_, ok := t.spyRelayer.ctx.Deadline()
		Expect(t, ok).To(BeTrue())

		Expect(t, t.spyRelayer.ctx.Err()).To(Not(BeNil()))
	})

	o.Spec("it should cancel the context to the relayer when the task finishes", func(t TE) {
		req, err := http.NewRequest("GET", "http://some.url", nil)
		Expect(t, err).To(BeNil())

		t.h.ServeHTTP(t.recorder, req)
		Expect(t, t.spyRelayer.ctx.Err()).To(Not(BeNil()))
	})
}

type spyRelayer struct {
	mu    sync.Mutex
	block bool

	ctx context.Context
	r   *http.Request
	u   *url.URL
	err error

	resp    faas.Response
	respErr error
}

func newSpyRelayer() *spyRelayer {
	u, err := url.Parse("http://some.url/some-id")
	if err != nil {
		panic(err)
	}
	return &spyRelayer{
		u: u,
	}
}

func (s *spyRelayer) Relay(r *http.Request) (*url.URL, func() (faas.Response, error), error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ctx = r.Context()
	s.r = r
	return s.u, func() (faas.Response, error) {
		if s.block {
			var wg sync.WaitGroup
			wg.Add(1)
			wg.Wait()
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.resp, s.respErr
	}, s.err
}

type spyWorkSubmitter struct {
	ctx context.Context
	w   internalapi.Work
}

func newSpyWorkSubmitter() *spyWorkSubmitter {
	return &spyWorkSubmitter{}
}

func (s *spyWorkSubmitter) SubmitWork(ctx context.Context, w internalapi.Work) {
	s.ctx = ctx
	s.w = w
}
