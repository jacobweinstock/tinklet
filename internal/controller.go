package internal

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/go-logr/logr"
	"github.com/hashicorp/go-multierror"
	"github.com/philippgille/gokv"
	"github.com/pkg/errors"
	"github.com/tinkerbell/tink/protos/workflow"
	"google.golang.org/grpc"
)

type actionState string

const (
	actionNotRunning actionState = "not running"
	actionRunning    actionState = "running"
	actionComplete   actionState = "complete"
	actionFailed     actionState = "failed"
)

type workflowState map[string]actionState

// RunControlLoop runs the tinklet control loop that watchs for workflows and runs them
func RunControlLoop(ctx context.Context, log logr.Logger, config Configuration, store gokv.Store) error {
	for {
		select {
		case <-ctx.Done():
			log.V(0).Info("stopping control loop")
			return nil
		default:
		}
		time.Sleep(3 * time.Second)

		workflowID, actions, reportStatus, err := getActionsList(ctx, log, config.TinkServer, store)
		if err != nil {
			//log.V(0).Error(err, "error getting workflows")
			continue
		}

		dockerClient, err := client.NewClientWithOpts()
		if err != nil {
			log.V(0).Error(err, "error creating docker client")
			continue
		}
		var workflowCompleteSuccessfully bool
		//timer := time.NewTimer(actions.)
		// global timeout here
		for _, action := range actions.GetActionList() {
			// if task action is not complete, run it
			// get the action status from the db and check if it has been run
			record := workflowState{}
			found, err := store.Get(workflowID, &record)
			if err != nil {
				log.V(0).Error(err, "error checking local state", "action", action.Name)
				break
			}
			if !found {
				log.V(0).Error(err, "error record not found in local state", "action", action.Name)
				break
			}
			// we dont continue if any action in the workflow fails. retries will come later
			if record[workflowID] == actionFailed {
				break
			}

			val, found := record[action.Name]
			if !found {
				record[action.Name] = actionRunning
				err = store.Set(workflowID, record)
				if err != nil {
					log.V(0).Error(err, "error creating local state", "action", action.Name)
					break
				}
			}

			if val != actionRunning && val != actionComplete {
				err = actionExecutionFlow(ctx, log, dockerClient, *action, types.ImagePullOptions{}, reportStatus, workflowID)
				if err != nil {
					workflowCompleteSuccessfully = false
					log.V(0).Error(err, "error running action", "action", &action)
					record[action.Name] = actionComplete
					record[workflowID] = actionFailed
					err = store.Set(workflowID, record)
					if err != nil {
						log.V(0).Error(err, "error trying to set state in local state for workflow", "desiredState", record)
					}
					break
				}
				workflowCompleteSuccessfully = true
				record[action.Name] = actionComplete
				err = store.Set(workflowID, record)
				if err != nil {
					log.V(0).Error(err, "error setting state in local state for workflow")
				}
			}
		}
		if workflowCompleteSuccessfully {
			// workflow complete successfully, update tink
			log.V(0).Info("place holder", "msg to tink", "workflow successful")
		} else {
			// check if workflow in tink has been updated to be failed
			// if it has do nothing, else update it to failed
		}
	}
}

type reporter func(ctx context.Context, in *workflow.WorkflowActionStatus, opts ...grpc.CallOption) (*workflow.Empty, error)

func getActionsList(ctx context.Context, log logr.Logger, tinkServer string, store gokv.Store) (string, *workflow.WorkflowActionList, reporter, error) {
	conn, err := grpc.Dial(tinkServer, grpc.WithInsecure())
	if err != nil {
		//log.V(0).Error(err, "error connecting to tink server", "address", tinkServer)
		return "", nil, nil, err
	}
	workflowClient := workflow.NewWorkflowServiceClient(conn)
	workflows, err := GetAllWorkflows(ctx, workflowClient, filterByMac("00:50:56:25:11:0e"), filterByState)
	if err != nil {
		//log.V(0).Error(err, "error getting workflows")
		return "", nil, nil, err
	}

	// for the moment only execute the first workflow found
	if len(workflows) == 0 {
		return "", nil, nil, errors.New("didnt find any workflows")
	}

	workflowID := workflows[0].Id
	actions, err := workflowClient.GetWorkflowActions(ctx, &workflow.WorkflowActionsRequest{WorkflowId: workflowID})
	if err != nil {
		//log.V(0).Error(err, "error getting workflow actions")
		return "", nil, nil, err
	}

	// put the workflow in the DB
	found, _ := store.Get(workflowID, &workflowState{})
	if !found {
		rec1 := workflowState{}
		err = store.Set(workflowID, rec1)
		if err != nil {
			//log.V(0).Error(err, "error creating local state for workflow")
			return "", nil, nil, err
		}
	}
	return workflowID, actions, workflowClient.ReportActionStatus, nil
}

