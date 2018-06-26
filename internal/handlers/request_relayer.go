package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/apoydence/cf-faas"
	"github.com/gorilla/mux"
)

type RequestRelayer struct {
	log        *log.Logger
	addr       string
	pathPrefix string

	mu sync.Mutex
	m  map[string]struct {
		writer chan<- faas.Response
		errs   chan<- error
		req    *faas.Request
	}
}

func NewRequestRelayer(addr, pathPrefix string, log *log.Logger) *RequestRelayer {
	return &RequestRelayer{
		log:        log,
		addr:       addr,
		pathPrefix: pathPrefix,
		m: make(map[string]struct {
			writer chan<- faas.Response
			errs   chan<- error
			req    *faas.Request
		}),
	}
}

func (r *RequestRelayer) Relay(req *http.Request) (*url.URL, func() (faas.Response, error), error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	path := fmt.Sprintf("/%s/%d%d", r.pathPrefix, rand.Int63(), time.Now().UnixNano())

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return nil, nil, err
	}

	wc, we := make(chan faas.Response, 1), make(chan error, 1)

	r.m[path] = struct {
		writer chan<- faas.Response
		errs   chan<- error
		req    *faas.Request
	}{
		req: &faas.Request{
			Path:         req.URL.Path,
			Method:       req.Method,
			Body:         body,
			Header:       req.Header,
			URLVariables: mux.Vars(req),
		},
		writer: wc,
		errs:   we,
	}

	u, err := url.Parse(fmt.Sprintf("%s%s", r.addr, path))
	if err != nil {
		return nil, nil, err
	}

	return u, func() (faas.Response, error) {
		defer func() {
			r.mu.Lock()
			defer r.mu.Unlock()
			delete(r.m, path)
		}()

		select {
		case <-req.Context().Done():
			return faas.Response{}, req.Context().Err()
		case resp := <-wc:
			return resp, nil
		case err := <-we:
			return faas.Response{}, err
		}
	}, nil
}

func (r *RequestRelayer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	defer func() {
		req.Body.Close()
		io.Copy(ioutil.Discard, req.Body)
	}()

	if req.Header.Get("X-Forwarded-Proto") != "https" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"rejecting non-https requests"}`))
		return
	}

	switch req.Method {
	case http.MethodGet:
		r.mu.Lock()
		request, ok := r.m[req.URL.Path]
		r.mu.Unlock()
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err := json.NewEncoder(w).Encode(request.req); err != nil {
			r.log.Printf("failed to send request to GET request: %s", err)
		}
	case http.MethodPost:
		r.mu.Lock()
		request, ok := r.m[req.URL.Path]
		delete(r.m, req.URL.Path)
		r.mu.Unlock()
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		var resp faas.Response
		if err := json.NewDecoder(req.Body).Decode(&resp); err != nil {
			r.log.Printf("failed to unmarshal response from POST request: %s", err)
			w.WriteHeader(http.StatusExpectationFailed)

			request.errs <- err
			return
		}

		request.writer <- resp
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
