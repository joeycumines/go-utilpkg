# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go monorepo (`go-utilpkg`) that consolidates multiple related modules. Modules are synced to their individual published repositories using the `grit` tool. The root contains no packages - all code lives in subdirectories.

### Key Modules

- **eventloop**: High-performance JavaScript-compatible event loop with timers, promises, microtasks, and cross-platform I/O polling (kqueue/epoll/IOCP)
- **goja-eventloop**: Adapter integrating eventloop with the goja JavaScript engine
- **logiface**: Performance logger interface with adapters for zerolog, logrus, stumpy
- **microbatch**, **longpoll**, **smartpoll**: Asynchronous batching and polling utilities
- **catrate**: Multi-window rate limiting
- **grpc-proxy**: HTTP/2 gRPC proxy
- **floater**: Decimal formatting for `math/big.Rat`
- **sql**: SQL utilities with MySQL export
- **prompt**: Terminal prompting library

## Build System

This project uses a sophisticated multi-module Makefile system. The Makefile automatically discovers all Go modules (via `go.mod` files) and provides targets for each.

### Core Commands

```bash
# Build and test everything (all modules)
make all

# Build all modules
make build

# Run all tests
make test

# Run tests with race detector
make test.race

# Linting and static analysis
make fmt
make vet
make staticcheck
make betteralign
make deadcode

# Coverage
make cover

# Clean artifacts
make clean
```

### Module-Specific Commands

Use period-separated slugs based on directory paths:

```bash
# Build specific module
make build.eventloop
make test.microbatch
make vet.logiface

# All checks for specific module
make all.goja-eventloop

# Run specific test in eventloop (from project.mk)
make test-promise-race-concurrent
```

### Grit Publishing

```bash
# Publish specific module to its separate repo
make grit.eventloop

# Publish all modules
make grit
```

See `project.mk` for the grit destination mappings.

### Customization

- **config.mk**: User-specific configuration (uncommitted)
- **project.mk**: Project-specific configuration (committed)

The eventloop module has its own `Makefile` with additional targets for coverage analysis and custom test runners.

## Development Workflow

1. **Always verify all tests pass** across all three OS platforms (Darwin, Linux, Windows). The `make all` target must pass 100% before considering work complete.
2. **Never commit with failing tests** - timing-dependent failures are treated as bugs to be fixed, not as flakiness to be ignored.
3. **Use subdirectory Makefiles** for complex commands rather than long shell invocations.
4. **Pipe command output** when running long commands: `cmd 2>&1 | fold -w 200 | tee build.log | tail -n 15`

## Memory Protocol

When working on multi-step tasks:

1. **Start**: Update `./blueprint.json` with all sub-tasks and `./WIP.md` with your action plan
2. **During**: Update these files to track progress; use subagents to verify progress (don't assume)
3. **End**: Ensure both files are coherent and reflect reality; `./blueprint.json` must be 100% complete

## Architecture Notes

### Eventloop

The eventloop is the most complex module. Key components:

- **Loop**: Core scheduler managing timers, tasks, and I/O polling
- **JS**: JavaScript-compatible timer APIs (`setTimeout`, `setInterval`, etc.)
- **ChainedPromise**: Promise/A+ implementation with microtask scheduling
- **Pollers**: Platform-specific I/O implementations (kqueue for macOS, epoll for Linux, IOCP for Windows)
- Thread-safe design with zero-allocation hot paths for performance

### Module Relationships

- `logiface` is the core interface; `logiface-*` packages are adapters
- `goja-eventloop` depends on `eventloop`
- `microbatch` is higher-level than `longpoll` with additional concurrency control

## Code Quality Standards

- Implement **general-purpose solutions** that work for all valid inputs, not just test cases
- **No hard-coding** values or creating solutions that only work for specific test inputs
- **Zero tolerance** for test failures - fix them properly rather than working around
- Write **high-quality, principled implementations** following best practices
- All new features require **new tests** for verification
