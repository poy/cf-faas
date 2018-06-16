package capi

import (
	"context"
	"sync"
)

type DropletGuidFetcher struct {
	mu sync.RWMutex

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

func (f *DropletGuidFetcher) FetchGuid(ctx context.Context, appName string) (appGuid, dropletGuid string, err error) {
	if appName == "" {
		return "", "", nil
	}

	appGuid, err = f.c.GetAppGuid(ctx, appName)
	if err != nil {
		return "", "", err
	}

	dropletGuid, err = f.c.GetDropletGuid(ctx, appGuid)
	if err != nil {
		return "", "", err
	}

	return appGuid, dropletGuid, nil
}
