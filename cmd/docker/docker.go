package docker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/jacobweinstock/tinklet/app"
	"github.com/jacobweinstock/tinklet/cmd/root"
	"github.com/jacobweinstock/tinklet/pkg/container/docker"
	"github.com/jacobweinstock/tinklet/pkg/grpcopts"
	"github.com/peterbourgon/ff/v3/ffcli"
	"github.com/tinkerbell/tink/protos/hardware"
	"github.com/tinkerbell/tink/protos/workflow"
	"google.golang.org/grpc"
)

const bootDeviceCmd = "docker"

// Config for the create subcommand, including a reference to the API client.
type Config struct {
	rootConfig *root.Config
}

func New(rootConfig *root.Config) *ffcli.Command {
	cfg := Config{
		rootConfig: rootConfig,
	}

	fs := flag.NewFlagSet(bootDeviceCmd, flag.ExitOnError)

	return &ffcli.Command{
		Name:       bootDeviceCmd,
		ShortUsage: "tinklet docker",
		ShortHelp:  "run the tinklet using the docker backend.",
		FlagSet:    fs,
		Exec:       cfg.Exec,
	}
}

func (c *Config) RegisterFlags(fs *flag.FlagSet) {

}

// Exec function for this command.
func (c *Config) Exec(ctx context.Context, args []string) error {
	return c.runDocker(ctx)
}

func (c *Config) runDocker(ctx context.Context) error {
	var dockerClient *client.Client
	var conn *grpc.ClientConn
	var err error
	// small control loop to create docker client and connect to tink server
	// keep trying so that if the problem is temporary or can be resolved the
	// tinklet doesn't stop and need to be restarted by an outside process or person.
	// TODO: signals arent being caught here, no way to exit
	for {
		select {
		case <-ctx.Done():
			return errors.New("breaking out")
		default:
		}
		// setup local container runtime client

		if dockerClient == nil {
			dockerClient, err = client.NewClientWithOpts()
			if err != nil {
				c.rootConfig.Log.V(0).Error(err, "error creating docker client")
				time.Sleep(time.Second * 3)
				continue
			}
		}

		// setup tink server grpc client
		if conn == nil {
			dialOpt, err := grpcopts.LoadTLSFromValue(c.rootConfig.TLS)
			if err != nil {
				c.rootConfig.Log.V(0).Error(err, "error creating gRPC client TLS dial option")
				time.Sleep(time.Second * 3)
				continue
			}

			conn, err = grpc.DialContext(ctx, c.rootConfig.Tink, dialOpt)
			if err != nil {
				c.rootConfig.Log.V(0).Error(err, "error connecting to tink server")
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
	// create a base64 encoded auth string per user defined repo
	registryAuth := make(map[string]string)
	for _, elem := range c.rootConfig.Registry {
		registryAuth[elem.Name] = encodeRegistryAuth(types.AuthConfig{
			Username: elem.User,
			Password: elem.Pass,
		})
	}

	var controllerWg sync.WaitGroup
	controllerWg.Add(1)

	go app.Controller(ctx, c.rootConfig.Log, c.rootConfig.Identifier, &docker.Client{Conn: dockerClient, RegistryAuth: registryAuth}, workflowClient, hardwareClient, &controllerWg)

	c.rootConfig.Log.V(0).Info("workflow action controller started")

	// graceful shutdown when a signal is caught
	<-ctx.Done()
	controllerWg.Wait()
	c.rootConfig.Log.V(0).Info("tinklet stopped, good bye")
	return nil
}

func encodeRegistryAuth(v types.AuthConfig) string {
	encodedAuth, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return base64.URLEncoding.EncodeToString(encodedAuth)
}
