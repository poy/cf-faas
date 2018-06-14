package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type WorkerPool struct {
	c                 TaskCreator
	f                 TokenFetcher
	q                 chan *url.URL
	log               *log.Logger
	addIn             time.Duration
	addr              string
	command           string
	appGuid           string
	instanceIndex     int
	skipSSLValidation bool

	mu        sync.Mutex
	taskCount int
}

type TaskCreator interface {
	CreateTask(ctx context.Context, command string) error
}

type TokenFetcher interface {
	Token() (string, error)
}

func NewWorkerPool(
	addr string,
	command string,
	appGuid string,
	instanceIndex int,
	skipSSLValidation bool,
	addTaskThreshold time.Duration,
	c TaskCreator,
	f TokenFetcher,
	log *log.Logger,
) *WorkerPool {
	p := &WorkerPool{
		log: log,
		c:   c,
		f:   f,
		q:   make(chan *url.URL),

		addIn:             addTaskThreshold,
		addr:              addr,
		command:           command,
		appGuid:           appGuid,
		instanceIndex:     instanceIndex,
		skipSSLValidation: skipSSLValidation,
	}

	go p.taskThreshold()

	return p
}

func (p *WorkerPool) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	u := <-p.q

	data, err := json.Marshal(struct {
		Href string `json:"href"`
	}{
		Href: u.String(),
	})
	if err != nil {
		p.log.Panicf("failed to marshal data: %s", err)
	}

	w.Write(data)
}

func (p *WorkerPool) SubmitWork(ctx context.Context, u *url.URL) {
	for {
		timer := time.NewTimer(p.addIn)
		defer timer.Stop()

		select {
		case <-ctx.Done():
			return
		case p.q <- u:
			return
		case <-timer.C:
			if p.tryAddToThreshold() {
				token, err := p.f.Token()
				if err != nil {
					log.Printf("failed to fetch token: %s", err)
					continue
				}

				go p.c.CreateTask(context.Background(), p.buildCommand(token))
			}
		}
	}
}

func (p *WorkerPool) tryAddToThreshold() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.taskCount < 5 {
		p.taskCount++
		return true
	}

	return false
}

func (p *WorkerPool) taskThreshold() {
	for range time.Tick(30 * time.Second) {
		p.mu.Lock()
		p.taskCount = 0
		p.mu.Unlock()
	}
}

func (p *WorkerPool) buildCommand(token string) string {
	var skipSSLFlag string
	if p.skipSSLValidation {
		skipSSLFlag = " -k"
	}

	return fmt.Sprintf(`
set -x
while true
do
set -e

export CF_AUTH_TOKEN="%s"
export SKIP_SSL_VALIDATION="%v"
export X_CF_APP_INSTANCE="%s:%d"

export CF_FAAS_RELAY_ADDR=$(timeout 30 curl -s%s %s -H "X-CF-APP-INSTANCE: $X_CF_APP_INSTANCE" -H "Authorization: $CF_AUTH_TOKEN" | jq -r .href)
if [ -z "$CF_FAAS_RELAY_ADDR" ]; then
	echo "failed to fetch work... exiting"
	exit 0
fi

set +e

%s
done
`, token, p.skipSSLValidation, p.appGuid, p.instanceIndex, skipSSLFlag, p.addr, p.command)
}
