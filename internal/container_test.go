package internal

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/google/go-cmp/cmp"
	"github.com/packethost/pkg/log/logr"
	"github.com/pkg/errors"
)

func TestActualPull(t *testing.T) {
	//t.Skip()
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

func TestRunContainer(t *testing.T) {
	t.Skip()
	cl, err := client.NewClientWithOpts()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	log, _, _ := logr.NewPacketLogr()
	conf := &container.Config{
		Image:        "nginx",
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

type testClient struct {
	client.ImageAPIClient
	mock clientMockHelper
}

type clientMockHelper struct {
	stringReadCloser io.ReadCloser
	imagePullErr     error
}

func (t *testClient) ImagePull(ctx context.Context, ref string, options types.ImagePullOptions) (io.ReadCloser, error) {
	return t.mock.stringReadCloser, t.mock.imagePullErr
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
			t.Log(tc.testImagePullErr)

			stringReader := strings.NewReader(tc.testString)
			helper := clientMockHelper{
				stringReadCloser: io.NopCloser(stringReader),
				imagePullErr:     tc.testImagePullErr,
			}
			mockClient := testClient{
				mock: helper,
			}
			ctx := context.Background()
			err := pullImage(ctx, &mockClient, "something", types.ImagePullOptions{})
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
