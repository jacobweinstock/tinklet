package internal

import (
	"context"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/google/go-cmp/cmp"
	"github.com/tinkerbell/tink/protos/workflow"
)

func TestActionToDockerConfig(t *testing.T) {
	withTty := func(tty bool) containerConfigOption {
		return func(args *container.Config) { args.Tty = tty }
	}
	expected := &container.Config{
		AttachStdout: true,
		AttachStderr: true,
		Tty:          false,
		Env:          []string{"test=one"},
		Cmd:          []string{"/bin/sh"},
		Image:        "nginx",
	}
	action := workflow.WorkflowAction{
		Image:       "nginx",
		Command:     []string{"/bin/sh"},
		Environment: []string{"test=one"},
	}
	got := actionToDockerContainerConfig(context.Background(), action, withTty(false)) // nolint
	if diff := cmp.Diff(got, expected); diff != "" {
		t.Fatal(diff)
	}
}

func TestActionToDockerHostConfig(t *testing.T) {
	withPid := func(pid string) containerHostOption {
		return func(args *container.HostConfig) { args.PidMode = container.PidMode(pid) }
	}
	expected := &container.HostConfig{
		Binds: []string{
			"/dev:/dev",
			"/dev/console:/dev/console",
			"/lib/firmware:/lib/firmware:ro",
		},
		PidMode:    container.PidMode("custom"),
		Privileged: true,
	}
	action := workflow.WorkflowAction{
		Image:       "nginx",
		Command:     []string{"/bin/sh"},
		Environment: []string{"test=one"},
		Volumes: []string{
			"/dev:/dev",
			"/dev/console:/dev/console",
			"/lib/firmware:/lib/firmware:ro",
		},
		Pid: "host",
	}
	got := actionToDockerHostConfig(context.Background(), action, withPid("custom")) // nolint
	if diff := cmp.Diff(got, expected); diff != "" {
		t.Fatal(diff)
	}
}
