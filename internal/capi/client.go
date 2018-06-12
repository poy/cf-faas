package capi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	addr     string
	appGuid  string
	doer     Doer
	interval time.Duration
}

type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

func NewClient(addr, appGuid string, interval time.Duration, d Doer) *Client {
	return &Client{
		doer:     d,
		addr:     addr,
		appGuid:  appGuid,
		interval: interval,
	}
}

func (c *Client) CreateTask(ctx context.Context, command string) error {
	u, err := url.Parse(c.addr)
	if err != nil {
		return err
	}
	u.Path = fmt.Sprintf("/v3/apps/%s/tasks", c.appGuid)

	marshalled, err := json.Marshal(struct {
		Command string `json:"command"`
	}{
		Command: command,
	})
	if err != nil {
		return err
	}

	req := &http.Request{
		URL:    u,
		Method: "POST",
		Body:   ioutil.NopCloser(bytes.NewReader(marshalled)),
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
	}
	req = req.WithContext(ctx)

	resp, err := c.doer.Do(req)
	if err != nil {
		return err
	}

	defer func(resp *http.Response) {
		// Fail safe to ensure the clients are being cleaned up
		io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
	}(resp)

	if resp.StatusCode != 202 {
		data, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, data)
	}

	for {
		var results struct {
			State string `json:"state"`
			Links struct {
				Self struct {
					Href string `json:"href"`
				} `json:"self"`
			} `json:"links"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
			return err
		}

		// Replace HTTPS with HTTP so the HTTP_PROXY can do the work for us
		results.Links.Self.Href = strings.Replace(results.Links.Self.Href, "https", "http", 1)

		resp.Body.Close()

		switch results.State {
		case "RUNNING":
			time.Sleep(c.interval)

			u, err := url.Parse(results.Links.Self.Href)
			if err != nil {
				return err
			}

			req := &http.Request{
				URL:    u,
				Method: "GET",
				Header: http.Header{},
			}
			req = req.WithContext(ctx)

			resp, err = c.doer.Do(req)
			if err != nil {
				return err
			}

			defer func(resp *http.Response) {
				// Fail safe to ensure the clients are being cleaned up
				io.Copy(ioutil.Discard, resp.Body)
				resp.Body.Close()
			}(resp)

			continue
		case "FAILED":
			return errors.New("task failed")
		default:
			return nil
		}
	}

	return nil
}

func (c *Client) ListTasks(appGuid string) ([]string, error) {
	var names []string
	addr := c.addr

	for {
		u, err := url.Parse(addr)
		if err != nil {
			return nil, err
		}
		u.Path = fmt.Sprintf("/v3/apps/%s/tasks", appGuid)

		req := &http.Request{
			URL:    u,
			Method: "GET",
			Header: http.Header{},
		}

		resp, err := c.doer.Do(req)
		if err != nil {
			return nil, err
		}

		defer func() {
			io.Copy(ioutil.Discard, resp.Body)
			resp.Body.Close()
		}()

		if resp.StatusCode != 200 {
			data, _ := ioutil.ReadAll(resp.Body)
			return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, data)
		}

		var tasks struct {
			Pagination struct {
				Next struct {
					Href string `json:"href"`
				} `json:"next"`
			} `json:"pagination"`
			Resources []struct {
				Name string `json:"name"`
			} `json:"resources"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
			return nil, err
		}

		for _, t := range tasks.Resources {
			names = append(names, t.Name)
		}

		if tasks.Pagination.Next.Href != "" {
			addr = tasks.Pagination.Next.Href
			continue
		}

		return names, nil
	}

}
