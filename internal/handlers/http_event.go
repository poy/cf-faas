package handlers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/apoydence/cf-faas"
)

type HTTPEvent struct {
	log *log.Logger
	r   Relayer
	s   WorkSubmitter

	command string
}

type Relayer interface {
	Relay(r *http.Request) (*url.URL, func() (faas.Response, error), error)
}

type WorkSubmitter interface {
	SubmitWork(ctx context.Context, u *url.URL)
}

func NewHTTPEvent(
	command string,
	r Relayer,
	s WorkSubmitter,
	log *log.Logger,
) *HTTPEvent {
	return &HTTPEvent{
		log:     log,
		r:       r,
		s:       s,
		command: command,
	}
}

func (e HTTPEvent) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	r = r.WithContext(ctx)

	u, f, err := e.r.Relay(r)
	if err != nil {
		e.log.Printf("relayer failed: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	e.s.SubmitWork(ctx, u)

	// blocks until the request has been fulfilled.
	resp, err := f()
	if err != nil {
		e.log.Printf("running task failed: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(resp.StatusCode)
	io.Copy(w, bytes.NewReader(resp.Body))
}

func (e HTTPEvent) buildCommand(relayURL *url.URL) string {
	return fmt.Sprintf(`
export CF_FAAS_RELAY_ADDR=%q

%s
	`, relayURL, e.command)
}
