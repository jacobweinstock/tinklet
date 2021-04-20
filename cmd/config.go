package cmd

import (
	"encoding/json"
	"flag"
	"fmt"
)

// configuration for Tinklet, values are normally set from flags, env, and file
type configuration struct {
	LogLevel string
	// Config is the location to a config file
	Config string
	// Identifier is the worker ID used to get workflow tasks to run
	Identifier string `validate:"required"`
	// Tink is the URL:Port for the tink server
	Tink string `validate:"required"`
	// TLS can be one of the following
	// 1. location on disk of a cert
	// example: /location/on/disk/of/cert
	// 2. URL from which to GET a cert
	// example: http://weburl:8080/cert
	// 3. boolean; true if the tink server (specified by the Tink key/value) has a cert from a known CA
	// false if the tink server does not have TLS enabled
	// example: true
	TLS string
	// Registry is a slice of container registries with credentials to use
	// during workflow task action execution
	Registry registries `yaml:"registries"`
}

// Registry details for a container registry
type Registry struct {
	// Name is the name of the registry, such as "docker.io"
	Name string
	User string
	Pass string
	// Secure is set to false if the registry is part of the list of
	// insecure registries. Insecure registries accept HTTP and/or accept
	// HTTPS with certificates from unknown CAs.
	//Secure bool
}

// needed for (*flag.FlagSet).Var
type registries []Registry

func initFlagSetToConfig(appName string, config *configuration) *flag.FlagSet {
	fs := flag.NewFlagSet(appName, flag.ExitOnError)
	fs.StringVar(&config.LogLevel, "loglevel", "info", "log level (optional)")
	fs.StringVar(&config.Config, "config", "tinklet.yaml", "config file (optional)")
	fs.StringVar(&config.Identifier, "identifier", "", "worker id (required)")
	fs.StringVar(&config.Tink, "tink", "", "tink server url (required)\n192.168.1.214:42113")
	fs.StringVar(&config.TLS, "tls", "", "tink server TLS (optional) (default \"false\")\n- file:///path/to/cert/tink.cert\n- http://tink-server:42114/cert\n- boolean (false - no TLS, true - tink has a cert from known CA)")
	registryExample := `{"name":"localhost:5000","user":"admin","password":"password123"}`
	fs.Var(&config.Registry, "registry", fmt.Sprintf("container image registry (optional)\n%v", registryExample))
	return fs
}

func (i *registries) String() string {
	out, err := json.Marshal(*i)
	if err != nil {
		return ""
	}
	return string(out)
}

func (i *registries) Set(value string) error {
	var r Registry
	err := json.Unmarshal([]byte(value), &r)
	if err != nil {
		return err
	}
	*i = append(*i, r)
	return nil
}
