package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/apoydence/cf-faas/internal/handlers"
	"github.com/apoydence/cf-faas/internal/manifest"
	cfgroupcache "github.com/apoydence/cf-groupcache"
	gocapi "github.com/apoydence/go-capi"
	"github.com/golang/groupcache"
)

func main() {
	log := log.New(os.Stderr, "", log.LstdFlags)
	log.Printf("Starting CF FaaS...")
	defer log.Printf("Closing CF FaaS...")

	cfg := LoadConfig(log)

	go startHealthEndpoint(cfg)

	capiClient := gocapi.NewClient(
		cfg.VcapApplication.CAPIAddr,
		cfg.VcapApplication.ApplicationID,
		cfg.VcapApplication.SpaceID,
		http.DefaultClient,
	)

	gcPool := cfgroupcache.NewHTTPPoolOpts(
		"http://"+cfg.VcapApplication.ApplicationURIs[0],
		cfg.VcapApplication.ApplicationID,
		&groupcache.HTTPPoolOptions{
			// Some random thing that won't be a viable path
			BasePath: "/_group_cache_32723262323249873240/",
		},
	)

	peerManager := cfgroupcache.NewPeerManager(
		"http://"+cfg.VcapApplication.ApplicationURIs[0],
		cfg.VcapApplication.ApplicationID,
		gcPool,
		capiClient,
		log,
	)
	updateCachePeers(peerManager)

	// Bootstrap
	bootstrapCtx, bootstrapCancel := context.WithCancel(context.Background())
	bootstrapRouter := handlers.NewRouter(
		"http://"+cfg.VcapApplication.ApplicationURIs[0],
		cfg.VcapApplication.ApplicationName,
		cfg.VcapApplication.ApplicationID,
		cfg.InstanceIndex,
		gcPool,
		capiClient,
		handlers.NewRequestRelayer,
		handlers.NewWorkerPool,
		handlers.NewHTTPEvent,
		handlers.NewCache,
		log,
	).BuildHandler(parseHTTPManifest(bootstrapCtx, cfg, log))

	hotSwap := handlers.NewHotSwap(bootstrapRouter)

	var wg, ready sync.WaitGroup
	wg.Add(1)
	ready.Add(1)
	defer wg.Wait()
	go func() {
		defer wg.Done()
		ready.Done()
		log.Fatal(
			http.ListenAndServe(
				fmt.Sprintf(":%d", cfg.Port),
				hotSwap,
			),
		)
	}()

	ready.Wait()

	router := handlers.NewRouter(
		"http://"+cfg.VcapApplication.ApplicationURIs[0],
		cfg.VcapApplication.ApplicationName,
		cfg.VcapApplication.ApplicationID,
		cfg.InstanceIndex,
		gcPool,
		capiClient,
		handlers.NewRequestRelayer,
		handlers.NewWorkerPool,
		handlers.NewHTTPEvent,
		handlers.NewCache,
		log,
	).BuildHandler(parseManifest(context.Background(), cfg, log))

	hotSwap.Swap(router)
	bootstrapCancel()
}

func parseHTTPManifest(ctx context.Context, cfg Config, log *log.Logger) (context.Context, []string, []manifest.HTTPFunction) {
	return ctx, cfg.BootstrapManifest.AppNames(cfg.VcapApplication.ApplicationName), cfg.BootstrapManifest.Functions
}

func parseManifest(ctx context.Context, cfg Config, log *log.Logger) (context.Context, []string, []manifest.HTTPFunction) {
	resolver := manifest.NewResolver(
		cfg.ResolverURLs,
		http.DefaultClient,
	)

	fs, err := resolver.Resolve(cfg.Manifest)
	if err != nil {
		log.Fatalf("failed to resolve manifest: %s", err)
	}

	return ctx, cfg.Manifest.AppNames(cfg.VcapApplication.ApplicationName), fs
}

func startHealthEndpoint(cfg Config) {
	log.Fatal(
		http.ListenAndServe(
			fmt.Sprintf(":%d", cfg.HealthPort),
			nil,
		),
	)
}

func updateCachePeers(peerManager *cfgroupcache.PeerManager) {
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
}
