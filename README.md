# tinklet

[![Test and Build](https://github.com/jacobweinstock/tinklet/actions/workflows/ci.yaml/badge.svg)](https://github.com/jacobweinstock/tinklet/actions/workflows/ci.yaml)
[![Code Coverage](https://img.shields.io/endpoint?url=https://gist.githubusercontent.com/jacobweinstock/9d00cc54b39121e62d88ab6e02cec6dd/raw/branch-main.json)](https://gist.github.com/jacobweinstock/9d00cc54b39121e62d88ab6e02cec6dd#file-branch-main-coverage)
[![Go Report Card](https://goreportcard.com/badge/github.com/jacobweinstock/tinklet)](https://goreportcard.com/report/github.com/jacobweinstock/tinklet)

>:warning: This is a WIP as it is not a drop in replacement for [tink-worker](https://docs.tinkerbell.org/services/tink-worker/) yet :warning:

tinklet is an implementation of the [tinkerbell worker](https://docs.tinkerbell.org/services/tink-worker/).

## Goals

- support multiple container runtimes
- define standard interfaces for preparing container environments and running actions
- designed for ease of maintainance, testability, and extension

## Notable Features

- support for multiple container runtimes [docker, kubernetes]
- auth for multiple container registries
- multiple client-side TLS options for communicating with Tink server

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

Tinklet uses the design philosophies from [here](https://github.com/jacobweinstock/DesignPhilosophy) and well as the following:

1. prefer easy to understand over easy to do
2. `pkg` packages do not log only return errors
3. `pkg` is generic, reuseable code
4. functions/methods should follow the single responsibility principle
5. less code, less bugs
6. prefer explicit over implicit
7. avoid global/package level variables as much as possible

## Why not contribute to [tink-worker](https://docs.tinkerbell.org/services/tink-worker/)?

I found that for tink-worker to accommodate some of my desired updates and changes it would need significant modification to its foundation.
Unfortunately, I felt that the codebase was too fragile to make these changes.
Understanding the execution flow of the codebase was also a significant barrier to overcome.

The following are some of the factors I felt contribute to the fragility of tink-worker:

- Low unit test coverage 
- Large, complex, and difficult to test functions 
- Scattered calls to `os.Exit` and `os.Getenv`
- Tight coupling to the container runtime (docker)
- Difficult to modify or test tightly coupled functions and methods
- The action execution flow code is complex and difficult to follow and understand
- The Tink server interactions are difficult to follow and understand
- The Tink workflow status reporting call result is coupled to the worker exit status
- Lack of documentation and code comments
