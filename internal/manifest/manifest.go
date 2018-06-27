package manifest

import (
	"errors"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
)

type Manifest struct {
	Functions []Function `yaml:"functions"`
}

type Function struct {
	Handler    Handler     `yaml:"handler"`
	HTTPEvents []HTTPEvent `yaml:"http_events"`
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

		if len(f.HTTPEvents) == 0 {
			return errors.New("invalid empty http_events")
		}

		for _, e := range f.HTTPEvents {
			if e.Path == "" {
				return errors.New("invalid empty path")
			}

			if e.Method == "" {
				return errors.New("invalid empty method")
			}
		}
	}

	return nil
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
