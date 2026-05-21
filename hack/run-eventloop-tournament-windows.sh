#!/usr/bin/env bash
# hack/run-eventloop-tournament-windows.sh
#
# Runs the complete eventloop tournament NATIVELY on the Windows host (real
# windows/amd64 go.exe, never WSL), with the environment that lets the libuv
# cgo lane build and link. Designed to be invoked through hack/run-on-windows.sh
# (which transfers the repo and execs this script via Git bash), e.g. from the
# config.mk eventloop-tournament-windows target:
#
#   hack/run-on-windows.sh moo bash hack/run-eventloop-tournament-windows.sh
#
# Environment knobs (all optional, with portable defaults):
#   LIBUV_PKG_CONFIG_PATH  pkg-config search dir holding libuv.pc
#                          (default: the mingw8-built libuv prefix)
#   MINGW_BIN              directory of the gcc toolchain cgo should use
#                          (default: the mingw-w64 8.1.0 install on this host)
#   CGO_LDFLAGS            extra cgo link flags (default forces static linking
#                          so test binaries need no runtime DLLs)
#
# Any positional arguments after the options are forwarded verbatim to
# `make eventloop-tournament-bench`. This is how the caller injects
# repository-identity make variables the workspace cannot derive itself: the
# transferred tree has NO .git directory, so `git rev-parse HEAD` and
# `git status` both fail there. The caller (which runs on a real checkout)
# captures them first and forwards them as
#   EVENTLOOP_TOURNAMENT_HEAD=<sha> EVENTLOOP_TOURNAMENT_SOURCE_STATE=clean|dirty
# where eventloop-tournament-bench honors the override in its meta lines.
#
# All host-specific paths are overridable so this is portable across machines
# once libuv is built (see hack/build-libuv-mingw8.sh). Output goes to stdout
# exactly as `make eventloop-tournament-bench` produces it; the caller tees it
# to a log.
set -euo pipefail

MINGW_BIN="${MINGW_BIN:-C:/Program Files/mingw-w64/x86_64-8.1.0-posix-seh-rt_v6-rev0/mingw64/bin}"
LIBUV_PKG_CONFIG_PATH="${LIBUV_PKG_CONFIG_PATH:-C:/Users/under/libuv-mingw8/lib/pkgconfig}"
export PATH="$MINGW_BIN:$PATH"
export PKG_CONFIG_PATH="$LIBUV_PKG_CONFIG_PATH"
export CGO_ENABLED="${CGO_ENABLED:-1}"
export CGO_LDFLAGS="${CGO_LDFLAGS:--static-libgcc -static-libstdc++ -static}"

# Fail early with a clear message if libuv is not discoverable, rather than
# letting the tournament silently skip the libuv lane.
if ! command -v pkg-config >/dev/null 2>&1; then
	echo "run-eventloop-tournament-windows: pkg-config not found on PATH" >&2
	exit 2
fi
if ! pkg-config --exists libuv; then
	echo "run-eventloop-tournament-windows: pkg-config cannot find libuv (PKG_CONFIG_PATH=$PKG_CONFIG_PATH)" >&2
	echo "  build it first with hack/build-libuv-mingw8.sh" >&2
	exit 2
fi

exec make eventloop-tournament-bench "$@"
