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

// executeAction takes a workflow->template->task->action spec and executes the container image
func executeAction(ctx context.Context, action workflow.WorkflowAction) error {
	return nil
}

// reportActionStatus will send a report to tink server. this should normally be called for
// each action that is run.
func reportActionStatus(ctx context.Context, status *workflow.WorkflowActionStatus) error {
	return nil
}

// actionToDockerContainerConfig takes a workflowAction and translates it to a docker container config
func actionToDockerContainerConfig(ctx context.Context, workflowAction workflow.WorkflowAction, opts ...containerConfigOption) container.Config {
	defaultConfig := container.Config{
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
		Env:          workflowAction.Environment,
		Cmd:          workflowAction.Command,
		Image:        workflowAction.Image,
	}
	for _, opt := range opts {
		opt(&defaultConfig)
	}
	return defaultConfig
}

func actionToDockerHostConfig(ctx context.Context, workflowAction workflow.WorkflowAction, opts ...containerHostOption) container.HostConfig {
	defaultConfig := container.HostConfig{
		Binds:      workflowAction.Volumes,
		PidMode:    container.PidMode(workflowAction.Pid),
		Privileged: true,
	}
	for _, opt := range opts {
		opt(&defaultConfig)
	}
	return defaultConfig
}
