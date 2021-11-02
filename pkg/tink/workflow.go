package tink

import (
	"context"
	"io"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/tinkerbell/tink/protos/workflow"
)

type actFilterByFunc = func([]*workflow.WorkflowAction) []*workflow.WorkflowAction

// configOpt allows modifying the container config defaults.
type configOpt func(*container.Config)

// hostOpt allows modifying the container host config defaults.
type hostOpt func(*container.HostConfig)

// ToDockerConf takes a workflowAction and translates it to a docker container config.
func ToDockerConf(_ context.Context, workflowAction *workflow.WorkflowAction, opts ...configOpt) *container.Config {
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

// ActionToDockerHostConfig converts a tink action spec to a container host config spec.
func ActionToDockerHostConfig(_ context.Context, workflowAction *workflow.WorkflowAction, opts ...hostOpt) *container.HostConfig {
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

// GetWorkflowContexts returns a slice of workflow contexts (a context is whether there is a workflow task assigned to this workerID)
// if the returned slice is not empty then there is something to be executed by this workerID.
func GetWorkflowContexts(ctx context.Context, workerID string, client workflow.WorkflowServiceClient) ([]*workflow.WorkflowContext, error) {
	contexts, err := client.GetWorkflowContexts(ctx, &workflow.WorkflowContextRequest{WorkerId: workerID})
	if err != nil {
		return nil, errors.WithMessage(err, "error getting workflow contexts")
	}

	var wks []*workflow.WorkflowContext
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		aWorkflow, recvErr := contexts.Recv()
		if errors.Is(recvErr, io.EOF) {
			break
		}
		if recvErr != nil {
			// TODO: do we need to catch all the errors?
			err = multierror.Append(err, recvErr)
			continue
		}
		wks = append(wks, aWorkflow)
	}
	if err != nil {
		return nil, err
	}

	return wks, nil
}

// GetActionsList will get all workflows actions for a given workflowID. It will optionally, filter the workflow actions based on any filterByFunc passed in.
func GetActionsList(ctx context.Context, workflowID string, workflowClient workflow.WorkflowServiceClient, filterByFunc ...actFilterByFunc) (actions []*workflow.WorkflowAction, err error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	resp, err := workflowClient.GetWorkflowActions(ctx, &workflow.WorkflowActionsRequest{WorkflowId: workflowID})
	if err != nil {
		return nil, errors.WithMessage(err, "GetActionsList failed")
	}
	acts := resp.GetActionList()
	// run caller defined filtering
	for index := range filterByFunc {
		if filterByFunc[index] != nil {
			actionsFiltered := filterByFunc[index](resp.GetActionList())
			acts = actionsFiltered
		}
	}

	return acts, nil
}

// FilterActionsByWorkerID will return only workflows whose hardware devices contains the given mac.
func FilterActionsByWorkerID(id string) actFilterByFunc {
	return func(actions []*workflow.WorkflowAction) []*workflow.WorkflowAction {
		var filteredActions []*workflow.WorkflowAction
		for _, elem := range actions {
			if elem.GetWorkerId() == id {
				filteredActions = append(filteredActions, elem)
			}
		}
		return filteredActions
	}
}
