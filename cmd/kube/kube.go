package kube

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"time"

	"github.com/jacobweinstock/tinklet/app"
	"github.com/jacobweinstock/tinklet/cmd/root"
	"github.com/jacobweinstock/tinklet/pkg/container/kube"
	"github.com/jacobweinstock/tinklet/pkg/grpcopts"
	"github.com/peterbourgon/ff/v3"
	"github.com/peterbourgon/ff/v3/ffcli"
	"github.com/pkg/errors"
	"github.com/tinkerbell/tink/protos/hardware"
	"github.com/tinkerbell/tink/protos/workflow"
	"google.golang.org/grpc"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const kubeCmd = "kube"

// Config for the create subcommand, including a reference to the API client.
type Config struct {
	rootConfig *root.Config
	KubeConfig string
	grpcClient *grpc.ClientConn
	kubeClient kubernetes.Interface
}

func New(rootConfig *root.Config) *ffcli.Command {
	cfg := Config{
		rootConfig: rootConfig,
	}

	fs := flag.NewFlagSet(kubeCmd, flag.ExitOnError)
	cfg.RegisterFlags(fs)

	return &ffcli.Command{
		Name:       kubeCmd,
		ShortUsage: "tinklet kube",
		ShortHelp:  "run the tinklet using the kubernetes backend.",
		FlagSet:    fs,
		Options:    []ff.Option{ff.WithIgnoreUndefined(false)},
		Exec:       cfg.Exec,
	}
}

func (c *Config) RegisterFlags(fs *flag.FlagSet) {
	fs.StringVar(&c.KubeConfig, "kubeconfig", "", "kube config file")
}

// Exec function for this command.
func (c *Config) Exec(ctx context.Context, args []string) error {
	c.setupClients(ctx)
	// setup the workflow rpc service client - enables us to get workflows
	workflowClient := workflow.NewWorkflowServiceClient(c.grpcClient)
	// setup the hardware rpc service client - enables us to get the workerID (which is the hardware data ID)
	hardwareClient := hardware.NewHardwareServiceClient(c.grpcClient)
	k := &kube.Client{Conn: c.kubeClient, RegistryAuth: c.rootConfig.RegistryAuth}
	control := app.Controller{WorkflowClient: workflowClient, HardwareClient: hardwareClient, Backend: k}
	// get the hardware ID (aka worker ID) from tink
	c.rootConfig.Log.V(0).Info("acquiring worker ID from tink server", "identifier", c.rootConfig.ID)
	hardwareID := control.GetHardwareID(ctx, c.rootConfig.Log, c.rootConfig.ID)
	c.rootConfig.Log.V(0).Info("worker ID acquired", "id", hardwareID)
	c.rootConfig.Log.V(0).Info("workflow action controller started")
	// Start blocks until the ctx is canceled
	control.Start(ctx, c.rootConfig.Log, hardwareID)
	return nil
}

// setupClients is a small control loop to create kube client and tink server client.
// it keeps trying so that if the problem is temporary or can be resolved the
// tinklet doesn't stop and need to be restarted by an outside process or person.
func (c *Config) setupClients(ctx context.Context) {
	const wait int = 3
	waitTime := time.Duration(wait) * time.Second
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		// setup local container runtime client
		var err error
		c.kubeClient, err = k8sLoadConfig(c.KubeConfig)
		if err != nil {
			c.rootConfig.Log.V(0).Error(err, "error creating kubernetes client")
			time.Sleep(waitTime)
			continue
		}

		// setup tink server grpc client
		if c.grpcClient == nil {
			dialOpt, err := grpcopts.LoadTLSFromValue(c.rootConfig.TLS)
			if err != nil {
				c.rootConfig.Log.V(0).Error(err, "error creating gRPC client TLS dial option")
				time.Sleep(waitTime)
				continue
			}

			c.grpcClient, err = grpc.DialContext(ctx, c.rootConfig.Tink, dialOpt)
			if err != nil {
				c.rootConfig.Log.V(0).Error(err, "error connecting to tink server")
				time.Sleep(waitTime)
				continue
			}
		}
		break
	}
}

// TODO: should we do a quick test that we can communicate with the cluster?
func k8sLoadConfig(filePath string) (kubernetes.Interface, error) {
	var config *rest.Config
	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		// try a local config
		config, err = k8sConfigFromFile(filePath)
		if err != nil {
			return nil, err
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to create K8s clientset")
	}
	return clientset, nil
}

func k8sConfigFromFile(filePath string) (*rest.Config, error) {
	if filePath != "" {
		return clientcmd.BuildConfigFromFlags("", filePath)
	}
	home, exists := os.LookupEnv("HOME")
	if !exists {
		home = "/root"
	}

	configPath := filepath.Join(home, ".kube", "config")
	config, err := clientcmd.BuildConfigFromFlags("", configPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create K8s config from file")
	}
	return config, nil
}
