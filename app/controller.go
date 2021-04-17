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
	"github.com/jacobweinstock/tinklet/pkg/container"
	"github.com/jacobweinstock/tinklet/pkg/tink"
	"github.com/pkg/errors"
	"github.com/tinkerbell/tink/protos/hardware"
	"github.com/tinkerbell/tink/protos/workflow"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Reconciler control loop for executing workflow task actions
// 1. is there a workflow task to execute?
// 1a. if yes - get workflow tasks based on workflowID and workerID
// 1b. if no - ask again later
// TODO: assume action executions are idempotent, meaning keep trying them until they they succeed
// TODO; make action executions declarative, meaning we can determine current status and desired state. allows retrying executions
func Reconciler(ctx context.Context, log logr.Logger, identifier string, dockerClient dClient, workflowClient workflow.WorkflowServiceClient, hardwareClient hardware.HardwareServiceClient, stopControllerWg *sync.WaitGroup) {
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
		}
	}
}

type dClient interface {
	client.ContainerAPIClient
	client.ImageAPIClient
}

// ActionExecutionFlow is the lifecyle of a container execution
// business/domain logic for executing an action
// =============================================
// 1. Pull the image
// 2. Create the container
// 3. Start the container
// 4. Removal of container is go "deferred"
// 5. Wait and watch for container exit status or timeout
func ActionExecutionFlow(ctx context.Context, log logr.Logger, dockerClient dClient, imageName string, pullOpts types.ImagePullOptions, containerConfig *tainer.Config, hostConfig *tainer.HostConfig, containerName string, timeout time.Duration) error {
	// 1. Pull the image
	if err := container.PullImage(ctx, dockerClient, imageName, pullOpts); err != nil {
		return errors.Wrap(&ExecutionError{Msg: "image pull failed"}, err.Error())
	}
	// 2. create container
	containerID, err := container.CreateContainer(ctx, dockerClient, containerName, containerConfig, hostConfig)
	if err != nil {
		return errors.Wrap(&ExecutionError{Msg: "creating container failed"}, err.Error())
	}
	// 3. Removal of container is go "deferred"
	defer dockerClient.ContainerRemove(ctx, containerID, types.ContainerRemoveOptions{Force: true}) // nolint
	// 4. Start container
	if err = dockerClient.ContainerStart(ctx, containerID, types.ContainerStartOptions{}); err != nil {
		return errors.Wrap(&ExecutionError{Msg: "starting container failed"}, err.Error())
	}
	// 5. Wait and watch for container exit status or timeout
	timer := time.NewTimer(timeout)
	var detail types.ContainerJSON
LOOP:
	for {
		select {
		case r := <-timer.C:
			return &TimeoutError{TimeoutValue: time.Duration(r.Unix())}
		default:
			var ok bool
			ok, detail, err = container.ContainerExecComplete(ctx, dockerClient, containerID)
			if err != nil {
				return errors.Wrap(&ExecutionError{Msg: "waiting for container failed"}, err.Error())
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
	}
	logs, _ := container.ContainerGetLogs(ctx, dockerClient, containerID, types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
	return fmt.Errorf("msg: container execution was unsuccessful; logs: %v;  exitCode: %v; details: %v", logs, detail.State.ExitCode, detail.State.Error)
}
