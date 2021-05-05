package root

import (
	"context"
	"encoding/json"
	"flag"
	"strings"

	"github.com/go-logr/logr"
	"github.com/jacobweinstock/ffyaml"
	"github.com/peterbourgon/ff/v3"
	"github.com/peterbourgon/ff/v3/ffcli"
)

const appName = "tinklet"

type Config struct {
	LogLevel string
	// Config is the location to a config file
	Config string
	// Identifier is the worker ID used to get workflow tasks to run
	Identifier string `validate:"required,mac|ip"`
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
	AppName  string
	Log      logr.Logger
}

// needed for (*flag.FlagSet).Var
type registries []Registry

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

func New() (*ffcli.Command, *Config) {
	var cfg Config
	cfg.AppName = appName

	fs := flag.NewFlagSet(appName, flag.ExitOnError)
	cfg.RegisterFlags(fs)

	return &ffcli.Command{
		ShortUsage: "tinklet [flags] <subcommand>",
		FlagSet:    fs,
		Options: []ff.Option{
			ff.WithEnvVarPrefix(strings.ToUpper(appName)),
			ff.WithConfigFileFlag("config"),
			ff.WithConfigFileParser(ffyaml.Parser),
			ff.WithAllowMissingConfigFile(true),
			ff.WithIgnoreUndefined(true),
		},
		Exec: cfg.Exec,
	}, &cfg

}

func (c *Config) RegisterFlags(fs *flag.FlagSet) {
	fs.StringVar(&c.LogLevel, "loglevel", "info", "log level (optional)")
	fs.StringVar(&c.Config, "config", "tinklet.yaml", "config file (optional)")
	fs.StringVar(&c.Identifier, "identifier", "", "worker id (required)")
	fs.StringVar(&c.Tink, "tink", "", "tink server url (required)")
	fs.StringVar(&c.TLS, "tls", "false", "tink server TLS (optional)")
	fs.Var(&c.Registry, "registry", "container image registry (optional)")
	//- file:///path/to/cert/tink.cert\n- http://tink-server:42114/cert\n- boolean (false - no TLS, true - tink has a cert from known CA)
}

// Exec function for this command.
func (c *Config) Exec(context.Context, []string) error {
	// The root command has no meaning, so if it gets executed,
	// display the usage text to the user instead.
	return flag.ErrHelp
}

func (i *registries) String() string {
	out, err := json.Marshal(&Registry{})
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
