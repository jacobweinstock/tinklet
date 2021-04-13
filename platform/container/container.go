// build +linux
package container

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/docker/docker/api/types"
	tainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/jacobweinstock/tinklet/platform"
	"github.com/pkg/errors"
)

// PullImage is what you would expect from a `docker pull` cli command
// pulls an image from a remote registry
func PullImage(ctx context.Context, cli client.ImageAPIClient, image string, pullOpts types.ImagePullOptions) error {
	out, err := cli.ImagePull(ctx, image, pullOpts)
	if err != nil {
		return errors.Wrapf(err, "error pulling image: %v", image)
	}
	defer out.Close()
	fd := json.NewDecoder(out)
	var imagePullStatus struct {
		Error string `json:"error"`
	}
	for {
		if err := fd.Decode(&imagePullStatus); err != nil {
			if err == io.EOF {
				break
			}
			return errors.Wrapf(err, "error pulling image: %v", image)
		}
		if imagePullStatus.Error != "" {
			return errors.Wrapf(errors.New(imagePullStatus.Error), "error pulling image: %v", image)
		}
	}
	return nil
}

// CreateContainer creates a container that is not started
func CreateContainer(ctx context.Context, dockerClient client.ContainerAPIClient, containerName string, containerConfig *tainer.Config, hostConfig *tainer.HostConfig) (id string, err error) {
	resp, err := dockerClient.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, containerName)
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

// ContainerWaiter watches a container waiting for timeout or container not running (get exit status)
func ContainerWaiter(ctx context.Context, dockerClient client.ContainerAPIClient, timeout time.Duration, containerID string) error {
	timer := time.NewTimer(timeout)
LOOP:
	for {
		select {
		case <-timer.C:
			return &platform.TimeoutError{TimeoutValue: timeout}
		default:
			details, err := dockerClient.ContainerInspect(ctx, containerID)
			if err != nil {
				return errors.Wrap(err, "unable to inspect container")
			}
			// container execution completed successfully
			if details.State.Status == "exited" && details.State.ExitCode == 0 {
				break LOOP
			}
			// container execution completed unsuccessfully
			if details.State.Status != "running" && details.State.ExitCode != 0 {
				stdout, err := dockerClient.ContainerLogs(ctx, containerID, types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
				if err != nil {
					return err
					//return errors.Wrapf(err, "container execution was unsuccessful; exit code: %v; state err: %v; logs err: %v", details.State.ExitCode, details.State.Error, err.Error())
				}
				defer stdout.Close()
				buf := new(bytes.Buffer)
				// TODO: handle error? or just ignore?
				_, _ = buf.ReadFrom(stdout)
				newStr := buf.String()
				return fmt.Errorf("msg: container execution was unsuccessful; stdout: %v;  exitCode: %v; details: %v", newStr, details.State.ExitCode, details.State.Error)
			}
		}
	}
	return nil
}
