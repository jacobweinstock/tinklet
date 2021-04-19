package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/jacobweinstock/tinklet/app"
	"github.com/tinkerbell/tink/protos/hardware"
	"github.com/tinkerbell/tink/protos/workflow"
	"google.golang.org/grpc"
)

// Execute sets up the config and logging, then run the tinklet control loop
func Execute(ctx context.Context) error {
	config := &configuration{}
	if err := initConfig(config); err != nil {
		return err
	}

	log := defaultLogger(config.LogLevel)
	log.V(0).Info("tinklet started", "tink_server", config.Tink, "worker_id", config.Identifier, "config", config)

	var dockerClient *client.Client
	var conn *grpc.ClientConn
	var err error
	// small control loop to create docker client and connect to tink server
	// keep trying so that if the problem is temporary or can be resolved the
	// tinklet doesn't stop and need to be restarted by an outside process or person.
	for {
		// setup local container runtime client
		if dockerClient == nil {
			dockerClient, err = client.NewClientWithOpts()
			if err != nil {
				log.V(0).Error(err, "error creating docker client")
				time.Sleep(time.Second * 3)
				continue
			}
		}

		// setup tink server grpc client
		if conn == nil {
			conn, err = grpc.Dial(config.Tink, grpc.WithInsecure())
			if err != nil {
				log.V(0).Error(err, "error connecting to tink server")
				time.Sleep(time.Second * 3)
				continue
			}
		}
		break
	}

	// setup the workflow rpc service client - enables us to get workflows
	workflowClient := workflow.NewWorkflowServiceClient(conn)
	// setup the hardware rpc service client - enables us to the workerID (which is the hardware data ID)
	hardwareClient := hardware.NewHardwareServiceClient(conn)
	registryAuth := make(map[string]string)
	for _, elem := range config.Registry {
		registryAuth[elem.Name] = encodeRegistryAuth(types.AuthConfig{
			Username: elem.User,
			Password: elem.Pass,
		})
	}

	var controllerWg sync.WaitGroup
	controllerWg.Add(1)
	go app.Reconciler(ctx, log, config.Identifier, registryAuth, dockerClient, workflowClient, hardwareClient, &controllerWg)
	log.V(0).Info("workflow action controller started")

	// graceful shutdown when a signal is caught
	<-ctx.Done()
	controllerWg.Wait()
	log.V(0).Info("tinklet stopped, good bye")
	return nil

}

func encodeRegistryAuth(v types.AuthConfig) string {
	encodedAuth, _ := json.Marshal(v)
	return base64.URLEncoding.EncodeToString(encodedAuth)
}
