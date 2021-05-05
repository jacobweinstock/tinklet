package kube

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/jacobweinstock/tinklet/app"
	"github.com/jacobweinstock/tinklet/cmd/root"
	"github.com/jacobweinstock/tinklet/pkg/container/kube"
	"github.com/jacobweinstock/tinklet/pkg/grpcopts"
	"github.com/peterbourgon/ff/v3/ffcli"
	"github.com/pkg/errors"
	"github.com/tinkerbell/tink/protos/hardware"
	"github.com/tinkerbell/tink/protos/workflow"
	"google.golang.org/grpc"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const bootDeviceCmd = "kube"

// Config for the create subcommand, including a reference to the API client.
type Config struct {
	rootConfig *root.Config
	KubeConfig string
}

func New(rootConfig *root.Config) *ffcli.Command {
	cfg := Config{
		rootConfig: rootConfig,
	}

	fs := flag.NewFlagSet(bootDeviceCmd, flag.ExitOnError)
	cfg.RegisterFlags(fs)

	return &ffcli.Command{
		Name:       bootDeviceCmd,
		ShortUsage: "tinklet kube",
		ShortHelp:  "run the tinklet using the kubernetes backend.",
		FlagSet:    fs,
		Exec:       cfg.Exec,
	}
}

func (c *Config) RegisterFlags(fs *flag.FlagSet) {
	fs.StringVar(&c.KubeConfig, "kubeconfig", "", "kube config file")
}

// Exec function for this command.
func (c *Config) Exec(ctx context.Context, args []string) error {
	return c.runKube(ctx)
}

func (c *Config) runKube(ctx context.Context) error {
	var kubeClient kubernetes.Interface
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
		kubeClient, err = k8sLoadConfig(c.KubeConfig)
		if err != nil {
			c.rootConfig.Log.V(0).Error(err, "error creating kubernetes client")
			time.Sleep(time.Second * 3)
			continue
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
	k := &kube.Client{Conn: kubeClient, RegistryAuth: registryAuth}
	// setup the workflow rpc service client - enables us to get workflows
	workflowClient := workflow.NewWorkflowServiceClient(conn)
	// setup the hardware rpc service client - enables us to the workerID (which is the hardware data ID)
	hardwareClient := hardware.NewHardwareServiceClient(conn)
	go app.Controller(ctx, c.rootConfig.Log, c.rootConfig.Identifier, k, workflowClient, hardwareClient, &controllerWg)
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
