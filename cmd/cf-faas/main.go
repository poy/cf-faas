package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strings"
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

	u, err := url.Parse(strings.Replace(cfg.VcapApplication.ApplicationURIs[0], "https", "http", 1))
	if err != nil {
		log.Fatalf("failed to parse application URI: %s", err)
	}
	u.Scheme = "http"

	relayer := handlers.NewRequestRelayer(u.String(), fmt.Sprintf("%s/relayer", internalID), log)
	r.Handle(fmt.Sprintf("/%s/relayer/{id}", internalID), relayer).Methods(http.MethodGet, http.MethodPost)

	capiClient := capi.NewClient(
		cfg.VcapApplication.CAPIAddr,
		cfg.VcapApplication.ApplicationID,
		cfg.VcapApplication.SpaceID,
		time.Second,
		http.DefaultClient,
	)

	var appNames []string
	ma := map[string]bool{}
	for _, f := range manifest.Functions {
		if f.Handler.AppName == "" {
			f.Handler.AppName = cfg.VcapApplication.ApplicationName
		}

		if ma[f.Handler.AppName] {
			continue
		}

		ma[f.Handler.AppName] = true
		appNames = append(appNames, f.Handler.AppName)
	}

	poolPath := fmt.Sprintf("/%s/pool/%d%d", internalID, rand.Int63(), time.Now().UnixNano())
	poolAddr := fmt.Sprintf("http://%s%s", cfg.VcapApplication.ApplicationURIs[0], poolPath)
	pool := handlers.NewWorkerPool(
		poolAddr,
		appNames,
		fmt.Sprintf("%s:%d", cfg.VcapApplication.ApplicationID, cfg.InstanceIndex),
		time.Second,
		capiClient,
		log,
	)
	r.Handle(poolPath, pool).Methods(http.MethodGet)

	for _, f := range manifest.Functions {
		appName := f.Handler.AppName
		if f.Handler.AppName == "" {
			appName = cfg.VcapApplication.ApplicationName
		}

		eh := handlers.NewHTTPEvent(f.Handler.Command, appName, relayer, pool, log)
		for _, e := range f.HTTPEvents {
			r.Handle(e.Path, eh).Methods(e.Method)
		}
	}

	return r
}
