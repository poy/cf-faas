package faas

import "time"

type ConvertRequest struct {
	Functions []ConvertFunction `json:"functions"`
}

type ConvertFunction struct {
	Handler ConvertHandler                      `json:"handler"`
	Events  map[string][]map[string]interface{} `json:"events"`
}

type ConvertHandler struct {
	Command string `json:"command"`
	AppName string `json:"app_name,omitempty"`
}

type ConvertResponse struct {
	Functions []ConvertHTTPFunction `json:"functions"`
}

type ConvertHTTPFunction struct {
	Handler ConvertHandler     `json:"handler"`
	Events  []ConvertHTTPEvent `json:"events"`
}

type ConvertHTTPEvent struct {
	Path   string `yaml:"path"`
	Method string `yaml:"method"`
	Cache  struct {
		Duration time.Duration `yaml:"duration"`
		Header   []string      `yaml:"header"`
	} `yaml:"cache"`
}
