package manifest_test

import (
	"bytes"
	"errors"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/apoydence/cf-faas/internal/manifest"
	"github.com/apoydence/onpar"
	. "github.com/apoydence/onpar/expect"
	. "github.com/apoydence/onpar/matchers"
)

type TR struct {
	*testing.T
	spyDoer *spyDoer
	r       *manifest.Resolver
}

func TestResolver(t *testing.T) {
	t.Parallel()
	o := onpar.New()
	defer o.Run(t)

	o.BeforeEach(func(t *testing.T) TR {
		spyDoer := newSpyDoer()

		spyDoer.m["POST:http://url.a"] = &http.Response{
			StatusCode: 200,
			Body: ioutil.NopCloser(strings.NewReader(`{
				"functions":[
					{
						"handler":{
							"command":"some-command"
						},
						"events": [{
						  "path":"/v1/a1",
						  "method":"PUT"
					    }]
					},
					{
						"handler":{
							"command":"some-command"
						},
						"events": [{
						  "path":"/v1/a2",
						  "method":"DELETE"
					    }]
					}
				]
			}`)),
		}

		spyDoer.m["POST:http://url.b"] = &http.Response{
			StatusCode: 200,
			Body: ioutil.NopCloser(strings.NewReader(`{
				"functions":[
					{
						"handler":{
							"command":"some-command"
						},
						"events": [{
						  "path":"/v1/b1",
						  "method":"PUT"
					    }]
					}
				]
			}`)),
		}

		spyDoer.m["POST:http://invalid.json"] = &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(strings.NewReader(`invalid`)),
		}

		spyDoer.m["POST:http://invalid.event"] = &http.Response{
			StatusCode: 200,
			Body: ioutil.NopCloser(strings.NewReader(`{
				"functions":[
					{
						"events": [{
						  "path":"/v1/b1",
						  "method":"PUT"
					    }]
					}
				]
			}`)),
		}

		spyDoer.m["POST:http://invalid.status"] = &http.Response{
			StatusCode: 400,
			Body: ioutil.NopCloser(strings.NewReader(`{
				"functions":[
					{
						"handler":{
							"command":"some-command"
						},
						"events": [{
						  "path":"/v1/b1",
						  "method":"PUT"
					    }]
					}
				]
			}`)),
		}

		return TR{
			T:       t,
			spyDoer: spyDoer,
			r: manifest.NewResolver(map[string]string{
				"other-a":        "http://url.a",
				"other-b":        "http://url.b",
				"invalid-url":    "-:-",
				"invalid-json":   "http://invalid.json",
				"invalid-event":  "http://invalid.event",
				"invalid-status": "http://invalid.status",
			}, spyDoer),
		}
	})

	o.Spec("resolves all events", func(t TR) {
		fs, err := t.r.Resolve(manifest.Manifest{
			Functions: []manifest.Function{
				{
					Handler: manifest.Handler{
						Command: "some-command",
					},
					Events: map[string][]map[string]interface{}{
						"http": []map[string]interface{}{
							{
								"path":   "/v1/path",
								"method": "GET",
								"cache": map[string]interface{}{
									"duration": "1m",
								},
							},
							{
								"path":   "/v1/other-path",
								"method": "PUT",
							},
						},
						"other-a": []map[string]interface{}{
							{
								"some-key": "some-data",
							},
						},
						"other-b": []map[string]interface{}{
							{
								"some-other-key": "some-other-data",
							},
						},
					},
				},
			},
		})

		Expect(t, err).To(BeNil())
		Expect(t, fs).To(HaveLen(4))
		Expect(t, fs).To(Contain(
			manifest.HTTPFunction{
				Handler: manifest.Handler{
					Command: "some-command",
				},
				Events: []manifest.HTTPEvent{
					{
						Path:   "/v1/path",
						Method: "GET",
						Cache: struct {
							Duration time.Duration `yaml:"duration"`
							Header   []string      `yaml:"header"`
						}{
							Duration: time.Minute,
						},
					},
					{
						Path:   "/v1/other-path",
						Method: "PUT",
					},
				},
			},
		))

		Expect(t, fs).To(Contain(
			manifest.HTTPFunction{
				Handler: manifest.Handler{
					Command: "some-command",
				},
				Events: []manifest.HTTPEvent{
					{
						Path:   "/v1/a1",
						Method: "PUT",
					},
				},
			},
		))

		Expect(t, fs).To(Contain(
			manifest.HTTPFunction{
				Handler: manifest.Handler{
					Command: "some-command",
				},
				Events: []manifest.HTTPEvent{
					{
						Path:   "/v1/a2",
						Method: "DELETE",
					},
				},
			},
		))

		Expect(t, fs).To(Contain(
			manifest.HTTPFunction{
				Handler: manifest.Handler{
					Command: "some-command",
				},
				Events: []manifest.HTTPEvent{
					{
						Path:   "/v1/b1",
						Method: "PUT",
					},
				},
			},
		))
	})

	o.Spec("it sends the function to the URL", func(t TR) {
		_, err := t.r.Resolve(manifest.Manifest{
			Functions: []manifest.Function{
				{
					Handler: manifest.Handler{
						Command: "some-command",
					},
					Events: map[string][]map[string]interface{}{
						"other-a": []map[string]interface{}{
							{
								"some-key": "some-data",
							},
						},
					},
				},
			},
		})

		Expect(t, err).To(BeNil())
		Expect(t, t.spyDoer.reqs).To(HaveLen(1))
		Expect(t, t.spyDoer.bodies[0]).To(MatchJSON(`{
			"handler": {
				"command":"some-command"
			},
			"events": {
				"other-a":[
					{
						"some-key":"some-data"
					}
				]
			}
		}`))
	})

	o.Spec("it returns an error if a URL fails to parse", func(t TR) {
		_, err := t.r.Resolve(manifest.Manifest{
			Functions: []manifest.Function{
				{
					Handler: manifest.Handler{
						Command: "some-command",
					},
					Events: map[string][]map[string]interface{}{
						"invalid-url": []map[string]interface{}{
							{
								"some-key": "some-data",
							},
						},
					},
				},
			},
		})

		Expect(t, err).To(Not(BeNil()))
	})

	o.Spec("it returns an error if the doer fails", func(t TR) {
		t.spyDoer.err = errors.New("some-error")
		_, err := t.r.Resolve(manifest.Manifest{
			Functions: []manifest.Function{
				{
					Handler: manifest.Handler{
						Command: "some-command",
					},
					Events: map[string][]map[string]interface{}{
						"other-a": []map[string]interface{}{
							{
								"some-key": "some-data",
							},
						},
					},
				},
			},
		})

		Expect(t, err).To(Not(BeNil()))
	})

	o.Spec("it returns an error if a result is not valid JSON", func(t TR) {
		_, err := t.r.Resolve(manifest.Manifest{
			Functions: []manifest.Function{
				{
					Handler: manifest.Handler{
						Command: "some-command",
					},
					Events: map[string][]map[string]interface{}{
						"invalid-json": []map[string]interface{}{
							{
								"some-key": "some-data",
							},
						},
					},
				},
			},
		})

		Expect(t, err).To(Not(BeNil()))
	})

	o.Spec("it returns an error if a result is not a valid HTTPFunction", func(t TR) {
		_, err := t.r.Resolve(manifest.Manifest{
			Functions: []manifest.Function{
				{
					Handler: manifest.Handler{
						Command: "some-command",
					},
					Events: map[string][]map[string]interface{}{
						"invalid-event": []map[string]interface{}{
							{
								"some-key": "some-data",
							},
						},
					},
				},
			},
		})

		Expect(t, err).To(Not(BeNil()))
	})

	o.Spec("it returns an error if a result is not a 200", func(t TR) {
		_, err := t.r.Resolve(manifest.Manifest{
			Functions: []manifest.Function{
				{
					Handler: manifest.Handler{
						Command: "some-command",
					},
					Events: map[string][]map[string]interface{}{
						"invalid-status": []map[string]interface{}{
							{
								"some-key": "some-data",
							},
						},
					},
				},
			},
		})

		Expect(t, err).To(Not(BeNil()))
	})
}

type spyDoer struct {
	m      map[string]*http.Response
	reqs   []*http.Request
	bodies []string
	err    error
}

func newSpyDoer() *spyDoer {
	return &spyDoer{
		m: make(map[string]*http.Response),
	}
}

func (s *spyDoer) Do(r *http.Request) (*http.Response, error) {
	s.reqs = append(s.reqs, r)

	if r.Body == nil {
		r.Body = ioutil.NopCloser(bytes.NewReader(nil))
	}

	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		panic(err)
	}
	s.bodies = append(s.bodies, string(data))

	if resp, ok := s.m[r.Method+":"+r.URL.String()]; ok {
		return resp, s.err
	}

	panic(r.Method + ":" + r.URL.String())
}
