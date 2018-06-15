package capi_test

import (
	"context"
	"errors"
	"testing"

	"github.com/apoydence/cf-faas/internal/capi"
	"github.com/apoydence/onpar"
	. "github.com/apoydence/onpar/expect"
	. "github.com/apoydence/onpar/matchers"
)

type TD struct {
	*testing.T
	spyDropletClient *spyDropletClient
	f                *capi.DropletGuidFetcher
}

func TestDropletGuidFetcher(t *testing.T) {
	t.Parallel()
	o := onpar.New()
	defer o.Run(t)

	o.BeforeEach(func(t *testing.T) TD {
		spyDropletClient := newSpyDropletClient()
		return TD{
			T:                t,
			spyDropletClient: spyDropletClient,
			f:                capi.NewDropletGuidFetcher(spyDropletClient),
		}
	})

	o.Spec("it asks for the name and then the droplet guid", func(t TD) {
		t.spyDropletClient.appResult = "some-guid"
		t.spyDropletClient.dropletResult = "droplet-guid"
		ctx, _ := context.WithCancel(context.Background())

		guid, err := t.f.FetchGuid(ctx, "some-name")
		Expect(t, err).To(BeNil())

		Expect(t, guid).To(Equal("droplet-guid"))

		Expect(t, t.spyDropletClient.appCtx).To(Equal(ctx))
		Expect(t, t.spyDropletClient.appName).To(Equal("some-name"))

		Expect(t, t.spyDropletClient.dropletCtx).To(Equal(ctx))
		Expect(t, t.spyDropletClient.appGuid).To(Equal("some-guid"))
	})

	o.Spec("it returns an error if fetching the app guid fails", func(t TD) {
		t.spyDropletClient.appResult = "some-guid"
		t.spyDropletClient.dropletErr = errors.New("some-error")
		_, err := t.f.FetchGuid(context.Background(), "some-name")
		Expect(t, err).To(Not(BeNil()))
	})

	o.Spec("it returns an error if fetching the droplet guid fails", func(t TD) {
		t.spyDropletClient.dropletErr = errors.New("some-error")
		_, err := t.f.FetchGuid(context.Background(), "some-name")
		Expect(t, err).To(Not(BeNil()))
	})

	o.Spec("it returns an empty string for an empty app name", func(t TD) {
		name, err := t.f.FetchGuid(context.Background(), "")
		Expect(t, err).To(BeNil())
		Expect(t, name).To(HaveLen(0))
		Expect(t, t.spyDropletClient.appCtx).To(BeNil())
	})
}

type spyDropletClient struct {
	appCtx    context.Context
	appName   string
	appResult string
	appErr    error

	dropletCtx    context.Context
	appGuid       string
	dropletResult string
	dropletErr    error
}

func newSpyDropletClient() *spyDropletClient {
	return &spyDropletClient{}
}

func (s *spyDropletClient) GetAppGuid(ctx context.Context, appName string) (string, error) {
	s.appCtx = ctx
	s.appName = appName
	return s.appResult, s.appErr
}

func (s *spyDropletClient) GetDropletGuid(ctx context.Context, appGuid string) (string, error) {
	s.dropletCtx = ctx
	s.appGuid = appGuid
	return s.dropletResult, s.dropletErr
}
