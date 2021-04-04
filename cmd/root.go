package cmd

import (
	"context"
	"time"

	"github.com/docker/docker/client"
	"github.com/jacobweinstock/goconfig"
	"github.com/jacobweinstock/tinklet/internal"
	"github.com/packethost/pkg/log/logr"
	"github.com/tinkerbell/tink/protos/hardware"
	"github.com/tinkerbell/tink/protos/workflow"
	"google.golang.org/grpc"
)

// Execute sets up the config and logging, then run the tinklet control loop
func Execute(ctx context.Context) error {
	// set default config values
	config := internal.Configuration{
		LogLevel: "info",
	}
	cfgParser := goconfig.NewParser(
		goconfig.WithPrefix("TINKLET"),
		goconfig.WithFile("tinklet.yaml"),
	)
	err := cfgParser.Parse(&config)
	if err != nil {
		return err
	}

	log, _, _ := logr.NewPacketLogr(
		logr.WithServiceName("tinklet"),
		logr.WithLogLevel(config.LogLevel),
	)
	log.V(0).Info("tinklet started", "tink_server", config.TinkServer, "worker_id", config.Identifier, "container_registry", config.Registry.Name)

	var dockerClient *client.Client
	var conn *grpc.ClientConn
	// small control loop to create docker client and connect to tink server
	// keep trying so that if the problem is temporary or can be resolved the
	// tinklet doesn't stop and need to be restarted by an outside process or person.
	for {
		time.Sleep(time.Second * 3)
		// setup local container runtime client
		if dockerClient == nil {
			dockerClient, err = client.NewClientWithOpts()
			if err != nil {
				log.V(0).Error(err, "error creating docker client")
				continue
			}
		}

		// setup tink server grpc client
		if conn == nil {
			conn, err = grpc.Dial(config.TinkServer, grpc.WithInsecure())
			if err != nil {
				log.V(0).Error(err, "error connecting to tink server")
				continue
			}
		}

		break
	}

	// setup the workflow rpc service client
	workflowClient := workflow.NewWorkflowServiceClient(conn)
	// setup the hardware rpc service client
	hardwareClient := hardware.NewHardwareServiceClient(conn)

	log.V(0).Info("workflow action controller started")
	return internal.WorkflowActionController(ctx, log, config, dockerClient, workflowClient, hardwareClient)
}
