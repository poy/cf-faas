package faas

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"
)

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

type Handler interface {
	Handle(Request) (Response, error)
}

type HandlerFunc func(Request) (Response, error)

func (f HandlerFunc) Handle(r Request) (Response, error) {
	return f(r)
}

func Start(h Handler) {
	fmt.Println("!!!!!!!!!!!! START")
	log := log.New(os.Stderr, "[FAAS]", log.LstdFlags)
	cfg := loadConfig(log)

	httpClient := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: cfg.SkipSSLValadation,
			},
		},
	}

	request := getRequest(cfg, httpClient, log)
	resp, err := h.Handle(request)
	if err != nil {
		log.Printf("handler error: %s", err)
		postResponse(Response{
			StatusCode: http.StatusInternalServerError,
		}, cfg, httpClient, log)
		return
	}

	postResponse(resp, cfg, httpClient, log)
}

func getRequest(cfg config, httpClient *http.Client, log *log.Logger) Request {
	req, err := http.NewRequest(http.MethodGet, cfg.RelayAddr, nil)
	if err != nil {
		log.Fatalf("failed to build request with CF_FAAS_RELAY_ADDR: %s", err)
	}
	req.Header.Set("Authorization", cfg.Authorization)
	req.Header.Set("X-CF-APP-INSTANCE", cfg.CFAppInstance)
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
	req.Header.Set("Authorization", cfg.Authorization)
	req.Header.Set("X-CF-APP-INSTANCE", cfg.CFAppInstance)
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
	RelayAddr         string
	SkipSSLValadation bool
	Authorization     string
	CFAppInstance     string
}

func loadConfig(log *log.Logger) config {
	cfg := config{
		RelayAddr:         os.Getenv("CF_FAAS_RELAY_ADDR"),
		SkipSSLValadation: os.Getenv("SKIP_SSL_VALIDATION") == "true",
		Authorization:     os.Getenv("CF_AUTH_TOKEN"),
		CFAppInstance:     os.Getenv("X_CF_APP_INSTANCE"),
	}

	if cfg.RelayAddr == "" {
		log.Fatalf("CF_FAAS_RELAY_ADDR is empty")
	}

	if cfg.Authorization == "" {
		log.Fatalf("CF_AUTH_TOKEN is empty")
	}

	if cfg.CFAppInstance == "" {
		log.Fatalf("X_CF_APP_INSTANCE is empty")
	}

	return cfg
}
