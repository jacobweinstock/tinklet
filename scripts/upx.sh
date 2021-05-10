#!/usr/bin/env bash

set -ex
./scripts/upx-$(go env GOOS) $1