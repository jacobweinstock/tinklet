package internal

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/google/go-cmp/cmp"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/packethost/pkg/log/logr"
	"github.com/pkg/errors"
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

func (t *mockClient) ImagePull(ctx context.Context, ref string, options types.ImagePullOptions) (io.ReadCloser, error) {
	return t.mock.stringReadCloser, t.mock.imagePullErr
}

func (t *mockClient) ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *specs.Platform, containerName string) (container.ContainerCreateCreatedBody, error) {
	return container.ContainerCreateCreatedBody{ID: t.mock.createID, Warnings: t.mock.createWarnings}, t.mock.createErr
}

func (t *mockClient) ContainerInspect(ctx context.Context, container string) (types.ContainerJSON, error) {
	return types.ContainerJSON{ContainerJSONBase: &types.ContainerJSONBase{ID: t.mock.inspectID, State: t.mock.inspectState}}, t.mock.inspectErr
}

func (t *mockClient) ContainerLogs(ctx context.Context, container string, options types.ContainerLogsOptions) (io.ReadCloser, error) {
	return t.mock.logsReadCloser, t.mock.logsErr
}

func TestActualPull(t *testing.T) {
	t.Skip()
	cl, err := client.NewClientWithOpts()
	if err != nil {
		t.Fatal(err)
	}

	imageName := "alpine:latest"
	var pullOpts types.ImagePullOptions

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	err = pullImage(ctx, cl, imageName, pullOpts)
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
	log, _, _ := logr.NewPacketLogr()
	conf := &container.Config{
		Image:        "alpine",
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
	}
	hostConf := &container.HostConfig{
		Privileged: true,
	}
	id, err := createContainer(ctx, log, cl, "jacob-test", conf, hostConf)
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
			id, err := createContainer(context.Background(), nil, &mClient, "testing", nil, nil)
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

func TestContainerWait(t *testing.T) {

	tests := map[string]struct {
		expectedContainerID string
		expectedWarnings    []string
		expectedInspectErr  error
		expectedLogsErr     error
		expectedStdout      string
		expectedState       *types.ContainerState
		timeout             time.Duration
	}{
		"success": {expectedContainerID: "12345", expectedState: &types.ContainerState{Status: "exited", ExitCode: 0}},
		"error":   {expectedContainerID: "12345", expectedState: &types.ContainerState{Status: "exited", ExitCode: 127}, expectedStdout: "error running command", expectedLogsErr: errors.New("container execution was unsuccessful; exit code: 127; state err: ; logs err: test: test")},
		//"error: timeout": {expectedContainerID: "12345", expectedInspectErr: errors.New("context timeout"), timeout: time.Nanosecond},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			stringReader := strings.NewReader(tc.expectedStdout)
			helper := clientMockHelper{
				mockContainerInspect: mockContainerInspect{
					inspectErr:   nil,
					inspectID:    tc.expectedContainerID,
					inspectState: tc.expectedState,
				},
				mockContainerLogs: mockContainerLogs{
					logsReadCloser: io.NopCloser(stringReader),
					logsErr:        tc.expectedLogsErr,
				},
			}
			mClient := mockClient{mock: helper}
			err := containerWaiter(context.Background(), &mClient, time.Duration(2), tc.expectedContainerID)
			if err != nil {
				if tc.expectedLogsErr != nil {
					if diff := cmp.Diff(err.Error(), tc.expectedLogsErr.Error()); diff != "" {
						t.Log(err.Error())
						t.Log(tc.expectedLogsErr.Error())
						t.Fatal(diff)
					}
				} else {
					t.Fatal(err)
				}
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
			testString:       "{\"status\": \"hello\",\"error\":\"\"}{\"status\":\"world\",\"error\":\"\"}",
			testImagePullErr: nil,
			testErr:          nil,
		},
		"fail": {
			testString:       "{\"error\": \"\"}",
			testImagePullErr: errors.New("Tested, failure of the image pull"),
			testErr:          errors.New("error pulling image: something: Tested, failure of the image pull"),
		},
		"fail_partial": {
			testString:       "{\"status\": \"hello\",\"error\":\"\"}{\"status\":\"world\",\"error\":\"Tested, failure of No space left on device\"}",
			testImagePullErr: nil,
			testErr:          errors.New("error pulling image: something: Tested, failure of No space left on device"),
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
			err := pullImage(ctx, &mClient, "something", types.ImagePullOptions{})
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
