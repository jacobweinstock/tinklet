// build +linux
package internal

import (
	"context"
	"encoding/json"
	"io"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
)

// pullImage is what you would expect from a `docker pull` cli command
// pulls an image from a remote registry
func pullImage(ctx context.Context, cli *client.Client, image string, pullOpts types.ImagePullOptions) error {
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
