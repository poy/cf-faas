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

	"github.com/apoydence/cf-faas/api"
)

type HTTPEvent struct {
	log *log.Logger
	r   Relayer
	c   TaskCreator

	command string
}

type Relayer interface {
	Relay(r *http.Request) (*url.URL, func() (api.Response, error), error)
}

type TaskCreator interface {
	CreateTask(ctx context.Context, command string) error
}

func NewHTTPEvent(
	command string,
	r Relayer,
	c TaskCreator,
	log *log.Logger,
) *HTTPEvent {
	return &HTTPEvent{
		log:     log,
		r:       r,
		c:       c,
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

	errs := make(chan error, 2)
	resps := make(chan api.Response)

	go func() {
		if err := e.c.CreateTask(r.Context(), e.buildCommand(u)); err != nil {
			println("ASDF")
			e.log.Printf("creating a task failed: %s", err)
			errs <- err
			return
		}
	}()

	go func() {
		resp, err := f()
		if err != nil {
			e.log.Printf("running task failed: %s", err)
			errs <- err
			return
		}
		resps <- resp
	}()

	select {
	case <-errs:
		w.WriteHeader(http.StatusInternalServerError)
	case resp := <-resps:
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, bytes.NewReader(resp.Body))
	}
}

func (e HTTPEvent) buildCommand(relayURL *url.URL) string {
	return fmt.Sprintf(`
export CF_FAAS_RELAY_ADDR=%q

%s
	`, relayURL, e.command)
}
