package main

import (
	"log"
	"net/http"
	"os"

	faas "github.com/apoydence/cf-faas"
	"github.com/apoydence/cf-faas/internal/capi"
	"github.com/apoydence/cf-faas/internal/handlers"
	gocapi "github.com/apoydence/go-capi"
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

	taskRunner := capi.NewTaskRunner(
		cfg.ScriptAppName,
		client,
	)

	faas.Start(handlers.NewRunTask(
		cfg.Command,
		cfg.ExpectedHeaders,
		taskRunner,
		log,
	))
}
