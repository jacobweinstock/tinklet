BINARY := tinklet
OSFLAG := $(shell go env GOHOSTOS)
REPO:= github.com/jacobweinstock

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[32m%-30s\033[0m %s\n", $$1, $$2}'

.PHONY: test
test: ## Run unit tests
	go test -v -covermode=count ./...

.PHONY: test-ci
test-ci: ## Run unit tests for CI
	go mod tidy
	go test -coverprofile=cover.out ./... > test.output

.PHONY: cover
cover: ## Run unit tests with coverage report
	go test -coverprofile=cover.out ./... || true
	go tool cover -func=cover.out
	rm -rf cover.out

.PHONY: update-code-coverage-badge
update-code-coverage-badge: ## updates the code coverage badge, for use in CI
	scripts/upload_coverage.sh

.PHONY: goimports
goimports: ## run goimports updating files in place
	@echo be sure goimports is installed
	goimports -w .

.PHONY: goimports-check
goimports-check: ## run goimports displaying diffs
	@echo be sure goimports is installed
	goimports -d . | (! grep .)

.PHONY: all-checks
all-checks: cover lint goimports ## run all tests and formatters
	go vet ./...

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

.PHONY: image
image: ## Builds container image
	docker build -t tinklet:local .

PHONY: run-server
run-server: ## run server locally
ifeq (, $(shell which jq))
	go run ./bin/${BINARY} server
else
	scripts/run-tinklet.sh
endif
# BEGIN: lint-install /Users/jacobweinstock/repos/jacobweinstock/tinklet
# http://github.com/tinkerbell/lint-install

GOLINT_VERSION ?= v1.42.1
HADOLINT_VERSION ?= v2.7.0
SHELLCHECK_VERSION ?= v0.7.2
YAMLLINT_VERSION ?= 1.26.3
LINT_OS := $(shell uname)
LINT_ARCH := $(shell uname -m)
LINT_ROOT := $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))

# shellcheck and hadolint lack arm64 native binaries: rely on x86-64 emulation
ifeq ($(LINT_OS),Darwin)
	ifeq ($(LINT_ARCH),arm64)
		LINT_ARCH=x86_64
	endif
endif

LINT_LOWER_OS = $(shell echo $(LINT_OS) | tr '[:upper:]' '[:lower:]')
GOLINT_CONFIG = $(LINT_ROOT)/.golangci.yml
YAMLLINT_ROOT = out/linters/yamllint-$(YAMLLINT_VERSION)

.PHONY: lint
lint: out/linters/shellcheck-$(SHELLCHECK_VERSION)-$(LINT_ARCH)/shellcheck out/linters/hadolint-$(HADOLINT_VERSION)-$(LINT_ARCH) out/linters/golangci-lint-$(GOLINT_VERSION)-$(LINT_ARCH) $(YAMLLINT_ROOT)/dist/bin/yamllint
	find . -name go.mod -execdir "$(LINT_ROOT)/out/linters/golangci-lint-$(GOLINT_VERSION)-$(LINT_ARCH)" run -c "$(GOLINT_CONFIG)" \;
	out/linters/hadolint-$(HADOLINT_VERSION)-$(LINT_ARCH) $(shell find . -name "*Dockerfile")
	out/linters/shellcheck-$(SHELLCHECK_VERSION)-$(LINT_ARCH)/shellcheck $(shell find . -name "*.sh")
	PYTHONPATH=$(YAMLLINT_ROOT)/dist $(YAMLLINT_ROOT)/dist/bin/yamllint .

.PHONY: fix
fix: out/linters/shellcheck-$(SHELLCHECK_VERSION)-$(LINT_ARCH)/shellcheck out/linters/golangci-lint-$(GOLINT_VERSION)-$(LINT_ARCH)
	find . -name go.mod -execdir "$(LINT_ROOT)/out/linters/golangci-lint-$(GOLINT_VERSION)-$(LINT_ARCH)" run -c "$(GOLINT_CONFIG)" --fix \;
	out/linters/shellcheck-$(SHELLCHECK_VERSION)-$(LINT_ARCH)/shellcheck $(shell find . -name "*.sh") -f diff | git apply -p2 -

out/linters/shellcheck-$(SHELLCHECK_VERSION)-$(LINT_ARCH)/shellcheck:
	mkdir -p out/linters
	rm -rf out/linters/shellcheck-*
	curl -sSfL https://github.com/koalaman/shellcheck/releases/download/$(SHELLCHECK_VERSION)/shellcheck-$(SHELLCHECK_VERSION).$(LINT_LOWER_OS).$(LINT_ARCH).tar.xz | tar -C out/linters -xJf -
	mv out/linters/shellcheck-$(SHELLCHECK_VERSION) out/linters/shellcheck-$(SHELLCHECK_VERSION)-$(LINT_ARCH)

out/linters/hadolint-$(HADOLINT_VERSION)-$(LINT_ARCH):
	mkdir -p out/linters
	rm -rf out/linters/hadolint-*
	curl -sfL https://github.com/hadolint/hadolint/releases/download/v2.6.1/hadolint-$(LINT_OS)-$(LINT_ARCH) > out/linters/hadolint-$(HADOLINT_VERSION)-$(LINT_ARCH)
	chmod u+x out/linters/hadolint-$(HADOLINT_VERSION)-$(LINT_ARCH)

out/linters/golangci-lint-$(GOLINT_VERSION)-$(LINT_ARCH):
	mkdir -p out/linters
	rm -rf out/linters/golangci-lint-*
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b out/linters $(GOLINT_VERSION)
	mv out/linters/golangci-lint out/linters/golangci-lint-$(GOLINT_VERSION)-$(LINT_ARCH)

$(YAMLLINT_ROOT)/dist/bin/yamllint:
	mkdir -p out/linters
	rm -rf out/linters/yamllint-*
	curl -sSfL https://github.com/adrienverge/yamllint/archive/refs/tags/v$(YAMLLINT_VERSION).tar.gz | tar -C out/linters -zxf -
	cd $(YAMLLINT_ROOT) && pip3 install . -t dist
# END: lint-install /Users/jacobweinstock/repos/jacobweinstock/tinklet
