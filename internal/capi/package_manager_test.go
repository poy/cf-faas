package capi_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/apoydence/cf-faas/internal/capi"
	"github.com/apoydence/onpar"
	. "github.com/apoydence/onpar/expect"
	. "github.com/apoydence/onpar/matchers"
)

type TM struct {
	*testing.T
	spyPackageClient *spyPackageClient
	spyDoer          *spyDoer
	tempDir          string
	m                *capi.PackageManager
}

func TestPackageManager(t *testing.T) {
	t.Parallel()
	o := onpar.New()
	defer o.Run(t)

	o.BeforeEach(func(t *testing.T) TM {
		spyPackageClient := newSpyPackageClient()
		spyDoer := newSpyDoer()

		tempDir, err := ioutil.TempDir("", "")
		if err != nil {
			panic(err)
		}

		return TM{
			T:                t,
			spyPackageClient: spyPackageClient,
			spyDoer:          spyDoer,
			tempDir:          tempDir,
			m: capi.NewPackageManager(
				[]string{"a", "b", "c"},
				time.Microsecond,
				tempDir,
				spyPackageClient,
				spyDoer,
				log.New(ioutil.Discard, "", 0),
			),
		}
	})

	o.Spec("it fetches the packages", func(t TM) {
		t.spyPackageClient.SetAppResult("a", "guid-a", nil)
		t.spyPackageClient.SetAppResult("b", "guid-b", nil)
		t.spyPackageClient.SetAppResult("c", "guid-c", nil)

		Expect(t, t.spyPackageClient.AppNames).To(ViaPolling(Contain(
			"a", "b", "c",
		)))

		Expect(t, t.spyPackageClient.AppCtx()).To(Not(BeNil()))
		_, ok := t.spyPackageClient.AppCtx().Deadline()
		Expect(t, ok).To(BeTrue())

		Expect(t, t.spyPackageClient.AppGuids).To(ViaPolling(Contain(
			"guid-a", "guid-b", "guid-c",
		)))

		Expect(t, t.spyPackageClient.PackageCtx()).To(Not(BeNil()))
		_, ok = t.spyPackageClient.PackageCtx().Deadline()
		Expect(t, ok).To(BeTrue())
	})

	o.Spec("it downloads the packages", func(t TM) {
		t.spyDoer.m["GET:http://download.x"] = &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewReader(createZip())),
		}
		t.spyPackageClient.SetAppResult("a", "guid-a", nil)
		t.spyPackageClient.SetPackageResults("guid-a", "package-guid-a", "http://download.x")

		Expect(t, t.spyDoer.Req).To(ViaPolling(Not(BeNil())))
		Expect(t, t.spyDoer.Req().Method).To(Equal(http.MethodGet))
		Expect(t, t.spyDoer.Req().URL.String()).To(Equal("http://download.x"))

		f, err := os.Open(path.Join(t.tempDir, "package-guid-a", "some-file.txt"))
		Expect(t, err).To(BeNil())
		data, err := ioutil.ReadAll(f)
		Expect(t, err).To(BeNil())
		Expect(t, string(data)).To(Equal("some-body"))

		pDir, err := t.m.PackageForApp("a")
		Expect(t, err).To(BeNil())
		Expect(t, pDir).To(Equal(path.Join(t.tempDir, "package-guid-a")))
	})

	o.Spec("it returns an error for an unknown app", func(t TM) {
		_, err := t.m.PackageForApp("unknown")
		Expect(t, err).To(Not(BeNil()))
	})

	o.Spec("it does not fetch for a guid if it gets back an error", func(t TM) {
		t.spyPackageClient.SetAppResult("a", "guid-a", errors.New("some-error"))

		Expect(t, t.spyPackageClient.AppGuids).To(Always(HaveLen(0)))
	})

	o.Spec("it does not download for a package if it gets back an error", func(t TM) {
		t.spyPackageClient.SetAppResult("a", "guid-a", nil)

		Expect(t, t.spyDoer.Req).To(Always(BeNil()))
	})
}

type spyPackageClient struct {
	mu            sync.Mutex
	appCtx        context.Context
	appNames      []string
	appGuidResult map[string]string
	appGuidErr    error

	packageCtx     context.Context
	appGuids       []string
	packageResults map[string]struct {
		packageGuid  string
		downloadAddr string
	}
	packageErr error
}

func newSpyPackageClient() *spyPackageClient {
	return &spyPackageClient{
		appGuidResult: make(map[string]string),
		packageResults: make(map[string]struct {
			packageGuid  string
			downloadAddr string
		}),
	}
}

func (s *spyPackageClient) GetAppGuid(ctx context.Context, appName string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appCtx = ctx
	s.appNames = append(s.appNames, appName)

	results, ok := s.appGuidResult[appName]
	if !ok {
		return "", errors.New("not found")
	}

	return results, s.appGuidErr
}

func (s *spyPackageClient) SetAppResult(k, v string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appGuidErr = err
	s.appGuidResult[k] = v
}

func (s *spyPackageClient) AppNames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	results := make([]string, len(s.appNames))
	copy(results, s.appNames)

	return results
}

func (s *spyPackageClient) AppCtx() context.Context {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.appCtx
}

func (s *spyPackageClient) SetPackageResults(appGuid, packageGuid, downloadAddr string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.packageResults[appGuid] = struct {
		packageGuid  string
		downloadAddr string
	}{
		packageGuid:  packageGuid,
		downloadAddr: downloadAddr,
	}
}

func (s *spyPackageClient) GetPackageGuid(ctx context.Context, appGuid string) (guid, downloadAddr string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.packageCtx = ctx
	s.appGuids = append(s.appGuids, appGuid)

	results, ok := s.packageResults[appGuid]
	if !ok {
		return "", "", errors.New("not found")
	}

	return results.packageGuid, results.downloadAddr, s.packageErr
}

func (s *spyPackageClient) PackageCtx() context.Context {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.packageCtx
}

func (s *spyPackageClient) AppGuids() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	results := make([]string, len(s.appGuids))
	copy(results, s.appGuids)

	return results
}

func createZip() []byte {
	buf := &bytes.Buffer{}
	w := zip.NewWriter(buf)

	f, err := w.Create("some-file.txt")
	if err != nil {
		panic(err)
	}
	_, err = f.Write([]byte("some-body"))
	if err != nil {
		panic(err)
	}

	if err := w.Close(); err != nil {
		panic(err)
	}

	return buf.Bytes()
}
