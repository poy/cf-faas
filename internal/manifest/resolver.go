package manifest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	faas "github.com/poy/cf-faas"
)

type Resolver struct {
	urls map[string]string
	d    Doer
}

type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

func NewResolver(urls map[string]string, d Doer) *Resolver {
	return &Resolver{
		urls: urls,
		d:    d,
	}
}

func (r *Resolver) Resolve(m Manifest) ([]HTTPFunction, error) {
	var results []HTTPFunction

	// The value will only have the events for the corresponding key name.
	reqs := make(map[string][]faas.ConvertFunction)

	for _, f := range m.Functions {
		for eventName, es := range f.Events {
			if eventName == "http" {
				fs, err := r.parseHTTPEvent(f, es)
				if err != nil {
					return nil, err
				}

				results = append(results, fs)
				continue
			}

			ff := faas.ConvertFunction{
				Handler: faas.ConvertHandler{
					Command: f.Handler.Command,
					AppName: f.Handler.AppName,
				},
				Events: make(map[string][]faas.GenericData),
			}

			for _, e := range es {
				ff.Events[eventName] = append(ff.Events[eventName], faas.GenericData(e))
			}

			reqs[eventName] = append(reqs[eventName], ff)
		}
	}

	for eventName, fs := range reqs {
		convertReq := faas.ConvertRequest{
			Functions: fs,
		}
		data, err := json.Marshal(convertReq)
		if err != nil {
			return nil, err
		}

		req, err := http.NewRequest(http.MethodPost, r.urls[eventName], bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("building request for %s (eventName=%s): %s", r.urls[eventName], eventName, err)
		}

		resp, err := r.d.Do(req)
		if err != nil {
			return nil, fmt.Errorf("requesting results for %s (eventName=%s): %s", r.urls[eventName], eventName, err)
		}

		fs, err := r.readFunctions(resp)
		if err != nil {
			return nil, fmt.Errorf("reading results for %s (eventName=%s): %s", r.urls[eventName], eventName, err)
		}

		results = append(results, fs...)
	}

	for _, f := range results {
		if err := f.Validate(); err != nil {
			return nil, err
		}
	}

	return results, nil
}

func (r *Resolver) parseHTTPEvent(f Function, events []GenericData) (HTTPFunction, error) {
	var es []HTTPEvent

	data, err := json.Marshal(events)
	if err != nil {
		return HTTPFunction{}, err
	}

	var he []struct {
		Path   string `json:"path"`
		Method string `json:"method"`
		Cache  struct {
			Duration string   `json:"duration"`
			Header   []string `json:"header"`
		} `json:"cache"`
	}

	if err := json.Unmarshal(data, &he); err != nil {
		return HTTPFunction{}, err
	}

	for _, h := range he {
		d, err := time.ParseDuration(h.Cache.Duration)
		if err != nil && h.Cache.Duration != "" {
			log.Fatalf("failed to parse HTTPEvent.Cache duration: %s", err)
		}

		es = append(es, HTTPEvent{
			Path:   h.Path,
			Method: h.Method,
			Cache: struct {
				Duration time.Duration `yaml:"duration"`
				Header   []string      `yaml:"header"`
			}{
				Duration: d,
				Header:   h.Cache.Header,
			},
		})
	}

	return HTTPFunction{
		Handler: f.Handler,
		Events:  es,
	}, nil
}

func (r *Resolver) readFunctions(resp *http.Response) ([]HTTPFunction, error) {
	defer func() {
		io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, data)
	}

	var h faas.ConvertResponse
	if err := json.NewDecoder(resp.Body).Decode(&h); err != nil {
		return nil, err
	}

	var results []HTTPFunction
	for _, f := range h.Functions {
		hf := HTTPFunction{
			Handler: Handler{
				Command: f.Handler.Command,
				AppName: f.Handler.AppName,
			},
		}

		for _, e := range f.Events {
			hf.Events = append(hf.Events, HTTPEvent{
				Path:   e.Path,
				Method: e.Method,
				Cache:  e.Cache,
			})
		}

		results = append(results, hf)
	}

	return results, nil
}
