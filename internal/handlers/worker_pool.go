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
	q       chan *url.URL
	log     *log.Logger
	addIn   time.Duration
	addr    string
	command string

	mu        sync.Mutex
	taskCount int
}

type TaskCreator interface {
	CreateTask(ctx context.Context, command string) error
}

func NewWorkerPool(
	addr string,
	command string,
	addTaskThreshold time.Duration,
	c TaskCreator,
	log *log.Logger,
) *WorkerPool {
	p := &WorkerPool{
		log: log,
		c:   c,
		q:   make(chan *url.URL),

		addIn:   addTaskThreshold,
		addr:    addr,
		command: command,
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
				go p.c.CreateTask(context.Background(), p.buildCommand())
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

func (p *WorkerPool) buildCommand() string {
	return fmt.Sprintf(`
set -x
while true
do
set -e

export CF_FAAS_RELAY_ADDR=$(timeout 30 curl -s %s | jq -r .href)
if [ -z "$CF_FAAS_RELAY_ADDR" ]; then
	echo "failed to fetch work... exiting"
	exit 0
fi

set +e

%s
done
`, p.addr, p.command)
}
