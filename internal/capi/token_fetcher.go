package capi

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
)

type TokenFetcher struct {
	addr string
	doer Doer
}

func NewTokenFetcher(addr string, doer Doer) *TokenFetcher {
	return &TokenFetcher{
		addr: addr,
		doer: doer,
	}
}

func (f *TokenFetcher) Token() (string, error) {
	req, err := http.NewRequest("GET", f.addr+"/tokens", nil)
	if err != nil {
		return "", err
	}

	resp, err := f.doer.Do(req)
	if err != nil {
		return "", err
	}

	defer func() {
		io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var s struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return "", err
	}

	return s.AccessToken, nil
}
