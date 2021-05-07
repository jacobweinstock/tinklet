package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/jacobweinstock/tinklet/pkg/errs"
	"github.com/jacobweinstock/tinklet/pkg/tink"
	"github.com/pkg/errors"
	"github.com/tinkerbell/tink/protos/workflow"
)

type conn interface {
	client.ContainerAPIClient
	client.ImageAPIClient
}

type Client struct {
	Conn         conn
	RegistryAuth map[string]string
	action       *workflow.WorkflowAction
	containerID  string
	workflowID   string
}

func getRegistryAuth(regAuth map[string]string, imageName string) string {
	for reg, auth := range regAuth {
		if strings.HasPrefix(imageName, reg) {
			return auth
		}
	}
	return ""
}

func (c *Client) PrepareEnv(ctx context.Context, id string) error {
	return nil
}

func (c *Client) CleanEnv(ctx context.Context) error {
	return nil
}

func (c *Client) SetActionData(ctx context.Context, workflowID string, action *workflow.WorkflowAction) {
	c.action = action
	c.workflowID = workflowID
}

func (c *Client) Prepare(ctx context.Context, imageName string) (id string, err error) {
	// 1. Pull the image
	if pullErr := c.pullImage(ctx, imageName, types.ImagePullOptions{RegistryAuth: getRegistryAuth(c.RegistryAuth, imageName)}); pullErr != nil {
		return "", pullErr
	}
	// 2. create container
	containerName := fmt.Sprintf("%v-%v", strings.ReplaceAll(c.action.Name, " ", "-"), time.Now().UnixNano())
	id, err = c.createContainer(ctx, containerName, tink.ToDockerConf(ctx, c.action), tink.ActionToDockerHostConfig(ctx, c.action))
	if err != nil {
		return "", err
	}
	c.containerID = id
	return id, nil
}

func (c *Client) Run(ctx context.Context, id string) error {
	if err := c.Conn.ContainerStart(ctx, id, types.ContainerStartOptions{}); err != nil {
		return err
	}
	// 5. Wait and watch for container exit status or timeout
	timer := time.NewTimer(time.Duration(c.action.Timeout) * time.Second)
	var detail types.ContainerJSON
LOOP:
	for {
		select {
		case <-timer.C:
			return &errs.TimeoutError{TimeoutValue: time.Duration(c.action.Timeout) * time.Second}
		default:
			var ok bool
			var err error
			ok, detail, err = c.containerExecComplete(ctx, id)
			if err != nil {
				return errors.Wrap(err, "waiting for container failed")
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
	logs, _ := c.containerGetLogs(ctx, id, types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
	return fmt.Errorf("msg: container execution was unsuccessful; logs: %v;  exitCode: %v; details: %v", logs, detail.State.ExitCode, detail.State.Error)
}

func (c *Client) Destroy(ctx context.Context) error {
	return c.Conn.ContainerRemove(ctx, c.containerID, types.ContainerRemoveOptions{Force: true})
}

// pullImage is what you would expect from a `docker pull` cli command
// pulls an image from a remote registry
func (c *Client) pullImage(ctx context.Context, image string, pullOpts types.ImagePullOptions) error {
	out, err := c.Conn.ImagePull(ctx, image, pullOpts)
	if err != nil {
		return errors.Wrapf(err, "error pulling image: %v", image)
	}
	defer out.Close()
	fd := json.NewDecoder(out)
	var imagePullStatus struct {
		Error string `json:"error"`
	}
	for {
		if err := fd.Decode(&imagePullStatus); err != nil {
			if err == io.EOF {
				break
			}
			return errors.Wrapf(err, "error pulling image: %v", image)
		}
		if imagePullStatus.Error != "" {
			return errors.Wrapf(errors.New(imagePullStatus.Error), "error pulling image: %v", image)
		}
	}
	return nil
}

// createContainer creates a container that is not started
func (c *Client) createContainer(ctx context.Context, containerName string, containerConfig *container.Config, hostConfig *container.HostConfig) (id string, err error) {
	resp, err := c.Conn.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, containerName)
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

// containerGetLogs returns the logs of a container
func (c *Client) containerGetLogs(ctx context.Context, containerID string, options types.ContainerLogsOptions) (string, error) {
	reader, err := c.Conn.ContainerLogs(ctx, containerID, options)
	if err != nil {
		return "", err
	}
	defer reader.Close()
	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(reader)
	stdout := buf.String()
	return stdout, err
}

// containerExecComplete checks if a container run has completed or not. completed is defined as having an "exited" or "dead" status.
// see types.ContainerJSON.State.Status for all status options
func (c *Client) containerExecComplete(ctx context.Context, containerID string) (complete bool, details types.ContainerJSON, err error) {
	detail, err := c.Conn.ContainerInspect(ctx, containerID)
	if err != nil {
		return false, types.ContainerJSON{}, errors.Wrap(err, "unable to inspect container")
	}

	if detail.State.Status == "exited" || detail.State.Status == "dead" {
		return true, detail, nil
	}
	return false, types.ContainerJSON{}, nil
}
