package internal

import (
	"context"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

func TestActualPull(t *testing.T) {
	t.Skip()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	cl, err := client.NewClientWithOpts()
	if err != nil {
		t.Fatal(err)
	}

	imageName := "alpine:latest"
	var pullOpts types.ImagePullOptions

	err = pullImage(ctx, cl, imageName, pullOpts)
	if err != nil {
		t.Fatal(err)
	}
}
