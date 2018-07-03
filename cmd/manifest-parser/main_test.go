package main_test

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"
	"time"

	. "github.com/apoydence/onpar/expect"
	. "github.com/apoydence/onpar/matchers"
)

func TestParseManifest(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Second)
	defer cancel()

	buf := bytes.Buffer{}
	defer func() {
		if t.Failed() {
			t.Log(buf.String())
		}
	}()

	Expect(t, startTestExec(ctx, t, &buf,
		"MANIFEST="+m,
	)()).To(BeNil())

	Expect(t, strings.TrimSpace(buf.String())).To(Or(
		Equal("/v1/second-open,/v2/first-open"),
		Equal("/v2/first-open,/v1/second-open"),
	))
}

func TestParseResolversSuccess(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Second)
	defer cancel()

	buf := bytes.Buffer{}
	defer func() {
		if t.Failed() {
			t.Log(buf.String())
		}
	}()

	Expect(t, startTestExec(ctx, t, &buf,
		"MANIFEST="+m,
		"VALIDATE_RESOLVERS=true",
		"RESOLVER_URLS=other-event:some.url,another-event:other.url",
	)()).To(BeNil())
}

func TestParseResolversFailure(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Second)
	defer cancel()

	buf := bytes.Buffer{}
	defer func() {
		if t.Failed() {
			t.Log(buf.String())
		}
	}()

	Expect(t, startTestExec(ctx, t, &buf,
		"MANIFEST="+m,
		"VALIDATE_RESOLVERS=true",
		"RESOLVER_URLS=another-event:other.url", // missing resolver for other-event
	)()).To(Not(BeNil()))
}

const m = `---

functions:
- handler:
   app_name: faas-droplet-echo
   command: ./echo
  events:
    http:
    - path: /v1/default_closed
      method: POST
    - path: /v2/first-open
      method: POST
      no_auth: true
    other-event:
    - some: data
- handler:
   app_name: faas-droplet-echo
   command: ./echo
  events:
    http:
    - path: /v1/second-open
      no_auth: true
      method: POST
    - path: /v2/closed
      method: POST
      no_auth: false
    another-event:
    - some: data
`

func startTestExec(ctx context.Context, t *testing.T, writer io.Writer, envs ...string) func() error {
	t.Helper()

	tempDir, err := ioutil.TempDir("", "build-artifacts")
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.CommandContext(ctx, "go", "build", "-o", path.Join(tempDir, "manifest-parser"), "github.com/apoydence/cf-faas/cmd/manifest-parser")

	cmd.Env = []string{"GOPATH=" + gopath(t)}
	buf := &bytes.Buffer{}
	cmd.Stderr = writer
	cmd.Stdout = writer
	if err := cmd.Run(); err != nil {
		t.Log(buf.String())
		t.Fatal(err)
	}

	cmd = exec.CommandContext(ctx, path.Join(tempDir, "manifest-parser"))
	cmd.Env = envs
	cmd.Stderr = writer
	cmd.Stdout = writer
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	return cmd.Wait
}

func gopath(t *testing.T) string {
	if os.Getenv("GOPATH") != "" {
		return os.Getenv("GOPATH")
	}

	// *nix
	if os.Getenv("HOME") != "" {
		return path.Join(os.Getenv("HOME"), "go")
	}

	// windows
	if os.Getenv("USERPROFILE") != "" {
		return path.Join(os.Getenv("USERPROFILE"), "go")
	}

	t.Fatal("unable to create GOPATH")
	return ""
}