func filterByMac(mac string) func(workflows []*workflow.Workflow) []*workflow.Workflow {
	return func(workflows []*workflow.Workflow) []*workflow.Workflow {
		var filteredWorkflows []*workflow.Workflow
		for _, elem := range workflows {
			if strings.Contains(elem.Hardware, mac) {
				filteredWorkflows = append(filteredWorkflows, elem)
			}
		}
		return filteredWorkflows
	}
}

var filterByState = func(workflows []*workflow.Workflow) []*workflow.Workflow {
	var filteredWorkflows []*workflow.Workflow
	for _, elem := range workflows {
		if elem.State != workflow.State_STATE_SUCCESS && elem.State != workflow.State_STATE_TIMEOUT {
			filteredWorkflows = append(filteredWorkflows, elem)
		}
	}
	return filteredWorkflows
}

// actionExecutionFlow does the follow
// 1. prepare an action; 1a. pull image 1b. create configs 1c. create container
// 2. send a status report
// 3. start container
// 4. wait for exit status or timeout
// 5. send a status report
// 6. remove container
func actionExecutionFlow(ctx context.Context, log logr.Logger, dockerClient *client.Client, action workflow.WorkflowAction, pullOpts types.ImagePullOptions, reportStatus reporter, workflowID string) error {
	// 1. Prepare
	// 1a. pull image
	err := pullImage(ctx, dockerClient, action.Image, pullOpts)
	if err != nil {
		return err
	}
	// 1b. create configs
	cfg := actionToDockerContainerConfig(ctx, action)
	hostCfg := actionToDockerHostConfig(ctx, action)
	// 1c. create container
	containerID, err := createContainer(ctx, log, dockerClient, strings.ReplaceAll(action.Name, " ", "-"), &cfg, &hostCfg)
	if err != nil {
		return err
	}
	// 6. remove container
	defer dockerClient.ContainerRemove(ctx, containerID, types.ContainerRemoveOptions{Force: true})

	// 2. send status report
	_, err = reportStatus(ctx, &workflow.WorkflowActionStatus{
		WorkflowId:   workflowID,
		TaskName:     action.TaskName,
		ActionName:   action.Name,
		ActionStatus: workflow.State_STATE_RUNNING,
		WorkerId:     action.WorkerId,
	})
	if err != nil {
		return err
	}

	// 3. start container
	err = dockerClient.ContainerStart(ctx, containerID, types.ContainerStartOptions{})
	if err != nil {
		return err
	}

	// 4. wait for timeout or container not running (get exit status)
	statusReport := &workflow.WorkflowActionStatus{
		WorkflowId:   workflowID,
		TaskName:     action.TaskName,
		ActionName:   action.Name,
		ActionStatus: workflow.State_STATE_SUCCESS,
		WorkerId:     action.WorkerId,
	}
	waitErr := containerWaiter(ctx, dockerClient, (time.Duration(action.Timeout) * time.Second), containerID)
	if waitErr != nil {
		statusReport.ActionStatus = workflow.State_STATE_FAILED
		statusReport.Message = waitErr.Error()
	}

	// 5. send status report
	_, err = reportStatus(ctx, statusReport)
	if err != nil {
		return multierror.Append(waitErr, err)
	}

	return waitErr
}

// containerWaiter watches a container waiting for timeout or container not running (get exit status)
func containerWaiter(ctx context.Context, dockerClient *client.Client, timeout time.Duration, containerID string) error {
	timer := time.NewTimer(timeout)
LOOP:
	for {
		select {
		case <-timer.C:
			return fmt.Errorf("timeout (%v seconds) reached", timeout)
		default:
			details, err := dockerClient.ContainerInspect(ctx, containerID)
			if err != nil {
				return errors.WithMessage(err, "error: unable to inspect container")
			}
			// container execution completed successfully
			if details.State.Status == "exited" && details.State.ExitCode == 0 {
				break LOOP
			}
			// container execution completed unsuccessfully
			if details.State.Status != "running" && details.State.ExitCode != 0 {
				stdout, err := dockerClient.ContainerLogs(ctx, containerID, types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
				if err != nil {
					fmt.Println(err)
				}
				defer stdout.Close()
				buf := new(bytes.Buffer)
				buf.ReadFrom(stdout)
				newStr := buf.String()
				return errors.WithMessagef(errors.New(details.State.Error), "exit code: %v, stdout: %v", details.State.ExitCode, newStr)
			}
		}
	}
	return nil
}
