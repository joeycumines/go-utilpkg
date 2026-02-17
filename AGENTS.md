# AGENTS.md / CLAUDE.md

## Project Overview

This is a Go monorepo (`go-utilpkg`) that consolidates multiple related modules. Modules are synced to their individual published repositories using the `grit` tool. The root contains no packages - all code lives in subdirectories.

### Architectural Decision Records (ADRs)

ADR files are stored in docs/adr/. On beginning a session, list the contents of docs/adr/ as an enumerated list (filenames only) so you know the names of existing architecture decisions. Do not expand or summarize ADR contents unless explicitly asked — only enumerate filenames by default.

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

**Note:** This project requires GNU Make 4+. Use `gmake` instead of `make` on macOS.

### Core Commands

```bash
# Build and test everything (all modules)
gmake all

# Build all modules
gmake build

# Run all tests
gmake test

# Run tests with race detector
gmake test.race

# Linting and static analysis
gmake fmt
gmake vet
gmake staticcheck
gmake betteralign
gmake deadcode

# Coverage
gmake cover

# Clean artifacts
gmake clean
```

### Module-Specific Commands

Use period-separated slugs based on directory paths:

```bash
# Build specific module
gmake build.eventloop
gmake test.microbatch
gmake vet.logiface

# All checks for specific module
gmake all.goja-eventloop

# Run specific test in eventloop (from project.mk)
gmake test-promise-race-concurrent
```

### Grit Publishing

```bash
# Publish specific module to its separate repo
gmake grit.eventloop

# Publish all modules
gmake grit
```

See `project.mk` for the grit destination mappings.

### Customization

- **config.mk**: User-specific configuration (uncommitted)
- **project.mk**: Project-specific configuration (committed)

The eventloop module has its own `Makefile` with additional targets for coverage analysis and custom test runners.

### Module Relationships

- `logiface` is the core interface; `logiface-*` packages are adapters
- `goja-eventloop` depends on `eventloop`
- `microbatch` is higher-level than `longpoll` with additional concurrency control

## Development Workflow

1. **Always verify all tests pass** across all three OS platforms (Darwin, Linux, Windows). The `gmake all` target must pass 100% before considering work complete.
2. **Never commit with failing tests** - timing-dependent failures are treated as bugs to be fixed, not as flakiness to be ignored.
3. **Use subdirectory Makefiles** for complex commands rather than long shell invocations.

## Code Quality Standards

- Implement **general-purpose solutions** that work for all valid inputs, not just test cases
- **No hard-coding** values or creating solutions that only work for specific test inputs
- **Zero tolerance** for test failures - fix them properly rather than working around
- Write **high-quality, principled implementations** following best practices
- All new features require **new tests** for verification
- **Do not use testify packages**. Only use the built-in `testing` package.
