package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/apoydence/cf-faas/internal/internalapi"
)

type WorkSubmitter interface {
	Submit(work internalapi.Work)
}

type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

func Run(
	addr string,
	appInstance string,
	waitFor time.Duration,
	s WorkSubmitter,
	d Doer,
	log *log.Logger,
) {
	for {
		req, err := http.NewRequest(http.MethodGet, addr, nil)
		if err != nil {
			log.Fatalf("failed to create request for %s: %s", addr, err)
		}
		req.Header.Set("X-CF-APP-INSTANCE", appInstance)
		req.Header.Set("CACHE_BUSTER", fmt.Sprint(time.Now().UnixNano(), rand.Int63()))

		ctx, _ := context.WithTimeout(context.Background(), waitFor)
		req = req.WithContext(ctx)

		resp, err := d.Do(req)
		if err != nil {
			log.Printf("failed to make request: %s", err)
			return
		}

		if resp.StatusCode != http.StatusOK {
			data, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Printf("failed to read body: %s", err)
				return
			}
			log.Printf("got unexpected status code %d: %s", resp.StatusCode, data)
			return
		}

		var work internalapi.Work
		err = json.NewDecoder(resp.Body).Decode(&work)
		if err != nil {
			log.Printf("failed to unmarshal work: %s", err)
			return
		}

		go s.Submit(work)
	}
}
