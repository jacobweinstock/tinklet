package internal

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/tinkerbell/tink/protos/hardware"
	"github.com/tinkerbell/tink/protos/workflow"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ReportActionStatusController handles sending action status reports to tink server.
// channel is a FIFO queue so we dont lose order. for the moment, only retry a ras (report action status) once.
func ReportActionStatusController(ctx context.Context, log logr.Logger, wg *sync.WaitGroup, rasChan chan func() error) {
	for ras := range rasChan {
		err := ras()
		if err != nil {
			err := ras()
			if err != nil {
				log.V(0).Error(err, "reporting action status failed")
			}
		}
		wg.Done()
	}
}

// WorkflowActionController runs the tinklet control loop that watches for workflows to executes
func WorkflowActionController(ctx context.Context, log logr.Logger, config Configuration, dockerClient *client.Client, workflowClient workflow.WorkflowServiceClient, hardwareClient hardware.HardwareServiceClient) error {
	for {
		select {
		case <-ctx.Done():
			log.V(0).Info("stopping controller")
			return nil
		default:
		}
		time.Sleep(3 * time.Second)

		// get the worker_id from tink server
		workerID, err := getHardwareID(ctx, hardwareClient, config.Identifier)
		if err != nil {
			log.V(0).Error(err, "error getting workerID from tink server")
			continue
		}

		// the first workflowID found and its associated actions are returned.
		// workflows will be filtered by: 1. mac address that do not mac the specified 2. workflows that are complete
		workflows, err := getAllWorkflows(ctx, workflowClient, filterWorkflowsByMac(config.Identifier), filterByComplete())
		if err != nil {
			continue
		}
		workflowID, actions, err := getActionsList(ctx, workflowClient, workflows, filterActionsByWorkerID(workerID))
		if err != nil {
			//log.V(0).Info("no action list retrieved", "msg", err.Error(), "workerID", config.Identifier)
			continue
		}
		log.V(0).Info("found a workflow to execute", "workflow", workflowID, "actions", &actions)

		// pull the remote state locally for use in evaluation of actions to execute
		// TODO what happens here when there are multiple tasks in a workflow? do we need to check the workerID?
		state, err := workflowClient.GetWorkflowContext(ctx, &workflow.GetRequest{Id: workflowID})
		if err != nil {
			log.V(0).Error(err, "error getting workflow state")
			continue
		}

		reportActionStatusChan := make(chan func() error)
		var reportActionStatusWG sync.WaitGroup
		go ReportActionStatusController(ctx, log, &reportActionStatusWG, reportActionStatusChan)
		log.V(0).Info(fmt.Sprintf("report action status controller for workflow: %v started", workflowID))
		// TODO: global timeout should go here and be checked after each action is executed
		for index, action := range actions {
			// if action is not complete, run it.
			// an action is not complete if the index is less than or equal to the states current action index
			// we check the inverse of that and continue if true
			// what if the action is running? run it again?
			if index > int(state.GetCurrentActionIndex()) {
				log.V(0).Info("action complete, moving on to the next", "action", action.Name, "local index", index, "state index", state.GetCurrentActionIndex())
				continue
			}
			// what if the action is running? wait for it? why is it running and this instance of the tinklet doesnt know about it?
			// check if its running locally or not, run it if it is not?
			if state.CurrentActionState == workflow.State_STATE_RUNNING {
				log.V(0).Info("action state reports this action as running, this case is not handled well, tinklet is going to run it regardless", "action", action.Name)
			}

			// send status report to tink server that we're starting. in a goroutine so we dont block action executions.
			// this is a design decision to prioritize executing actions over whether the report action status call is success or not.
			// this incurs one trade off of having report action status calls possibly failing while actions succeed.
			reportActionStatusWG.Add(1)
			go func() {
				reportActionStatusChan <- func() error {
					_, err := workflowClient.ReportActionStatus(ctx, &workflow.WorkflowActionStatus{
						WorkflowId:   workflowID,
						TaskName:     action.TaskName,
						ActionName:   action.Name,
						ActionStatus: workflow.State_STATE_RUNNING,
						Seconds:      0,
						Message:      "starting execution",
						CreatedAt:    &timestamppb.Timestamp{},
						WorkerId:     action.WorkerId,
					})
					return err
				}
			}()

			log.V(0).Info("executing action", "action", &action)
			start := time.Now()
			err = executionFlow(ctx, log,
				dockerClient,
				action.Image,
				types.ImagePullOptions{},
				actionToDockerContainerConfig(ctx, *action),                                            // nolint
				actionToDockerHostConfig(ctx, *action),                                                 // nolint
				fmt.Sprintf("%v-%v", strings.ReplaceAll(action.Name, " ", "-"), time.Now().UnixNano()), // spaces in a container name are not valid, we also add a timestamp so the container name is always unique.                                                // nolint,
				(time.Duration(action.Timeout) * time.Second),
			)
			elapsed := time.Since(start)
			actionFailed := false
			actStatus := workflow.State_STATE_SUCCESS
			if err != nil {
				log.V(0).Error(err, "action completed with an error", "action", &action)
				actionFailed = true
				switch errors.Cause(err).(type) {
				case *timeoutError:
					actStatus = workflow.State_STATE_TIMEOUT
				case *executionError:
					actStatus = workflow.State_STATE_FAILED
				}
			}

			// update the local state
			state.CurrentActionState = actStatus
			// send status report that we've finished. in a goroutine so we dont block action executions.
			reportActionStatusWG.Add(1)
			go func() {
				reportActionStatusChan <- func() error {
					_, err := workflowClient.ReportActionStatus(ctx, &workflow.WorkflowActionStatus{
						WorkflowId:   workflowID,
						TaskName:     action.TaskName,
						ActionName:   action.Name,
						ActionStatus: actStatus,
						Seconds:      int64(elapsed.Seconds()),
						Message:      "action complete",
						CreatedAt:    &timestamppb.Timestamp{},
						WorkerId:     action.WorkerId,
					})
					return err
				}
			}()

			// increment the local state current action index to the next value, why do i need to increment the state current action index here?
			// TODO: understand why setting state.CurrentActionIndex = int64(index) seems to break ReportActionStatus calls
			state.CurrentActionIndex = int64(index + 1)
			// update the local state
			state.CurrentAction = action.Name

			reportActionStatusWG.Wait()
			if actionFailed {
				log.V(0).Info("failure", "action", action.Name)
				break
			}
			log.V(0).Info("success", "action", action.Name)

		}
		// as each workflow gets its own reportActionStatusChan, we need to close this reportActionStatusChan now that the workflow is complete
		close(reportActionStatusChan)
		log.V(0).Info("workflow complete", "workflow_id", workflowID, "success", "TODO: set workflow success value")
	}
}
