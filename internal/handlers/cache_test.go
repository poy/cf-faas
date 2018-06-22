package handlers_test

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/apoydence/cf-faas/internal/handlers"
	"github.com/apoydence/onpar"
	. "github.com/apoydence/onpar/expect"
	. "github.com/apoydence/onpar/matchers"
)

type TC struct {
	*testing.T
	spyHTTPHandler *spyHTTPHandler
	c              http.Handler
	recorder       *httptest.ResponseRecorder
}

func TestCache(t *testing.T) {
	t.Parallel()
	o := onpar.New()
	defer o.Run(t)

	o.BeforeEach(func(t *testing.T) TC {
		spyHTTPHandler := newSpyHTTPHandler()
		return TC{
			T:              t,
			spyHTTPHandler: spyHTTPHandler,
			c: handlers.NewCache(
				fmt.Sprintf("some-name-%d", time.Now().UnixNano()),
				[]string{"a", "c", "e", "g"},
				spyHTTPHandler,
				log.New(ioutil.Discard, "", 0),
			),
			recorder: httptest.NewRecorder(),
		}
	})

	o.Spec("it does caches GET requests", func(t TC) {
		req, err := http.NewRequest(http.MethodGet, "http://some.url", nil)
		Expect(t, err).To(BeNil())
		req.Header.Set("a", "b")
		req.Header.Set("c", "d")
		req.Header.Set("e", "f")
		req.Header.Set("g", "h")
		req.Header.Set("i", "j")

		t.c.ServeHTTP(t.recorder, req)

		Expect(t, t.spyHTTPHandler.r.Method).To(Equal(http.MethodGet))
		Expect(t, t.spyHTTPHandler.r.URL.String()).To(Equal("http://some.url"))
		Expect(t, t.spyHTTPHandler.r.Header.Get("a")).To(Equal("b"))
		Expect(t, t.spyHTTPHandler.r.Header.Get("c")).To(Equal("d"))
		Expect(t, t.spyHTTPHandler.r.Header.Get("e")).To(Equal("f"))
		Expect(t, t.spyHTTPHandler.r.Header.Get("g")).To(Equal("h"))

		Expect(t, t.recorder.Code).To(Equal(234))
		Expect(t, t.recorder.Body.String()).To(Equal("called 1"))

		t.recorder = httptest.NewRecorder()
		req, err = http.NewRequest(http.MethodGet, "http://some.url", nil)
		Expect(t, err).To(BeNil())
		req.Header.Set("a", "b")
		req.Header.Set("c", "d")
		req.Header.Set("e", "f")
		req.Header.Set("g", "h")
		req.Header.Set("i", "different-but-doesnt-matter")
		t.c.ServeHTTP(t.recorder, req)

		Expect(t, t.spyHTTPHandler.called).To(Equal(1))
		Expect(t, t.recorder.Code).To(Equal(234))
		Expect(t, t.recorder.Body.String()).To(Equal("called 1"))
	})

	o.Spec("it does not cache non-GET requests", func(t TC) {
		req, err := http.NewRequest(http.MethodPut, "http://some.url", nil)
		Expect(t, err).To(BeNil())
		t.c.ServeHTTP(t.recorder, req)

		Expect(t, t.spyHTTPHandler.r).To(Equal(req))
	})
}

type spyHTTPHandler struct {
	called int
	r      *http.Request
}

func newSpyHTTPHandler() *spyHTTPHandler {
	return &spyHTTPHandler{}
}

func (s *spyHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(234)
	s.called++
	w.Write([]byte(fmt.Sprintf("called %d", s.called)))
	s.r = r
}
