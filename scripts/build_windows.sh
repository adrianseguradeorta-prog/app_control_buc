#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../app"
go test ./...
go run -buildvcs=false ./cmd/package "$@"
