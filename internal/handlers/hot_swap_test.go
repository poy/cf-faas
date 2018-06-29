package handlers_test

import (
	"net/http/httptest"
	"testing"

	"github.com/apoydence/cf-faas/internal/handlers"
	"github.com/apoydence/onpar"
	. "github.com/apoydence/onpar/expect"
	. "github.com/apoydence/onpar/matchers"
)

type TS struct {
	*testing.T
	currentHandler *spyHandler
	newHandler     *spyHandler

	h *handlers.HotSwap
}

func TestHotSwap(t *testing.T) {
	t.Parallel()
	o := onpar.New()
	defer o.Run(t)

	o.BeforeEach(func(t *testing.T) TS {
		currentHandler := newSpyHandler()
		newHandler := newSpyHandler()

		return TS{
			T:              t,
			currentHandler: currentHandler,
			newHandler:     newHandler,
			h:              handlers.NewHotSwap(currentHandler),
		}
	})

	o.Spec("writes all requests to the current handler", func(t TS) {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest("get", "http://some.url", nil)
		t.h.ServeHTTP(recorder, req)

		Expect(t, t.currentHandler.r).To(Equal(req))
	})

	o.Spec("writes all requests to the new handler", func(t TS) {
		t.h.Swap(t.newHandler)
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest("get", "http://some.url", nil)
		t.h.ServeHTTP(recorder, req)

		Expect(t, t.newHandler.r).To(Equal(req))
	})

	o.Spec("it survives the race detector", func(t TS) {
		go func() {
			for i := 0; i < 100; i++ {
				recorder := httptest.NewRecorder()
				req := httptest.NewRequest("get", "http://some.url", nil)
				t.h.ServeHTTP(recorder, req)
			}
		}()

		go func() {
			for i := 0; i < 100; i++ {
				recorder := httptest.NewRecorder()
				req := httptest.NewRequest("get", "http://some.url", nil)
				t.h.ServeHTTP(recorder, req)
			}
		}()

		for i := 0; i < 100; i++ {
			t.h.Swap(t.newHandler)
		}
	})
}
