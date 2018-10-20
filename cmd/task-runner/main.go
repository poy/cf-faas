package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"

	faas "github.com/poy/cf-faas"
	"github.com/poy/cf-faas/internal/capi"
	"github.com/poy/cf-faas/internal/handlers"
	gocapi "github.com/poy/go-capi"
)

func main() {
	log := log.New(os.Stderr, "[TASK-RUNNER]", log.LstdFlags)
	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("invalid configuration: %s", err)
	}

	client := gocapi.NewClient(
		cfg.VcapApplication.CAPIAddr,
		cfg.VcapApplication.ApplicationID,
		cfg.VcapApplication.SpaceID,
		http.DefaultClient,
	)

	var taskRunner handlers.TaskRunner = capi.NewTaskRunner(
		cfg.ScriptAppName,
		client,
	)

	if !cfg.CreateTask {
		taskRunner = handlers.TaskRunnerFunc(func(command, name string) (string, error) {
			ctx, _ := context.WithTimeout(context.Background(), time.Minute)
			cmd := exec.CommandContext(ctx, "/bin/bash", append([]string{"-c"}, command)...)

			for k, v := range map[string]string{
				"HTTP_PROXY": cfg.HttpProxy,
			} {
				cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
			}

			out, err := cmd.Output()
			if err != nil {
				return "", err
			}

			return string(out), nil
		})
	}

	faas.Start(handlers.NewRunTask(
		cfg.Command,
		cfg.ExpectedHeaders,
		!cfg.CreateTask,
		taskRunner,
		log,
	))
}
