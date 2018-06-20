package scheduler

import (
	"log"

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
	envs map[string]string
	log  *log.Logger
}

func NewRunner(m PackageManager, e Executor, envs map[string]string, log *log.Logger) *Runner {
	return &Runner{
		m:    m,
		e:    e,
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

	r.e.Execute(path, envs, work.Command)
}
