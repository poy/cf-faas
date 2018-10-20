package capi_test

import (
	"errors"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/poy/cf-faas/internal/capi"
	"github.com/poy/onpar"
	. "github.com/poy/onpar/expect"
	. "github.com/poy/onpar/matchers"
)

type TF struct {
	*testing.T

	spyDoer *spyDoer
	f       *capi.TokenFetcher
}

func TestTokenFetcher(t *testing.T) {
	t.Parallel()
	o := onpar.New()
	defer o.Run(t)

	o.BeforeEach(func(t *testing.T) TF {
		spyDoer := newSpyDoer()
		return TF{
			T:       t,
			spyDoer: spyDoer,
			f:       capi.NewTokenFetcher("http://some.url", spyDoer),
		}
	})

	o.Spec("it returns the token", func(t TF) {
		t.spyDoer.m["GET:http://some.url/tokens"] = &http.Response{
			StatusCode: http.StatusOK,
			Body:       ioutil.NopCloser(strings.NewReader(`{"access_token":"some-token"}`)),
		}
		token, err := t.f.Token()
		Expect(t, err).To(BeNil())

		Expect(t, token).To(Equal("some-token"))
	})

	o.Spec("it returns an error for an invalid response", func(t TF) {
		t.spyDoer.err = errors.New("some-error")
		t.spyDoer.m["GET:http://some.url/tokens"] = &http.Response{
			StatusCode: http.StatusOK,
			Body:       ioutil.NopCloser(strings.NewReader(`invalid`)),
		}
		_, err := t.f.Token()
		Expect(t, err).To(Not(BeNil()))
	})

	o.Spec("it returns an error for a doer error", func(t TF) {
		t.spyDoer.err = errors.New("some-error")
		t.spyDoer.m["GET:http://some.url/tokens"] = &http.Response{
			StatusCode: http.StatusOK,
			Body:       ioutil.NopCloser(strings.NewReader(`{"access_token":"some-token"}`)),
		}
		_, err := t.f.Token()
		Expect(t, err).To(Not(BeNil()))
	})

	o.Spec("it returns an error for a non-200", func(t TF) {
		t.spyDoer.m["GET:http://some.url/tokens"] = &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       ioutil.NopCloser(strings.NewReader(``)),
		}
		_, err := t.f.Token()
		Expect(t, err).To(Not(BeNil()))
	})
}
