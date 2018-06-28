package faas

import "time"

type ConvertFunctionRequest struct {
	Handler ConvertFunctionHandler              `json:"handler"`
	Events  map[string][]map[string]interface{} `json:"events"`
}

type ConvertFunctionHandler struct {
	Command string `json:"command"`
	AppName string `json:"app_name,omitempty"`
}

type ConvertFunctionResponse struct {
	Functions []ConvertFunctionResult `json:"functions"`
}

type ConvertFunctionResult struct {
	Handler ConvertFunctionHandler     `json:"handler"`
	Events  []ConvertFunctionHTTPEvent `json:"events"`
}

type ConvertFunctionHTTPEvent struct {
	Path   string `yaml:"path"`
	Method string `yaml:"method"`
	Cache  struct {
		Duration time.Duration `yaml:"duration"`
		Header   []string      `yaml:"header"`
	} `yaml:"cache"`
}
