package internal

import (
	"context"

	"github.com/docker/docker/api/types/container"
	"github.com/tinkerbell/tink/protos/workflow"
)

// containerConfigOption allows modifying the container config defaults
type containerConfigOption func(*container.Config)

// containerHostOption allows modifying the container host config defaults
type containerHostOption func(*container.HostConfig)

// actionToDockerContainerConfig takes a workflowAction and translates it to a docker container config
func actionToDockerContainerConfig(ctx context.Context, workflowAction workflow.WorkflowAction, opts ...containerConfigOption) *container.Config { // nolint
	defaultConfig := &container.Config{
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
		Env:          workflowAction.Environment,
		Cmd:          workflowAction.Command,
		Image:        workflowAction.Image,
	}
	for _, opt := range opts {
		opt(defaultConfig)
	}
	return defaultConfig
}

func actionToDockerHostConfig(ctx context.Context, workflowAction workflow.WorkflowAction, opts ...containerHostOption) *container.HostConfig { // nolint
	defaultConfig := &container.HostConfig{
		Binds:      workflowAction.Volumes,
		PidMode:    container.PidMode(workflowAction.Pid),
		Privileged: true,
	}
	for _, opt := range opts {
		opt(defaultConfig)
	}
	return defaultConfig
}
