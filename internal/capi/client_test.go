package capi_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/apoydence/cf-faas/internal/capi"
	"github.com/apoydence/onpar"
	. "github.com/apoydence/onpar/expect"
	. "github.com/apoydence/onpar/matchers"
)

type TC struct {
	*testing.T
	spyDoer *spyDoer
	c       *capi.Client
}

func TestClientCreateTask(t *testing.T) {
	t.Parallel()
	o := onpar.New()
	defer o.Run(t)

	o.BeforeEach(func(t *testing.T) TC {
		spyDoer := newSpyDoer()
		return TC{
			T:       t,
			spyDoer: spyDoer,
			c:       capi.NewClient("http://some-addr.com", "some-guid", "space-guid", time.Millisecond, spyDoer),
		}
	})

	o.Spec("it hits CAPI correct", func(t TC) {
		err := t.c.CreateTask(context.Background(), "some-command")
		Expect(t, err).To(BeNil())

		Expect(t, t.spyDoer.req.Method).To(Equal("POST"))
		Expect(t, t.spyDoer.req.URL.String()).To(Equal("http://some-addr.com/v3/apps/some-guid/tasks"))
		Expect(t, t.spyDoer.req.Header.Get("Content-Type")).To(Equal("application/json"))
		Expect(t, t.spyDoer.body).To(MatchJSON(`{"command":"some-command"}`))
	})

	o.Spec("it includes the droplet guid if provided", func(t TC) {
		err := t.c.CreateTask(context.Background(), "some-command")
		Expect(t, err).To(BeNil())

		Expect(t, t.spyDoer.req.Method).To(Equal("POST"))
		Expect(t, t.spyDoer.req.URL.String()).To(Equal("http://some-addr.com/v3/apps/some-guid/tasks"))
		Expect(t, t.spyDoer.req.Header.Get("Content-Type")).To(Equal("application/json"))
		Expect(t, t.spyDoer.body).To(MatchJSON(`{"command":"some-command"}`))
	})

	o.Spec("it requests the status of the task", func(t TC) {
		t.spyDoer.m["POST:http://some-addr.com/v3/apps/some-guid/tasks"] = &http.Response{
			StatusCode: 202,
			Body:       ioutil.NopCloser(strings.NewReader(`{"lines":{"self":{"href":"https://xx.succeeded"}},"state":"RUNNING"}`)),
		}

		t.spyDoer.m["GET:http://xx.succeeded"] = &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(strings.NewReader(`{"links":{"self":{"href":"https://xx.succeeded"}},"state":"SUCCEEDED"}`)),
		}
		err := t.c.CreateTask(context.Background(), "some-command")
		Expect(t, err).To(BeNil())

		t.spyDoer.m["POST:http://some-addr.com/v3/apps/some-other-guid/tasks"] = &http.Response{
			StatusCode: 202,
			Body:       ioutil.NopCloser(strings.NewReader(`{"links":{"self":{"href":"http://xx.failed"}},"state":"RUNNING"}`)),
		}

		t.spyDoer.m["GET:http://xx.failed"] = &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(strings.NewReader(`{"guid":"task-guid","state":"FAILED"}`)),
		}
		err = t.c.CreateTask(context.Background(), "some-command")
		Expect(t, err).To(Not(BeNil()))
	})

	o.Spec("context cancels the request", func(t TC) {
		t.spyDoer.m["POST:http://some-addr.com/v3/apps/some-guid/tasks"] = &http.Response{
			StatusCode: 202,
			Body:       ioutil.NopCloser(strings.NewReader(`{"lines":{"self":{"href":"http://xx.succeeded"}},"state":"RUNNING"}`)),
		}

		t.spyDoer.m["GET:http://xx.succeeded"] = &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(strings.NewReader(`{"links":{"self":{"href":"http://xx.succeeded"}},"state":"RUNNING"}`)),
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		t.c.CreateTask(ctx, "some-command")
		Expect(t, t.spyDoer.req.Context().Err()).To(Not(BeNil()))
	})

	o.Spec("it returns an error if a non-202 is received", func(t TC) {
		t.spyDoer.m["POST:http://some-addr.com/v3/apps/some-guid/tasks"] = &http.Response{
			StatusCode: 500,
			Body:       ioutil.NopCloser(bytes.NewReader(nil)),
		}
		err := t.c.CreateTask(context.Background(), "some-command")
		Expect(t, err).To(Not(BeNil()))
	})

	o.Spec("it returns an error if the addr is invalid", func(t TC) {
		t.c = capi.NewClient("::invalid", "some-id", "space-guid", time.Millisecond, t.spyDoer)
		err := t.c.CreateTask(context.Background(), "some-command")
		Expect(t, err).To(Not(BeNil()))
	})

	o.Spec("it returns an error if the request fails", func(t TC) {
		t.spyDoer.err = errors.New("some-error")
		err := t.c.CreateTask(context.Background(), "some-command")
		Expect(t, err).To(Not(BeNil()))
	})
}

