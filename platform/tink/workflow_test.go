package tink

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/tinkerbell/tink/protos/workflow"
	"google.golang.org/grpc"
)

type mocker struct {
	failListWorkflows           bool
	failListWorkflowsRecvFunc   bool
	failGetWorkflowActions      bool
	failGetWorkflowContexts     bool
	failGetWorkflowContextsRecv error
	numWorkflowsToMock          int
	numContextsToMock           int
	start                       int
}

func TestGetActionsList(t *testing.T) {
	testCases := map[string]struct {
		expectedWorkflows []*workflow.WorkflowAction
		err               error
		ctxTimeout        time.Duration
		mock              *mocker
		filterByFunc      actionsFilterByFunc
	}{
		"success":                 {expectedWorkflows: []*workflow.WorkflowAction{{TaskName: "os-install", Name: "start", Image: "alpine", Timeout: 0, Command: []string{"/bin/sh", "sleep", "5"}, WorkerId: "12345"}}, mock: &mocker{}},
		"fail to get action list": {err: errors.Wrap(errors.New("ah bad!"), "GetActionsList failed"), mock: &mocker{failGetWorkflowActions: true}},
		"success with filter":     {expectedWorkflows: []*workflow.WorkflowAction{{TaskName: "os-install", Name: "start", Image: "alpine", Timeout: 0, Command: []string{"/bin/sh", "sleep", "5"}, WorkerId: "12345"}}, mock: &mocker{}, filterByFunc: FilterActionsByWorkerID("12345")},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
			defer cancel()

			var workflows []*workflow.WorkflowAction
			var err error
			if tc.filterByFunc != nil {
				workflows, err = GetActionsList(ctx, "12345", tc.mock.getMockedWorkflowServiceClient(), tc.filterByFunc)
			} else {
				workflows, err = GetActionsList(ctx, "12345", tc.mock.getMockedWorkflowServiceClient())
			}
			if err != nil {
				if tc.err != nil {
					if diff := cmp.Diff(err.Error(), tc.err.Error()); diff != "" {
						t.Fatal(diff)
					}
				} else {
					t.Fatal(err)
				}
			} else {
				if diff := cmp.Diff(workflows, tc.expectedWorkflows, cmpopts.IgnoreUnexported(workflow.WorkflowAction{})); diff != "" {
					t.Fatal(diff)
				}
			}
		})
	}
}

func TestGetWorkflowContexts(t *testing.T) {
	testCases := map[string]struct {
		expectedWorkflows []*workflow.WorkflowContext
		err               error
		ctxTimeout        time.Duration
		mock              *mocker
	}{
		"success":                     {expectedWorkflows: []*workflow.WorkflowContext{{WorkflowId: "12345"}}, mock: &mocker{numContextsToMock: 1}},
		"fail to get contexts":        {err: errors.Wrap(errors.New("failed"), "error getting workflow contexts"), mock: &mocker{failGetWorkflowContexts: true}},
		"fail to stream any contexts": {err: &multierror.Error{Errors: []error{errors.New("lkdfj")}}, mock: &mocker{numContextsToMock: 1, failGetWorkflowContextsRecv: errors.New("lkdfj")}},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
			defer cancel()

			workflows, err := GetWorkflowContexts(ctx, "12345", tc.mock.getMockedWorkflowServiceClient())
			if err != nil {
				if tc.err != nil {
					if diff := cmp.Diff(err.Error(), tc.err.Error()); diff != "" {
						t.Fatal(diff)
					}
				} else {
					t.Fatal(err)
				}
			} else {
				if diff := cmp.Diff(workflows, tc.expectedWorkflows, cmpopts.IgnoreUnexported(workflow.WorkflowContext{})); diff != "" {
					t.Fatal(diff)
				}
			}
		})
	}
}

