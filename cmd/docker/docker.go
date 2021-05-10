package docker

import (
	"context"
	"flag"
	"fmt"
	"time"

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

const dockerCmd = "docker"

// Config for the create subcommand, including a reference to the API client.
type Config struct {
	rootConfig   *root.Config
	grpcClient   *grpc.ClientConn
	dockerClient *client.Client
}

func New(rootConfig *root.Config) *ffcli.Command {
	cfg := Config{
		rootConfig: rootConfig,
	}

	fs := flag.NewFlagSet(dockerCmd, flag.ExitOnError)

	return &ffcli.Command{
		Name:       dockerCmd,
		ShortUsage: "tinklet docker",
		ShortHelp:  "run the tinklet using the docker backend.",
		FlagSet:    fs,
		Exec:       cfg.Exec,
	}
}

// Exec function for this command.
func (c *Config) Exec(ctx context.Context, args []string) error {
	c.setupClients(ctx)
	// setup the workflow rpc service client - enables us to get workflows
	workflowClient := workflow.NewWorkflowServiceClient(c.grpcClient)
	// setup the hardware rpc service client - enables us to get the workerID (which is the hardware data ID)
	hardwareClient := hardware.NewHardwareServiceClient(c.grpcClient)
	d := &docker.Client{Conn: c.dockerClient, RegistryAuth: c.rootConfig.RegistryAuth}
	control := app.Controller{WorkflowClient: workflowClient, HardwareClient: hardwareClient, Backend: d}
	// get the hardware ID (aka worker ID) from tink
	c.rootConfig.Log.V(0).Info("acquiring worker ID from tink server", "identifier", c.rootConfig.ID)
	hardwareID := control.GetHardwareID(ctx, c.rootConfig.Log, c.rootConfig.ID)
	c.rootConfig.Log.V(0).Info("worker ID acquired", "id", hardwareID)
	c.rootConfig.Log.V(0).Info("workflow action controller started")
	// Start blocks until the ctx is canceled
	control.Start(ctx, c.rootConfig.Log, hardwareID)
	return nil
}

// setupClients is a small control loop to create docker client and tink server client.
// it keeps trying so that if the problem is temporary or can be resolved the
// tinklet doesn't stop and need to be restarted by an outside process or person.
func (c *Config) setupClients(ctx context.Context) {
	const wait int = 3
	waitTime := time.Duration(wait) * time.Second
	log := c.rootConfig.Log.WithValues("retry_interval", fmt.Sprintf("%v", waitTime))
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		// setup local container runtime client
		var err error
		if c.dockerClient == nil {
			c.dockerClient, err = client.NewClientWithOpts()
			if err != nil {
				log.V(0).Info("unable to create docker client", "msg", err.Error())
				time.Sleep(waitTime)
				continue
			}
		}

		// setup tink server grpc client
		if c.grpcClient == nil {
			dialOpt, err := grpcopts.LoadTLSFromValue(c.rootConfig.TLS)
			if err != nil {
				log.V(0).Info("unable to create gRPC client TLS dial option", "msg", err.Error())
				time.Sleep(waitTime)
				continue
			}

			c.grpcClient, err = grpc.DialContext(ctx, c.rootConfig.Tink, dialOpt)
			if err != nil {
				log.V(0).Info("error connecting to tink server", "msg", err.Error())
				time.Sleep(waitTime)
				continue
			}
		}
		break
	}
}
