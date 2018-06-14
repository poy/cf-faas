package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/url"
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
	internalID := fmt.Sprintf("%d%d", rand.Int63(), time.Now().UnixNano())

	r := mux.NewRouter()

	u, err := url.Parse(cfg.VcapApplication.ApplicationURIs[0])
	if err != nil {
		log.Fatalf("failed to parse application URI: %s", err)
	}
	u.Scheme = "https"

	relayer := handlers.NewRequestRelayer(u.String(), fmt.Sprintf("%s/relayer", internalID), log)
	r.Handle(fmt.Sprintf("/%s/relayer/{id}", internalID), relayer).Methods(http.MethodGet, http.MethodPost)

	capiClient := capi.NewClient(
		cfg.VcapApplication.CAPIAddr,
		cfg.VcapApplication.ApplicationID,
		time.Second,
		http.DefaultClient,
	)

	for _, f := range manifest.Functions {
		poolPath := fmt.Sprintf("/%s/pool/%d%d", internalID, rand.Int63(), time.Now().UnixNano())
		poolAddr := fmt.Sprintf("https://%s%s", cfg.VcapApplication.ApplicationURIs[0], poolPath)
		pool := handlers.NewWorkerPool(
			poolAddr,
			f.Handler.Command,
			cfg.VcapApplication.ApplicationID,
			cfg.CFInstanceIndex,
			cfg.SkipSSLValidation,
			time.Second,
			capiClient,
			log,
		)
		r.Handle(poolPath, pool).Methods(http.MethodGet)

		eh := handlers.NewHTTPEvent(f.Handler.Command, relayer, pool, log)
		for _, e := range f.HTTPEvents {
			r.Handle(e.Path, eh).Methods(e.Method)
		}
	}

	return r
}
