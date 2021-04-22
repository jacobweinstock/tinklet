package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/go-playground/validator/v10"
	"github.com/jacobweinstock/ffyaml"
	"github.com/jacobweinstock/tinklet/app"
	"github.com/jacobweinstock/tinklet/pkg/container/docker"
	"github.com/jacobweinstock/tinklet/pkg/container/kube"
	"github.com/peterbourgon/ff"
	"github.com/pkg/errors"
	"github.com/tinkerbell/tink/protos/hardware"
	"github.com/tinkerbell/tink/protos/workflow"
	"google.golang.org/grpc"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const appName = "tinklet"

// Execute sets up the config and logging, then run the tinklet control loop
func Execute(ctx context.Context) error {
	config := &configuration{}

	if err := ff.Parse(initFlagSetToConfig(appName, config), os.Args[1:],
		ff.WithEnvVarPrefix(strings.ToUpper(appName)),
		ff.WithConfigFileFlag("config"),
		ff.WithConfigFileParser(ffyaml.Parser),
		ff.WithAllowMissingConfigFile(true),
		ff.WithIgnoreUndefined(true),
	); err != nil {
		return err
	}
	if err := validator.New().Struct(config); err != nil {
		return err
	}

	log := defaultLogger(config.LogLevel)
	log.V(0).Info("tinklet started", "tink_server", config.Tink, "worker_id", config.Identifier, "config", config)

	var dockerClient *client.Client
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
		if config.Kube {
			kubeClient = connectToK8s()
		} else {
			if dockerClient == nil {
				dockerClient, err = client.NewClientWithOpts()
				if err != nil {
					log.V(0).Error(err, "error creating docker client")
					time.Sleep(time.Second * 3)
					continue
				}
			}
		}

		// setup tink server grpc client
		if conn == nil {
			dialOpt, err := loadTLSFromValue(config.TLS)
			if err != nil {
				log.V(0).Error(err, "error creating gRPC client TLS dial option")
				time.Sleep(time.Second * 3)
				continue
			}

			conn, err = grpc.DialContext(ctx, config.Tink, dialOpt)
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
	// create a base64 encoded auth string per user defined repo
	registryAuth := make(map[string]string)
	for _, elem := range config.Registry {
		registryAuth[elem.Name] = encodeRegistryAuth(types.AuthConfig{
			Username: elem.User,
			Password: elem.Pass,
		})
	}

	var controllerWg sync.WaitGroup
	controllerWg.Add(1)
	if config.Kube {
		go app.Controller(ctx, log, config.Identifier, &kube.Client{Conn: kubeClient, RegistryAuth: registryAuth}, workflowClient, hardwareClient, &controllerWg)
	} else {
		go app.Controller(ctx, log, config.Identifier, &docker.Client{Conn: dockerClient, RegistryAuth: registryAuth}, workflowClient, hardwareClient, &controllerWg)
	}
	log.V(0).Info("workflow action controller started")

	// graceful shutdown when a signal is caught
	<-ctx.Done()
	controllerWg.Wait()
	log.V(0).Info("tinklet stopped, good bye")
	return nil

}

func encodeRegistryAuth(v types.AuthConfig) string {
	encodedAuth, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return base64.URLEncoding.EncodeToString(encodedAuth)
}

func connectToK8s() kubernetes.Interface {
	home, exists := os.LookupEnv("HOME")
	if !exists {
		home = "/root"
	}

	configPath := filepath.Join(home, ".kube", "config")

	config, err := clientcmd.BuildConfigFromFlags("", configPath)
	if err != nil {
		log.Panicln("failed to create K8s config")
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Panicln("Failed to create K8s clientset")
	}

	return clientset
}
