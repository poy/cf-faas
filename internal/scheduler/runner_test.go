package scheduler_test

import (
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"testing"

	"github.com/poy/cf-faas/internal/internalapi"
	"github.com/poy/cf-faas/internal/scheduler"
	"github.com/poy/onpar"
	. "github.com/poy/onpar/expect"
	. "github.com/poy/onpar/matchers"
)

type TR struct {
	*testing.T
	spyPackageManager *spyPackageManager
	spyExecutor       *spyExecutor
	spyDoer           *spyDoer
	r                 *scheduler.Runner
}

func TestRunenr(t *testing.T) {
	t.Parallel()
	o := onpar.New()
	defer o.Run(t)

	o.BeforeEach(func(t *testing.T) TR {
		spyPackageManager := newSpyPackageManager()
		spyExecutor := newSpyExecutor()
		spyDoer := newSpyDoer()
		return TR{
			T:                 t,
			spyPackageManager: spyPackageManager,
			spyExecutor:       spyExecutor,
			spyDoer:           spyDoer,
			r:                 scheduler.NewRunner(spyPackageManager, spyExecutor, spyDoer, map[string]string{"a": "b", "c": "d"}, log.New(ioutil.Discard, "", 0)),
		}
	})

	o.Spec("it executes work from the correct path", func(t TR) {
		t.spyPackageManager.result = "some-path"
		t.r.Submit(internalapi.Work{
			Href:    "http://some.work",
			Command: "some command",
			AppName: "some-app-name",
		})

		Expect(t, t.spyPackageManager.appName).To(Equal("some-app-name"))
		Expect(t, t.spyExecutor.cwd).To(Equal("some-path"))
		Expect(t, t.spyExecutor.envs["a"]).To(Equal("b"))
		Expect(t, t.spyExecutor.envs["c"]).To(Equal("d"))
		Expect(t, t.spyExecutor.envs["CF_FAAS_RELAY_ADDR"]).To(Equal("http://some.work"))
		Expect(t, t.spyExecutor.command).To(Equal("some command"))
	})

	o.Spec("it does not submit work if PackageManager returns an error", func(t TR) {
		t.spyPackageManager.result = "some-path"
		t.spyPackageManager.err = errors.New("some-error")
		t.r.Submit(internalapi.Work{
			Href:    "http://some.work",
			Command: "some-command",
			AppName: "some-app-name",
		})
		Expect(t, t.spyExecutor.cwd).To(Equal(""))
	})

	o.Spec("it sends a 500 to the client if the executor fails", func(t TR) {
		t.spyExecutor.err = errors.New("some-error")
		t.r.Submit(internalapi.Work{
			Href:    "http://some.work",
			Command: "some-command",
			AppName: "some-app-name",
		})

		Expect(t, t.spyDoer.req).To(Not(BeNil()))
		Expect(t, t.spyDoer.req.URL.String()).To(Equal("http://some.work"))
		Expect(t, t.spyDoer.req.Method).To(Equal(http.MethodPost))
		Expect(t, t.spyDoer.body).To(MatchJSON(`{"status_code":500}`))

		_, ok := t.spyDoer.req.Context().Deadline()
		Expect(t, ok).To(BeTrue())
	})
}

type spyPackageManager struct {
	appName string

	result string
	err    error
}

func newSpyPackageManager() *spyPackageManager {
	return &spyPackageManager{}
}

func (s *spyPackageManager) PackageForApp(appName string) (string, error) {
	s.appName = appName
	return s.result, s.err
}

type spyExecutor struct {
	cwd     string
	envs    map[string]string
	command string
	err     error
}

func newSpyExecutor() *spyExecutor {
	return &spyExecutor{}
}

func (s *spyExecutor) Execute(cwd string, envs map[string]string, command string) error {
	s.cwd = cwd
	s.envs = envs
	s.command = command
	return s.err
}
