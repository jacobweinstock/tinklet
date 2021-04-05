// build +linux
package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
)

// pullImage is what you would expect from a `docker pull` cli command
// pulls an image from a remote registry
func pullImage(ctx context.Context, cli client.ImageAPIClient, image string, pullOpts types.ImagePullOptions) error {
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

// createContainer creates a container that is not started
func createContainer(ctx context.Context, log logr.Logger, dockerClient client.ContainerAPIClient, containerName string, containerConfig *container.Config, hostConfig *container.HostConfig) (id string, err error) {
	resp, err := dockerClient.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, containerName)
	if err != nil {
		return "", err
	}
	if len(resp.Warnings) > 0 {
		log.V(0).Info("creating container resulting in the following warnings", "warnings", resp.Warnings)
	}
	return resp.ID, nil
}

// containerWaiter watches a container waiting for timeout or container not running (get exit status)
func containerWaiter(ctx context.Context, dockerClient client.ContainerAPIClient, timeout time.Duration, containerID string) error {
	timer := time.NewTimer(timeout)
LOOP:
	for {
		select {
		case <-timer.C:
			return &timeoutError{timeoutValue: timeout}
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

// PCSRW Process
// =============
// 1. Pull the image
// 2. Create the container
// 3. Start the container
// 4. Removal of container is go "deferred"
// 5. Wait and watch for container exit status or timeout
func executionFlow(ctx context.Context, log logr.Logger, dockerClient client.CommonAPIClient, imageName string, pullOpts types.ImagePullOptions, containerConfig *container.Config, hostConfig *container.HostConfig, containerName string, timeout time.Duration) error {
	// 1. Pull the image
	err := pullImage(ctx, dockerClient, imageName, pullOpts)
	if err != nil {
		return errors.Wrap(&executionError{msg: "image pull failed"}, err.Error())
	}
	// 2. create container
	containerID, err := createContainer(ctx, log, dockerClient, containerName, containerConfig, hostConfig)
	if err != nil {
		return errors.Wrap(&executionError{msg: "creating container failed"}, err.Error())
	}
	// 3. Removal of container is go "deferred"
	defer dockerClient.ContainerRemove(ctx, containerID, types.ContainerRemoveOptions{Force: true}) // nolint
	// 4. Start container
	err = dockerClient.ContainerStart(ctx, containerID, types.ContainerStartOptions{})
	if err != nil {
		return errors.Wrap(&executionError{msg: "starting container failed"}, err.Error())
	}
	// 5. Wait and watch for container exit status or timeout
	err = containerWaiter(ctx, dockerClient, timeout, containerID)
	if err != nil {
		return errors.Wrap(&executionError{msg: "waiting for container failed"}, err.Error())
	}

	return nil
}
