package internal

import (
	"context"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/go-logr/logr"
	"github.com/tinkerbell/tink/protos/workflow"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// RunController runs the tinklet control loop that watches for workflows to executes
func RunController(ctx context.Context, log logr.Logger, config Configuration) error {
	for {
		select {
		case <-ctx.Done():
			log.V(0).Info("stopping controller")
			return nil
		default:
		}
		time.Sleep(3 * time.Second)

		// setup local container runtime client
		dockerClient, err := client.NewClientWithOpts()
		if err != nil {
			log.V(0).Error(err, "error creating docker client")
			continue
		}
		// setup tink server grpc client
		conn, err := grpc.Dial(config.TinkServer, grpc.WithInsecure())
		if err != nil {
			log.V(0).Error(err, "error connecting to tink server")
			continue
		}
		// setup the workflow rpc service client
		workflowClient := workflow.NewWorkflowServiceClient(conn)

		// the first workflowID found and its associated actions are returned.
		// workflows will be filtered by: 1. mac address that do not mac the specified 2. workflows that are complete
		workflowID, actions, err := getActionsList(ctx, workflowClient, filterWorkflowsByMac("00:50:56:25:11:0e"), filterByComplete())
		if err != nil {
			log.V(0).Info("no action list retrieved", "msg", err.Error())
			continue
		}
		log.V(0).Info("found a workflow to execute", "workflow", workflowID, "actions", &actions)

		// pull the remote state locally for use in evaluation of actions to execute
		state, err := getWorkflowState(ctx, workflowID, workflowClient)
		if err != nil {
			log.V(0).Error(err, "error getting workflow state")
			continue
		}

		// create a WaitGroup for when calling Tink server with reportActionStatus'
		var reportActionStatusWG sync.WaitGroup
		// TODO: global timeout should go here and be checked after each action is executed

		for index := range actions {
			// TODO: make sure we only run actions that whose worker is equal to our mac address.
			// not currently possible as tink server doesnt store a map of worker_id to device address

			// if action is not complete, run it.
			// an action is not complete if the index is less than or equal to the states current action index
			// we check the inverse of that and continue if true
			// what if the action is running? run it again?
			if index > int(state.GetCurrentActionIndex()) {
				log.V(0).Info("action complete, moving on to the next", "action", actions[index].Name, "local index", index, "state index", state.GetCurrentActionIndex())
				continue
			}
			// what if the action is running? wait for it? why is it running and this instance of the tinklet doesnt know about it?
			// check if its running locally or not, run it if it is not?
			if state.CurrentActionState == workflow.State_STATE_RUNNING {
				log.V(0).Info("action state reports this action as running, this case is not handled well, tinklet is going to run it regardless", "action", actions[index].Name)
			}

			// update the local state
			state.CurrentAction = actions[index].Name
			state.CurrentActionState = workflow.State_STATE_RUNNING
			reportActionStatusWG.Add(1)
			// send status report to tink server that we're starting
			go sendReportActionStatus(ctx, &reportActionStatusWG, workflowClient.ReportActionStatus, &workflow.WorkflowActionStatus{
				WorkflowId:   workflowID,
				TaskName:     actions[index].TaskName,
				ActionName:   actions[index].Name,
				ActionStatus: workflow.State_STATE_RUNNING,
				Seconds:      0,
				Message:      "starting execution",
				CreatedAt:    &timestamppb.Timestamp{},
				WorkerId:     actions[index].WorkerId,
			})

			actionFailed := false
			actStatus := workflow.State_STATE_SUCCESS
			log.V(0).Info("executing action", "action", &actions[index])
			err = actionExecutionFlow(ctx, log, dockerClient, *actions[index], types.ImagePullOptions{}, workflowID) // nolint
			if err != nil {
				log.V(0).Error(err, "action completed with an error", "action", &actions[index])
				actionFailed = true
				switch err.(type) {
				case *timeoutError:
					actStatus = workflow.State_STATE_TIMEOUT
				case *actionFailedError:
					actStatus = workflow.State_STATE_FAILED
				}
			}

			// update the local state
			state.CurrentActionState = actStatus
			reportActionStatusWG.Add(1)
			// send status report that we've finished
			go sendReportActionStatus(ctx, &reportActionStatusWG, workflowClient.ReportActionStatus, &workflow.WorkflowActionStatus{
				WorkflowId:   workflowID,
				TaskName:     actions[index].TaskName,
				ActionName:   actions[index].Name,
				ActionStatus: actStatus,
				Seconds:      0,
				Message:      "action complete",
				CreatedAt:    &timestamppb.Timestamp{},
				WorkerId:     actions[index].WorkerId,
			})

			// set the local state current action index
			if index+1 > len(actions) {
				state.CurrentActionIndex = int64(index)
			} else {
				state.CurrentActionIndex = int64(index + 1)
			}

			if actionFailed {
				log.V(0).Info("failure", "action", actions[index].Name)
				break
			}
			log.V(0).Info("success", "action", actions[index].Name)
		}
		// wait until all ReportActionStatus are sent or TODO: timeout reached
		reportActionStatusWG.Wait()
	}
}
