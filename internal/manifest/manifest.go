package manifest

import (
	"errors"
	"log"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
)

type Manifest struct {
	Functions []Function `yaml:"functions"`
}

type Function struct {
	Handler Handler                             `yaml:"handler"`
	Events  map[string][]map[string]interface{} `yaml:"events"`
}

type Handler struct {
	Command string `yaml:"command"`
	AppName string `yaml:"app_name"`
}

type HTTPEvent struct {
	Path   string `yaml:"path"`
	Method string `yaml:"method"`
	Cache  struct {
		Duration time.Duration `yaml:"duration"`
		Header   []string      `yaml:"header"`
	} `yaml:"cache"`
}

type HTTPFunction struct {
	Handler Handler     `yaml:"handler"`
	Events  []HTTPEvent `yaml:"events"`
}

func (f HTTPFunction) Validate() error {
	if f.Handler.Command == "" {
		return errors.New("invalid empty command")
	}

	if len(f.Events) == 0 {
		return errors.New("invalid empty events")
	}

	for _, e := range f.Events {
		if e.Path == "" {
			return errors.New("invalid empty path")
		}

		if e.Method == "" {
			return errors.New("invalid empty method")
		}
	}

	return nil
}

func (m *Manifest) UnmarshalEnv(data string) error {
	if err := yaml.NewDecoder(strings.NewReader(data)).Decode(&m); err != nil {
		return err
	}

	if len(m.Functions) == 0 {
		return errors.New("no functions defined")
	}

	for _, f := range m.Functions {
		if f.Handler.Command == "" {
			return errors.New("invalid empty command")
		}

		if len(f.Events) == 0 {
			return errors.New("invalid empty events")
		}
	}

	m.convertTypes()

	return nil
}

func (m *Manifest) convertTypes() {
	// We don't want map[interface{}]interface{}. It doesn't play well with
	// JSON.
	for _, f := range m.Functions {
		for _, e := range f.Events {
			for _, v := range e {
				for k, vv := range v {
					v[k] = m.convertMap(vv)
				}
			}
		}
	}
}

func (m *Manifest) convertMap(i interface{}) interface{} {
	mi, ok := i.(map[interface{}]interface{})
	if !ok {
		return i
	}

	newM := make(map[string]interface{})

	for k, v := range mi {
		s, ok := k.(string)
		if !ok {
			log.Fatalf("invalid manifest: key value is not a string")
		}

		newM[s] = m.convertMap(v)
	}

	return newM
}

func (m *Manifest) AppNames(defaultName string) []string {
	var appNames []string
	ma := map[string]bool{}
	for _, f := range m.Functions {
		if f.Handler.AppName == "" {
			f.Handler.AppName = defaultName
		}

		if ma[f.Handler.AppName] {
			continue
		}

		ma[f.Handler.AppName] = true
		appNames = append(appNames, f.Handler.AppName)
	}
	return appNames
}
