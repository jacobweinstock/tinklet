package internal

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/tinkerbell/tink/protos/workflow"
)

// getAllWorkflows job is to talk with tink server and retrieve all workflows.
// Optionally, if one or more filterByFunc is passed in,
// these filterByFunc will be used to filter all the workflows that were retrieved.
func getAllWorkflows(ctx context.Context, client workflow.WorkflowServiceClient, filterByFunc ...func(workflow.WorkflowServiceClient, []*workflow.Workflow) []*workflow.Workflow) ([]*workflow.Workflow, error) {
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

// getActionsList will get all workflows, filter and return the first workflow's action list
func getActionsList(ctx context.Context, workflowClient workflow.WorkflowServiceClient, filterActionsByFunc func([]*workflow.WorkflowAction) []*workflow.WorkflowAction, filterWorkflowsByFunc ...func(workflow.WorkflowServiceClient, []*workflow.Workflow) []*workflow.Workflow) (workflowID string, actions []*workflow.WorkflowAction, err error) {
	workflows, err := getAllWorkflows(ctx, workflowClient, filterWorkflowsByFunc...)
	if err != nil {
		return "", nil, errors.WithMessage(err, "getAllWorkflows failed")
	}

	// for the moment only execute the first workflow found
	if len(workflows) == 0 {
		return "", nil, errors.New("no workflows found")
	}
	workflowID = workflows[0].Id
	resp, err := workflowClient.GetWorkflowActions(ctx, &workflow.WorkflowActionsRequest{WorkflowId: workflowID})
	if err != nil {
		return "", nil, errors.WithMessage(err, "GetWorkflowActions failed")
	}
	var acts []*workflow.WorkflowAction
	if filterActionsByFunc != nil {
		acts = filterActionsByFunc(resp.GetActionList())
	} else {
		acts = resp.GetActionList()
	}

	return workflowID, acts, nil
}

// filterWorkflowsByMac will return only workflows whose hardware devices contains the given mac
func filterWorkflowsByMac(mac string) func(workflowClient workflow.WorkflowServiceClient, workflows []*workflow.Workflow) []*workflow.Workflow {
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

// filterWorkflowsByMac will return only workflows whose hardware devices contains the given mac
func filterActionsByWorkerID(id string) func([]*workflow.WorkflowAction) []*workflow.WorkflowAction {
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

// filterByComplete will return workflows that finished. Finished is if all actions in a workflow are completed.
func filterByComplete() func(workflowClient workflow.WorkflowServiceClient, workflows []*workflow.Workflow) []*workflow.Workflow {
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
