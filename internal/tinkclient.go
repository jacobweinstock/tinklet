package internal

import (
	"context"
	"io"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	tt "github.com/tinkerbell/tink/protos/template"
	tw "github.com/tinkerbell/tink/protos/workflow"
)

func GetWorkflows(ctx context.Context, client tw.WorkflowServiceClient, filterByFunc ...func([]*tw.Workflow) []*tw.Workflow) ([]*tw.Workflow, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*60)
	defer cancel()
	allWorkflows, err := client.ListWorkflows(ctx, &tw.Empty{})
	if err != nil {
		return nil, errors.WithMessage(err, "error getting workflows")
	}
	// for loop in order to get all the workflows. server streams responses
	var wks []*tw.Workflow
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

	// puts filtering into the hands of the caller
	for index := range filterByFunc {
		if filterByFunc[index] != nil {
			wksFiltered := filterByFunc[0](wks)
			wks = wksFiltered
		}
	}
	return wks, nil
}

func GetTemplate(ctx context.Context, tinkClient tt.TemplateServiceClient, templateID string) (*tt.WorkflowTemplate, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*60)
	defer cancel()
	return tinkClient.GetTemplate(ctx, &tt.GetRequest{
		GetBy: &tt.GetRequest_Id{
			Id: templateID,
		},
	})
}
