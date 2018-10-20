package manifest_test

import (
	"testing"
	"time"

	"github.com/poy/cf-faas/internal/manifest"
	"github.com/poy/onpar"
	. "github.com/poy/onpar/expect"
	. "github.com/poy/onpar/matchers"
)

func TestManifestUnmarshal(t *testing.T) {
	t.Parallel()
	o := onpar.New()
	defer o.Run(t)

	o.Spec("it returns a manifest", func(t *testing.T) {
		var m manifest.Manifest
		err := m.UnmarshalEnv(`
functions:
- handler:
   app_name: faas-droplet-echo
   command: ./echo
  events:
    http:
    - path: /v1/goecho
      method: POST
      cache:
        duration: 1m
        sub-type:
          sub-key: sub-value
`)
		Expect(t, err).To(BeNil())

		Expect(t, m.Functions).To(HaveLen(1))
		Expect(t, m.Functions[0].Handler).To(Equal(manifest.Handler{
			AppName: "faas-droplet-echo",
			Command: "./echo",
		}))
		Expect(t, m.Functions[0].Events).To(HaveLen(1))
		Expect(t, m.Functions[0].Events["http"]).To(Equal(
			[]manifest.GenericData{
				{
					"path":   "/v1/goecho",
					"method": "POST",
					"cache": manifest.GenericData{
						"duration": "1m",
						"sub-type": manifest.GenericData{
							"sub-key": "sub-value",
						},
					},
				},
			},
		))
	})

	o.Spec("it returns an error if there are no functions", func(t *testing.T) {
		var m manifest.Manifest
		err := m.UnmarshalEnv(`{}`)
		Expect(t, err).To(Not(BeNil()))
	})

	o.Spec("it returns an error if there is a function without a command", func(t *testing.T) {
		var m manifest.Manifest
		err := m.UnmarshalEnv(`
functions:
- handler:
   app_name: faas-droplet-echo
  events:
    http:
    - path: /v1/goecho
      method: POST`)
		Expect(t, err).To(Not(BeNil()))
	})

	o.Spec("it returns an error if there is a function without any events", func(t *testing.T) {
		var m manifest.Manifest
		err := m.UnmarshalEnv(`
functions:
- handler:
   app_name: faas-droplet-echo
   command: ./echo`)
		Expect(t, err).To(Not(BeNil()))
	})
}

func TestManiestAppNames(t *testing.T) {
	t.Parallel()
	o := onpar.New()
	defer o.Run(t)

	o.Spec("it lists every app name used", func(t *testing.T) {
		m := manifest.Manifest{
			Functions: []manifest.Function{
				{
					Handler: manifest.Handler{
						AppName: "app-name-1",
					},
				},
				{
					Handler: manifest.Handler{
						AppName: "app-name-2",
					},
				},
				{
					Handler: manifest.Handler{
						AppName: "app-name-1",
					},
				},
				{
					Handler: manifest.Handler{
					// Use default
					},
				},
			},
		}

		Expect(t, m.AppNames("default-name")).To(HaveLen(3))
		Expect(t, m.AppNames("default-name")).To(Contain("app-name-1", "app-name-2", "default-name"))
	})
}

func TestManiestOpenEndpoints(t *testing.T) {
	t.Parallel()
	o := onpar.New()
	defer o.Run(t)

	o.Spec("it lists every open endpoint (no_auth=true)", func(t *testing.T) {
		m := manifest.Manifest{
			Functions: []manifest.Function{
				{
					Handler: manifest.Handler{
						AppName: "app-name-1",
					},
					Events: map[string][]manifest.GenericData{
						"http": []manifest.GenericData{
							{
								"path": "/v1/path",
							},
						},
					},
				},
				{
					Handler: manifest.Handler{
						AppName: "app-name-2",
					},
					Events: map[string][]manifest.GenericData{
						"http": []manifest.GenericData{
							{
								"path":    "/v2/path",
								"no_auth": true,
							},
						},
					},
				},
				{
					Handler: manifest.Handler{
						AppName: "app-name-1",
					},
					Events: map[string][]manifest.GenericData{
						"http": []manifest.GenericData{
							{
								"path":    "/v3/path",
								"no_auth": false,
							},
						},
					},
				},
				{
					Handler: manifest.Handler{
						AppName: "app-name-1",
					},
					Events: map[string][]manifest.GenericData{
						"http": []manifest.GenericData{
							{
								"path":    99, // obviously invalid
								"no_auth": true,
							},
						},
					},
				},
				{
					Handler: manifest.Handler{
					// Use default
					},
				},
			},
		}

		Expect(t, m.OpenEndpoints()).To(HaveLen(1))
		Expect(t, m.OpenEndpoints()).To(Contain("/v2/path"))
	})
}

func TestManiestEventNames(t *testing.T) {
	t.Parallel()
	o := onpar.New()
	defer o.Run(t)

	o.Spec("it lists every open endpoint (no_auth=true)", func(t *testing.T) {
		m := manifest.Manifest{
			Functions: []manifest.Function{
				{
					Handler: manifest.Handler{
						AppName: "app-name-1",
					},
					Events: map[string][]manifest.GenericData{
						"event": []manifest.GenericData{
							{
								"path": "/v1/path",
							},
						},
					},
				},
				{
					Handler: manifest.Handler{
						AppName: "app-name-2",
					},
					Events: map[string][]manifest.GenericData{
						"other-event": []manifest.GenericData{
							{
								"path": "/v2/path",
							},
						},
					},
				},
				{
					Handler: manifest.Handler{
						AppName: "app-name-1",
					},
					Events: map[string][]manifest.GenericData{
						"other-event": []manifest.GenericData{
							{
								"path": "/v3/path",
							},
						},
					},
				},
				{
					Handler: manifest.Handler{
					// Use default
					},
				},
			},
		}

		Expect(t, m.EventNames()).To(HaveLen(2))
		Expect(t, m.EventNames()).To(Contain("event", "other-event"))
	})
}

