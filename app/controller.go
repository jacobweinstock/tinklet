package app

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	tainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/go-logr/logr"
	"github.com/jacobweinstock/tinklet/platform"
	"github.com/jacobweinstock/tinklet/platform/container"
	"github.com/jacobweinstock/tinklet/platform/tink"
	"github.com/pkg/errors"
	"github.com/tinkerbell/tink/protos/hardware"
	"github.com/tinkerbell/tink/protos/workflow"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ReportActionStatusController is generic in that it the chan is just a func that returns an error
// but currently only handles sending action status reports to tink server.
// channel is a FIFO queue so we dont lose order. for the moment, only retry a ras (report action status) once.
func ReportActionStatusController(ctx context.Context, log logr.Logger, sharedWg *sync.WaitGroup, rasChan chan func() error, doneWg *sync.WaitGroup) {
	for {
		select {
		case <-ctx.Done():
			log.V(0).Info("stopping report action status controller")
			doneWg.Done()
			return
		case ras := <-rasChan:
			err := ras()
			if err != nil {
				err := ras()
				if err != nil {
					log.V(0).Error(err, "reporting action status failed")
				}
			}
			sharedWg.Done()
		}
	}
}

// 1. is there a workflow task to execute?
// 1a. if yes - get workflow tasks based on workflowID and workerID
// 1b. if no - ask again later
// TODO: assume action executions are idempotent, meaning keep trying them until they they succeed
// TODO; make action executions declarative, meaning we can determine current status and desired state. allows retrying executions
func Reconciler(ctx context.Context, log logr.Logger, identifier string, dockerClient Client, workflowClient workflow.WorkflowServiceClient, hardwareClient hardware.HardwareServiceClient, stopControllerWg *sync.WaitGroup) {
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

		// get the worker_id from tink server
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
					// we only log here because we prefer the running of actions over being able to report them
					actionLog.V(0).Error(reportErr, "error sending action status report")
				}
				actionLog.V(0).Info("executing action")
				start := time.Now()
				err = ActionExecutionFlow(ctx, log, dockerClient, elem.Image,
					types.ImagePullOptions{},
					tink.ActionToDockerContainerConfig(ctx, elem), // nolint
					tink.ActionToDockerHostConfig(ctx, elem),      // nolint
					// spaces in a container name are not valid, add a timestamp so the container name is always unique.
					fmt.Sprintf("%v-%v", strings.ReplaceAll(elem.Name, " ", "-"), time.Now().UnixNano()),
					(time.Duration(elem.Timeout) * time.Second),
				)
				elapsed := time.Since(start)
				actStatus := workflow.State_STATE_SUCCESS
				var actionFailed bool
				if err != nil {
					actionFailed = true
					actionLog.V(0).Error(err, "action completed with an error")
					switch errors.Cause(err).(type) {
					case *platform.TimeoutError:
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
		}
	}
}

type Client interface {
	client.ContainerAPIClient
	client.ImageAPIClient
}

