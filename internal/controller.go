package internal

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/tinkerbell/tink/protos/hardware"
	"github.com/tinkerbell/tink/protos/workflow"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ReportActionStatusController handles sending action status reports to tink server.
// channel is a FIFO queue so we dont lose order
func ReportActionStatusController(ctx context.Context, log logr.Logger, wg *sync.WaitGroup, jobChan chan func() error) {
	for job := range jobChan {
		err := job()
		if err != nil {
			err := job()
			if err != nil {
				log.V(0).Error(err, "reporting action status failed")
			}
		}
		wg.Done()
		/*
			var statusSendSuccessfully bool
			for {
				time.Sleep(3 * time.Second)
				if statusSendSuccessfully {
					break
				}
				err := job()
				if err != nil {
					log.V(0).Error(err, "reporting action status failed")
					continue
				}
				statusSendSuccessfully = true
				wg.Done()
			}
		*/
	}
}

// WorkflowActionController runs the tinklet control loop that watches for workflows to executes
func WorkflowActionController(ctx context.Context, log logr.Logger, config Configuration, dockerClient *client.Client, conn *grpc.ClientConn) error {
	for {
		select {
		case <-ctx.Done():
			log.V(0).Info("stopping controller")
			return nil
		default:
		}
		time.Sleep(3 * time.Second)

		// setup the workflow rpc service client
		workflowClient := workflow.NewWorkflowServiceClient(conn)
		// setup the hardware rpc service client
		hardwareClient := hardware.NewHardwareServiceClient(conn)

		// get the worker_id from tink server
		workerID, err := getHardwareID(ctx, hardwareClient, config.Identifier)
		if err != nil {
			log.V(0).Error(err, "error getting workerID from tink server")
			continue
		}

		// the first workflowID found and its associated actions are returned.
		// workflows will be filtered by: 1. mac address that do not mac the specified 2. workflows that are complete
		workflowID, actions, err := getActionsList(ctx, workflowClient, filterActionsByWorkerID(workerID), filterWorkflowsByMac(config.Identifier), filterByComplete())
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
		log.V(0).Info(fmt.Sprintf("workflow %v report action status controller started", workflowID))
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
			err = actionExecutionFlow(ctx, log, dockerClient, *action, types.ImagePullOptions{}, workflowID) // nolint
			elapsed := time.Since(start)
			actionFailed := false
			actStatus := workflow.State_STATE_SUCCESS
			if err != nil {
				log.V(0).Error(err, "action completed with an error", "action", &action)
				actionFailed = true
				switch errors.Cause(err).(type) {
				case *timeoutError:
					actStatus = workflow.State_STATE_TIMEOUT
				case *actionFailedError:
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
		// close the reportActionStatusChan
		close(reportActionStatusChan)
		log.V(0).Info("workflow complete", "workflow_id", workflowID)
	}
}
