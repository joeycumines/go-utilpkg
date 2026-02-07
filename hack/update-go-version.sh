#!/bin/sh

if ! {
  command -v find >/dev/null 2>&1 &&
    command -v go >/dev/null 2>&1 &&
    [ "$#" -eq 1 ] &&
    [ -n "$1" ]
}; then
  echo "Unexpected environment or arguments. This script requires 'find' and 'go' commands and exactly one argument (the Go version)." >&2
  exit 1
fi

exec find . -name go.mod -exec /bin/sh -c 'go mod edit -go="$1" -toolchain= "$2"' - "$1" {} \; &&
  go work use
