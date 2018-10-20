package handlers_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/poy/cf-faas/internal/handlers"
	"github.com/poy/onpar"
	. "github.com/poy/onpar/expect"
	. "github.com/poy/onpar/matchers"
	"github.com/gorilla/mux"
)

type TR struct {
	*testing.T
	r        *handlers.RequestRelayer
	recorder *httptest.ResponseRecorder
}

func TestRequestRelayer(t *testing.T) {
	t.Parallel()
	o := onpar.New()
	defer o.Run(t)

	o.BeforeEach(func(t *testing.T) TR {
		return TR{
			T:        t,
			recorder: httptest.NewRecorder(),
			r:        handlers.NewRequestRelayer("http://some.url", "some-prefix", log.New(ioutil.Discard, "", 0)),
		}
	})

	o.Spec("it sends the data of the request to the GET", func(t TR) {
		expectedData := make([]byte, 10*1024)
		rand.Read(expectedData)

		req, err := http.NewRequest("PUT", "http://some.url/v1/some-var-1/some-var-2", bytes.NewReader(expectedData))
		Expect(t, err).To(BeNil())
		req.Header.Add("A", "a")
		req.Header.Add("A", "aa")
		req.Header.Add("B", "b")

		var called bool
		defer func() {
			Expect(t, called).To(BeTrue())
		}()

		r := mux.NewRouter()
		r.HandleFunc("/v1/{var-1}/{var-2}", func(_ http.ResponseWriter, req *http.Request) {
			called = true
			addr, _, err := t.r.Relay(req)
			Expect(t, err).To(BeNil())
			Expect(t, strings.HasPrefix(addr.Path, "/some-prefix")).To(BeTrue())

			req, err = http.NewRequest("GET", addr.String(), bytes.NewReader(nil))
			Expect(t, err).To(BeNil())
			req.Header.Set("X-Forwarded-Proto", "https")

			t.r.ServeHTTP(t.recorder, req)
			Expect(t, t.recorder.Code).To(Equal(http.StatusOK))

			var m map[string]interface{}
			Expect(t, json.Unmarshal(t.recorder.Body.Bytes(), &m)).To(BeNil())
			Expect(t, m["body"]).To(Equal(base64.StdEncoding.EncodeToString(expectedData)))
			Expect(t, m["method"]).To(Equal(http.MethodPut))
			Expect(t, m["path"]).To(Equal("/v1/some-var-1/some-var-2"))
			Expect(t, m["headers"]).To(Equal(map[string]interface{}{
				"A": []interface{}{"a", "aa"},
				"B": []interface{}{"b"},
			}))
			Expect(t, m["url_variables"]).To(Equal(map[string]interface{}{
				"var-1": "some-var-1",
				"var-2": "some-var-2",
			}))
		})
		r.ServeHTTP(httptest.NewRecorder(), req)
	})

	o.Spec("it writes response back to ResponseWriter on POST", func(t TR) {
		req, err := http.NewRequest("PUT", "http://some.url/v1/some-path", bytes.NewReader(nil))
		Expect(t, err).To(BeNil())

		addr, f, err := t.r.Relay(req)
		Expect(t, err).To(BeNil())

		req, err = http.NewRequest("POST", addr.String(), strings.NewReader(fmt.Sprintf(`
		{
			"status_code":234,
			"body": %q
		}
		`, base64.StdEncoding.EncodeToString([]byte("hello")))))

		Expect(t, err).To(BeNil())
		req.Header.Set("X-Forwarded-Proto", "https")

		t.r.ServeHTTP(t.recorder, req)
		Expect(t, t.recorder.Code).To(Equal(http.StatusOK))

		resp, err := f()
		Expect(t, err).To(BeNil())
		Expect(t, resp.StatusCode).To(Equal(234))
		Expect(t, string(resp.Body)).To(Equal("hello"))
	})

	o.Spec("it rejects requests that dont have X-Forwarded-Proto for HTTPS", func(t TR) {
		req, err := http.NewRequest("PUT", "http://some.url/v1/some-path", bytes.NewReader(nil))
		Expect(t, err).To(BeNil())

		addr, _, err := t.r.Relay(req)
		Expect(t, err).To(BeNil())

		req, err = http.NewRequest("GET", addr.String(), bytes.NewReader(nil))
		Expect(t, err).To(BeNil())

		t.r.ServeHTTP(t.recorder, req)
		Expect(t, t.recorder.Code).To(Equal(http.StatusBadRequest))

		req, err = http.NewRequest("POST", addr.String(), strings.NewReader(fmt.Sprintf(`
		{
			"status_code":234,
			"body": %q
		}
		`, base64.StdEncoding.EncodeToString([]byte("hello")))))

		Expect(t, err).To(BeNil())
		t.recorder = httptest.NewRecorder()
		t.r.ServeHTTP(t.recorder, req)
		Expect(t, t.recorder.Code).To(Equal(http.StatusBadRequest))
	})

	o.Spec("it writes 500 to ResponseWriter on invalid POST", func(t TR) {
		req, err := http.NewRequest("PUT", "http://some.url/v1/some-path", bytes.NewReader(nil))
		Expect(t, err).To(BeNil())

		addr, f, err := t.r.Relay(req)
		Expect(t, err).To(BeNil())

		req, err = http.NewRequest("POST", addr.String(), strings.NewReader("invalid"))
		Expect(t, err).To(BeNil())
		req.Header.Set("X-Forwarded-Proto", "https")
		t.r.ServeHTTP(t.recorder, req)

		_, err = f()
		Expect(t, err).To(Not(BeNil()))

		Expect(t, t.recorder.Code).To(Equal(http.StatusExpectationFailed))
	})

	o.Spec("it returns an error if context is cancelled", func(t TR) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		req, err := http.NewRequest("GET", "http://some.url", bytes.NewReader(nil))
		Expect(t, err).To(BeNil())
		req = req.WithContext(ctx)

		_, f, err := t.r.Relay(req)
		Expect(t, err).To(BeNil())

		_, err = f()
		Expect(t, err).To(Not(BeNil()))
	})

	o.Spec("it removes ID when context is cancelled", func(t TR) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		req, err := http.NewRequest("GET", "http://some.url", bytes.NewReader(nil))
		Expect(t, err).To(BeNil())
		req = req.WithContext(ctx)
		addr, f, err := t.r.Relay(req)
		Expect(t, err).To(BeNil())
		f()

		req, err = http.NewRequest("GET", addr.String(), bytes.NewReader(nil))
		Expect(t, err).To(BeNil())
		req.Header.Set("X-Forwarded-Proto", "https")

		t.r.ServeHTTP(t.recorder, req)
		Expect(t, t.recorder.Code).To(Equal(http.StatusNotFound))

		req, err = http.NewRequest("POST", addr.String(), bytes.NewReader(nil))
		Expect(t, err).To(BeNil())
		req.Header.Set("X-Forwarded-Proto", "https")

		t.recorder = httptest.NewRecorder()
		t.r.ServeHTTP(t.recorder, req)
		Expect(t, t.recorder.Code).To(Equal(http.StatusNotFound))
	})

	o.Spec("it returns a 404 for an unknown ID", func(t TR) {
		req, err := http.NewRequest("GET", "http://some.url/invalid", bytes.NewReader(nil))
		Expect(t, err).To(BeNil())
		req.Header.Set("X-Forwarded-Proto", "https")

		t.r.ServeHTTP(t.recorder, req)
		Expect(t, t.recorder.Code).To(Equal(http.StatusNotFound))
	})

	o.Spec("it returns a 405 for non GET or POST", func(t TR) {
		req, err := http.NewRequest("PUT", "http://some.url", bytes.NewReader(nil))
		Expect(t, err).To(BeNil())
		req.Header.Set("X-Forwarded-Proto", "https")

		t.r.ServeHTTP(t.recorder, req)
		Expect(t, t.recorder.Code).To(Equal(http.StatusMethodNotAllowed))
	})
}