func TestClientListTasks(t *testing.T) {
	t.Parallel()
	o := onpar.New()
	defer o.Run(t)

	o.BeforeEach(func(t *testing.T) TC {
		spyDoer := newSpyDoer()

		spyDoer.m["GET:http://some-addr.com/v3/apps/some-guid/tasks"] = &http.Response{
			StatusCode: 200,
			Body: ioutil.NopCloser(strings.NewReader(
				`{
					"pagination": {
					  "next": {
					    "href": "http://some-addr.com/v3/apps/some-guid/tasks?page=2&per_page=2"
					  }
					},
					"resources":[
					  {"name": "task-1"},
					  {"name": "task-2"},
					  {"name": "task-3"}
					]
				}`,
			)),
		}

		spyDoer.m["GET:http://some-addr.com/v3/apps/some-guid/tasks?page=2&per_page=2"] = &http.Response{
			StatusCode: 200,
			Body: ioutil.NopCloser(strings.NewReader(
				`{
					"resources":[
					  {"name": "task-4"},
					  {"name": "task-5"},
					  {"name": "task-6"}
					]
				}`,
			)),
		}

		return TC{
			T:       t,
			spyDoer: spyDoer,
			c:       capi.NewClient("http://some-addr.com", "some-id", "space-guid", time.Millisecond, spyDoer),
		}
	})

	o.Spec("it hits CAPI correct", func(t TC) {
		tasks, err := t.c.ListTasks("some-guid")
		Expect(t, err).To(BeNil())

		Expect(t, tasks).To(Equal([]string{
			"task-1", "task-2", "task-3", // Page 1
			"task-4", "task-5", "task-6", // Page 2
		}))
	})

	o.Spec("it returns an error if a non-200 is received", func(t TC) {
		t.spyDoer.m["GET:http://some-addr.com/v3/apps/some-guid/tasks"] = &http.Response{
			StatusCode: 500,
			Body:       ioutil.NopCloser(bytes.NewReader(nil)),
		}
		_, err := t.c.ListTasks("some-guid")
		Expect(t, err).To(Not(BeNil()))
	})

	o.Spec("it returns an error if the request fails", func(t TC) {
		t.spyDoer.err = errors.New("some-error")
		_, err := t.c.ListTasks("some-guid")
		Expect(t, err).To(Not(BeNil()))
	})

	o.Spec("it returns an error if the response is invalid JSON", func(t TC) {
		t.spyDoer.m["GET:http://some-addr.com/v3/apps/some-guid/tasks"] = &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(strings.NewReader(`invalid`)),
		}

		_, err := t.c.ListTasks("some-guid")
		Expect(t, err).To(Not(BeNil()))
	})
}

