package app

import (
	"context"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/jacobweinstock/tinklet/pkg/tink"
	"github.com/pkg/errors"
	"github.com/tinkerbell/tink/protos/hardware"
	"github.com/tinkerbell/tink/protos/workflow"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Runner interface {
	PrepareEnv(ctx context.Context, id string) error
	CleanEnv(ctx context.Context) error
	// Prepare should create (not run) any containers/pods, setup the environment, mounts, etc
	Prepare(ctx context.Context, imageName string) (id string, err error)
	// Run should execution the action and wait for completion
	Run(ctx context.Context, id string) error
	// Destroy should handle removing all things created/setup in Prepare
	Destroy(ctx context.Context) error
	// SetActionData gets the action data into the implementation
	SetActionData(ctx context.Context, workflowID string, action *workflow.WorkflowAction)
}

// RunController starts the control loop and waits for a context cancel/done to return
func RunController(ctx context.Context, log logr.Logger, id string, workflowClient workflow.WorkflowServiceClient, hardwareClient hardware.HardwareServiceClient, backend Runner) {
	var controllerWg sync.WaitGroup
	controllerWg.Add(1)
	go controller(ctx, log, id, backend, workflowClient, hardwareClient, &controllerWg)
	log.V(0).Info("workflow action controller started")

	// graceful shutdown when a signal is caught
	<-ctx.Done()
	controllerWg.Wait()
	log.V(0).Info("tinklet stopped, good bye")
}

// controller is the control loop for executing workflow task actions
// 1. is there a workflow task to execute?
// 1a. if yes - get workflow tasks based on workflowID and workerID
// 1b. if no - ask again later
// TODO: assume action executions are idempotent, meaning keep trying them until they they succeed
// TODO; make action executions declarative, meaning we can determine current status and desired state. allows retrying executions
func controller(ctx context.Context, log logr.Logger, identifier string, runner Runner, workflowClient workflow.WorkflowServiceClient, hardwareClient hardware.HardwareServiceClient, stopControllerWg *sync.WaitGroup) {
	initialLog := log
	for {
		log = initialLog
		select {
		case <-ctx.Done():
			log.V(0).Info("stopping controller")
			stopControllerWg.Done()
			return
		default:
		}
		time.Sleep(3 * time.Second)

		// get the worker_id from tink server, this is the hardware id
		workerID, err := tink.GetHardwareID(ctx, hardwareClient, identifier)
		if err != nil {
			log.V(0).Error(err, "error getting workerID from tink server")
			continue
		}
		log = log.WithValues("workerID", workerID)

		// 1. is there a workflow task to execute?
		workflowIDs, err := tink.GetWorkflowContexts(ctx, workerID, workflowClient)
		if err != nil {
			// 1b. err then try again later, ie. continue loop
			log.V(0).Info("no actions to execute")
			continue
		}

		// 1a. for each workflow, get the associated actions based on workerID and execute them
		// if the workflowIDs is an empty slice try again later, ie. continue loop
		for _, id := range workflowIDs {
			// get the workflow tasks associated with the workflowID and workerID
			acts, err := tink.GetActionsList(ctx, id.GetWorkflowId(), workflowClient, tink.FilterActionsByWorkerID(workerID))
			if err != nil {
				break
			}
			// prepare environment for the workflow task
			if err := runner.PrepareEnv(ctx, id.GetWorkflowId()); err != nil {
				log.V(0).Error(err, "unable to prepare environment")
				break
			}
			for _, elem := range acts {
				actionLog := log.WithValues("action", &elem)
				_, reportErr := workflowClient.ReportActionStatus(ctx, &workflow.WorkflowActionStatus{
					WorkflowId:   id.GetWorkflowId(),
					TaskName:     elem.TaskName,
					ActionName:   elem.Name,
					ActionStatus: workflow.State_STATE_RUNNING,
					Seconds:      0,
					Message:      "starting execution",
					CreatedAt:    &timestamppb.Timestamp{},
					WorkerId:     workerID,
				})
				if reportErr != nil {
					// we only log here and then continue running actions because we prefer
					// the running of actions over being able to report them
					actionLog.V(0).Error(reportErr, "error sending action status report")
				}

				actionLog.V(0).Info("executing action")
				start := time.Now()
				err = actionFlow(ctx, runner, elem, elem.Image, id.WorkflowId)
				elapsed := time.Since(start)
				actStatus := workflow.State_STATE_SUCCESS
				var actionFailed bool
				if err != nil {
					actionFailed = true
					actionLog.V(0).Error(err, "action completed with an error")
					switch errors.Cause(err).(type) {
					case *TimeoutError:
						actStatus = workflow.State_STATE_TIMEOUT
					default:
						actStatus = workflow.State_STATE_FAILED
					}
				}
				_, reportErr = workflowClient.ReportActionStatus(ctx, &workflow.WorkflowActionStatus{
					WorkflowId:   id.GetWorkflowId(),
					TaskName:     elem.TaskName,
					ActionName:   elem.Name,
					ActionStatus: actStatus,
					Seconds:      int64(elapsed.Seconds()),
					Message:      "action complete",
					CreatedAt:    &timestamppb.Timestamp{},
					WorkerId:     workerID,
				})
				if reportErr != nil {
					actionLog.V(0).Error(reportErr, "error sending action status report")
				}
				actionLog.V(0).Info("action complete", "err", err)
				if actionFailed {
					break
				}
			}
			// clean up workflow task environment
			if err := runner.CleanEnv(ctx); err != nil {
				log.V(0).Error(err, "unable to clean up environment")
				break
			}
		}
	}
}

// actionFlow is the lifecyle of an action execution
// business/domain logic for executing an action
func actionFlow(ctx context.Context, client Runner, action *workflow.WorkflowAction, imageName string, workflowID string) error {
	// 1. Set the action data
	client.SetActionData(ctx, workflowID, action)
	// 2. Removal of environment (containers, etc)
	defer client.Destroy(ctx) // nolint
	// 3. Prepare to run the action
	id, err := client.Prepare(ctx, imageName)
	if err != nil {
		return err
	}
	// 4. Run the action
	return client.Run(ctx, id)
}
