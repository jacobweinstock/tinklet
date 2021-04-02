package internal

import (
	"context"
	"io"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/tinkerbell/tink/protos/workflow"
)

// GetAllWorkflows job is to talk with tink server and retrieve all workflows.
// Optionally, if one or more filterByFunc is passed in,
// these filterByFunc will be used to filter all the workflows that were retrieved.
func GetAllWorkflows(ctx context.Context, client workflow.WorkflowServiceClient, filterByFunc ...func([]*workflow.Workflow) []*workflow.Workflow) ([]*workflow.Workflow, error) {
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
			wksFiltered := filterByFunc[0](wks)
			wks = wksFiltered
		}
	}
	return wks, nil
}
