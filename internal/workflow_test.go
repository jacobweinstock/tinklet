package internal

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/tinkerbell/tink/protos/workflow"
	tw "github.com/tinkerbell/tink/protos/workflow"
	"google.golang.org/grpc"
)

type mocker struct {
	failListWorkflows         bool
	failListWorkflowsRecvFunc bool
	numWorkflowsToMock        int
	start                     int
}

func TestGetWorkflowsOfficial(t *testing.T) {
	testCases := map[string]struct {
		expectedWorkflows []*tw.Workflow
		err               error
		ctxTimeout        time.Duration
		mock              *mocker
		filterByFunc      func(tw.WorkflowServiceClient, []*tw.Workflow) []*tw.Workflow
	}{
		"success":            {expectedWorkflows: []*tw.Workflow{{Id: "1"}, {Id: "2"}, {Id: "3"}}, mock: &mocker{numWorkflowsToMock: 3}},
		"fail ListWorkflows": {err: errors.New("error getting workflows: failed"), mock: &mocker{failListWorkflows: true}},
		"fail recv":          {err: &multierror.Error{Errors: []error{errors.New("failed")}}, mock: &mocker{failListWorkflowsRecvFunc: true, numWorkflowsToMock: 1}},
		"success with filter": {expectedWorkflows: []*tw.Workflow{{Id: "1"}, {Id: "2"}}, mock: &mocker{numWorkflowsToMock: 3}, filterByFunc: func(workflowClient tw.WorkflowServiceClient, workflows []*tw.Workflow) []*tw.Workflow {
			var filteredWorkflows []*tw.Workflow
			for _, elem := range workflows {
				if elem.Id != "3" {
					filteredWorkflows = append(filteredWorkflows, elem)
				}
			}
			return filteredWorkflows
		}},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
			defer cancel()

			var workflows []*tw.Workflow
			var err error
			if tc.filterByFunc != nil {
				workflows, err = getAllWorkflows(ctx, tc.mock.getMockedWorkflowServiceClient(), tc.filterByFunc)
			} else {
				workflows, err = getAllWorkflows(ctx, tc.mock.getMockedWorkflowServiceClient())
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
				if diff := cmp.Diff(workflows, tc.expectedWorkflows, cmpopts.IgnoreUnexported(tw.Workflow{})); diff != "" {
					t.Fatal(diff)
				}
			}
		})
	}
}
func TestGetWorkflows(t *testing.T) {
	t.Skip()
	conn, err := grpc.Dial("192.168.1.214:42113", grpc.WithInsecure())
	if err != nil {
		t.Fatal(err)
	}

	var filterByMac workflowsFilterByFunc = func(workflowClient workflow.WorkflowServiceClient, workflows []*tw.Workflow) []*tw.Workflow {
		var filteredWorkflows []*tw.Workflow
		for _, elem := range workflows {
			if strings.Contains(elem.Hardware, "00:50:56:25:11:0e") {
				filteredWorkflows = append(filteredWorkflows, elem)
			}
		}
		return filteredWorkflows
	}

	var filterByState workflowsFilterByFunc = func(workflowClient workflow.WorkflowServiceClient, workflows []*workflow.Workflow) []*workflow.Workflow {
		var filteredWorkflows []*tw.Workflow
		for _, elem := range workflows {
			if elem.State != tw.State_STATE_SUCCESS && elem.State != tw.State_STATE_TIMEOUT {
				filteredWorkflows = append(filteredWorkflows, elem)
			}
		}
		return filteredWorkflows
	}

	client := tw.NewWorkflowServiceClient(conn)
	workflows, err := getAllWorkflows(context.Background(), client, filterByMac, filterByState)
	if err != nil {
		t.Fatal(err)
	}
	if len(workflows) > 0 {
		t.Log(workflows[0])
	}

	t.Log(len(workflows))

	t.Fatal()
}

func (m *mocker) getMockedWorkflowServiceClient() *tw.WorkflowServiceClientMock {
	var workflowSvcClient tw.WorkflowServiceClientMock
	var recvFunc tw.WorkflowService_ListWorkflowsClientMock

	recvFunc.RecvFunc = func() (*tw.Workflow, error) {
		if m.start == m.numWorkflowsToMock {
			return nil, io.EOF
		}
		m.start++
		return &tw.Workflow{Id: fmt.Sprintf("%v", m.start)}, nil
	}
	workflowSvcClient.ListWorkflowsFunc = func(ctx context.Context, in *tw.Empty, opts ...grpc.CallOption) (tw.WorkflowService_ListWorkflowsClient, error) {
		return &recvFunc, nil
	}

	if m.failListWorkflows {
		workflowSvcClient.ListWorkflowsFunc = func(ctx context.Context, in *tw.Empty, opts ...grpc.CallOption) (tw.WorkflowService_ListWorkflowsClient, error) {
			return nil, errors.New("failed")
		}
	}

	if m.failListWorkflowsRecvFunc {
		recvFunc.RecvFunc = func() (*tw.Workflow, error) {
			if m.start == m.numWorkflowsToMock {
				return nil, io.EOF
			}
			m.start++
			return nil, errors.New("failed")
		}
		workflowSvcClient.ListWorkflowsFunc = func(ctx context.Context, in *tw.Empty, opts ...grpc.CallOption) (tw.WorkflowService_ListWorkflowsClient, error) {
			return &recvFunc, nil
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
	workflowID, actions, err := getActionsList(context.Background(), tw.NewWorkflowServiceClient(conn), nil)
	if err != nil {
		t.Fatal(err)
	}

	t.Log(workflowID)
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
	got := actionToDockerContainerConfig(context.Background(), &action, withTty(false)) // nolint
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
	got := actionToDockerHostConfig(context.Background(), &action, withPid("custom")) // nolint
	if diff := cmp.Diff(got, expected); diff != "" {
		t.Fatal(diff)
	}
}
