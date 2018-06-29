package handlers_test

import (
	"context"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	gocapi "github.com/apoydence/go-capi"

	"github.com/apoydence/cf-faas/internal/handlers"
	"github.com/apoydence/cf-faas/internal/manifest"
	"github.com/apoydence/onpar"
	. "github.com/apoydence/onpar/expect"
	. "github.com/apoydence/onpar/matchers"
)

type TRR struct {
	*testing.T
	stubConstructorRequestRelayer *stubConstructorRequestRelayer
	stubConstructorWorkerPool     *stubConstructorWorkerPool
	stubConstructorHTTPEvent      *stubConstructorHTTPEvent
	stubConstructorCache          *stubConstructorCache
	groupcachePool                *spyHandler
	r                             *handlers.Router
	m                             []manifest.HTTPFunction
}

func TestRouter(t *testing.T) {
	t.Parallel()
	o := onpar.New()
	defer o.Run(t)

	o.BeforeEach(func(t *testing.T) TRR {
		stubConstructorRequestRelayer := newStubConstructorRequestRelayer()
		stubConstructorWorkerPool := newStubConstructorWorkerPool()
		stubConstructorHTTPEvent := newStubConstructorHTTPEvent()
		stubConstructorCache := newStubConstructorCache()
		groupcachePool := newSpyHandler()
		m := []manifest.HTTPFunction{
			{
				Handler: manifest.Handler{
					Command: "some-command",
				},
				Events: []manifest.HTTPEvent{
					{
						Path:   "/v1/some-path",
						Method: "GET",
						Cache: struct {
							Duration time.Duration `yaml:"duration"`
							Header   []string      `yaml:"header"`
						}{
							Duration: time.Second,
							Header:   []string{"A", "B"},
						},
					},
				},
			},
			{
				Handler: manifest.Handler{
					Command: "some-command",
				},
				Events: []manifest.HTTPEvent{
					{
						Path:   "/v1/some-path",
						Method: "GET",
					},
				},
			},
		}
		return TRR{
			T: t,
			m: m,
			stubConstructorRequestRelayer: stubConstructorRequestRelayer,
			stubConstructorWorkerPool:     stubConstructorWorkerPool,
			stubConstructorHTTPEvent:      stubConstructorHTTPEvent,
			stubConstructorCache:          stubConstructorCache,
			groupcachePool:                groupcachePool,
			r: handlers.NewRouter(
				"http://some.url",
				"some-application",
				"some-id",
				99,
				groupcachePool,
				&gocapi.Client{},
				stubConstructorRequestRelayer.New,
				stubConstructorWorkerPool.New,
				stubConstructorHTTPEvent.New,
				stubConstructorCache.New,
				log.New(ioutil.Discard, "", 0),
			),
		}
	})

	o.Spec("it creates and registers a RequestRelayer", func(t TRR) {
		h := t.r.BuildHandler(context.Background(), nil, t.m)
		Expect(t, t.stubConstructorRequestRelayer.addr).To(Equal("http://some.url"))
		Expect(t, t.stubConstructorRequestRelayer.pathPrefix).To(ContainSubstring("relayer"))
		Expect(t, t.stubConstructorRequestRelayer.log).To(Not(BeNil()))

		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(
			"DELETE", // DELETE is not accepted by RequestRelayer
			t.stubConstructorRequestRelayer.addr+"/"+t.stubConstructorRequestRelayer.pathPrefix+"/some-id",
			nil,
		)
		h.ServeHTTP(recorder, req)
		Expect(t, recorder.Code).To(Equal(http.StatusMethodNotAllowed))
	})

	o.Spec("it registers the groupcache pool", func(t TRR) {
		h := t.r.BuildHandler(context.Background(), nil, t.m)

		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(
			"GET",
			"http://some.url/_group_cache_32723262323249873240/some-name/some-key",
			nil,
		)
		h.ServeHTTP(recorder, req)
		Expect(t, t.groupcachePool.w).To(Equal(recorder))
	})

	o.Spec("it creates and registers a WorkerPool", func(t TRR) {
		h := t.r.BuildHandler(context.Background(), []string{"some-application"}, t.m)
		Expect(t, t.stubConstructorWorkerPool.addr).To(And(
			ContainSubstring("http://some.url"),
			ContainSubstring("/pool"),
		))
		Expect(t, t.stubConstructorWorkerPool.ctx).To(Not(BeNil()))
		Expect(t, t.stubConstructorWorkerPool.appNames).To(Contain("some-application"))
		Expect(t, t.stubConstructorWorkerPool.appInstance).To(Equal("some-id:99"))
		Expect(t, t.stubConstructorWorkerPool.addTaskThreshold).To(Equal(time.Second))
		Expect(t, t.stubConstructorWorkerPool.addTaskThreshold).To(Equal(time.Second))
		Expect(t, t.stubConstructorWorkerPool.taskCreator).To(Not(BeNil()))
		Expect(t, t.stubConstructorWorkerPool.log).To(Not(BeNil()))

		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(
			"DELETE", // DELETE is not accepted by WorkerPool
			t.stubConstructorWorkerPool.addr,
			nil,
		)
		h.ServeHTTP(recorder, req)
		Expect(t, recorder.Code).To(Equal(http.StatusMethodNotAllowed))
	})

	o.Spec("it creates and registers an HTTPEvent for each function", func(t TRR) {
		h := t.r.BuildHandler(context.Background(), nil, t.m)
		Expect(t, t.stubConstructorHTTPEvent.command).To(Equal("some-command"))
		Expect(t, t.stubConstructorHTTPEvent.appName).To(Equal("some-application"))
		Expect(t, t.stubConstructorHTTPEvent.relayer).To(Not(BeNil()))
		Expect(t, t.stubConstructorHTTPEvent.submitter).To(Not(BeNil()))
		Expect(t, t.stubConstructorHTTPEvent.log).To(Not(BeNil()))

		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(
			"GET",
			"http://some.url/v1/some-path",
			nil,
		)

		// We didn't return a properly setup HTTPEvent from our stub. It
		// should just blow up.
		Expect(t, func() {
			h.ServeHTTP(recorder, req)
		}).To(Panic())
	})

	o.Spec("it creates and registers a cache for each function", func(t TRR) {
		h := t.r.BuildHandler(context.Background(), nil, t.m)
		_ = h
		Expect(t, t.stubConstructorCache.name).To(Not(Equal("")))
		Expect(t, t.stubConstructorCache.headers).To(Equal([]string{"A", "B"}))
		Expect(t, t.stubConstructorCache.handler).To(Not(BeNil()))
		Expect(t, t.stubConstructorCache.duration).To(Equal(time.Second))
		Expect(t, t.stubConstructorCache.log).To(Not(BeNil()))

		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(
			"GET",
			"http://some.url/v1/some-path",
			nil,
		)

		// We didn't return a properly setup Cache from our stub. It
		// should just blow up.
		Expect(t, func() {
			h.ServeHTTP(recorder, req)
		}).To(Panic())
	})
}

