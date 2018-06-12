package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/apoydence/cf-faas/internal/capi"
	"github.com/apoydence/cf-faas/internal/handlers"
	"github.com/gorilla/mux"
)

func main() {
	log := log.New(os.Stderr, "", log.LstdFlags)
	log.Printf("Starting CF FaaS...")
	defer log.Printf("Closing CF FaaS...")

	cfg := LoadConfig(log)

	manifest := LoadManifest(cfg.Manifest, log)

	// Setup health endpoint
	go func() {
		log.Fatal(
			http.ListenAndServe(
				fmt.Sprintf(":%d", cfg.HealthPort),
				nil,
			),
		)
	}()

	log.Fatal(
		http.ListenAndServe(
			fmt.Sprintf(":%d", cfg.Port),
			setupRouting(cfg, manifest, log),
		),
	)
}

func setupRouting(cfg Config, manifest Manifest, log *log.Logger) http.Handler {
	r := mux.NewRouter()

	relayer := handlers.NewRequestRelayer(cfg.VcapApplication.ApplicationURIs[0], log)
	r.Handle("/{id}", relayer).Methods(http.MethodGet, http.MethodPost)

	capiClient := capi.NewClient(
		cfg.VcapApplication.CAPIAddr,
		cfg.VcapApplication.ApplicationID,
		time.Second,
		http.DefaultClient,
	)

	for _, f := range manifest.Functions {
		eh := handlers.NewHTTPEvent(f.Handler.Command, relayer, capiClient, log)
		for _, e := range f.HTTPEvents {
			r.Handle(e.Path, eh).Methods(e.Method)
		}
	}

	return r
}
