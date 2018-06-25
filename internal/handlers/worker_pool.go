package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/apoydence/cf-faas/internal/internalapi"
	gocapi "github.com/apoydence/go-capi"
)

type WorkerPool struct {
	c           TaskCreator
	q           chan work
	log         *log.Logger
	addIn       time.Duration
	appInstance string
	addr        string
	appNames    []string

	mu        sync.Mutex
	taskCount int
}

type work struct {
	w   internalapi.Work
	ctx context.Context
}

type TaskCreator interface {
	RunTask(ctx context.Context, command, name, dropletGuid, appGuid string) (gocapi.Task, error)
}

func NewWorkerPool(
	addr string,
	appNames []string,
	appInstance string,
	addTaskThreshold time.Duration,
	c TaskCreator,
	log *log.Logger,
) *WorkerPool {
	p := &WorkerPool{
		log: log,
		c:   c,
		q:   make(chan work),

		appInstance: appInstance,
		appNames:    appNames,
		addIn:       addTaskThreshold,
		addr:        addr,
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

	data, err := json.Marshal(wo.w)
	if err != nil {
		p.log.Panicf("failed to marshal data: %s", err)
	}

	if _, err := w.Write(data); err != nil {
		go p.SubmitWork(wo.ctx, wo.w)
	}
}

func (p *WorkerPool) SubmitWork(ctx context.Context, w internalapi.Work) {
	timer := time.NewTimer(p.addIn)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case p.q <- work{w: w, ctx: ctx}:
			return
		case <-timer.C:
			if p.tryAddToThreshold() {
				go func() {
					// Leave out name, droplet and app name. Their defaults
					// are good enough.
					if _, err := p.c.RunTask(context.Background(), p.buildCommand(), "", "", ""); err != nil {
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

func (p *WorkerPool) buildCommand() string {
	return fmt.Sprintf(`#!/bin/bash

PORT=9999 PROXY_HEALTH_PORT=10000 ./proxy &
echo $! > /tmp/pids
sleep 2

X_CF_APP_INSTANCE=%q APP_NAMES=%q HTTP_PROXY=http://localhost:9999 POOL_ADDR=%q ./worker &
echo $! >> /tmp/pids

# Close everything, otherwise the container won't be reset
function kill_everything {
    for pid in $(cat /tmp/pids)
    do
        kill -9 $pid &>/dev/null || true
    done
	exit 0
}

# Watch pids
while true
do
    for pid in $(cat /tmp/pids)
    do
        ps -p $pid &> /dev/null || kill_everything
    done
    sleep 10
done
`, p.appInstance, strings.Join(p.appNames, ","), p.addr)
}
