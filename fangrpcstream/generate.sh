#!/usr/bin/env bash

# (re)generates all auto-generated source etc

set -euo pipefail || exit 1
trap 'echo [FAILURE] line_number=${LINENO} exit_code=${?} bash_version=${BASH_VERSION}' ERR

command -v protoc >/dev/null 2>&1

CGO_ENABLED=0 go install google.golang.org/protobuf/cmd/protoc-gen-go || true
command -v protoc-gen-go >/dev/null 2>&1

CGO_ENABLED=0 go install google.golang.org/grpc/cmd/protoc-gen-go-grpc || true
command -v protoc-gen-go-grpc >/dev/null 2>&1

# shellcheck disable=SC2054
cmd=(
  protoc
  --go_out=. --go_opt=paths=source_relative
  --go-grpc_out=. --go-grpc_opt=paths=source_relative
)

find . -type f -name '*.proto' -exec "${cmd[@]}" {} +
