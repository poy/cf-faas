package capi

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"sync"
	"time"

	"github.com/bluele/gcache"
)

type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

type PackageManager struct {
	c PackageClient
	d Doer

	mu    sync.RWMutex
	m     map[string]string
	ready sync.WaitGroup

	cache    gcache.Cache
	appNames []string
	dataDir  string
	log      *log.Logger
}

type PackageClient interface {
	GetAppGuid(ctx context.Context, appName string) (string, error)
	GetPackageGuid(ctx context.Context, appGuid string) (guid, downloadAddr string, err error)
}

func NewPackageManager(
	appNames []string,
	interval time.Duration,
	dataDir string,
	c PackageClient,
	d Doer,
	log *log.Logger,
) *PackageManager {
	m := &PackageManager{
		c:        c,
		d:        d,
		m:        make(map[string]string),
		dataDir:  dataDir,
		appNames: appNames,
		log:      log,
	}
	m.ready.Add(1)
	m.cache = gcache.
		New(100).
		LRU().
		LoaderFunc(m.loadPackage).
		EvictedFunc(func(key, value interface{}) {
			pi := key.(struct {
				appName      string
				packageGuid  string
				downloadAddr string
			})

			if err := os.RemoveAll(path.Join(dataDir, pi.packageGuid)); err != nil {
				log.Printf("failed to cleanup package: %s", err)
				return
			}
		}).
		Build()
	go m.start(interval)
	return m
}

func (m *PackageManager) PackageForApp(appName string) (string, error) {
	m.ready.Wait()

	m.mu.RLock()
	defer m.mu.RUnlock()

	dir, ok := m.m[appName]
	if !ok {
		return "", errors.New("unknown app")
	}

	return dir, nil
}

func (m *PackageManager) start(interval time.Duration) {
	f := func() {
		var wg sync.WaitGroup
		defer wg.Wait()
		for _, appName := range m.appNames {
			wg.Add(1)
			go func(appName string) {
				defer wg.Done()
				m.downloadForApp(appName)
			}(appName)
		}
	}

	f()
	m.ready.Done()
	for range time.Tick(interval) {
		f()
	}
}

func (m *PackageManager) loadPackage(key interface{}) (interface{}, error) {
	pi := key.(struct {
		appName      string
		packageGuid  string
		downloadAddr string
	})

	req, err := http.NewRequest(http.MethodGet, pi.downloadAddr, nil)
	if err != nil {
		m.log.Printf("failed to build download request: %s", err)
		return nil, err
	}

	resp, err := m.d.Do(req)
	if err != nil {
		m.log.Printf("failed to make download request: %s %v", err, resp.Header)
		return nil, err
	}

	defer func() {
		io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
	}()

	zipPath := path.Join(m.dataDir, pi.packageGuid+".zip")
	f, err := os.Create(zipPath)
	if err != nil {
		m.log.Printf("failed to create zip file: %s", err)
		return nil, err
	}

	_, err = io.Copy(f, resp.Body)
	if err != nil {
		m.log.Printf("failed to download data: %s", err)
		return nil, err
	}

	if err := f.Close(); err != nil {
		m.log.Printf("failed to close zip file: %s", err)
		return nil, err
	}

	r, err := zip.OpenReader(zipPath)
	if err != nil {
		m.log.Printf("failed to open package zip file: %s", err)
		return nil, err
	}

	dir := path.Join(m.dataDir, pi.packageGuid)
	if err := os.Mkdir(dir, os.ModePerm); err != nil {
		m.log.Printf("failed to create package directory %s: %s", dir, err)
		return nil, err
	}

	for _, f := range r.File {
		ff, err := os.Create(path.Join(dir, f.Name))
		if err != nil {
			m.log.Printf("failed to create file: %s", err)
			return nil, err
		}

		rc, err := f.Open()
		if err != nil {
			m.log.Printf("failed to open package zip file: %s", err)
			return nil, err
		}

		_, err = io.Copy(ff, rc)
		if err != nil {
			m.log.Printf("failed to read package zip file: %s", err)
			return nil, err
		}
		ff.Close()
		os.Chmod(path.Join(dir, f.Name), os.ModePerm)
	}

	m.mu.Lock()
	m.m[pi.appName] = dir
	m.mu.Unlock()

	return dir, nil
}

func (m *PackageManager) downloadForApp(appName string) {
	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	appGuid, err := m.c.GetAppGuid(ctx, appName)
	if err != nil {
		m.log.Printf("failed to fetch app guid for %s: %s", appName, err)
		return
	}

	ctx, _ = context.WithTimeout(context.Background(), 5*time.Second)
	packageGuid, downloadAddr, err := m.c.GetPackageGuid(ctx, appGuid)
	if err != nil {
		m.log.Printf("failed to fetch package info: %s", err)
		return
	}

	m.cache.Get(struct {
		appName      string
		packageGuid  string
		downloadAddr string
	}{
		appName:      appName,
		packageGuid:  packageGuid,
		downloadAddr: downloadAddr,
	})
}
