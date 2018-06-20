package scheduler

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/apoydence/cf-faas/internal/internalapi"
)

type PackageManager interface {
	PackageForApp(appName string) (string, error)
}

type Executor interface {
	Execute(cwd string, envs map[string]string, command string) error
}

type ExecutorFunc func(cwd string, envs map[string]string, command string) error

func (f ExecutorFunc) Execute(cwd string, envs map[string]string, command string) error {
	return f(cwd, envs, command)
}

type Runner struct {
	m    PackageManager
	e    Executor
	d    Doer
	envs map[string]string
	log  *log.Logger
}

func NewRunner(
	m PackageManager,
	e Executor,
	d Doer,
	envs map[string]string,
	log *log.Logger,
) *Runner {
	return &Runner{
		m:    m,
		e:    e,
		d:    d,
		envs: envs,
		log:  log,
	}
}

func (r *Runner) Submit(work internalapi.Work) {
	path, err := r.m.PackageForApp(work.AppName)
	if err != nil {
		log.Printf("failed to fetch package for app %s: %s", work.AppName, err)
		return
	}

	envs := map[string]string{
		"CF_FAAS_RELAY_ADDR": work.Href,
	}
	for k, v := range r.envs {
		envs[k] = v
	}

	if err := r.e.Execute(path, envs, work.Command); err != nil {
		req, err := http.NewRequest(http.MethodPost, work.Href, strings.NewReader(`{"status_code":500}`))
		if err != nil {
			r.log.Printf("failed to build request: %s", err)
		}

		ctx, _ := context.WithTimeout(context.Background(), time.Second)
		req = req.WithContext(ctx)

		if _, err := r.d.Do(req); err != nil {
			r.log.Printf("failed to submit request: %s", err)
		}
	}
}