func (m *mocker) getMockedWorkflowServiceClient() *workflow.WorkflowServiceClientMock {
	var workflowSvcClient workflow.WorkflowServiceClientMock
	var recvFunc workflow.WorkflowService_ListWorkflowsClientMock

	recvFunc.RecvFunc = func() (*workflow.Workflow, error) {
		if m.start == m.numWorkflowsToMock {
			return nil, io.EOF
		}
		m.start++
		return &workflow.Workflow{Id: fmt.Sprintf("%v", m.start)}, nil
	}
	workflowSvcClient.ListWorkflowsFunc = func(ctx context.Context, in *workflow.Empty, opts ...grpc.CallOption) (workflow.WorkflowService_ListWorkflowsClient, error) {
		return &recvFunc, nil
	}

	if m.failListWorkflows {
		workflowSvcClient.ListWorkflowsFunc = func(ctx context.Context, in *workflow.Empty, opts ...grpc.CallOption) (workflow.WorkflowService_ListWorkflowsClient, error) {
			return nil, errors.New("failed")
		}
	}

	if m.failListWorkflowsRecvFunc {
		recvFunc.RecvFunc = func() (*workflow.Workflow, error) {
			if m.start == m.numWorkflowsToMock {
				return nil, io.EOF
			}
			m.start++
			return nil, errors.New("failed")
		}
		workflowSvcClient.ListWorkflowsFunc = func(ctx context.Context, in *workflow.Empty, opts ...grpc.CallOption) (workflow.WorkflowService_ListWorkflowsClient, error) {
			return &recvFunc, nil
		}
	}

	workflowSvcClient.GetWorkflowActionsFunc = func(ctx context.Context, in *workflow.WorkflowActionsRequest, opts ...grpc.CallOption) (*workflow.WorkflowActionList, error) {
		var err error
		resp := &workflow.WorkflowActionList{
			ActionList: []*workflow.WorkflowAction{
				{
					TaskName: "os-install",
					Name:     "start",
					Image:    "alpine",
					Timeout:  0,
					Command:  []string{"/bin/sh", "sleep", "5"},
					WorkerId: "12345",
				},
			},
		}
		if m.failGetWorkflowActions {
			resp = nil
			err = errors.New("ah bad!")
		}
		return resp, err
	}

	workflowSvcClient.GetWorkflowContextsFunc = func(ctx context.Context, in *workflow.WorkflowContextRequest, opts ...grpc.CallOption) (workflow.WorkflowService_GetWorkflowContextsClient, error) {
		var recvFuncContexts WorkflowService_GetWorkflowContextsClientMock
		recvFuncContexts.RecvFunc = func() (*workflow.WorkflowContext, error) {
			if m.start == m.numContextsToMock {
				return nil, io.EOF
			}
			m.start++
			return &workflow.WorkflowContext{
				WorkflowId:           "12345",
				CurrentWorker:        "",
				CurrentTask:          "",
				CurrentAction:        "",
				CurrentActionIndex:   0,
				CurrentActionState:   0,
				TotalNumberOfActions: 0,
			}, m.failGetWorkflowContextsRecv
		}
		return &recvFuncContexts, nil
	}
	if m.failGetWorkflowContexts {
		workflowSvcClient.GetWorkflowContextsFunc = func(ctx context.Context, in *workflow.WorkflowContextRequest, opts ...grpc.CallOption) (workflow.WorkflowService_GetWorkflowContextsClient, error) {
			return nil, errors.New("failed")
		}
	}

	return &workflowSvcClient
}

func TestReporting(t *testing.T) {
	t.Skip()
	conn, err := grpc.Dial("192.168.1.214:42113", grpc.WithInsecure())
	if err != nil {
		t.Fatal(err)
	}
	actions, err := GetActionsList(context.Background(), "", workflow.NewWorkflowServiceClient(conn))
	if err != nil {
		t.Fatal(err)
	}

	t.Log(actions)

	t.Fatal()
}

func TestActionToDockerConfig(t *testing.T) {
	withTty := func(tty bool) containerConfigOption {
		return func(args *container.Config) { args.Tty = tty }
	}
	expected := &container.Config{
		AttachStdout: true,
		AttachStderr: true,
		Tty:          false,
		Env:          []string{"test=one"},
		Cmd:          []string{"/bin/sh"},
		Image:        "nginx",
	}
	action := workflow.WorkflowAction{
		Image:       "nginx",
		Command:     []string{"/bin/sh"},
		Environment: []string{"test=one"},
	}
	got := ActionToDockerContainerConfig(context.Background(), &action, withTty(false)) // nolint
	if diff := cmp.Diff(got, expected); diff != "" {
		t.Fatal(diff)
	}
}

func TestActionToDockerHostConfig(t *testing.T) {
	withPid := func(pid string) containerHostOption {
		return func(args *container.HostConfig) { args.PidMode = container.PidMode(pid) }
	}
	expected := &container.HostConfig{
		Binds: []string{
			"/dev:/dev",
			"/dev/console:/dev/console",
			"/lib/firmware:/lib/firmware:ro",
		},
		PidMode:    container.PidMode("custom"),
		Privileged: true,
	}
	action := workflow.WorkflowAction{
		Image:       "nginx",
		Command:     []string{"/bin/sh"},
		Environment: []string{"test=one"},
		Volumes: []string{
			"/dev:/dev",
			"/dev/console:/dev/console",
			"/lib/firmware:/lib/firmware:ro",
		},
		Pid: "host",
	}
	got := ActionToDockerHostConfig(context.Background(), &action, withPid("custom")) // nolint
	if diff := cmp.Diff(got, expected); diff != "" {
		t.Fatal(diff)
	}
}

func TestWorkflowContexts(t *testing.T) {
	t.Skip()
	conn, err := grpc.Dial("192.168.1.214:42113", grpc.WithInsecure())
	if err != nil {
		t.Fatal(err)
	}
	workerID := "0eba0bf8-3772-4b4a-ab9f-6ebe93b90a94"
	//workerID = "b3fa8f4f-f8f9-4a11-bdcc-265465aa87b7"
	ws := workflow.NewWorkflowServiceClient(conn)
	c, err := ws.GetWorkflowContexts(context.Background(), &workflow.WorkflowContextRequest{
		WorkerId: workerID,
	})
	if err != nil {
		t.Fatal(err)
	}
	var wks []*workflow.WorkflowContext
	for {
		aWorkflow, recvErr := c.Recv()
		if recvErr == io.EOF {
			break
		}
		if recvErr != nil {
			err = multierror.Append(err, recvErr)
			continue
		}
		wks = append(wks, aWorkflow)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Log(wks)

	wctx, err := ws.GetWorkflowContext(context.Background(), &workflow.GetRequest{
		Id: "5675e7d4-9995-11eb-b5c7-0242ac120007",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%+v", wctx)

	aclist, err := ws.GetWorkflowActions(context.Background(), &workflow.WorkflowActionsRequest{
		WorkflowId: "5675e7d4-9995-11eb-b5c7-0242ac120007",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, elem := range aclist.GetActionList() {
		t.Logf("%+v", elem)
	}
	t.Fatal()
}