func TestHTTPFunctionValidate(t *testing.T) {
	t.Parallel()
	o := onpar.New()
	defer o.Run(t)

	o.Spec("it does notreturn an error if all is well", func(t *testing.T) {
		f := manifest.HTTPFunction{
			Handler: manifest.Handler{
				Command: "some-command",
			},
			Events: []manifest.HTTPEvent{
				{
					Path:   "/v1/path",
					Method: "GET",
				},
			},
		}

		Expect(t, f.Validate()).To(BeNil())
	})

	o.Spec("it returns an error if the Command is not set", func(t *testing.T) {
		f := manifest.HTTPFunction{
			Handler: manifest.Handler{},
			Events: []manifest.HTTPEvent{
				{
					Path:   "/v1/path",
					Method: "GET",
				},
			},
		}

		Expect(t, f.Validate()).To(Not(BeNil()))
	})

	o.Spec("it returns an error if there aren't any events", func(t *testing.T) {
		f := manifest.HTTPFunction{
			Handler: manifest.Handler{
				Command: "some-command",
			},
		}

		Expect(t, f.Validate()).To(Not(BeNil()))
	})

	o.Spec("it returns an error if the Handler.Path is not set", func(t *testing.T) {
		f := manifest.HTTPFunction{
			Handler: manifest.Handler{
				Command: "some-command",
			},
			Events: []manifest.HTTPEvent{
				{
					Method: "GET",
				},
			},
		}

		Expect(t, f.Validate()).To(Not(BeNil()))
	})

	o.Spec("it returns an error if the Handler.Method is not set", func(t *testing.T) {
		f := manifest.HTTPFunction{
			Handler: manifest.Handler{
				Command: "some-command",
			},
			Events: []manifest.HTTPEvent{
				{
					Path: "/v1/path",
				},
			},
		}

		Expect(t, f.Validate()).To(Not(BeNil()))
	})
}

func TestHTTPHTTPManifestUnmarshal(t *testing.T) {
	t.Parallel()
	o := onpar.New()
	defer o.Run(t)

	o.Spec("it returns a manifest", func(t *testing.T) {
		var m manifest.HTTPManifest
		err := m.UnmarshalEnv(`
functions:
- handler:
   app_name: faas-droplet-echo
   command: ./echo
  events:
  - path: /v1/goecho
    method: POST
    cache:
      duration: 1m
`)
		Expect(t, err).To(BeNil())

		Expect(t, m.Functions).To(HaveLen(1))
		Expect(t, m.Functions[0].Handler).To(Equal(manifest.Handler{
			AppName: "faas-droplet-echo",
			Command: "./echo",
		}))
		Expect(t, m.Functions[0].Events).To(HaveLen(1))
		Expect(t, m.Functions[0].Events).To(Contain(
			manifest.HTTPEvent{
				Path:   "/v1/goecho",
				Method: "POST",
				Cache: struct {
					Duration time.Duration `yaml:"duration"`
					Header   []string      `yaml:"header"`
				}{
					Duration: time.Minute,
				},
			},
		))
	})

	o.Spec("it handles an empty string", func(t *testing.T) {
		var m manifest.HTTPManifest
		err := m.UnmarshalEnv(``)

		Expect(t, err).To(BeNil())
	})

	o.Spec("it returns an error if there is a function without a command", func(t *testing.T) {
		var m manifest.HTTPManifest
		err := m.UnmarshalEnv(`
functions:
- handler:
   app_name: faas-droplet-echo
  events:
  - path: /v1/goecho
    method: POST`)
		Expect(t, err).To(Not(BeNil()))
	})

	o.Spec("it returns an error if there is a function without any events", func(t *testing.T) {
		var m manifest.HTTPManifest
		err := m.UnmarshalEnv(`
functions:
- handler:
   app_name: faas-droplet-echo
   command: ./echo`)
		Expect(t, err).To(Not(BeNil()))
	})
}

func TestHTTPManiestAppNames(t *testing.T) {
	t.Parallel()
	o := onpar.New()
	defer o.Run(t)

	o.Spec("it lists every app name used", func(t *testing.T) {
		m := manifest.HTTPManifest{
			Functions: []manifest.HTTPFunction{
				{
					Handler: manifest.Handler{
						AppName: "app-name-1",
					},
				},
				{
					Handler: manifest.Handler{
						AppName: "app-name-2",
					},
				},
				{
					Handler: manifest.Handler{
						AppName: "app-name-1",
					},
				},
				{
					Handler: manifest.Handler{
					// Use default
					},
				},
			},
		}

		Expect(t, m.AppNames("default-name")).To(HaveLen(3))
		Expect(t, m.AppNames("default-name")).To(Contain("app-name-1", "app-name-2", "default-name"))
	})
}
