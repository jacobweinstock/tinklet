package app

import (
	"context"
	"time"

	"github.com/docker/docker/api/types"
	tainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/go-logr/logr"
	"github.com/jacobweinstock/tinklet/platform"
	"github.com/jacobweinstock/tinklet/platform/container"
	"github.com/pkg/errors"
)

// business/domain logic for executing an action
// =============================================
// 1. Pull the image
// 2. Create the container
// 3. Start the container
// 4. Removal of container is go "deferred"
// 5. Wait and watch for container exit status or timeout
func ActionExecutionFlow(ctx context.Context, log logr.Logger, dockerClient client.CommonAPIClient, imageName string, pullOpts types.ImagePullOptions, containerConfig *tainer.Config, hostConfig *tainer.HostConfig, containerName string, timeout time.Duration) error {
	// 1. Pull the image
	err := container.PullImage(ctx, dockerClient, imageName, pullOpts)
	if err != nil {
		return errors.Wrap(&platform.ExecutionError{Msg: "image pull failed"}, err.Error())
	}
	// 2. create container
	containerID, err := container.CreateContainer(ctx, dockerClient, containerName, containerConfig, hostConfig)
	if err != nil {
		return errors.Wrap(&platform.ExecutionError{Msg: "creating container failed"}, err.Error())
	}
	// 3. Removal of container is go "deferred"
	defer dockerClient.ContainerRemove(ctx, containerID, types.ContainerRemoveOptions{Force: true}) // nolint
	// 4. Start container
	err = dockerClient.ContainerStart(ctx, containerID, types.ContainerStartOptions{})
	if err != nil {
		return errors.Wrap(&platform.ExecutionError{Msg: "starting container failed"}, err.Error())
	}
	// 5. Wait and watch for container exit status or timeout
	err = container.ContainerWaiter(ctx, dockerClient, timeout, containerID)
	if err != nil {
		return errors.Wrap(&platform.ExecutionError{Msg: "waiting for container failed"}, err.Error())
	}

	return nil
}
