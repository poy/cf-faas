package main

import (
	"errors"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
)

type Manifest struct {
	Functions map[string]Function `yaml:"functions"`
}

type Function struct {
	Handler    Handler     `yaml:"handler"`
	HTTPEvents []HTTPEvent `yaml:"http_events"`
}

type Handler struct {
	Command string `yaml:"command"`
	AppName string `yaml:"app_name"`
	Cache   struct {
		Duration time.Duration `yaml:"duration"`
		Header   []string      `yaml:"header"`
	} `yaml:"cache"`
}

type HTTPEvent struct {
	Path   string `yaml:"path"`
	Method string `yaml:"method"`
}

func (m *Manifest) UnmarshalEnv(data string) error {
	if err := yaml.NewDecoder(strings.NewReader(data)).Decode(m); err != nil {
		return err
	}

	if len(m.Functions) == 0 {
		errors.New("no functions defined")
	}

	for _, f := range m.Functions {
		if f.Handler.Command == "" {
			errors.New("invalid empty command")
		}

		if len(f.HTTPEvents) == 0 {
			errors.New("invalid empty http_events")
		}

		for _, e := range f.HTTPEvents {
			if e.Path == "" {
				errors.New("invalid empty path")
			}

			if e.Method == "" {
				errors.New("invalid empty method")
			}
		}
	}

	return nil
}
