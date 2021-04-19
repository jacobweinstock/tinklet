package app

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/packethost/pkg/log/logr"
	"github.com/pkg/errors"
	"github.com/tinkerbell/tink/protos/hardware"
	"github.com/tinkerbell/tink/protos/workflow"
	"google.golang.org/grpc"
)

type mockClient struct {
	client.ContainerAPIClient
	client.ImageAPIClient
	mock clientMockHelper
}

type clientMockHelper struct {
	stringReadCloser io.ReadCloser
	imagePullErr     error
	mockContainerCreate
	mockContainerStart
	mockContainerInspect
	mockContainerLogs
}

type mockContainerCreate struct {
	createErr error
	createID  string
	//createWarnings []string
}

type mockContainerStart struct {
	startErr error
}

type mockContainerInspect struct {
	inspectErr   error
	inspectID    string
	inspectState *types.ContainerState
}

type mockContainerLogs struct {
	logsReadCloser io.ReadCloser
	logsErr        error
}

func (t *mockClient) ImagePull(ctx context.Context, ref string, options types.ImagePullOptions) (io.ReadCloser, error) {
	return t.mock.stringReadCloser, t.mock.imagePullErr
}

func (t *mockClient) ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *specs.Platform, containerName string) (container.ContainerCreateCreatedBody, error) {
	return container.ContainerCreateCreatedBody{ID: t.mock.createID, Warnings: []string{}}, t.mock.createErr
}

func (t *mockClient) ContainerStart(ctx context.Context, container string, options types.ContainerStartOptions) error {
	return t.mock.startErr
}

func (t *mockClient) ContainerRemove(ctx context.Context, container string, options types.ContainerRemoveOptions) error {
	return nil
}

func (t *mockClient) ContainerInspect(ctx context.Context, container string) (types.ContainerJSON, error) {
	return types.ContainerJSON{ContainerJSONBase: &types.ContainerJSONBase{ID: t.mock.inspectID, State: t.mock.inspectState}}, t.mock.inspectErr
}

func (t *mockClient) ContainerLogs(ctx context.Context, container string, options types.ContainerLogsOptions) (io.ReadCloser, error) {
	return t.mock.logsReadCloser, t.mock.logsErr
}

