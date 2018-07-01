package handlers_test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	faas "github.com/apoydence/cf-faas"
	"github.com/apoydence/cf-faas/internal/handlers"
	"github.com/apoydence/onpar"
	. "github.com/apoydence/onpar/expect"
	. "github.com/apoydence/onpar/matchers"
)

type TT struct {
	*testing.T
	spyTaskRunner *spyTaskRunner
	h             faas.Handler
}

func TestRunTask(t *testing.T) {
	t.Parallel()
	o := onpar.New()
	defer o.Run(t)

	o.BeforeEach(func(t *testing.T) TT {
		spyTaskRunner := newSpyTaskRunner()
		return TT{
			T:             t,
			spyTaskRunner: spyTaskRunner,
			h: handlers.NewRunTask(
				"some-command",
				[]string{"a", "b"},
				spyTaskRunner,
				log.New(ioutil.Discard, "", 0),
			),
		}
	})

	o.Spec("runs a task", func(t TT) {
		t.spyTaskRunner.result = "task-guid"
		resp, err := t.h.Handle(faas.Request{})
		Expect(t, err).To(BeNil())

		Expect(t, t.spyTaskRunner.command).To(ContainSubstring("some-command"))
		Expect(t, t.spyTaskRunner.name).To(Not(Equal("")))
		Expect(t, resp.StatusCode).To(Equal(http.StatusOK))
		Expect(t, resp.Body).To(MatchJSON(fmt.Sprintf(`{"task_guid":"task-guid","task_name":%q}`, t.spyTaskRunner.name)))
	})

	o.Spec("it includes expected headers in the name", func(t TT) {
		req := faas.Request{
			Path:   "/v1/some/path",
			Method: "GET",
			Header: http.Header{
				"A": []string{"b", "c"},
				"D": []string{"e"},
			},
		}
		t.h.Handle(req)

		nameA := t.spyTaskRunner.name

		req = faas.Request{
			Path:   "/v1/some/path",
			Method: "GET",
			Header: http.Header{
				"A": []string{"x", "y"},
				"D": []string{"e"},
			},
		}
		t.h.Handle(req)

		nameB := t.spyTaskRunner.name
		Expect(t, nameA).To(Not(Equal(nameB)))
	})

	o.Spec("names a task deterministically for the expected headers", func(t TT) {
		req := faas.Request{
			Path:   "/v1/some/path",
			Method: "GET",
			Header: http.Header{
				"A": []string{"b", "c"},
				"D": []string{"e"},
			},
		}
		t.h.Handle(req)

		Expect(t, t.spyTaskRunner.name).To(Not(Equal("")))
		name := t.spyTaskRunner.name
		for i := 0; i < 1000; i++ {
			req := faas.Request{
				Path:   "/v1/some/path",
				Method: "GET",
				Header: http.Header{
					"A": []string{"b", "c"},
					"D": []string{"e"},
					fmt.Sprintf("dont-include-%d", time.Now().UnixNano()): []string{fmt.Sprint(time.Now().UnixNano())},
				},
			}
			t.h.Handle(req)

			Expect(t, t.spyTaskRunner.name).To(Equal(name))
		}
	})

	o.Spec("it returns an error if the TaskRunner returns an error", func(t TT) {
		t.spyTaskRunner.err = errors.New("some-error")
		_, err := t.h.Handle(faas.Request{})
		Expect(t, err).To(Not(BeNil()))
	})
}

type spyDoer struct {
	mu     sync.Mutex
	m      map[string]*http.Response
	reqs   []*http.Request
	bodies [][]byte

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
	s.reqs = append(s.reqs, req)

	var body []byte
	if req.Body != nil {
		var err error
		body, err = ioutil.ReadAll(req.Body)
		if err != nil {
			panic(err)
		}
	}
	s.bodies = append(s.bodies, body)

	r, ok := s.m[fmt.Sprintf("%s:%s", req.Method, req.URL.String())]
	if !ok {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(strings.NewReader(`{"state":"SUCCEEDED"}`)),
		}, s.err
	}

	return r, s.err
}

func (s *spyDoer) Reqs() []*http.Request {
	s.mu.Lock()
	defer s.mu.Unlock()

	results := make([]*http.Request, len(s.reqs))
	copy(results, s.reqs)
	return results
}

type spyTaskRunner struct {
	command string
	name    string
	result  string
	err     error
}

func newSpyTaskRunner() *spyTaskRunner {
	return &spyTaskRunner{}
}

func (s *spyTaskRunner) RunTask(command, name string) (string, error) {
	s.command = command
	s.name = name
	return s.result, s.err
}