func TestClientGetAppGuid(t *testing.T) {
	t.Parallel()
	o := onpar.New()
	defer o.Run(t)

	o.BeforeEach(func(t *testing.T) TC {
		spyDoer := newSpyDoer()
		return TC{
			T:       t,
			spyDoer: spyDoer,
			c:       capi.NewClient("http://some-addr.com", "some-id", "space-guid", time.Millisecond, spyDoer),
		}
	})

	o.Spec("it hits CAPI correct", func(t TC) {
		t.spyDoer.m["GET:http://some-addr.com/v2/apps?q=name%3Asome-name&q=space_guid%3Aspace-guid"] = &http.Response{
			StatusCode: 200,
			Body: ioutil.NopCloser(strings.NewReader(
				`{
					"resources": [{
					  "metadata": {
					    "guid": "some-guid"
					  }
					}]
				}`,
			)),
		}

		guid, err := t.c.GetAppGuid(context.Background(), "some-name")
		Expect(t, err).To(BeNil())

		Expect(t, guid).To(Equal("some-guid"))

		Expect(t, t.spyDoer.req.Method).To(Equal("GET"))
		Expect(t, t.spyDoer.req.Header.Get("Accept")).To(Equal("application/json"))
	})

	o.Spec("it returns an error for empty results", func(t TC) {
		t.spyDoer.m["GET:http://some-addr.com/v2/apps?q=name%3Asome-name&q=space_guid%3Aspace-guid"] = &http.Response{
			StatusCode: 200,
			Body: ioutil.NopCloser(strings.NewReader(
				`{
					"resources": []
				}`,
			)),
		}

		_, err := t.c.GetAppGuid(context.Background(), "some-name")
		Expect(t, err).To(Not(BeNil()))
	})

	o.Spec("it returns an error if a non-200 is received", func(t TC) {
		t.spyDoer.m["GET:http://some-addr.com/v2/apps?q=name%3Asome-name&q=space_guid%3Aspace-guid"] = &http.Response{
			StatusCode: 500,
			Body:       ioutil.NopCloser(bytes.NewReader(nil)),
		}
		_, err := t.c.GetAppGuid(context.Background(), "some-name")
		Expect(t, err).To(Not(BeNil()))
	})

	o.Spec("it returns an error if the request fails", func(t TC) {
		t.spyDoer.err = errors.New("some-error")
		_, err := t.c.GetAppGuid(context.Background(), "some-name")
		Expect(t, err).To(Not(BeNil()))
	})

	o.Spec("it returns an error if the response is invalid JSON", func(t TC) {
		t.spyDoer.m["GET:http://some-addr.com/v2/apps?q=name%3Asome-name&q=space_guid%3Aspace-guid"] = &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(strings.NewReader(`invalid`)),
		}

		_, err := t.c.GetAppGuid(context.Background(), "some-name")
		Expect(t, err).To(Not(BeNil()))
	})

	o.Spec("it uses the given context", func(t TC) {
		t.spyDoer.m["GET:http://some-addr.com/v2/apps?q=name%3Asome-name&q=space_guid%3Aspace-guid"] = &http.Response{
			StatusCode: 200,
			Body: ioutil.NopCloser(strings.NewReader(
				`{
					"resources": [{
					  "metadata": {
					    "guid": "some-guid"
					  }
					}]
				}`,
			)),
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		t.c.GetAppGuid(ctx, "some-name")
		Expect(t, t.spyDoer.req.Context().Err()).To(Not(BeNil()))
	})
}

func TestClientGetDropletGuid(t *testing.T) {
	t.Parallel()
	o := onpar.New()
	defer o.Run(t)

	o.BeforeEach(func(t *testing.T) TC {
		spyDoer := newSpyDoer()
		return TC{
			T:       t,
			spyDoer: spyDoer,
			c:       capi.NewClient("http://some-addr.com", "some-id", "space-guid", time.Millisecond, spyDoer),
		}
	})

	o.Spec("it hits CAPI correct", func(t TC) {
		t.spyDoer.m["GET:http://some-addr.com/v3/apps/some-guid/droplets/current"] = &http.Response{
			StatusCode: 200,
			Body: ioutil.NopCloser(strings.NewReader(
				`{
				   "guid": "droplet-guid"
				}`,
			)),
		}

		guid, err := t.c.GetDropletGuid(context.Background(), "some-guid")
		Expect(t, err).To(BeNil())

		Expect(t, guid).To(Equal("droplet-guid"))

		Expect(t, t.spyDoer.req.Method).To(Equal("GET"))
		Expect(t, t.spyDoer.req.Header.Get("Accept")).To(Equal("application/json"))
	})

	o.Spec("it returns an error for empty results", func(t TC) {
		t.spyDoer.m["GET:http://some-addr.com/v3/apps/app-guid/droplets/current"] = &http.Response{
			StatusCode: 200,
			Body: ioutil.NopCloser(strings.NewReader(
				`{
				}`,
			)),
		}

		_, err := t.c.GetDropletGuid(context.Background(), "app-guid")
		Expect(t, err).To(Not(BeNil()))
	})

	o.Spec("it returns an error if a non-200 is received", func(t TC) {
		t.spyDoer.m["GET:http://some-addr.com/v3/apps/app-guid/droplets/current"] = &http.Response{
			StatusCode: 500,
			Body:       ioutil.NopCloser(bytes.NewReader(nil)),
		}
		_, err := t.c.GetDropletGuid(context.Background(), "app-guid")
		Expect(t, err).To(Not(BeNil()))
	})

	o.Spec("it returns an error if the request fails", func(t TC) {
		t.spyDoer.err = errors.New("some-error")
		_, err := t.c.GetDropletGuid(context.Background(), "app-guid")
		Expect(t, err).To(Not(BeNil()))
	})

	o.Spec("it returns an error if the response is invalid JSON", func(t TC) {
		t.spyDoer.m["GET:http://some-addr.com/v3/apps/app-guid/droplets/current"] = &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(strings.NewReader(`invalid`)),
		}

		_, err := t.c.GetDropletGuid(context.Background(), "app-guid")
		Expect(t, err).To(Not(BeNil()))
	})

	o.Spec("it uses the given context", func(t TC) {
		t.spyDoer.m["GET:http://some-addr.com/v3/apps/app-guid/droplets/current"] = &http.Response{
			StatusCode: 200,
			Body: ioutil.NopCloser(strings.NewReader(
				`{
				  "guid": "app-guid"
				}`,
			)),
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		t.c.GetDropletGuid(ctx, "app-guid")
		Expect(t, t.spyDoer.req.Context().Err()).To(Not(BeNil()))
	})
}

