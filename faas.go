package faas

import "net/http"

type Request struct {
	Path    string      `json:"path"`
	Method  string      `json:"method"`
	Headers http.Header `json:"headers"`
	Body    []byte      `json:"body"`
}

type Response struct {
	StatusCode int    `json:"status_code"`
	Body       []byte `json:"body"`
}
