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
	c       TaskCreator
	f       TokenFetcher
	q       chan work
	log     *log.Logger
	addIn   time.Duration
	addr    string
	command string
	appGuid string

	dropletAppName    string
	dropletFetcher    DropletGuidFetcher
	instanceIndex     int
	skipSSLValidation bool

	mu        sync.Mutex
	taskCount int
}

type work struct {
	u   *url.URL
	ctx context.Context
}

type TaskCreator interface {
	CreateTask(ctx context.Context, command, appGuid, dropletGuid string) error
}

type TokenFetcher interface {
	Token() (string, error)
}

type DropletGuidFetcher interface {
	FetchGuid(ctx context.Context, appName string) (appGuid, dropletGuid string, err error)
}

func NewWorkerPool(
	addr string,
	command string,
	appGuid string,

	dropletAppName string,
	dropletFetcher DropletGuidFetcher,

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
		q:   make(chan work),

		addIn:             addTaskThreshold,
		addr:              addr,
		command:           command,
		appGuid:           appGuid,
		dropletAppName:    dropletAppName,
		dropletFetcher:    dropletFetcher,
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

	ctx, _ := context.WithTimeout(r.Context(), 30*time.Second)

	var wo work
	select {
	case wo = <-p.q:
	case <-ctx.Done():
		return
	}

	data, err := json.Marshal(struct {
		Href string `json:"href"`
	}{
		Href: wo.u.String(),
	})
	if err != nil {
		p.log.Panicf("failed to marshal data: %s", err)
	}

	if _, err := w.Write(data); err != nil {
		go p.SubmitWork(wo.ctx, wo.u)
	}
}

func (p *WorkerPool) SubmitWork(ctx context.Context, u *url.URL) {
	for {
		timer := time.NewTimer(p.addIn)
		defer timer.Stop()

		select {
		case <-ctx.Done():
			return
		case p.q <- work{u: u, ctx: ctx}:
			return
		case <-timer.C:
			if p.tryAddToThreshold() {
				token, err := p.f.Token()
				if err != nil {
					log.Printf("failed to fetch token: %s", err)
					continue
				}

				appGuid, dropletGuid, err := p.dropletFetcher.FetchGuid(ctx, p.dropletAppName)
				if err != nil {
					log.Printf("failed to fetch droplet guid: %s", err)
					continue
				}

				go func() {
					if err := p.c.CreateTask(context.Background(), p.buildCommand(token), appGuid, dropletGuid); err != nil {
						log.Printf("creating a task failed: %s", err)
					}
				}()
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
