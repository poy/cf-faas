package capi_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/poy/cf-faas/internal/capi"
	gocapi "github.com/poy/go-capi"
	"github.com/poy/onpar"
	. "github.com/poy/onpar/expect"
	. "github.com/poy/onpar/matchers"
)

type TR struct {
	*testing.T
	r         *capi.TaskRunner
	spyClient *spyClient
}

func TestTaskRunner(t *testing.T) {
	t.Parallel()
	o := onpar.New()
	defer o.Run(t)

	o.BeforeEach(func(t *testing.T) TR {
		spyClient := newSpyClient()
		return TR{
			T:         t,
			spyClient: spyClient,
			r:         capi.NewTaskRunner("some-app-name", spyClient),
		}
	})

	o.Spec("it starts a task if a task with the same name doesn't exist", func(t TR) {
		t.spyClient.appGuid = "some-app"
		t.spyClient.runTask = gocapi.Task{
			Guid: "task-guid",
		}

		t.spyClient.listTasks = []gocapi.Task{
			{Name: "some-other-name", Guid: "some-other-guid"},
		}

		guid, err := t.r.RunTask("some-command", "some-name")
		Expect(t, err).To(BeNil())
		Expect(t, guid).To(Equal("task-guid"))

		// context
		Expect(t, t.spyClient.appGuidCtx).To(Not(BeNil()))
		deadline, ok := t.spyClient.appGuidCtx.Deadline()
		Expect(t, ok).To(BeTrue())
		Expect(t, float64(deadline.Sub(time.Now()))).To(And(
			BeAbove(0),
			BeBelow(float64(time.Minute)),
		))

		Expect(t, t.spyClient.appGuidName).To(Equal("some-app-name"))

		// context
		Expect(t, t.spyClient.runCtx).To(Not(BeNil()))
		deadline, ok = t.spyClient.runCtx.Deadline()
		Expect(t, ok).To(BeTrue())
		Expect(t, float64(deadline.Sub(time.Now()))).To(And(
			BeAbove(0),
			BeBelow(float64(time.Minute)),
		))

		Expect(t, t.spyClient.runCommand).To(Equal("some-command"))
		Expect(t, t.spyClient.runName).To(Equal("some-name"))
		Expect(t, t.spyClient.runDropletGuid).To(Equal(""))
		Expect(t, t.spyClient.runAppGuid).To(Equal("some-app"))
	})

	o.Spec("it returns an error if looking up the guid fails", func(t TR) {
		t.spyClient.appGuidErr = errors.New("some-error")
		_, err := t.r.RunTask("some-command", "some-name")
		Expect(t, err).To(Not(BeNil()))
	})

	o.Spec("it returns an error if creating the task fails", func(t TR) {
		t.spyClient.runErr = errors.New("some-error")
		_, err := t.r.RunTask("some-command", "some-name")
		Expect(t, err).To(Not(BeNil()))
	})

	o.Spec("it returns an error if listing tasks fails", func(t TR) {
		t.spyClient.listErr = errors.New("some-error")
		_, err := t.r.RunTask("some-command", "some-name")
		Expect(t, err).To(Not(BeNil()))
	})

	o.Spec("it does not start a task if one already exists with the name", func(t TR) {
		t.spyClient.appGuid = "some-app"
		t.spyClient.listTasks = []gocapi.Task{
			{Name: "some-name", Guid: "some-guid"},
			{Name: "some-other-name", Guid: "some-other-guid"},
		}

		guid, err := t.r.RunTask("some-command", "some-name")
		Expect(t, err).To(BeNil())
		Expect(t, guid).To(Equal("some-guid"))

		// context
		Expect(t, t.spyClient.listCtx).To(Not(BeNil()))
		deadline, ok := t.spyClient.listCtx.Deadline()
		Expect(t, ok).To(BeTrue())
		Expect(t, float64(deadline.Sub(time.Now()))).To(And(
			BeAbove(0),
			BeBelow(float64(time.Minute)),
		))

		Expect(t, t.spyClient.listAppGuid).To(Equal("some-app"))
		Expect(t, t.spyClient.listQuery["names"]).To(Equal([]string{"some-name"}))

		Expect(t, t.spyClient.runCtx).To(BeNil())
	})
}

type spyClient struct {
	appGuidCtx  context.Context
	appGuidName string
	appGuid     string
	appGuidErr  error

	runCtx         context.Context
	runCommand     string
	runName        string
	runDropletGuid string
	runAppGuid     string

	runTask gocapi.Task
	runErr  error

	listCtx     context.Context
	listAppGuid string
	listQuery   map[string][]string

	listTasks []gocapi.Task
	listErr   error
}

func newSpyClient() *spyClient {
	return &spyClient{}
}

func (s *spyClient) GetAppGuid(ctx context.Context, appName string) (string, error) {
	s.appGuidCtx = ctx
	s.appGuidName = appName

	return s.appGuid, s.appGuidErr
}

func (s *spyClient) RunTask(ctx context.Context, command, name, dropletGuid, appGuid string) (gocapi.Task, error) {
	s.runCtx = ctx
	s.runCommand = command
	s.runName = name
	s.runDropletGuid = dropletGuid
	s.runAppGuid = appGuid

	return s.runTask, s.runErr
}

func (s *spyClient) ListTasks(ctx context.Context, appGuid string, query map[string][]string) ([]gocapi.Task, error) {
	s.listCtx = ctx
	s.listAppGuid = appGuid
	s.listQuery = query

	return s.listTasks, s.listErr
}
