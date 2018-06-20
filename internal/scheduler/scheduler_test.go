package scheduler_test

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/apoydence/cf-faas/internal/internalapi"
	"github.com/apoydence/cf-faas/internal/scheduler"
	"github.com/apoydence/onpar"
	. "github.com/apoydence/onpar/expect"
	. "github.com/apoydence/onpar/matchers"
)

type TS struct {
	*testing.T
	spyDoer          *spyDoer
	spyWorkSubmitter *spyWorkSubmitter
}

func TestScheduler(t *testing.T) {
	t.Parallel()
	o := onpar.New()
	defer o.Run(t)

	o.BeforeEach(func(t *testing.T) TS {
		ts := TS{
			T:                t,
			spyDoer:          newSpyDoer(),
			spyWorkSubmitter: newSpyWorkSubmitter(),
		}

		return ts
	})

	o.Spec("it does a GET to fetch work", func(t TS) {
		t.spyDoer.m["GET:http://some.url"] = &http.Response{
			Body:       ioutil.NopCloser(strings.NewReader(`{"href":"http://some.work"}`)),
			StatusCode: 200,
		}
		start(t)
		Expect(t, t.spyDoer.Req).To(ViaPolling(Not(BeNil())))
		Expect(t, t.spyDoer.Req().Method).To(Equal(http.MethodGet))
		Expect(t, t.spyDoer.Req().URL.String()).To(Equal("http://some.url"))
		Expect(t, t.spyDoer.Req().Header.Get("X-CF-APP-INSTANCE")).To(Equal("app-instance"))

		Expect(t, t.spyWorkSubmitter.Work().Href).To(Equal("http://some.work"))
	})

	o.Spec("it keeps asking for work", func(t TS) {
		t.spyDoer.m["GET:http://some.url"] = &http.Response{
			Body:       ioutil.NopCloser(strings.NewReader(`{"href":"http://some.work"}`)),
			StatusCode: 200,
		}
		start(t)
		Expect(t, t.spyDoer.Called).To(ViaPolling(BeAbove(1)))
	})

	o.Spec("it exits if it gets a non-200", func(t TS) {
		t.spyDoer.m["GET:http://some.url"] = &http.Response{
			Body:       ioutil.NopCloser(strings.NewReader(`{"href":"http://some.work"}`)),
			StatusCode: 500,
		}
		Expect(t, start(t)).To(ViaPolling(BeClosed()))
	})

	o.Spec("it exits if it doesn't get work", func(t TS) {
		t.spyDoer.block = true
		Expect(t, start(t)).To(ViaPolling(BeClosed()))
	})
}

func start(t TS) chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		scheduler.Run("http://some.url", "app-instance", 100*time.Millisecond, t.spyWorkSubmitter, t.spyDoer, log.New(ioutil.Discard, "", 0))
	}()
	return done
}

type spyDoer struct {
	block  bool
	mu     sync.Mutex
	m      map[string]*http.Response
	req    *http.Request
	body   []byte
	called int

	err error
}

func newSpyDoer() *spyDoer {
	return &spyDoer{
		m: make(map[string]*http.Response),
	}
}

func (s *spyDoer) Do(req *http.Request) (*http.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.called++

	if s.block {
		var c chan struct{}
		select {
		case <-c:
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	}

	s.req = req

	if req.Body != nil {
		body, err := ioutil.ReadAll(req.Body)
		if err != nil {
			panic(err)
		}
		s.body = body
	}

	r, ok := s.m[fmt.Sprintf("%s:%s", req.Method, req.URL.String())]
	if !ok {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(strings.NewReader(`{}`)),
		}, s.err
	}

	return r, s.err
}

func (s *spyDoer) Called() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.called
}

func (s *spyDoer) Req() *http.Request {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.req
}

type spyWorkSubmitter struct {
	mu   sync.Mutex
	work internalapi.Work
}

func newSpyWorkSubmitter() *spyWorkSubmitter {
	return &spyWorkSubmitter{}
}

func (s *spyWorkSubmitter) Submit(work internalapi.Work) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.work = work
}

func (s *spyWorkSubmitter) Work() internalapi.Work {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.work
}
