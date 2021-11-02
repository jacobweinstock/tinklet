package docker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	tainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/google/go-cmp/cmp"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	"github.com/tinkerbell/tink/protos/workflow"
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
	mockContainerInspect
	mockContainerLogs
	mockContainerStart
}

type mockContainerCreate struct {
	createErr      error
	createID       string
	createWarnings []string
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

type mockContainerStart struct {
	startErr error
}

func (t *mockClient) ImagePull(ctx context.Context, ref string, options types.ImagePullOptions) (io.ReadCloser, error) {
	return t.mock.stringReadCloser, t.mock.imagePullErr
}

func (t *mockClient) ContainerCreate(ctx context.Context, config *tainer.Config, hostConfig *tainer.HostConfig, networkingConfig *network.NetworkingConfig, platform *specs.Platform, containerName string) (tainer.ContainerCreateCreatedBody, error) {
	return tainer.ContainerCreateCreatedBody{ID: t.mock.createID, Warnings: t.mock.createWarnings}, t.mock.createErr
}

func (t *mockClient) ContainerInspect(ctx context.Context, container string) (types.ContainerJSON, error) {
	return types.ContainerJSON{ContainerJSONBase: &types.ContainerJSONBase{ID: t.mock.inspectID, State: t.mock.inspectState}}, t.mock.inspectErr
}

func (t *mockClient) ContainerLogs(ctx context.Context, container string, options types.ContainerLogsOptions) (io.ReadCloser, error) {
	return t.mock.logsReadCloser, t.mock.logsErr
}

func (t *mockClient) ContainerStart(ctx context.Context, container string, options types.ContainerStartOptions) error {
	return t.mock.startErr
}

func TestActualPull(t *testing.T) {
	t.Skip()

	cl, err := client.NewClientWithOpts()
	if err != nil {
		t.Fatal(err)
	}

	imageName := "alpine:latest"
	authConfig := types.AuthConfig{
		Username: "admin",
		Password: "password",
	}
	encodedJSON, err := json.Marshal(authConfig)
	if err != nil {
		t.Fatal(errors.Wrap(err, "DOCKER AUTH"))
	}
	authStr := base64.URLEncoding.EncodeToString(encodedJSON)
	pullOpts := types.ImagePullOptions{RegistryAuth: authStr}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	cli := Client{Conn: cl}
	err = cli.pullImage(ctx, imageName, pullOpts)
	if err != nil {
		t.Fatal(err)
	}
}

func TestActualCreateContainer(t *testing.T) {
	t.Skip()
	cl, err := client.NewClientWithOpts()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	conf := &tainer.Config{
		Image:        "alpine",
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
	}
	hostConf := &tainer.HostConfig{
		Privileged: true,
	}
	cli := Client{Conn: cl}
	id, err := cli.createContainer(ctx, "jacob-test", conf, hostConf)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(id)
	t.Fatal()
}

func TestCreateContainer(t *testing.T) {
	tests := map[string]struct {
		expectedContainerID string
		expectedWarnings    []string
		expectedErr         error
	}{
		"success": {expectedContainerID: "12345"},
		"error":   {expectedErr: errors.New("failed to create container")},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			helper := clientMockHelper{
				mockContainerCreate: mockContainerCreate{
					createID:       tc.expectedContainerID,
					createErr:      tc.expectedErr,
					createWarnings: tc.expectedWarnings,
				},
			}
			mClient := mockClient{mock: helper}
			cli := Client{Conn: &mClient}
			id, err := cli.createContainer(context.Background(), "testing", nil, nil)
			if err != nil {
				if tc.expectedErr != nil {
					if diff := cmp.Diff(err.Error(), tc.expectedErr.Error()); diff != "" {
						t.Fatal(diff)
					}
				} else {
					t.Fatal(err)
				}
			}
			if diff := cmp.Diff(id, tc.expectedContainerID); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestContainerGetLogs(t *testing.T) {
	tests := map[string]struct {
		expectedContainerID string
		expectedStdout      string
		expectedErr         error
		expectedState       *types.ContainerState
		containerComplete   bool
	}{
		"success": {expectedContainerID: "12345", expectedState: &types.ContainerState{Status: "exited", ExitCode: 0}, expectedStdout: "hello"},
		"failed":  {expectedContainerID: "12345", expectedState: &types.ContainerState{Status: "exited", ExitCode: 0}, expectedErr: errors.New("failed to get container logs")},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			stringReader := strings.NewReader(tc.expectedStdout)
			helper := clientMockHelper{
				mockContainerLogs: mockContainerLogs{
					logsReadCloser: io.NopCloser(stringReader),
					logsErr:        tc.expectedErr,
				},
			}
			mClient := mockClient{mock: helper}
			cli := Client{Conn: &mClient}
			logs, err := cli.containerGetLogs(context.Background(), tc.expectedContainerID, types.ContainerLogsOptions{ShowStdout: true, ShowStderr: false})
			if err != nil {
				if tc.expectedErr != nil {
					if diff := cmp.Diff(err.Error(), tc.expectedErr.Error()); diff != "" {
						t.Fatal(diff)
					}
				} else {
					t.Fatal(err)
				}
			}
			if diff := cmp.Diff(logs, tc.expectedStdout); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestContainerRunComplete(t *testing.T) {
	tests := map[string]struct {
		expectedContainerID             string
		expectedInspectErr              error
		expectedContainerRunCompleteErr error
		expectedState                   *types.ContainerState
		containerComplete               bool
	}{
		"success":                    {expectedContainerID: "12345", expectedState: &types.ContainerState{Status: "exited", ExitCode: 0}, containerComplete: true},
		"error inspecting container": {expectedContainerID: "12345", expectedState: &types.ContainerState{Status: "exited", ExitCode: 127}, containerComplete: false, expectedInspectErr: errors.New("unable to inspect container"), expectedContainerRunCompleteErr: errors.Wrap(errors.New("unable to inspect container"), "unable to inspect container")},
		"container not complete":     {expectedContainerID: "12345", expectedState: &types.ContainerState{Status: "running", ExitCode: 0}, containerComplete: false},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			helper := clientMockHelper{
				mockContainerInspect: mockContainerInspect{
					inspectErr:   tc.expectedInspectErr,
					inspectID:    tc.expectedContainerID,
					inspectState: tc.expectedState,
				},
			}
			mClient := mockClient{mock: helper}
			cli := Client{Conn: &mClient}
			complete, _, err := cli.containerExecComplete(context.Background(), tc.expectedContainerID)
			if err != nil {
				if tc.expectedContainerRunCompleteErr != nil {
					if diff := cmp.Diff(err.Error(), tc.expectedContainerRunCompleteErr.Error()); diff != "" {
						t.Fatal(diff)
					}
				} else {
					t.Fatal(err)
				}
			}
			if diff := cmp.Diff(complete, tc.containerComplete); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestPullImage(t *testing.T) {
	tests := map[string]struct {
		testName         string
		testString       string
		testImagePullErr error
		testErr          error
	}{
		"success": {
			testString:       `{"status":"hello","error":""}{"status":"world","error":""}`,
			testImagePullErr: nil,
			testErr:          nil,
		},
		"fail": {
			testString:       `{"error": ""}`,
			testImagePullErr: errors.New("tested, failure of the image pull"),
			testErr:          errors.New("error pulling image: something: tested, failure of the image pull"),
		},
		"fail_partial": {
			testString:       `{"status": "hello","error":""}{"status":"world","error":"tested, failure of No space left on device"}`,
			testImagePullErr: nil,
			testErr:          errors.New("error pulling image: something: tested, failure of No space left on device"),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			stringReader := strings.NewReader(tc.testString)
			helper := clientMockHelper{
				stringReadCloser: io.NopCloser(stringReader),
				imagePullErr:     tc.testImagePullErr,
			}
			mClient := mockClient{mock: helper}
			ctx := context.Background()
			cli := Client{Conn: &mClient}
			err := cli.pullImage(ctx, "something", types.ImagePullOptions{})
			if err != nil {
				if tc.testErr != nil {
					if diff := cmp.Diff(err.Error(), tc.testErr.Error()); diff != "" {
						t.Fatal(diff)
					}
				}
			}
		})
	}
}

func TestGetRegistryAuth(t *testing.T) {
	type args struct {
		regAuth   map[string]string
		imageName string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{name: "auth found", args: args{regAuth: map[string]string{"registry.com": "auth"}, imageName: "registry.com/image"}, want: "auth"},
		{name: "auth not found", args: args{regAuth: map[string]string{"registry.com": "auth"}, imageName: "registry2.com/image:tag"}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getRegistryAuth(tt.args.regAuth, tt.args.imageName); got != tt.want {
				t.Errorf("getRegistryAuth() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPrepareEnv(t *testing.T) {
	c := &Client{}
	if err := c.PrepareEnv(context.Background(), "n/a"); err != nil {
		t.Fatalf("error should be nil, got: %v", err)
	}
}

func TestCleanEnv(t *testing.T) {
	c := &Client{}
	if err := c.CleanEnv(context.Background()); err != nil {
		t.Fatalf("error should be nil, got: %v", err)
	}
}

func TestSetActionData(t *testing.T) {
	c := &Client{}
	a := &workflow.WorkflowAction{TaskName: "mything"}
	wID := "1234"
	c.SetActionData(context.Background(), wID, a)
	if c.action != a {
		t.Fatalf("action should be: %v", a)
	}
	if c.workflowID != wID {
		t.Fatalf("workflowID should be: %v", wID)
	}
}

func TestPrepare(t *testing.T) {
	t.Skip("not implemented")
	helper := clientMockHelper{
		imagePullErr:     nil,
		stringReadCloser: io.NopCloser(strings.NewReader(`{"status":"hello","error":""}{"status":"world","error":""}`)),
		mockContainerCreate: mockContainerCreate{
			createID:       "1234",
			createErr:      nil,
			createWarnings: []string{},
		},
	}
	mClient := mockClient{mock: helper}
	c := &Client{Conn: &mClient, action: &workflow.WorkflowAction{Name: "mything"}}
	id := "test"
	i, err := c.Prepare(context.Background(), id)
	if err != nil {
		t.Fatalf("error should be nil, got: %v", err)
	}
	if i != "1234" {
		t.Fatalf("containerID should be: %v", "1234")
	}
}

func TestClient_Prepare(t *testing.T) {
	tests := []struct {
		name      string
		createID  string
		createErr error
		pullErr   error
	}{
		{name: "success", createID: "1234", createErr: nil, pullErr: nil},
		{name: "fail", createID: "", createErr: errors.New("create error"), pullErr: nil},
		{name: "fail_pull", createID: "", createErr: nil, pullErr: errors.New("pull error")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helper := clientMockHelper{
				imagePullErr:     tt.pullErr,
				stringReadCloser: io.NopCloser(strings.NewReader(`{"status":"hello","error":""}{"status":"world","error":""}`)),
				mockContainerCreate: mockContainerCreate{
					createID:       tt.createID,
					createErr:      tt.createErr,
					createWarnings: []string{},
				},
			}
			mClient := mockClient{mock: helper}
			c := &Client{Conn: &mClient, action: &workflow.WorkflowAction{Name: "mything"}}
			i, err := c.Prepare(context.Background(), "test")
			if err != nil {
				if tt.createErr != nil {
					if diff := cmp.Diff(err.Error(), tt.createErr.Error()); diff != "" {
						t.Fatal(diff)
					}
				} else if tt.pullErr != nil {
					if diff := cmp.Diff(err.Error(), fmt.Errorf("error pulling image: test: %v", tt.pullErr.Error()).Error()); diff != "" {
						t.Fatal(diff)
					}
				} else {
					t.Fatalf("error should be nil, got: %v", err)
				}
			}
			if i != tt.createID {
				t.Fatalf("containerID should be: %v", "1234")
			}
		})
	}
}

func TestClient_Run(t *testing.T) {
	tests := []struct {
		name       string
		startErr   error
		timeoutErr error
	}{
		{name: "container start error", startErr: fmt.Errorf("could not start container")},
		{name: "timeout err", startErr: nil, timeoutErr: fmt.Errorf("timeout reached: 2s")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helper := clientMockHelper{
				mockContainerStart: mockContainerStart{
					startErr: tt.startErr,
				},
				mockContainerInspect: mockContainerInspect{
					inspectErr: nil,
					inspectID:  "1234",
					inspectState: &types.ContainerState{
						Status: "running",
					},
				},
			}
			mClient := mockClient{mock: helper}
			c := &Client{Conn: &mClient, action: &workflow.WorkflowAction{Name: "mything", Timeout: 2}}
			err := c.Run(context.Background(), "12345")
			if err != nil {
				if tt.startErr != nil {
					if diff := cmp.Diff(err.Error(), tt.startErr.Error()); diff != "" {
						t.Fatal(diff)
					}
				} else if tt.timeoutErr != nil {
					if diff := cmp.Diff(err.Error(), tt.timeoutErr.Error()); diff != "" {
						t.Fatal(diff)
					}
				} else {
					t.Fatalf("error should be nil, got: %v", err)
				}
			}
		})
	}
}
