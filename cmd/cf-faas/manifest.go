package main

import (
	"log"
	"os"
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

func LoadManifest(path string, log *log.Logger) Manifest {
	f, err := os.Open(path)
	if err != nil {
		log.Fatalf("failed to open %s: %s", path, err)
	}

	var m Manifest
	if err := yaml.NewDecoder(f).Decode(&m); err != nil {
		log.Fatalf("failed to unmarshal manifest: %s", err)
	}

	if len(m.Functions) == 0 {
		log.Fatalf("no functions defined")
	}

	for _, f := range m.Functions {
		if f.Handler.Command == "" {
			log.Fatal("invalid empty command")
		}

		if len(f.HTTPEvents) == 0 {
			log.Fatal("invalid empty http_events")
		}

		for _, e := range f.HTTPEvents {
			if e.Path == "" {
				log.Fatal("invalid empty path")
			}

			if e.Method == "" {
				log.Fatal("invalid empty method")
			}
		}
	}

	return m
}
