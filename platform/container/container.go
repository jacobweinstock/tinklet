// build +linux
package container

import (
	"bytes"
	"context"
	"encoding/json"
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

func ContainerGetLogs(ctx context.Context, dockerClient client.ContainerAPIClient, containerID string, options types.ContainerLogsOptions) (string, error) {
	reader, err := dockerClient.ContainerLogs(ctx, containerID, options)
	if err != nil {
		return "", err
	}
	defer reader.Close()
	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(reader)
	stdout := buf.String()
	return stdout, err
}

// ContainerExecComplete checks if a container run has completed or not. completed is defined as having an "exited" or "dead" status.
// see types.ContainerJSON.State.Status for all status options
func ContainerExecComplete(ctx context.Context, dockerClient client.ContainerAPIClient, containerID string) (complete bool, details types.ContainerJSON, err error) {
	detail, err := dockerClient.ContainerInspect(ctx, containerID)
	if err != nil {
		return false, types.ContainerJSON{}, errors.Wrap(err, "unable to inspect container")
	}

	if detail.State.Status == "exited" || detail.State.Status == "dead" {
		return true, detail, nil
	}
	return false, types.ContainerJSON{}, nil
}
