# tinklet

[![Test and Build](https://github.com/jacobweinstock/tinklet/actions/workflows/ci.yaml/badge.svg)](https://github.com/jacobweinstock/tinklet/actions/workflows/ci.yaml)
[![Code Coverage](https://img.shields.io/endpoint?url=https://gist.githubusercontent.com/jacobweinstock/9d00cc54b39121e62d88ab6e02cec6dd/raw/branch-main.json)](https://gist.github.com/jacobweinstock/9d00cc54b39121e62d88ab6e02cec6dd#file-branch-main-coverage)
[![Go Report Card](https://goreportcard.com/badge/github.com/jacobweinstock/tinklet)](https://goreportcard.com/report/github.com/jacobweinstock/tinklet)

>:warning: This is a WIP as it is not a drop in replacement for [tink-worker](https://docs.tinkerbell.org/services/tink-worker/) yet :warning:

tinklet is an implementation of the [tinkerbell worker](https://docs.tinkerbell.org/services/tink-worker/).

## Notable Features

- auth for multiple container registries
- multiple client-side TLS options for communicating with Tink server (from file, from URL, off)

## Usage

```bash
Usage of tinklet:
  -config string
        config file (optional)
  -identifier string
        worker id
  -loglevel string
        log level (default "info")
  -registry value
        container image registry:
        '{"name":"localhost:5000","user":"admin","password":"password123"}'
  -tink string
        tink server url (192.168.1.214:42114)
  -tls string
        tink server TLS options:
        file:// or http:// or boolean (false - no TLS, true - tink has a cert from known CA)
```

## Design Philosophies

1. simple
2. prefer easy to understandable over easy to do
3. `pkg` packages do not log
4. `pkg` is generic, reuseable code
5. functions/methods are as test-able as possible
6. less code, less bugs
7. prefer explicit over implicit

## Lines of code

<details>
  <summary>tinklet:</summary>

```bash
--------------------------------------------------------------------------------
File                          files          blank        comment           code
--------------------------------------------------------------------------------
./app/controller.go                             10             28            155
./cmd/tinklet.go                                14             11             95
./pkg/tink/workflow.go                          13             10             89
./cmd/grpc_tls.go                                8             12             82
./pkg/container/container.go                     7              6             62
./cmd/config.go                                  7             21             45
./main.go                                        7              0             34
./pkg/tink/hardware.go                           7              6             30
./cmd/logger.go                                  4              1             23
./app/errors.go                                  5              2             20
--------------------------------------------------------------------------------
TOTAL                            10             82             97            635
--------------------------------------------------------------------------------
```

</details>

<details>
  <summary>tink-worker:</summary>

```bash
--------------------------------------------------------------------------
File                    files          blank        comment           code
--------------------------------------------------------------------------
./internal/worker.go                      54              9            436
./cmd/root.go                             30              8            138
./internal/action.go                      16              6             97
./internal/registry.go                    11              6             78
./main.go                                  8              1             23
--------------------------------------------------------------------------
TOTAL                       5            119             30            772
--------------------------------------------------------------------------
```

</details>

<details>
  <summary>How lines of code are calculated:</summary>

```bash
docker run --rm -v "${PWD}":/workdir hhatto/gocloc --exclude-ext=yaml,bash,md,Makefile --by-file $(find . -name "*.go" ! -name "*_test.go" -not -path "./scripts/*" ) 
```

</details>

