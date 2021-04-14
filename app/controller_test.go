package app

import (
	"bufio"
	"context"
	"encoding/json"
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
)

func TestReportActionStatusController(t *testing.T) {

	type output struct {
		Level        string  `json:"level"`
		Ts           float64 `json:"ts"`
		Caller       string  `json:"caller"`
		Msg          string  `json:"msg"`
		Service      string  `json:"service"`
		Error        string  `json:"error"`
		ErrorVerbose string  `json:"errorVerbose"`
		Stacktrace   string  `json:"stacktrace"`
	}
	expectedOutput := []output{
		{
			Level:   "error",
			Caller:  "app/controller.go:38",
			Msg:     "reporting action status failed",
			Service: "not/set",
			Error:   "test error 3",
		},
		{
			Level:   "info",
			Caller:  "app/controller.go:30",
			Msg:     "stopping report action status controller",
			Service: "not/set",
		},
	}
	capturedOutput := captureOutput(func() {
		ctx, cancel := context.WithCancel(context.Background())
		log, _, _ := logr.NewPacketLogr()
		var wg sync.WaitGroup
		var doneWg sync.WaitGroup
		rasChan := make(chan func() error)
		doneWg.Add(1)
		go ReportActionStatusController(ctx, log, &wg, rasChan, &doneWg)
		wg.Add(1)
		rasChan <- func() error {
			return nil
		}
		wg.Add(1)
		rasChan <- func() error {
			return nil
		}
		wg.Add(1)
		rasChan <- func() error {
			return errors.New("test error 3")
		}
		wg.Wait()
		cancel()
		doneWg.Wait()
	})
	var capturedOutputs []output

	for _, elem := range capturedOutput {
		var capturedOutputStruct output
		err := json.Unmarshal([]byte(elem), &capturedOutputStruct)
		if err != nil {
			t.Log(elem)
			t.Log(capturedOutput)
			t.Fatal(err)
		}
		capturedOutputs = append(capturedOutputs, capturedOutputStruct)
	}

	for index, elem := range capturedOutputs {
		if diff := cmp.Diff(elem, expectedOutput[index], cmpopts.IgnoreFields(output{}, "Ts", "ErrorVerbose", "Stacktrace")); diff != "" {
			t.Fatal(diff)
		}
	}

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
