package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/apoydence/cf-faas/internal/capi"
	"github.com/apoydence/cf-faas/internal/scheduler"
	gocapi "github.com/apoydence/go-capi"
)

func main() {
	log := log.New(os.Stderr, "[WORKER]", log.LstdFlags)
	log.Printf("Starting CF FaaS worker...")
	defer log.Printf("Closing CF FaaS worker...")

	cfg := LoadConfig(log)

	capiClient := gocapi.NewClient(
		cfg.VcapApplication.CAPIAddr,
		cfg.VcapApplication.ApplicationID,
		cfg.VcapApplication.SpaceID,
		http.DefaultClient,
	)

	packManager := capi.NewPackageManager(
		cfg.AppNames,
		15*time.Second,
		cfg.DataDir,
		capiClient,
		http.DefaultClient,
		log,
	)

	exec := scheduler.ExecutorFunc(func(cwd string, envs map[string]string, command string) error {
		ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)
		cmd := exec.CommandContext(ctx, "bash", append([]string{"-c"}, command)...)
		cmd.Dir = cwd
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		for k, v := range envs {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}

		return cmd.Run()
	})

	runner := scheduler.NewRunner(
		packManager,
		exec,
		http.DefaultClient,
		map[string]string{
			"HTTP_PROXY":        cfg.HTTPProxy,
			"X_CF_APP_INSTANCE": cfg.AppInstance,
			"VCAP_APPLICATION":  os.Getenv("VCAP_APPLICATION"),
		},
		log,
	)

	scheduler.Run(
		cfg.PoolAddr,
		cfg.AppInstance,
		30*time.Second,
		runner,
		http.DefaultClient,
		log,
	)
}
