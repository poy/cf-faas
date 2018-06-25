package handlers_test

import (
	"context"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/apoydence/cf-faas/internal/handlers"
	"github.com/apoydence/cf-faas/internal/internalapi"
	gocapi "github.com/apoydence/go-capi"
	"github.com/apoydence/onpar"
	. "github.com/apoydence/onpar/expect"
	. "github.com/apoydence/onpar/matchers"
)

type TP struct {
	*testing.T
	spyTaskCreator *spyTaskCreator
	p              *handlers.WorkerPool
	recorder       *httptest.ResponseRecorder
}

func TestWorkerPool(t *testing.T) {
	t.Parallel()
	o := onpar.New()
	defer o.Run(t)

	o.BeforeEach(func(t *testing.T) TP {
		spyTaskCreator := newSpyTaskCreator()

		return TP{
			T:              t,
			spyTaskCreator: spyTaskCreator,
			recorder:       httptest.NewRecorder(),
			p:              handlers.NewWorkerPool("https://some.url", []string{"a", "b"}, "app-instance", time.Millisecond, spyTaskCreator, log.New(ioutil.Discard, "", 0)),
		}
	})

	o.Spec("returns address to find work", func(t TP) {
		go t.p.SubmitWork(context.Background(), internalapi.Work{
			Href:    "http://some.url",
			Command: "some-command",
			AppName: "some-app",
		})

		req, err := http.NewRequest("GET", "http://some.url", nil)
		Expect(t, err).To(BeNil())

		t.p.ServeHTTP(t.recorder, req)
		Expect(t, t.recorder.Body.String()).To(MatchJSON(`{"href": "http://some.url","command":"some-command","app_name":"some-app"}`))
		Expect(t, t.recorder.Code).To(Equal(http.StatusOK))
	})

	o.Spec("adheres to the request context", func(t TP) {
		req, err := http.NewRequest("GET", "http://some.url", nil)
		Expect(t, err).To(BeNil())
		ctx, cancel := context.WithCancel(context.Background())
		req = req.WithContext(ctx)
		cancel()

		t.p.ServeHTTP(t.recorder, req)
		Expect(t, t.recorder.Body.Bytes()).To(HaveLen(0))
	})

	o.Spec("only spin up 5 tasks at a time", func(t TP) {
		go t.p.SubmitWork(context.Background(), internalapi.Work{
			Href:    "http://some.url",
			Command: "some-command",
			AppName: "some-app",
		})

		Expect(t, t.spyTaskCreator.Called).To(Always(BeBelow(6)))
	})

	o.Spec("schedules a new task if the work just sits", func(t TP) {
		ctx, _ := context.WithCancel(context.Background())
		go t.p.SubmitWork(ctx, internalapi.Work{
			Href:    "http://some.url",
			Command: "some-command",
			AppName: "some-app",
		})

		Expect(t, t.spyTaskCreator.Command).To(ViaPolling(Not(HaveLen(0))))
	})

	o.Spec("returns a 405 for anything other than a GET", func(t TP) {
		req, err := http.NewRequest("POST", "http://some.url", nil)
		Expect(t, err).To(BeNil())

		t.p.ServeHTTP(t.recorder, req)
		Expect(t, t.recorder.Code).To(Equal(http.StatusMethodNotAllowed))
	})
}

type spyTaskCreator struct {
	mu      sync.Mutex
	ctx     context.Context
	command string
	err     error
	called  int
}

func newSpyTaskCreator() *spyTaskCreator {
	return &spyTaskCreator{}
}

func (s *spyTaskCreator) RunTask(ctx context.Context, command, name, dropletGuid, appName string) (gocapi.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.called++
	s.ctx = ctx
	s.command = command
	return gocapi.Task{}, s.err
}

func (s *spyTaskCreator) Command() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.command
}

func (s *spyTaskCreator) Called() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.called
}

type spyTokenFetcher struct {
	token string
	err   error
}

func newSpyTokenFetcher() *spyTokenFetcher {
	return &spyTokenFetcher{}
}

func (s *spyTokenFetcher) Token() (string, error) {
	return s.token, s.err
}

type spyDropletFetcher struct {
	ctx     context.Context
	appName string
	appGuid string

	guid string
	err  error
}

func newSpyDropletFetcher() *spyDropletFetcher {
	return &spyDropletFetcher{}
}

func (s *spyDropletFetcher) FetchGuid(ctx context.Context, appName string) (string, string, error) {
	s.ctx = ctx
	s.appName = appName
	return s.appGuid, s.guid, s.err
}
