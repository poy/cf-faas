package capi

import (
	"context"
)

type DropletGuidFetcher struct {
	c DropletClient
}

type DropletClient interface {
	GetAppGuid(ctx context.Context, appName string) (string, error)
	GetDropletGuid(ctx context.Context, appGuid string) (string, error)
}

func NewDropletGuidFetcher(c DropletClient) *DropletGuidFetcher {
	return &DropletGuidFetcher{
		c: c,
	}
}

func (f *DropletGuidFetcher) FetchGuid(ctx context.Context, appName string) (string, error) {
	if appName == "" {
		return "", nil
	}

	appGuid, err := f.c.GetAppGuid(ctx, appName)
	if err != nil {
		return "", err
	}

	return f.c.GetDropletGuid(ctx, appGuid)
}
