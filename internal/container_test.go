package internal

import (
	"context"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/packethost/pkg/log/logr"
)

func TestActualPull(t *testing.T) {
	t.Skip()
	cl, err := client.NewClientWithOpts()
	if err != nil {
		t.Fatal(err)
	}

	imageName := "alpine:latest"
	var pullOpts types.ImagePullOptions

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	err = pullImage(ctx, cl, imageName, pullOpts)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunContainer(t *testing.T) {
	t.Skip()
	cl, err := client.NewClientWithOpts()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	log, _, _ := logr.NewPacketLogr()
	conf := &container.Config{
		Image:        "nginx",
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
	}
	hostConf := &container.HostConfig{
		Privileged: true,
	}
	id, err := createContainer(ctx, log, cl, "jacob-test", conf, hostConf)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(id)
	t.Fatal()
}
