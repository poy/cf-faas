package faas

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	envstruct "code.cloudfoundry.org/go-envstruct"
)

type Request struct {
	Path         string            `json:"path"`
	URLVariables map[string]string `json:"url_variables"`
	Method       string            `json:"method"`
	Header       http.Header       `json:"headers"`
	Body         []byte            `json:"body"`
}

type Response struct {
	StatusCode int         `json:"status_code"`
	Header     http.Header `json:"header"`
	Body       []byte      `json:"body"`
}

type Handler interface {
	Handle(Request) (Response, error)
}

type HandlerFunc func(Request) (Response, error)

func (f HandlerFunc) Handle(r Request) (Response, error) {
	return f(r)
}

func Start(h Handler) {
	log := log.New(os.Stderr, "[FAAS HANDLER] ", log.LstdFlags)
	cfg := loadConfig(log)

	request := getRequest(cfg, http.DefaultClient, log)
	resp, err := h.Handle(request)
	if err != nil {
		log.Printf("handler error: %s", err)
		postResponse(Response{
			StatusCode: http.StatusInternalServerError,
		}, cfg, http.DefaultClient, log)
		return
	}

	postResponse(resp, cfg, http.DefaultClient, log)
}

func getRequest(cfg config, httpClient *http.Client, log *log.Logger) Request {
	req, err := http.NewRequest(http.MethodGet, cfg.RelayAddr, nil)
	if err != nil {
		log.Fatalf("failed to build request with CF_FAAS_RELAY_ADDR: %s", err)
	}
	req.Header.Set("X-CF-APP-INSTANCE", cfg.AppInstance)
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Fatalf("failed to GET request: %s", err)
	}

	defer func(resp *http.Response) {
		io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
	}(resp)

	if resp.StatusCode != http.StatusOK {
		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Fatalf("failed to read body: %s", err)
		}
		log.Fatalf("unexpected status code while GETting request%d: %s", resp.StatusCode, data)
	}

	var r Request
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		log.Fatalf("failed to unmarshal request: %s", err)
	}
	return r
}

func postResponse(response Response, cfg config, httpClient *http.Client, log *log.Logger) {
	data, err := json.Marshal(response)
	if err != nil {
		log.Fatalf("failed to marshal response: %s", err)
	}

	req, err := http.NewRequest(http.MethodPost, cfg.RelayAddr, bytes.NewReader(data))
	if err != nil {
		log.Fatalf("failed to build request with CF_FAAS_RELAY_ADDR: %s", err)
	}
	req.Header.Set("X-CF-APP-INSTANCE", cfg.AppInstance)
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Fatalf("failed to POST response: %s", err)
	}

	defer func(resp *http.Response) {
		io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
	}(resp)

	if resp.StatusCode != http.StatusOK {
		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Fatalf("failed to read body: %s", err)
		}
		log.Fatalf("unexpected status code while POSTing results %d: %s", resp.StatusCode, data)
	}
}

type config struct {
	RelayAddr string `env:"CF_FAAS_RELAY_ADDR, required"`

	AppInstance string `env:"X_CF_APP_INSTANCE, requried"`
}

func loadConfig(log *log.Logger) config {
	cfg := config{}

	if err := envstruct.Load(&cfg); err != nil {
		log.Fatal(err)
	}

	return cfg
}
