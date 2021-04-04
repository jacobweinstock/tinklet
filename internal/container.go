// build +linux
package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/tinkerbell/tink/protos/workflow"
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
func createContainer(ctx context.Context, log logr.Logger, dockerClient *client.Client, containerName string, containerConfig *container.Config, hostConfig *container.HostConfig) (id string, err error) {
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
func containerWaiter(ctx context.Context, dockerClient *client.Client, timeout time.Duration, containerID string) error {
	timer := time.NewTimer(timeout)
LOOP:
	for {
		select {
		case <-timer.C:
			return &timeoutError{timeoutValue: timeout}
		default:
			details, err := dockerClient.ContainerInspect(ctx, containerID)
			if err != nil {
				return errors.Wrap(&actionFailedError{msg: "unable to inspect container"}, err.Error())
			}
			// container execution completed successfully
			if details.State.Status == "exited" && details.State.ExitCode == 0 {
				break LOOP
			}
			// container execution completed unsuccessfully
			if details.State.Status != "running" && details.State.ExitCode != 0 {
				stdout, err := dockerClient.ContainerLogs(ctx, containerID, types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
				if err != nil {
					return &actionFailedError{
						exitCode: details.State.ExitCode,
						details:  details.State.Error,
						msg:      fmt.Sprintf("container execution was unsuccessful: %v", err.Error()),
					}
				}
				defer stdout.Close()
				buf := new(bytes.Buffer)
				_, _ = buf.ReadFrom(stdout)
				newStr := buf.String()
				return &actionFailedError{
					stdout:   newStr,
					exitCode: details.State.ExitCode,
					details:  details.State.Error,
					msg:      "container execution was unsuccessful",
				}
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
// TODO: possible remove workflow.WorkflowAction, make this generic. pass in image, container name, container and host configs, and timeout
func actionExecutionFlow(ctx context.Context, log logr.Logger, dockerClient *client.Client, action workflow.WorkflowAction, pullOpts types.ImagePullOptions, workflowID string) error { // nolint
	// 1. Pull the image
	err := pullImage(ctx, dockerClient, action.Image, pullOpts)
	if err != nil {
		return errors.Wrap(&actionFailedError{msg: "image pull failed"}, err.Error())
	}
	// 2. create container
	containerID, err := createContainer(
		ctx,
		log,
		dockerClient,
		// spaces in a container name are not valid,
		// we also add a timestamp so the container name is always unique.
		fmt.Sprintf("%v-%v", strings.ReplaceAll(action.Name, " ", "-"), time.Now().UnixNano()),
		actionToDockerContainerConfig(ctx, action), // nolint
		actionToDockerHostConfig(ctx, action),      // nolint
	)
	if err != nil {
		return errors.Wrap(&actionFailedError{msg: "creating container failed"}, err.Error())
	}
	// 3. Removal of container is go "deferred"
	defer dockerClient.ContainerRemove(ctx, containerID, types.ContainerRemoveOptions{Force: true}) // nolint
	// 4. Start container
	err = dockerClient.ContainerStart(ctx, containerID, types.ContainerStartOptions{})
	if err != nil {
		return errors.Wrap(&actionFailedError{msg: "starting container failed"}, err.Error())
	}
	// 5. Wait and watch for container exit status or timeout
	err = containerWaiter(ctx, dockerClient, (time.Duration(action.Timeout) * time.Second), containerID)
	if err != nil {
		return errors.Wrap(&actionFailedError{msg: "waiting for container failed"}, err.Error())
	}

	return nil
}
