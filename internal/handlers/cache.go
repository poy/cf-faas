package handlers

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"

	faas "github.com/apoydence/cf-faas"
	"github.com/golang/groupcache"
)

type Cache struct {
	h       http.Handler
	g       *groupcache.Group
	headers map[string]bool
	log     *log.Logger
}

func NewCache(name string, headers []string, h http.Handler, log *log.Logger) *Cache {
	headersM := make(map[string]bool, len(headers))
	for _, header := range headers {
		headersM[strings.ToLower(header)] = true
	}

	c := &Cache{
		h:       h,
		headers: headersM,
		log:     log,
	}

	c.g = groupcache.NewGroup(name, 1<<20, groupcache.GetterFunc(c.get))

	return c
}

func (c *Cache) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.h.ServeHTTP(w, r)
		return
	}

	var headers []string
	for k, v := range r.Header {
		if !c.headers[strings.ToLower(k)] {
			continue
		}

		for _, vv := range v {
			headers = append(headers, fmt.Sprintf("%s:%s", k, vv))
		}
	}
	sort.Strings(headers)

	req := request{
		Request: faas.Request{
			Method: r.Method,
			Path:   r.URL.String(),
		},
		Headers: headers,
	}

	data, err := json.Marshal(req)
	if err != nil {
		c.log.Panicf("failed to marshal request: %s", err)
	}

	var b []byte
	bv := groupcache.AllocatingByteSliceSink(&b)
	if err := c.g.Get(nil, base64.URLEncoding.EncodeToString(data), bv); err != nil {
		c.h.ServeHTTP(w, r)
		return
	}

	var resp faas.Response
	if err := json.Unmarshal(b, &resp); err != nil {
		c.h.ServeHTTP(w, r)
		return
	}

	w.WriteHeader(resp.StatusCode)
	w.Write(resp.Body)
}

func (c *Cache) get(ctx groupcache.Context, key string, dest groupcache.Sink) error {
	plainKey, err := base64.URLEncoding.DecodeString(key)
	if err != nil {
		return err
	}

	var r request
	if err := json.Unmarshal([]byte(plainKey), &r); err != nil {
		return err
	}

	req, err := http.NewRequest(r.Method, r.Path, bytes.NewReader(nil))
	if err != nil {
		return err
	}

	for _, h := range r.Headers {
		splitUp := strings.SplitN(h, ":", 2)
		req.Header.Add(splitUp[0], splitUp[1])
	}

	recorder := httptest.NewRecorder()
	c.h.ServeHTTP(recorder, req)

	resp := faas.Response{
		StatusCode: recorder.Code,
		Body:       recorder.Body.Bytes(),
	}

	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}

	dest.SetBytes(data)

	return nil
}

type request struct {
	faas.Request
	Headers []string `json:"headers"`
}
