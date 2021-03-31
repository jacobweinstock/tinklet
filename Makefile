BINARY := tinklet
OSFLAG := $(shell go env GOHOSTOS)
REPO:= github.com/jacobweinstock

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[32m%-30s\033[0m %s\n", $$1, $$2}'

.PHONY: test
test: ## Run unit tests
	go test -v -covermode=count ./...

.PHONY: cover
cover: ## Run unit tests with coverage report
	go test -coverprofile=cover.out ./... || true
	go tool cover -func=cover.out
	rm -rf cover.out

.PHONY: lint
lint:  ## Run linting
	@echo be sure golangci-lint is installed: https://golangci-lint.run/usage/install/
	golangci-lint run

.PHONY: linux
linux: ## Compile for linux
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags '-s -w -extldflags "-static"' -o bin/${BINARY}-linux main.go

.PHONY: darwin
darwin: ## Compile for darwin
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -extldflags '-static'" -o bin/${BINARY}-darwin main.go

.PHONY: build
build: ## Compile the binary for the native OS
ifeq (${OSFLAG},linux)
	@$(MAKE) linux
else
	@$(MAKE) darwin
endif