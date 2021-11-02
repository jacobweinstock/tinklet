#!/usr/bin/env bash
# shellcheck disable=SC2046

set -ex
./scripts/upx-$(go env GOOS) "$1"