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

type workflowsFilterByFunc = func(workflow.WorkflowServiceClient, []*workflow.Workflow) []*workflow.Workflow
type actionsFilterByFunc = func([]*workflow.WorkflowAction) []*workflow.WorkflowAction

// containerConfigOption allows modifying the container config defaults
type containerConfigOption func(*container.Config)

// containerHostOption allows modifying the container host config defaults
type containerHostOption func(*container.HostConfig)

// ActionToDockerContainerConfig takes a workflowAction and translates it to a docker container config
func ActionToDockerContainerConfig(ctx context.Context, workflowAction *workflow.WorkflowAction, opts ...containerConfigOption) *container.Config { // nolint
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

func ActionToDockerHostConfig(ctx context.Context, workflowAction *workflow.WorkflowAction, opts ...containerHostOption) *container.HostConfig { // nolint
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
// if the returned slice is not empty then there is something to be executed by this workerID
func GetWorkflowContexts(ctx context.Context, workerID string, client workflow.WorkflowServiceClient) ([]*workflow.WorkflowContext, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*60)
	defer cancel()
	contexts, err := client.GetWorkflowContexts(ctx, &workflow.WorkflowContextRequest{
		WorkerId: workerID,
	})
	if err != nil {
		return nil, errors.WithMessage(err, "error getting workflow contexts")
	}

	var wks []*workflow.WorkflowContext
	for {
		aWorkflow, recvErr := contexts.Recv()
		if recvErr == io.EOF {
			break
		}
		if recvErr != nil {
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

// GetAllWorkflows job is to talk with tink server and retrieve all workflows.
// Optionally, if one or more filterByFunc is passed in,
// these filterByFunc will be used to filter all the workflows that were retrieved.
func GetAllWorkflows(ctx context.Context, client workflow.WorkflowServiceClient, filterByFunc ...workflowsFilterByFunc) ([]*workflow.Workflow, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*60)
	defer cancel()
	allWorkflows, err := client.ListWorkflows(ctx, &workflow.Empty{})
	if err != nil {
		return nil, errors.WithMessage(err, "error getting workflows")
	}
	// for loop; in order to get all the workflows because the server streams responses one by one.
	var wks []*workflow.Workflow
	for {
		aWorkflow, recvErr := allWorkflows.Recv()
		if recvErr == io.EOF {
			break
		}
		if recvErr != nil {
			err = multierror.Append(err, recvErr)
			continue
		}
		wks = append(wks, aWorkflow)
	}
	if err != nil {
		return nil, err
	}

	// run caller defined filtering
	for index := range filterByFunc {
		if filterByFunc[index] != nil {
			wksFiltered := filterByFunc[index](client, wks)
			wks = wksFiltered
		}
	}
	return wks, nil
}

// GetActionsList will get all workflows actions for a given workflowID. It will optionally, filter the workflow actions based on any filterByFunc passed in
func GetActionsList(ctx context.Context, workflowID string, workflowClient workflow.WorkflowServiceClient, filterByFunc ...actionsFilterByFunc) (actions []*workflow.WorkflowAction, err error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*60)
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

/*
// FilterWorkflowsByMac will return only workflows whose hardware devices contains the given mac
func FilterWorkflowsByMac(mac string) workflowsFilterByFunc {
	return func(workflowClient workflow.WorkflowServiceClient, workflows []*workflow.Workflow) []*workflow.Workflow {
		var filteredWorkflows []*workflow.Workflow
		for _, elem := range workflows {
			if strings.Contains(elem.Hardware, mac) {
				filteredWorkflows = append(filteredWorkflows, elem)
			}
		}
		return filteredWorkflows
	}
}
*/

// filterWorkflowsByMac will return only workflows whose hardware devices contains the given mac
func FilterActionsByWorkerID(id string) actionsFilterByFunc {
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

/*
// FilterByComplete will return workflows that finished. Finished is if all actions in a workflow are completed.
func FilterByComplete() workflowsFilterByFunc {
	return func(workflowClient workflow.WorkflowServiceClient, workflows []*workflow.Workflow) []*workflow.Workflow {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		var filteredWorkflows []*workflow.Workflow
		for _, elem := range workflows {
			state, err := workflowClient.GetWorkflowContext(ctx, &workflow.GetRequest{Id: elem.Id})
			if err != nil {
				continue
			}
			// as long as the workflow is not done or timed out we add it to the list
			if state.CurrentActionIndex+1 != state.TotalNumberOfActions {
				filteredWorkflows = append(filteredWorkflows, elem)
			}
		}
		return filteredWorkflows
	}
}
*/