func captureOutput(f func()) []string {
	rescueStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	f()
	w.Close()
	scanner := bufio.NewScanner(r)
	var output []string
	for scanner.Scan() {
		output = append(output, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
	os.Stdout = rescueStdout

	return output
}

func TestActionExecutionFlowPullFail(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	log, _, _ := logr.NewPacketLogr()
	imageName := "alpine"
	containerName := "test-container"
	timeout := time.Duration(5)

	dockerClient := mockClient{
		mock: clientMockHelper{
			imagePullErr: errors.New("pull failed"),
		},
	}
	err := ActionExecutionFlow(ctx, log, &dockerClient, imageName, types.ImagePullOptions{}, &container.Config{}, &container.HostConfig{}, containerName, timeout)
	if diff := cmp.Diff(err.Error(), "error pulling image: alpine: pull failed: msg: image pull failed; exit code: 0; details: ; stdout: "); diff != "" {
		t.Fatal(diff)
	}

}

func TestActionExecutionFlowCreateContainerFail(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	log, _, _ := logr.NewPacketLogr()
	imageName := "alpine"
	containerName := "test-container"
	timeout := time.Duration(5)

	stringReader := strings.NewReader("{\"status\": \"hello\",\"error\":\"\"}{\"status\":\"world\",\"error\":\"\"}")
	dockerClient := mockClient{
		mock: clientMockHelper{
			stringReadCloser: io.NopCloser(stringReader),
			mockContainerCreate: mockContainerCreate{
				createErr: errors.New("create failed"),
				createID:  "12345",
			},
		},
	}
	err := ActionExecutionFlow(ctx, log, &dockerClient, imageName, types.ImagePullOptions{}, &container.Config{}, &container.HostConfig{}, containerName, timeout)
	if diff := cmp.Diff(err.Error(), "create failed: msg: creating container failed; exit code: 0; details: ; stdout: "); diff != "" {
		t.Fatal(diff)
	}

}

func TestActionExecutionFlowContainerStartFail(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	log, _, _ := logr.NewPacketLogr()
	imageName := "alpine"
	containerName := "test-container"
	timeout := time.Duration(10)

	stringReader := strings.NewReader("{\"error\":\"\"}")
	dockerClient := mockClient{
		mock: clientMockHelper{
			stringReadCloser: io.NopCloser(stringReader),
			mockContainerCreate: mockContainerCreate{
				createID: "12345",
			},
			mockContainerStart: mockContainerStart{
				startErr: errors.New("failed to start"),
			},
		},
	}
	err := ActionExecutionFlow(ctx, log, &dockerClient, imageName, types.ImagePullOptions{}, &container.Config{}, &container.HostConfig{}, containerName, timeout)
	if diff := cmp.Diff(err.Error(), "failed to start: msg: starting container failed; exit code: 0; details: ; stdout: "); diff != "" {
		t.Fatal(diff)
	}
}

func TestActionExecutionFlowContainerExecCompleteFail(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	log, _, _ := logr.NewPacketLogr()
	imageName := "alpine"
	containerName := "test-container"
	timeout := time.Duration(10)

	stringReader := strings.NewReader("{\"error\":\"\"}")
	dockerClient := mockClient{
		mock: clientMockHelper{
			stringReadCloser: io.NopCloser(stringReader),
			mockContainerCreate: mockContainerCreate{
				createID: "12345",
			},
			mockContainerStart: mockContainerStart{
				startErr: nil,
			},
			mockContainerInspect: mockContainerInspect{
				inspectErr: errors.New("failed inspect container"),
			},
		},
	}
	err := ActionExecutionFlow(ctx, log, &dockerClient, imageName, types.ImagePullOptions{}, &container.Config{}, &container.HostConfig{}, containerName, timeout)
	if diff := cmp.Diff(err.Error(), "unable to inspect container: failed inspect container: msg: waiting for container failed; exit code: 0; details: ; stdout: "); diff != "" {
		t.Fatal(diff)
	}
}

func TestActionExecutionFlowContainerGetLogsFail(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	log, _, _ := logr.NewPacketLogr()
	imageName := "alpine"
	containerName := "test-container"
	timeout := time.Duration(10)

	stringReader := strings.NewReader("{\"error\":\"\"}")
	dockerClient := mockClient{
		mock: clientMockHelper{
			stringReadCloser: io.NopCloser(stringReader),
			mockContainerCreate: mockContainerCreate{
				createID: "12345",
			},
			mockContainerStart: mockContainerStart{
				startErr: nil,
			},
			mockContainerInspect: mockContainerInspect{
				inspectID: "12345",
				inspectState: &types.ContainerState{
					Status:   "exited",
					ExitCode: 127,
				},
			},
			mockContainerLogs: mockContainerLogs{
				logsReadCloser: io.NopCloser(strings.NewReader("no logs")),
				logsErr:        errors.New("logs failed"),
			},
		},
	}
	err := ActionExecutionFlow(ctx, log, &dockerClient, imageName, types.ImagePullOptions{}, &container.Config{}, &container.HostConfig{}, containerName, timeout)
	if diff := cmp.Diff(err.Error(), "msg: container execution was unsuccessful; logs: ;  exitCode: 127; details: "); diff != "" {
		t.Fatal(diff)
	}
}

func TestActionExecutionFlowSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	log, _, _ := logr.NewPacketLogr()
	imageName := "alpine"
	containerName := "test-container"
	timeout := time.Duration(10)

	stringReader := strings.NewReader("{\"error\":\"\"}")
	dockerClient := mockClient{
		mock: clientMockHelper{
			stringReadCloser: io.NopCloser(stringReader),
			mockContainerCreate: mockContainerCreate{
				createID: "12345",
			},
			mockContainerStart: mockContainerStart{
				startErr: nil,
			},
			mockContainerInspect: mockContainerInspect{
				inspectID: "12345",
				inspectState: &types.ContainerState{
					Status:   "exited",
					ExitCode: 0,
				},
			},
		},
	}
	err := ActionExecutionFlow(ctx, log, &dockerClient, imageName, types.ImagePullOptions{}, &container.Config{}, &container.HostConfig{}, containerName, timeout)
	if err != nil {
		t.Fatal(err)
	}
}

func TestReconciler(t *testing.T) {
	type action struct {
		TaskName string   `json:"task_name"`
		Name     string   `json:"name"`
		Image    string   `json:"image"`
		Command  []string `json:"command"`
		WorkerID string   `json:"worker_id"`
	}
	type actionOutput struct {
		Level    string      `json:"level"`
		Ts       float64     `json:"ts"`
		Caller   string      `json:"caller"`
		Msg      string      `json:"msg"`
		Service  string      `json:"service"`
		WorkerID string      `json:"workerID"`
		Action   action      `json:"action"`
		Err      interface{} `json:"err"`
	}

	expectedOutput := []actionOutput{
		{
			Level:    "info",
			Msg:      "executing action",
			Service:  "not/set",
			WorkerID: "0eba0bf8-3772-4b4a-ab9f-6ebe93b90a94",
			Action: action{
				TaskName: "os-install",
				Name:     "start",
				Image:    "alpine",
				Command: []string{
					"/bin/sh",
					"sleep",
					"1",
				},
				WorkerID: "0eba0bf8-3772-4b4a-ab9f-6ebe93b90a94",
			},
			Err: nil,
		},
		{
			Level:    "info",
			Msg:      "action complete",
			Service:  "not/set",
			WorkerID: "0eba0bf8-3772-4b4a-ab9f-6ebe93b90a94",
			Action: action{
				TaskName: "os-install",
				Name:     "start",
				Image:    "alpine",
				Command: []string{
					"/bin/sh",
					"sleep",
					"1",
				},
				WorkerID: "0eba0bf8-3772-4b4a-ab9f-6ebe93b90a94",
			},
			Err: nil,
		},
	}

	/*
		type controllerStop struct {
			Level   string  `json:"level"`
			Ts      float64 `json:"ts"`
			Caller  string  `json:"caller"`
			Msg     string  `json:"msg"`
			Service string  `json:"service"`
		}
	*/

	capturedOutput := captureOutput(func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		log, _, _ := logr.NewPacketLogr()
		var controllerWg sync.WaitGroup
		identifier := "192.168.1.12"

		stringReader := strings.NewReader("{\"error\":\"\"}")
		dockerClient := mockClient{
			mock: clientMockHelper{
				stringReadCloser: io.NopCloser(stringReader),
				mockContainerCreate: mockContainerCreate{
					createID: "12345",
				},
				mockContainerStart: mockContainerStart{
					startErr: nil,
				},
				mockContainerInspect: mockContainerInspect{
					inspectID: "12345",
					inspectState: &types.ContainerState{
						Status:   "exited",
						ExitCode: 0,
					},
				},
			},
		}
		mock := &hardwareServerMock{}
		hardwareClient := mock.getMockedHardwareServiceClient()
		mockr := mocker{numWorkflowsToMock: 1, numContextsToMock: 1}
		workflowClient := mockr.getMockedWorkflowServiceClient()

		controllerWg.Add(1)
		go Reconciler(ctx, log, identifier, map[string]string{"": ""}, &dockerClient, workflowClient, hardwareClient, &controllerWg)
		time.Sleep(5 * time.Second)
		cancel()
		<-ctx.Done()
		controllerWg.Wait()
	})

	var capturedOutputs []actionOutput
	for _, elem := range capturedOutput {
		var capturedOutputStruct actionOutput
		err := json.Unmarshal([]byte(elem), &capturedOutputStruct)
		if err == nil {
			if capturedOutputStruct.WorkerID != "" {
				capturedOutputs = append(capturedOutputs, capturedOutputStruct)
			}
		}
	}

	for index, elem := range capturedOutputs {
		if diff := cmp.Diff(elem, expectedOutput[index], cmpopts.IgnoreFields(actionOutput{}, "Ts", "Caller")); diff != "" {
			t.Fatal(diff)
		}
	}
}

