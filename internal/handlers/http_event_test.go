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

	"github.com/apoydence/cf-faas/api"
	"github.com/apoydence/cf-faas/internal/handlers"
	"github.com/apoydence/onpar"
	. "github.com/apoydence/onpar/expect"
	. "github.com/apoydence/onpar/matchers"
)

type TE struct {
	*testing.T
	h        http.Handler
	recorder *httptest.ResponseRecorder

	spyTaskCreator *spyTaskCreator
	spyRelayer     *spyRelayer
}

func TestHTTPEvent(t *testing.T) {
	t.Parallel()
	o := onpar.New()
	defer o.Run(t)

	o.BeforeEach(func(t *testing.T) TE {
		spyRelayer := newSpyRelayer()
		spyTaskCreator := newSpyTaskCreator()
		return TE{
			T:              t,
			recorder:       httptest.NewRecorder(),
			spyRelayer:     spyRelayer,
			spyTaskCreator: spyTaskCreator,
			h: handlers.NewHTTPEvent(
				"some-command",
				spyRelayer,
				spyTaskCreator,
				log.New(ioutil.Discard, "", 0),
			),
		}
	})

	o.Spec("creates a task that sets the relay addr as an env variable", func(t TE) {
		req, err := http.NewRequest("GET", "http://some.url", nil)
		Expect(t, err).To(BeNil())

		t.h.ServeHTTP(t.recorder, req)
		Expect(t, t.spyTaskCreator.Command).To(
			ViaPolling(
				ContainSubstring(`export CF_FAAS_RELAY_ADDR="http://some.url/some-id"`),
			),
		)
	})

	o.Spec("creates a task that invokes the command", func(t TE) {
		req, err := http.NewRequest("GET", "http://some.url", nil)
		Expect(t, err).To(BeNil())

		t.h.ServeHTTP(t.recorder, req)
		Expect(t, t.spyTaskCreator.Command).To(
			ViaPolling(
				ContainSubstring(`some-command`),
			),
		)
	})

	o.Spec("relayer should be given the request", func(t TE) {
		req, err := http.NewRequest("GET", "http://some.url", nil)
		Expect(t, err).To(BeNil())

		t.h.ServeHTTP(t.recorder, req)

		Expect(t, t.spyRelayer.r.URL).To(Equal(req.URL))
	})

	o.Spec("it should return the api.Response", func(t TE) {
		t.spyRelayer.resp = api.Response{
			StatusCode: 234,
			Body:       []byte("some-data"),
		}

		req, err := http.NewRequest("GET", "http://some.url", nil)
		Expect(t, err).To(BeNil())

		t.h.ServeHTTP(t.recorder, req)
		Expect(t, t.recorder.Code).To(Equal(234))
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

	o.Spec("it should return a 500 if creating a task fails", func(t TE) {
		t.spyRelayer.block = true
		t.spyTaskCreator.err = errors.New("some-error")

		req, err := http.NewRequest("GET", "http://some.url", nil)
		Expect(t, err).To(BeNil())

		t.h.ServeHTTP(t.recorder, req)
		Expect(t, t.recorder.Code).To(Equal(http.StatusInternalServerError))
	})

	o.Spec("it should use context from request for creating task", func(t TE) {
		ctx, cancel := context.WithCancel(context.Background())
		req, err := http.NewRequest("GET", "http://some.url", nil)
		Expect(t, err).To(BeNil())
		req = req.WithContext(ctx)

		t.h.ServeHTTP(t.recorder, req)
		cancel()

		Expect(t, t.spyTaskCreator.Command).To(ViaPolling(Not(HaveLen(0))))
		Expect(t, t.spyTaskCreator.ctx.Err()).To(Not(BeNil()))
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

	resp    api.Response
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

func (s *spyRelayer) Relay(r *http.Request) (*url.URL, func() (api.Response, error), error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ctx = r.Context()
	s.r = r
	return s.u, func() (api.Response, error) {
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

type spyTaskCreator struct {
	mu      sync.Mutex
	ctx     context.Context
	command string
	err     error
}

func newSpyTaskCreator() *spyTaskCreator {
	return &spyTaskCreator{}
}

func (s *spyTaskCreator) CreateTask(ctx context.Context, command string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ctx = ctx
	s.command = command
	return s.err
}

func (s *spyTaskCreator) Command() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.command
}
