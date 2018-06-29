package handlers

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/apoydence/cf-faas/internal/manifest"
	gocapi "github.com/apoydence/go-capi"
	"github.com/gorilla/mux"
)

type Router struct {
	applicationURI    string
	applicationName   string
	applicationID     string
	instanceIndex     int
	groupcachePool    http.Handler
	capiClient        *gocapi.Client
	newRequestRelayer func(addr, pathPrefix string, log *log.Logger) *RequestRelayer
	newWorkerPool     func(ctx context.Context, addr string, appNames []string, appInstance string, addTaskThreshold time.Duration, c TaskCreator, log *log.Logger) *WorkerPool
	newHTTPEvent      func(command string, appName string, r Relayer, s WorkSubmitter, log *log.Logger) *HTTPEvent
	newCache          func(name string, headers []string, h http.Handler, d time.Duration, log *log.Logger) *Cache
	log               *log.Logger
}

func NewRouter(
	applicationURI string,
	applicationName string,
	applicationID string,
	instanceIndex int,
	groupcachePool http.Handler,
	capiClient *gocapi.Client,
	newRequestRelayer func(addr, pathPrefix string, log *log.Logger) *RequestRelayer,
	newWorkerPool func(ctx context.Context, addr string, appNames []string, appInstance string, addTaskThreshold time.Duration, c TaskCreator, log *log.Logger) *WorkerPool,
	newHTTPEvent func(command string, appName string, r Relayer, s WorkSubmitter, log *log.Logger) *HTTPEvent,
	newCache func(name string, headers []string, h http.Handler, d time.Duration, log *log.Logger) *Cache,
	log *log.Logger,
) *Router {
	return &Router{
		applicationURI:    applicationURI,
		applicationName:   applicationName,
		applicationID:     applicationID,
		instanceIndex:     instanceIndex,
		groupcachePool:    groupcachePool,
		capiClient:        capiClient,
		newRequestRelayer: newRequestRelayer,
		newWorkerPool:     newWorkerPool,
		newHTTPEvent:      newHTTPEvent,
		newCache:          newCache,
		log:               log,
	}
}

func (r *Router) BuildHandler(ctx context.Context, appNames []string, functions []manifest.HTTPFunction) http.Handler {
	mux := mux.NewRouter()
	internalID := fmt.Sprintf("%d%d", rand.Int63(), time.Now().UnixNano())

	// Request Relayer
	relayer := r.newRequestRelayer(r.applicationURI, fmt.Sprintf("%s/relayer", internalID), r.log)
	mux.Handle(fmt.Sprintf("/%s/relayer/{id}", internalID), relayer).Methods(http.MethodGet, http.MethodPost)

	// Groupcache Pool
	mux.Handle("/_group_cache_32723262323249873240/{name}/{key}", r.groupcachePool)

	// WorkerPool
	poolPath := fmt.Sprintf("/%s/pool/%d%d", internalID, rand.Int63(), time.Now().UnixNano())
	pool := r.newWorkerPool(
		ctx,
		r.applicationURI+poolPath,
		appNames,
		fmt.Sprintf("%s:%d", r.applicationID, r.instanceIndex),
		time.Second,
		r.capiClient,
		r.log,
	)
	mux.Handle(poolPath, pool).Methods(http.MethodGet)

	// Functions
	r.buildFunctionHandlers(functions, mux, relayer, pool)

	return mux
}

func (r *Router) buildFunctionHandlers(functions []manifest.HTTPFunction, mux *mux.Router, relayer *RequestRelayer, pool *WorkerPool) {
	for _, f := range functions {
		appName := f.Handler.AppName
		if f.Handler.AppName == "" {
			appName = r.applicationName
		}

		eh := r.newHTTPEvent(
			f.Handler.Command,
			appName,
			relayer,
			pool,
			r.log,
		)

		for _, e := range f.Events {
			if e.Cache.Duration > 0 {
				ceh := r.newCache(
					base64.URLEncoding.EncodeToString([]byte(e.Path)),
					e.Cache.Header,
					eh,
					e.Cache.Duration,
					r.log,
				)
				mux.Handle(e.Path, ceh).Methods(e.Method)
				continue
			}

			mux.Handle(e.Path, eh).Methods(e.Method)
		}
	}
}
