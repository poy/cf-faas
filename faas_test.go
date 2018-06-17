package faas_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	faas "github.com/apoydence/cf-faas"
	"github.com/apoydence/onpar"
	. "github.com/apoydence/onpar/expect"
	. "github.com/apoydence/onpar/matchers"
)

func TestMain(m *testing.M) {
	// Check to see if we should run tests, or instead invoke Start and act as
	// it if we are actually running. This is used by the tests.
	switch os.Getenv("FAAS_TEST_MODE") {
	case "HAPPY_PATH_ECHO":
		faas.Start(faas.HandlerFunc(func(r faas.Request) (faas.Response, error) {
			data, err := json.Marshal(r)
			if err != nil {
				return faas.Response{}, err
			}

			return faas.Response{
				StatusCode: 200,
				Body:       data,
			}, nil
		}))
	case "UNHAPPY_PATH_ERR":
		faas.Start(faas.HandlerFunc(func(r faas.Request) (faas.Response, error) {
			return faas.Response{}, errors.New("some-error")
		}))
	default:
		os.Exit(m.Run())
	}
}

type TF struct {
	*testing.T
	execPath string
	server   *httptest.Server

	mu        sync.Mutex
	requests  []*http.Request
	responses []faas.Response

	writer func(io.Writer) int
}

func (t *TF) Requests() []*http.Request {
	t.mu.Lock()
	defer t.mu.Unlock()
	results := make([]*http.Request, len(t.requests))
	copy(results, t.requests)
	return results
}

func (t *TF) Responses() []faas.Response {
	t.mu.Lock()
	defer t.mu.Unlock()
	results := make([]faas.Response, len(t.responses))
	copy(results, t.responses)
	return results
}

func TestFaas(t *testing.T) {
	t.Parallel()
	o := onpar.New()
	defer o.Run(t)

	o.BeforeEach(func(t *testing.T) *TF {
		path, err := os.Executable()
		if err != nil {
			panic(err)
		}

		tf := &TF{
			T:        t,
			execPath: path,
		}

		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer r.Body.Close()

			tf.mu.Lock()
			defer tf.mu.Unlock()
			tf.requests = append(tf.requests, r)

			var resp faas.Response
			json.NewDecoder(r.Body).Decode(&resp)
			tf.responses = append(tf.responses, resp)

			if r.Method == http.MethodGet {
				buf := bytes.Buffer{}
				w.WriteHeader(tf.writer(&buf))
				w.Write(buf.Bytes())
			}
		}))

		tf.server = server

		return tf
	})

	o.AfterEach(func(t *TF) {
		t.server.Close()
	})

	o.Spec("GETs data from the configured endpoint and POSTs the results", func(t *TF) {
		t.writer = func(w io.Writer) int {
			json.NewEncoder(w).Encode(faas.Request{
				Path: "some-path",
			})
			return 200
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		cmd := exec.CommandContext(ctx, t.execPath)
		cmd.Env = []string{
			"FAAS_TEST_MODE=HAPPY_PATH_ECHO",

			"SKIP_SSL_VALIDATION=true",
			fmt.Sprintf("CF_FAAS_RELAY_ADDR=%s", t.server.URL),
			"CF_AUTH_TOKEN=some-token",
			"X_CF_APP_INSTANCE=some-app-instance",
		}

		Expect(t, cmd.Start()).To(BeNil())
		defer cmd.Wait()
		defer cancel()

		Expect(t, t.Requests).To(ViaPolling(HaveLen(2)))

		Expect(t, t.Requests()[0].Method).To(Equal(http.MethodGet))
		Expect(t, t.Requests()[0].Header.Get("Authorization")).To(Equal("some-token"))
		Expect(t, t.Requests()[0].Header.Get("X-CF-APP-INSTANCE")).To(Equal("some-app-instance"))

		Expect(t, t.Requests()[1].Method).To(Equal(http.MethodPost))
		Expect(t, t.Requests()[1].Header.Get("Authorization")).To(Equal("some-token"))
		Expect(t, t.Requests()[1].Header.Get("X-CF-APP-INSTANCE")).To(Equal("some-app-instance"))

		Expect(t, t.Responses()[1].StatusCode).To(Equal(http.StatusOK))
		var rr faas.Request
		Expect(t, json.Unmarshal(t.Responses()[1].Body, &rr)).To(BeNil())
		Expect(t, rr.Path).To(Equal("some-path"))
	})

	o.Spec("returns a 500 for an error from the handler", func(t *TF) {
		t.writer = func(w io.Writer) int {
			json.NewEncoder(w).Encode(faas.Request{
				Path: "some-path",
			})
			return 200
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		cmd := exec.CommandContext(ctx, t.execPath)
		cmd.Env = []string{
			"FAAS_TEST_MODE=UNHAPPY_PATH_ERR",

			"SKIP_SSL_VALIDATION=true",
			fmt.Sprintf("CF_FAAS_RELAY_ADDR=%s", t.server.URL),
			"CF_AUTH_TOKEN=some-token",
			"X_CF_APP_INSTANCE=some-app-instance",
		}

		Expect(t, cmd.Start()).To(BeNil())
		defer cmd.Wait()
		defer cancel()

		Expect(t, t.Requests).To(ViaPolling(HaveLen(2)))
		Expect(t, t.Responses()[1].StatusCode).To(Equal(http.StatusInternalServerError))
	})

	o.Spec("it does not continue with non-200", func(t *TF) {
		t.writer = func(w io.Writer) int {
			json.NewEncoder(w).Encode(faas.Request{
				Path: "some-path",
			})
			return 400
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		cmd := exec.CommandContext(ctx, t.execPath)
		cmd.Env = []string{
			"FAAS_TEST_MODE=HAPPY_PATH_ECHO",

			"SKIP_SSL_VALIDATION=true",
			fmt.Sprintf("CF_FAAS_RELAY_ADDR=%s", t.server.URL),
			"CF_AUTH_TOKEN=some-token",
			"X_CF_APP_INSTANCE=some-app-instance",
		}

		Expect(t, cmd.Start()).To(BeNil())
		defer cmd.Wait()
		defer cancel()

		Expect(t, t.Requests).To(ViaPolling(HaveLen(1)))
	})

	o.Spec("it does not continue with bad JSON request", func(t *TF) {
		t.writer = func(w io.Writer) int {
			w.Write([]byte("invalid"))
			return 200
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		cmd := exec.CommandContext(ctx, t.execPath)
		cmd.Env = []string{
			"FAAS_TEST_MODE=HAPPY_PATH_ECHO",

			"SKIP_SSL_VALIDATION=true",
			fmt.Sprintf("CF_FAAS_RELAY_ADDR=%s", t.server.URL),
			"CF_AUTH_TOKEN=some-token",
			"X_CF_APP_INSTANCE=some-app-instance",
		}

		Expect(t, cmd.Start()).To(BeNil())
		defer cmd.Wait()
		defer cancel()

		Expect(t, t.Requests).To(ViaPolling(HaveLen(1)))
	})
}