// business/domain logic for executing an action
// =============================================
// 1. Pull the image
// 2. Create the container
// 3. Start the container
// 4. Removal of container is go "deferred"
// 5. Wait and watch for container exit status or timeout
func ActionExecutionFlow(ctx context.Context, log logr.Logger, dockerClient Client, imageName string, pullOpts types.ImagePullOptions, containerConfig *tainer.Config, hostConfig *tainer.HostConfig, containerName string, timeout time.Duration) error {
	// 1. Pull the image
	err := container.PullImage(ctx, dockerClient, imageName, pullOpts)
	if err != nil {
		return errors.Wrap(&platform.ExecutionError{Msg: "image pull failed"}, err.Error())
	}
	// 2. create container
	containerID, err := container.CreateContainer(ctx, dockerClient, containerName, containerConfig, hostConfig)
	if err != nil {
		return errors.Wrap(&platform.ExecutionError{Msg: "creating container failed"}, err.Error())
	}
	// 3. Removal of container is go "deferred"
	defer dockerClient.ContainerRemove(ctx, containerID, types.ContainerRemoveOptions{Force: true}) // nolint
	// 4. Start container
	err = dockerClient.ContainerStart(ctx, containerID, types.ContainerStartOptions{})
	if err != nil {
		return errors.Wrap(&platform.ExecutionError{Msg: "starting container failed"}, err.Error())
	}
	// 5. Wait and watch for container exit status or timeout
	timer := time.NewTimer(timeout)
	var detail types.ContainerJSON
LOOP:
	for {
		select {
		case r := <-timer.C:
			return &platform.TimeoutError{TimeoutValue: time.Duration(r.Unix())}
		default:
			var ok bool
			ok, detail, err = container.ContainerExecComplete(ctx, dockerClient, containerID)
			if err != nil {
				return errors.Wrap(&platform.ExecutionError{Msg: "waiting for container failed"}, err.Error())
			}
			if ok {
				break LOOP
			}
		}
	}

	if detail.ContainerJSONBase == nil {
		return errors.New("container details was nil, cannot tell success or failure status without these details")
	}
	// container execution completed successfully
	if detail.State.ExitCode == 0 {
		return nil
	} else {
		logs, _ := container.ContainerGetLogs(ctx, dockerClient, containerID, types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
		return fmt.Errorf("msg: container execution was unsuccessful; logs: %v;  exitCode: %v; details: %v", logs, detail.State.ExitCode, detail.State.Error)
	}
}

// there are 2 overall processes that need to happen
// 1. query tink server to know whether there is a workflow task to run and what the details of that task are
// ideally tink server holds all the business logic here. tinklet shouldnt have to make any determinations about whether to run or wait.
// if tink server provides something to run, it runs immediately.
//
// 2. run the actions in the task, send status reports as the actions are executed
/*func ActionsController(ctx context.Context, log logr.Logger, tinkClient tinkInterface) {

}*/

// WorkflowActionController runs the tinklet control loop that watches for workflows to executes
// TODO: remove the business logic of when and what to execute from here, pass it in possibly, maybe an interface or a func?
// TODO: think about passing in the execution flow logic of an action, maybe an interface or a func?

/*
func WorkflowActionController(ctx context.Context, log logr.Logger, identifier string, dockerClient client.CommonAPIClient, workflowClient workflow.WorkflowServiceClient, hardwareClient hardware.HardwareServiceClient, reportActionStatusWG *sync.WaitGroup, reportActionStatusChan chan func() error, stopControllerWg *sync.WaitGroup) {
	initialLog := log
	for {
		log = initialLog
		select {
		case <-ctx.Done():
			log.V(0).Info("stopping workflow action controller")
			stopControllerWg.Done()
			return
		default:
		}
		time.Sleep(3 * time.Second)

		// get the worker_id from tink server
		workerID, err := tink.GetHardwareID(ctx, hardwareClient, identifier)
		if err != nil {
			log.V(0).Error(err, "error getting workerID from tink server")
			continue
		}
		log = log.WithValues("workerID", workerID)

		// the first workflowID found and its associated actions are returned.
		// workflows will be filtered by: 1. mac address that do not mac the specified 2. workflows that are complete
		workflows, err := tink.GetAllWorkflows(ctx, workflowClient, tink.FilterWorkflowsByMac(identifier), tink.FilterByComplete())
		if err != nil {
			continue
		}
		workflowID := workflows[0].Id
		actions, err := tink.GetActionsList(ctx, workflowID, workflowClient, tink.FilterActionsByWorkerID(workerID))
		if err != nil {
			//log.V(0).Info("no action list retrieved", "msg", err.Error(), "workerID", identifier)
			continue
		}
		log = log.WithValues("workflowID", workflowID)
		log.V(0).Info("found a workflow to execute", "actions", &actions)

		// pull the remote state locally for use in evaluation of actions to execute
		// TODO what happens here when there are multiple tasks in a workflow? do we need to check the workerID?
		state, err := workflowClient.GetWorkflowContext(ctx, &workflow.GetRequest{Id: workflowID})
		if err != nil {
			log.V(0).Error(err, "error getting workflow state")
			continue
		}

		// TODO: global timeout should go here and be checked after each action is executed
		for index, action := range actions {
			actionLog := log.WithValues("action", &action)
			// if action is not complete, run it.
			// an action is not complete if the index is less than or equal to the states current action index
			// we check the inverse of that and continue if true
			// what if the action is running? run it again?
			if index > int(state.GetCurrentActionIndex()) {
				actionLog.V(0).Info("action complete, moving on to the next", "local index", index, "state index", state.GetCurrentActionIndex())
				continue
			}
			// what if the action is running? wait for it? why is it running and this instance of the tinklet doesnt know about it?
			// check if its running locally or not, run it if it is not?
			if state.CurrentActionState == workflow.State_STATE_RUNNING {
				actionLog.V(0).Info("action state reports this action as running, this case is not handled well, tinklet is going to run it regardless")
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

			actionLog.V(0).Info("executing action")
			start := time.Now()
			err = tinklet.ActionExecutionFlow(ctx, log, dockerClient, action.Image,
				types.ImagePullOptions{},
				tink.ActionToDockerContainerConfig(ctx, action), // nolint
				tink.ActionToDockerHostConfig(ctx, action),      // nolint
				// spaces in a container name are not valid, add a timestamp so the container name is always unique.
				fmt.Sprintf("%v-%v", strings.ReplaceAll(action.Name, " ", "-"), time.Now().UnixNano()),
				(time.Duration(action.Timeout) * time.Second),
			)
			elapsed := time.Since(start)
			actionFailed := false
			actStatus := workflow.State_STATE_SUCCESS
			if err != nil {
				actionLog.V(0).Error(err, "action completed with an error")
				actionFailed = true
				switch errors.Cause(err).(type) {
				case *platform.TimeoutError:
					actStatus = workflow.State_STATE_TIMEOUT
				case *platform.ExecutionError:
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
				actionLog.V(0).Info("action failed")
				break
			}
			actionLog.V(0).Info("action succeeded")

		}
		log.V(0).Info("workflow complete", "success", "TODO: set an overall workflow success/failure value")
	}
}
*/