type hardwareServerMock struct{}

func (h *hardwareServerMock) getMockedHardwareServiceClient() *hardware.HardwareServiceClientMock {
	var hardwareSvcClient hardware.HardwareServiceClientMock

	hardwareSvcClient.ByMACFunc = func(ctx context.Context, in *hardware.GetRequest, opts ...grpc.CallOption) (*hardware.Hardware, error) {
		return &hardware.Hardware{Id: "0eba0bf8-3772-4b4a-ab9f-6ebe93b90a94"}, nil
	}

	hardwareSvcClient.ByIPFunc = func(ctx context.Context, in *hardware.GetRequest, opts ...grpc.CallOption) (*hardware.Hardware, error) {
		return &hardware.Hardware{Id: "0eba0bf8-3772-4b4a-ab9f-6ebe93b90a94"}, nil
	}

	return &hardwareSvcClient
}

type mocker struct {
	failListWorkflows         bool
	failListWorkflowsRecvFunc bool
	failGetWorkflowActions    bool
	failGetWorkflowContexts   bool
	numWorkflowsToMock        int
	numContextsToMock         int
	start                     int
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
	workflowSvcClient.ReportActionStatusFunc = func(ctx context.Context, in *workflow.WorkflowActionStatus, opts ...grpc.CallOption) (*workflow.Empty, error) {
		return &workflow.Empty{}, nil
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
					Command:  []string{"/bin/sh", "sleep", "1"},
					WorkerId: "0eba0bf8-3772-4b4a-ab9f-6ebe93b90a94",
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
				WorkflowId:           "0eba0bf8-3772-4b4a-ab9f-6ebe93b90a94",
				CurrentWorker:        "",
				CurrentTask:          "",
				CurrentAction:        "",
				CurrentActionIndex:   0,
				CurrentActionState:   0,
				TotalNumberOfActions: 0,
			}, nil
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
