# tinklet

[![Test and Build](https://github.com/jacobweinstock/tinklet/actions/workflows/ci.yaml/badge.svg)](https://github.com/jacobweinstock/tinklet/actions/workflows/ci.yaml)
[![Code Coverage](https://img.shields.io/endpoint?url=https://gist.githubusercontent.com/jacobweinstock/9d00cc54b39121e62d88ab6e02cec6dd/raw/branch-main.json)](https://gist.github.com/jacobweinstock/9d00cc54b39121e62d88ab6e02cec6dd#file-branch-main-coverage)
[![Go Report Card](https://goreportcard.com/badge/github.com/jacobweinstock/tinklet)](https://goreportcard.com/report/github.com/jacobweinstock/tinklet)

>:warning: This is a WIP as it is not a drop in replacement for [tink-worker](https://docs.tinkerbell.org/services/tink-worker/) yet :warning:

tinklet is an implementation of the [tinkerbell worker](https://docs.tinkerbell.org/services/tink-worker/).

## Notable Features

- auth for multiple container registries
- multiple client-side TLS options for communicating with Tink server
- multiple backends [docker, kubernetes] for running workflow task actions

## Usage

```bash
USAGE
  tinklet [flags] <subcommand>

SUBCOMMANDS
  docker  run the tinklet using the docker backend.
  kube    run the tinklet using the kubernetes backend.

FLAGS
  -config tinklet.yaml                       config file (optional)
  -identifier ...                            worker id (required)
  -loglevel info                             log level (optional)
  -registry {"Name":"","User":"","Pass":""}  container image registry (optional)
  -tink ...                                  tink server url (required)
  -tls false                                 tink server TLS (optional)
```

## Design Philosophies

1. prefer easy to understand over easy to do
2. `pkg` packages do not log only return errors
3. `pkg` is generic, reuseable code
4. functions/methods should follow the single responsibility principle
5. less code, less bugs
6. prefer explicit over implicit
7. avoid global/package level variables as much as possible
