package manifest_test

import (
	"testing"

	"github.com/apoydence/cf-faas/internal/manifest"
	"github.com/apoydence/onpar"
	. "github.com/apoydence/onpar/expect"
	. "github.com/apoydence/onpar/matchers"
)

func TestUnmarshal(t *testing.T) {
	t.Parallel()
	o := onpar.New()
	defer o.Run(t)

	o.Spec("it returns an error if there is a function without a method", func(t *testing.T) {
		var m manifest.Manifest
		err := m.UnmarshalEnv(`
functions:
  goecho:
    handler:
      app_name: faas-droplet-echo
      command: ./echo
    http_events:
    - path: /v1/goecho
      method: POST`)
		Expect(t, err).To(BeNil())

		Expect(t, m).To(Equal(manifest.Manifest{
			Functions: map[string]manifest.Function{
				"goecho": {
					Handler: manifest.Handler{
						AppName: "faas-droplet-echo",
						Command: "./echo",
					},
					HTTPEvents: []manifest.HTTPEvent{
						{
							Path:   "/v1/goecho",
							Method: "POST",
						},
					},
				},
			},
		}))
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
  goecho:
    handler:
      app_name: faas-droplet-echo
    http_events:
    - path: /v1/goecho
      method: POST`)
		Expect(t, err).To(Not(BeNil()))
	})

	o.Spec("it returns an error if there is a function without any events", func(t *testing.T) {
		var m manifest.Manifest
		err := m.UnmarshalEnv(`
functions:
  goecho:
    handler:
      app_name: faas-droplet-echo
      command: ./echo`)
		Expect(t, err).To(Not(BeNil()))
	})

	o.Spec("it returns an error if there is a function without a path", func(t *testing.T) {
		var m manifest.Manifest
		err := m.UnmarshalEnv(`
functions:
  goecho:
    handler:
      app_name: faas-droplet-echo
      command: ./echo
    http_events:
    - method: POST`)
		Expect(t, err).To(Not(BeNil()))
	})

	o.Spec("it returns an error if there is a function without a method", func(t *testing.T) {
		var m manifest.Manifest
		err := m.UnmarshalEnv(`
functions:
  goecho:
    handler:
      app_name: faas-droplet-echo
      command: ./echo
    http_events:
    - path: /v1/goecho`)
		Expect(t, err).To(Not(BeNil()))
	})
}
