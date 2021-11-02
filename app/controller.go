package app

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/jacobweinstock/tinklet/pkg/errs"
	"github.com/jacobweinstock/tinklet/pkg/tink"
	"github.com/pkg/errors"
	"github.com/tinkerbell/tink/protos/hardware"
	"github.com/tinkerbell/tink/protos/workflow"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Controller.
type Controller struct {
	WorkflowClient workflow.WorkflowServiceClient
	HardwareClient hardware.HardwareServiceClient
	Backend        Runner
}

// Runner interface what backend implementations must define.
type Runner interface {
	EnvironmentRunner
	ContainerRunner
}

// EnvironmentRunner is for preparing an environment for running workflow task actions.
type EnvironmentRunner interface {
	// PrepareEnv should do things like create namespaces, configs, secrets, etc
	PrepareEnv(ctx context.Context, id string) error
	// CleanEnv should remove anything that was created when PrepareEnv was called
	CleanEnv(ctx context.Context) error
}

// ContainerRunner defines the methods needed to run a workflow task action.
type ContainerRunner interface {
	// Prepare should create (not run) any containers/pods, setup the environment, mounts, etc
	Prepare(ctx context.Context, imageName string) (id string, err error)
	// Run should execution the action and wait for completion
	Run(ctx context.Context, id string) error
	// Destroy should handle removing all things created/setup in Prepare
	Destroy(ctx context.Context) error
	// SetActionData gets the action data into the implementation
	SetActionData(ctx context.Context, workflowID string, action *workflow.WorkflowAction)
}

func (c Controller) GetHardwareID(ctx context.Context, log logr.Logger, identifier string) string {
	idChan := make(chan string, 1)
	go func(idChn chan string) {
		idChn <- c.doGetHardwareID(ctx, log, identifier)
	}(idChan)

	select {
	case <-ctx.Done(): // graceful shutdown when a signal is caught
		return ""
	case id := <-idChan:
		return id
	}
}

func (c Controller) doGetHardwareID(ctx context.Context, log logr.Logger, identifier string) string {
	wait := 3
	waitTime := time.Duration(wait) * time.Second
	for {
		select {
		case <-ctx.Done():
			log.V(0).Info("stopped hardwareID control loop", "msg", ctx.Err())
			return ""
		default:
			// get the worker_id from tink server, this is the hardware id
			workerID, err := tink.GetHardwareID(ctx, c.HardwareClient, identifier)
			if err != nil {
				log.V(0).Info("unable to get worker ID from tink server", "id", identifier, "retry_interval", fmt.Sprintf("%v", waitTime))
				time.Sleep(waitTime)
				continue
			}
			return workerID
		}
	}
}

// Start the control loop and waits for a context cancel/done to return.
func (c Controller) Start(ctx context.Context, log logr.Logger, id string) {
	var controllerWg sync.WaitGroup
	controllerWg.Add(1)
	go c.run(ctx, log, id, &controllerWg)
	// graceful shutdown when a signal is caught
	<-ctx.Done()
	controllerWg.Wait()
}

// run is the control loop for executing workflow task actions
// 1. is there a workflow task to execute?
// 1a. if yes - get workflow tasks based on workflowID and workerID
// 1b. if no - ask again later
// TODO: assume action executions are idempotent, meaning keep trying them until they they succeed
// TODO: make action executions declarative, meaning we can determine current status and desired state. allows retrying executions.
// This might require the workflow spec to be updated. It currently leans heavily toward the imperative side, we'd want to rethink
// and have it favor a more declarative feel, describing desired state. Is that even possible?
func (c Controller) run(ctx context.Context, log logr.Logger, workerID string, stopControllerWg *sync.WaitGroup) {
	initialLog := log.WithValues("workerID", workerID)
	for {
		logger := initialLog
		select {
		case <-ctx.Done():
			logger.V(0).Info("stopping controller")
			stopControllerWg.Done()
			return
		default:
			// 1. is there a workflow task to execute?
			// this is a blocking call, it waits until a workflow is received
			workflowIDs, err := tink.GetWorkflowContexts(ctx, workerID, c.WorkflowClient)
			if err != nil {
				// 1b. err then try again later, ie. continue loop
				logger.V(0).Info("no actions to execute", "msg", err.Error())
				continue
			}
			c.execWorkflows(ctx, logger, workflowIDs, workerID)
		}
	}
}

