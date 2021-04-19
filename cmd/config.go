package cmd

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/peterbourgon/ff"
	"github.com/pkg/errors"
)

// configuration for Tinklet, values are normally set from flags, env, and file
type configuration struct {
	LogLevel   string
	Config     string
	Identifier string
	Tink       string
	Registry   registries
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

type registries []Registry

func initConfig(config *configuration) error {
	fs := flag.NewFlagSet("tinklet", flag.ExitOnError)
	fs.StringVar(&config.LogLevel, "loglevel", "info", "log level")
	fs.StringVar(&config.Config, "config", "", "config file (optional)")
	fs.StringVar(&config.Identifier, "identifier", "", "worker id")
	fs.StringVar(&config.Tink, "tink", "", "tink server url (192.168.1.214:42114)")
	fs.Var(&config.Registry, "registry", "container image registries")

	if err := ff.Parse(fs, os.Args[1:],
		ff.WithEnvVarPrefix("TINKLET"),
		ff.WithConfigFileFlag("config"),
		ff.WithConfigFileParser(Parser),
		ff.WithAllowMissingConfigFile(true),
		ff.WithIgnoreUndefined(true),
	); err != nil {
		return errors.Wrap(err, "error parsing config")
	}
	return nil
}

func (i *registries) String() string {
	var re []string
	v := `{"name":"%v","user":"%v","pass":"%v"}`
	for _, elem := range *i {
		re = append(re, fmt.Sprintf(v, elem.Name, elem.User, elem.Pass))
	}
	return fmt.Sprintf("[%v]", strings.Join(re, ","))
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
