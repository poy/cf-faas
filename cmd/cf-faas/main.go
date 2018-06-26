package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/apoydence/cf-faas/internal/handlers"
	cfgroupcache "github.com/apoydence/cf-groupcache"
	gocapi "github.com/apoydence/go-capi"
	"github.com/golang/groupcache"
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

	capiClient := gocapi.NewClient(
		cfg.VcapApplication.CAPIAddr,
		cfg.VcapApplication.ApplicationID,
		cfg.VcapApplication.SpaceID,
		http.DefaultClient,
	)

	route := "http://" + cfg.VcapApplication.ApplicationURIs[0]
	gcOpts := groupcache.HTTPPoolOptions{
		// Some random thing that won't be a viable path
		BasePath: "/_group_cache_32723262323249873240/",
	}
	gcPool := cfgroupcache.NewHTTPPoolOpts(route, cfg.VcapApplication.ApplicationID, &gcOpts)
	r.Handle("/_group_cache_32723262323249873240/{name}/{key}", gcPool)

	peerManager := cfgroupcache.NewPeerManager(
		route,
		cfg.VcapApplication.ApplicationID,
		gcPool,
		capiClient,
		log,
	)

	updatePeers := func() {
		ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
		peerManager.Tick(ctx)
	}
	updatePeers()

	go func() {
		for range time.Tick(15 * time.Second) {
			updatePeers()
		}
	}()

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
			if f.Handler.Cache.Duration > 0 {
				ceh := handlers.NewCache(
					base64.URLEncoding.EncodeToString([]byte(e.Path)),
					f.Handler.Cache.Header,
					eh,
					f.Handler.Cache.Duration,
					log,
				)
				r.Handle(e.Path, ceh).Methods(e.Method)
				continue
			}

			r.Handle(e.Path, eh).Methods(e.Method)
		}
	}

	return r
}