// actionFlow is the lifecycle of an action, business/domain logic for executing an action.
func actionFlow(ctx context.Context, client ContainerRunner, action *workflow.WorkflowAction, imageName, workflowID string) error {
	// 1. Set the action data
	client.SetActionData(ctx, workflowID, action)
	// 2. Removal of environment (containers, etc)
	defer client.Destroy(ctx) // nolint: errcheck // handle error?
	// 3. Prepare to run the action
	id, err := client.Prepare(ctx, imageName)
	if err != nil {
		return err
	}
	// 4. Run the action
	return client.Run(ctx, id)
}

// execWorkflows runs each workflow task in the slice of workflowIDs.
func (c Controller) execWorkflows(ctx context.Context, log logr.Logger, workflowIDs []*workflow.WorkflowContext, workerID string) {
	// 1a. for each workflow, get the associated actions based on workerID and execute them
	// if the workflowIDs is an empty slice try again later, ie. continue loop
	for _, id := range workflowIDs {
		// get the workflow tasks associated with the workflowID and workerID
		acts, err := tink.GetActionsList(ctx, id.GetWorkflowId(), c.WorkflowClient, tink.FilterActionsByWorkerID(workerID))
		if err != nil {
			break
		}
		// prepare environment for the workflow task
		if err = c.Backend.PrepareEnv(ctx, id.GetWorkflowId()); err != nil {
			log.V(0).Error(err, "unable to prepare environment")
			break
		}

		for _, act := range acts {
			// using the address in an iteration value in a loop is generally not safe, so we create a new value here
			act := act
			// &act gives us a double pointer. It gives us 2 things here
			// 1. removes any golangci-lint complaining about copying a lock value
			// 2. allows the logger to be able to parse the struct key/values into its own keys and values instead of one giant string
			// example:
			//	{"level":"info","msg":"debug","action":"task_name:\"this thing\"  name:\"one\"  image:\"alpine\"  worker_id:\"12345\""}
			// vs
			// 	{"level":"info","msg":"debug","action":{"task_name":"this thing","name":"one","image":"alpine","worker_id":"12345"}}
			actionLog := log.WithValues("action", &act)
			_, reportErr := c.WorkflowClient.ReportActionStatus(ctx, &workflow.WorkflowActionStatus{
				WorkflowId:   id.GetWorkflowId(),
				TaskName:     act.TaskName,
				ActionName:   act.Name,
				ActionStatus: workflow.State_STATE_RUNNING,
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
			err = actionFlow(ctx, c.Backend, act, act.Image, id.WorkflowId)
			elapsed := time.Since(start)
			actStatus := workflow.State_STATE_SUCCESS
			var actionFailed bool
			if err != nil {
				actionFailed = true
				actionLog.V(0).Error(err, "action completed with an error")
				switch errors.Cause(err).(type) {
				case *errs.TimeoutError:
					actStatus = workflow.State_STATE_TIMEOUT
				default:
					actStatus = workflow.State_STATE_FAILED
				}
			}
			_, reportErr = c.WorkflowClient.ReportActionStatus(ctx, &workflow.WorkflowActionStatus{
				WorkflowId:   id.GetWorkflowId(),
				TaskName:     act.TaskName,
				ActionName:   act.Name,
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
		if err := c.Backend.CleanEnv(ctx); err != nil {
			log.V(0).Error(err, "unable to clean up environment")
			break
		}
	}
}
