# tinklet

[![Test and Build](https://github.com/jacobweinstock/tinklet/actions/workflows/ci.yaml/badge.svg)](https://github.com/jacobweinstock/tinklet/actions/workflows/ci.yaml)
[![Code Coverage](https://img.shields.io/endpoint?url=https://gist.githubusercontent.com/jacobweinstock/9d00cc54b39121e62d88ab6e02cec6dd/raw/branch-main.json)](https://gist.github.com/jacobweinstock/9d00cc54b39121e62d88ab6e02cec6dd#file-branch-main-coverage)
[![Go Report Card](https://goreportcard.com/badge/github.com/jacobweinstock/tinklet)](https://goreportcard.com/report/github.com/jacobweinstock/tinklet)

tinklet is a smaller and fully tested implementation of the [tinkerbell worker](https://docs.tinkerbell.org/services/tink-worker/)

## Usage

Run the help command to see the cli options `tinklet -h`

## Lines of code

<details>
  <summary>tinklet:</summary>

```bash
-------------------------------------------------------------------------------------
File                               files          blank        comment           code
-------------------------------------------------------------------------------------
./app/controller.go                                  24            176            169
./platform/tink/workflow.go                          15             14            122
./cmd/tinklet.go                                     11             11             67
./platform/container/container.go                     7              6             62
./main.go                                             7              0             34
./platform/tink/hardware.go                           7              6             33
./platform/errors.go                                  5              2             20
./cmd/config.go                                       1              1             12
./platform/interfaces.go                              5             25              1
-------------------------------------------------------------------------------------
TOTAL                                  9             82            241            520
-------------------------------------------------------------------------------------
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