func TestClientGetPackageGuid(t *testing.T) {
	t.Parallel()
	o := onpar.New()
	defer o.Run(t)

	o.BeforeEach(func(t *testing.T) TC {
		spyDoer := newSpyDoer()
		return TC{
			T:       t,
			spyDoer: spyDoer,
			c:       capi.NewClient("http://some-addr.com", "some-id", "space-guid", time.Millisecond, spyDoer),
		}
	})

	o.Spec("it hits CAPI correct", func(t TC) {
		t.spyDoer.m["GET:http://some-addr.com/v3/apps/app-guid/droplets/current"] = &http.Response{
			StatusCode: 200,
			Body: ioutil.NopCloser(strings.NewReader(
				`{
				   "links": {
					   "package":{
						   "href":"https://xxx.1"
					   }
				   }
				}`,
			)),
		}

		t.spyDoer.m["GET:http://xxx.1"] = &http.Response{
			StatusCode: 200,
			Body: ioutil.NopCloser(strings.NewReader(
				`{
					"guid":"package-guid",
				    "links": {
					   "download":{
						   "href":"https://xxx.2"
					   }
				   }
				}`,
			)),
		}

		guid, download, err := t.c.GetPackageGuid(context.Background(), "app-guid")
		Expect(t, err).To(BeNil())

		Expect(t, guid).To(Equal("package-guid"))
		Expect(t, download).To(Equal("http://xxx.2"))

		Expect(t, t.spyDoer.req.Method).To(Equal("GET"))
		Expect(t, t.spyDoer.req.Header.Get("Accept")).To(Equal("application/json"))
	})

	o.Spec("it returns an error for empty results", func(t TC) {
		t.spyDoer.m["GET:http://some-addr.com/v3/apps/app-guid/droplets/current"] = &http.Response{
			StatusCode: 200,
			Body: ioutil.NopCloser(strings.NewReader(
				`{
				}`,
			)),
		}

		_, _, err := t.c.GetPackageGuid(context.Background(), "app-guid")
		Expect(t, err).To(Not(BeNil()))
	})

	o.Spec("it returns an error if a non-200 is received", func(t TC) {
		t.spyDoer.m["GET:http://some-addr.com/v3/apps/app-guid/droplets/current"] = &http.Response{
			StatusCode: 500,
			Body:       ioutil.NopCloser(bytes.NewReader(nil)),
		}
		_, _, err := t.c.GetPackageGuid(context.Background(), "app-guid")
		Expect(t, err).To(Not(BeNil()))
	})

	o.Spec("it returns an error if the request fails", func(t TC) {
		t.spyDoer.err = errors.New("some-error")
		_, _, err := t.c.GetPackageGuid(context.Background(), "app-guid")
		Expect(t, err).To(Not(BeNil()))
	})

	o.Spec("it returns an error if the response is invalid JSON", func(t TC) {
		t.spyDoer.m["GET:http://some-addr.com/v3/apps/app-guid/droplets/current"] = &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(strings.NewReader(`invalid`)),
		}

		_, _, err := t.c.GetPackageGuid(context.Background(), "app-guid")
		Expect(t, err).To(Not(BeNil()))
	})

	o.Spec("it uses the given context", func(t TC) {
		t.spyDoer.m["GET:http://some-addr.com/v3/apps/app-guid/droplets/current"] = &http.Response{
			StatusCode: 200,
			Body: ioutil.NopCloser(strings.NewReader(
				`{
				  "guid": "app-guid"
				}`,
			)),
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		t.c.GetPackageGuid(ctx, "app-guid")
		Expect(t, t.spyDoer.req.Context().Err()).To(Not(BeNil()))
	})
}

type spyDoer struct {
	mu   sync.Mutex
	m    map[string]*http.Response
	req  *http.Request
	body []byte

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
			StatusCode: 202,
			Body:       ioutil.NopCloser(strings.NewReader(`{"state":"SUCCEEDED"}`)),
		}, s.err
	}

	return r, s.err
}

func (s *spyDoer) Req() *http.Request {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.req
}
