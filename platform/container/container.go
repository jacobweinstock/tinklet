// build +linux
package container

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/docker/docker/api/types"
	tainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
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

// ContainerRunSuccessful will return nil if it completed successfully. will return an error with context for non successful container runs.
// Assumes a container is no longer running. Use ContainerRunComplete to check for running status
func ContainerRunSuccessful(ctx context.Context, dockerClient client.ContainerAPIClient, containerDetail types.ContainerJSON, containerID string) error {
	// container execution completed successfully
	if containerDetail.State.ExitCode == 0 {
		return nil
	} else {
		stdout, err := dockerClient.ContainerLogs(ctx, containerID, types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
		if err != nil {
			return err
			//return false, errors.Wrapf(err, "container execution was unsuccessful; exit code: %v; state err: %v; logs err: %v", details.State.ExitCode, details.State.Error, err.Error())
		}
		defer stdout.Close()
		buf := new(bytes.Buffer)
		// TODO: handle error? or keep ignoring?
		_, _ = buf.ReadFrom(stdout)
		newStr := buf.String()
		return fmt.Errorf("msg: container execution was unsuccessful; stdout: %v;  exitCode: %v; details: %v", newStr, containerDetail.State.ExitCode, containerDetail.State.Error)
	}
}

// ContainerRunComplete checks if a container run has completed or not
func ContainerRunComplete(ctx context.Context, dockerClient client.ContainerAPIClient, containerID string) (complete bool, details types.ContainerJSON, err error) {
	detail, err := dockerClient.ContainerInspect(ctx, containerID)
	if err != nil {
		return false, types.ContainerJSON{}, errors.Wrap(err, "unable to inspect container")
	}

	if detail.State.Status == "exited" || detail.State.Status == "dead" {
		return true, detail, nil
	}
	return false, types.ContainerJSON{}, nil
}
