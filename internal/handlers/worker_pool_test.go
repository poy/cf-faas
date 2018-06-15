package handlers_test

import (
	"context"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/apoydence/cf-faas/internal/handlers"
	"github.com/apoydence/onpar"
	. "github.com/apoydence/onpar/expect"
	. "github.com/apoydence/onpar/matchers"
)

type TP struct {
	*testing.T
	spyTaskCreator    *spyTaskCreator
	spyTokenFetcher   *spyTokenFetcher
	spyDropletFetcher *spyDropletFetcher
	p                 *handlers.WorkerPool
	recorder          *httptest.ResponseRecorder
	u                 *url.URL
}

func TestWorkerPool(t *testing.T) {
	t.Parallel()
	o := onpar.New()
	defer o.Run(t)

	o.BeforeEach(func(t *testing.T) TP {
		spyTaskCreator := newSpyTaskCreator()
		spyTokenFetcher := newSpyTokenFetcher()
		spyDropletFetcher := newSpyDropletFetcher()
		u, err := url.Parse("http://some-addr.url")
		if err != nil {
			panic(err)
		}

		return TP{
			T:                 t,
			u:                 u,
			spyTokenFetcher:   spyTokenFetcher,
			spyDropletFetcher: spyDropletFetcher,
			spyTaskCreator:    spyTaskCreator,
			recorder:          httptest.NewRecorder(),
			p:                 handlers.NewWorkerPool("https://some.url", "some-command", "app-guid", "app-name", spyDropletFetcher, 99, true, time.Millisecond, spyTaskCreator, spyTokenFetcher, log.New(ioutil.Discard, "", 0)),
		}
	})

	o.Spec("returns address to find work", func(t TP) {
		go t.p.SubmitWork(context.Background(), t.u)

		req, err := http.NewRequest("GET", "http://some.url", nil)
		Expect(t, err).To(BeNil())

		t.p.ServeHTTP(t.recorder, req)
		Expect(t, t.recorder.Body.String()).To(MatchJSON(`{"href": "http://some-addr.url"}`))
		Expect(t, t.recorder.Code).To(Equal(http.StatusOK))
	})

	o.Spec("only spin up 5 tasks at a time", func(t TP) {
		for i := 0; i < 100; i++ {
			go t.p.SubmitWork(context.Background(), t.u)
		}

		Expect(t, t.spyTaskCreator.Called).To(Always(BeBelow(6)))
	})

	o.Spec("schedules a new task if the work just sits", func(t TP) {
		t.spyTokenFetcher.token = "some-token"
		t.spyDropletFetcher.guid = "droplet-guid"
		ctx, _ := context.WithCancel(context.Background())
		go t.p.SubmitWork(ctx, t.u)

		Expect(t, t.spyTaskCreator.Command).To(ViaPolling(Not(HaveLen(0))))
		Expect(t, t.spyTaskCreator.Command()).To(ContainSubstring(`while true`))
		Expect(t, t.spyTaskCreator.Command()).To(ContainSubstring(`done`))
		Expect(t, t.spyTaskCreator.Command()).To(ContainSubstring(`export CF_AUTH_TOKEN="some-token"`))
		Expect(t, t.spyTaskCreator.Command()).To(ContainSubstring(`export SKIP_SSL_VALIDATION="true"`))
		Expect(t, t.spyTaskCreator.Command()).To(ContainSubstring(`export X_CF_APP_INSTANCE="app-guid:99"`))
		Expect(t, t.spyTaskCreator.Command()).To(ContainSubstring(`export CF_FAAS_RELAY_ADDR=$(timeout 30 curl -s -k https://some.url -H "X-CF-APP-INSTANCE: $X_CF_APP_INSTANCE" -H "Authorization: $CF_AUTH_TOKEN" | jq -r .href)`))
		Expect(t, t.spyTaskCreator.Command()).To(ContainSubstring(`if [ -z "$CF_FAAS_RELAY_ADDR" ]; then`))
		Expect(t, t.spyTaskCreator.Command()).To(ContainSubstring("some-command"))

		Expect(t, t.spyTaskCreator.DropletGuid()).To(Equal("droplet-guid"))

		Expect(t, t.spyDropletFetcher.ctx).To(Equal(ctx))
		Expect(t, t.spyDropletFetcher.appName).To(Equal("app-name"))
	})

	o.Spec("does not skip SSL validation unless SKIP_SSL_VALIDATION is true", func(t TP) {
		t.p = handlers.NewWorkerPool("https://some.url", "some-command", "app-guid", "app-name", t.spyDropletFetcher, 99, false, time.Millisecond, t.spyTaskCreator, t.spyTokenFetcher, log.New(ioutil.Discard, "", 0))
		go t.p.SubmitWork(context.Background(), t.u)

		Expect(t, t.spyTaskCreator.Command).To(ViaPolling(Not(HaveLen(0))))
		Expect(t, t.spyTaskCreator.Command()).To(ContainSubstring(`export SKIP_SSL_VALIDATION="false"`))
		Expect(t, t.spyTaskCreator.Command()).To(ContainSubstring(`export CF_FAAS_RELAY_ADDR=$(timeout 30 curl -s https://some.url -H "X-CF-APP-INSTANCE: $X_CF_APP_INSTANCE" -H "Authorization: $CF_AUTH_TOKEN" | jq -r .href)`))
	})

	o.Spec("returns a 405 for anything other than a GET", func(t TP) {
		req, err := http.NewRequest("POST", "http://some.url", nil)
		Expect(t, err).To(BeNil())

		t.p.ServeHTTP(t.recorder, req)
		Expect(t, t.recorder.Code).To(Equal(http.StatusMethodNotAllowed))
	})
}

type spyTaskCreator struct {
	mu          sync.Mutex
	ctx         context.Context
	command     string
	dropletGuid string
	err         error
	called      int
}

func newSpyTaskCreator() *spyTaskCreator {
	return &spyTaskCreator{}
}

func (s *spyTaskCreator) CreateTask(ctx context.Context, command, dropletGuid string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.called++
	s.ctx = ctx
	s.command = command
	s.dropletGuid = dropletGuid
	return s.err
}

func (s *spyTaskCreator) Command() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.command
}

func (s *spyTaskCreator) DropletGuid() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dropletGuid
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

	guid string
	err  error
}

func newSpyDropletFetcher() *spyDropletFetcher {
	return &spyDropletFetcher{}
}

func (s *spyDropletFetcher) FetchGuid(ctx context.Context, appName string) (string, error) {
	s.ctx = ctx
	s.appName = appName
	return s.guid, s.err
}