type stubConstructorRequestRelayer struct {
	addr       string
	pathPrefix string
	log        *log.Logger
}

func newStubConstructorRequestRelayer() *stubConstructorRequestRelayer {
	return &stubConstructorRequestRelayer{}
}

func (s *stubConstructorRequestRelayer) New(addr, pathPrefix string, log *log.Logger) *handlers.RequestRelayer {
	s.addr = addr
	s.pathPrefix = pathPrefix
	s.log = log
	return &handlers.RequestRelayer{}
}

type spyHandler struct {
	w http.ResponseWriter
	r *http.Request
}

func newSpyHandler() *spyHandler {
	return &spyHandler{}
}

func (s *spyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.w = w
	s.r = r
}

type stubConstructorWorkerPool struct {
	ctx              context.Context
	addr             string
	appNames         []string
	appInstance      string
	addTaskThreshold time.Duration
	taskCreator      handlers.TaskCreator
	log              *log.Logger
}

func newStubConstructorWorkerPool() *stubConstructorWorkerPool {
	return &stubConstructorWorkerPool{}
}

func (s *stubConstructorWorkerPool) New(ctx context.Context, addr string, appNames []string, appInstance string, addTaskThreshold time.Duration, c handlers.TaskCreator, log *log.Logger) *handlers.WorkerPool {
	s.ctx = ctx
	s.addr = addr
	s.appNames = appNames
	s.appInstance = appInstance
	s.addTaskThreshold = addTaskThreshold
	s.taskCreator = c
	s.log = log

	return &handlers.WorkerPool{}
}

type stubConstructorHTTPEvent struct {
	command   string
	appName   string
	relayer   handlers.Relayer
	submitter handlers.WorkSubmitter
	log       *log.Logger
}

func newStubConstructorHTTPEvent() *stubConstructorHTTPEvent {
	return &stubConstructorHTTPEvent{}
}

func (s *stubConstructorHTTPEvent) New(command string, appName string, r handlers.Relayer, submitter handlers.WorkSubmitter, log *log.Logger) *handlers.HTTPEvent {
	s.command = command
	s.appName = appName
	s.relayer = r
	s.submitter = submitter
	s.log = log

	return &handlers.HTTPEvent{}
}

type stubConstructorCache struct {
	name     string
	headers  []string
	handler  http.Handler
	duration time.Duration
	log      *log.Logger
}

func newStubConstructorCache() *stubConstructorCache {
	return &stubConstructorCache{}
}

func (s *stubConstructorCache) New(name string, headers []string, h http.Handler, d time.Duration, log *log.Logger) *handlers.Cache {
	s.name = name
	s.headers = headers
	s.handler = h
	s.duration = d
	s.log = log

	return &handlers.Cache{}
}
