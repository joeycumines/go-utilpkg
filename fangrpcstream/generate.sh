#!/usr/bin/env bash

# (re)generates all auto-generated source etc

set -euo pipefail || exit 1
trap 'echo [FAILURE] line_number=${LINENO} exit_code=${?} bash_version=${BASH_VERSION}' ERR

command -v go >/dev/null 2>&1
command -v protoc >/dev/null 2>&1

PATH="$(pwd)/bin:${PATH}"
export PATH

# shellcheck disable=SC2054
cmd=(
  protoc
  --go_out=. --go_opt=paths=source_relative
  --go-grpc_out=. --go-grpc_opt=paths=source_relative
)

find . -type f -name '*.proto' -exec "${cmd[@]}" {} +
