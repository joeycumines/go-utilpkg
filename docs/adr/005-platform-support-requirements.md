# ADR-005: Platform Support Requirements

## Context

The go-utilpkg monorepo contains multiple modules and packages, each with its own platform considerations. Some modules depend on system-specific APIs or behaviors, while others are pure Go and can work everywhere.

## Decision

**Default Rule:** All Go modules should be available and work on all platforms that they can possibly work on.

By default, this means all platforms unless there is a technical constraint preventing support (e.g., missing system APIs, required system dependencies, or fundamentally incompatible OS semantics).

When platform-specific code is required, idiomatic Go patterns—build tags, build constraints, and platform-specific source files—must be used to handle each supported platform.

## Rationale

Modules in this repository vary in their platform dependencies:

- **Pure Go modules** (e.g., `logiface`, `floater`) can and should work on macOS, Linux, and Windows with no constraints.
- **System-call dependent modules** (e.g., `eventloop`) use platform-specific APIs (epoll on Linux, kqueue on macOS, IOCP on Windows) and require careful platform-specific implementations.
- **Modules with system dependencies** may need build constraints when required libraries or APIs are unavailable on certain platforms.

The principle is simple: **maximize platform support by default, constrain only when necessary.** This ensures users can use these modules wherever possible without artificial limitations.

### Three-Platform Testing: Feasibility, Not Arbitrariness

The requirement to test on macOS, Linux, and Windows is driven by practical feasibility rather than policy dogmatism:

- These three platforms represent the major desktop/server operating systems in use.
- CI infrastructure and developer workflows are set up to efficiently test all three.
- If it were more feasible and easier to manage testing against additional platforms (e.g., BSD variants), those would also be tested.

## Implementation Requirements

1. **Idiomatic Go Platform Handling:**
   - Use build tags (`//go:build darwin`) and platform-specific file names (`fd_unix.go`, `fd_windows.go`) appropriately.
   - When a module cannot work on a platform, build constraints should prevent compilation clearly rather than causing runtime failures.

2. **Test Coverage:**
   - Tests must exist and pass on all supported platforms.
   - Platform-specific tests should be guarded with corresponding build tags.
   - Avoid `//go:build !windows` guards unless there is a genuine technical incompatibility.

3. **Documentation:**
   - Modules should document any platform limitations or special considerations.
   - If a module compiles but has limited or untested runtime support on a platform, this should be clearly stated.

## CI/CD Infrastructure

The project's GitHub Actions workflow (`.github/workflows/make.yaml`) is configured to run the full test suite with all checks on all three platforms:

- **Strategy:** Matrix build with `ubuntu-latest`, `macos-latest`, and `windows-latest`.
- **Execution:** Runs `make all` on each platform, which includes building, testing, linting, and static analysis across all modules.
- **Fail-fast:** All tests must pass on all platforms for a PR to be mergeable.

Platform-specific test failures are treated as bugs to be fixed, not as expected skips. If a test legitimately cannot run on a platform, it must be properly guarded with build tags so it isn't compiled there.

## Local Development

Developers can verify platform compatibility locally using the provided Makefile setup.

The repository includes `example.config.mk` in the root, which demonstrates how to test on multiple platforms from a development machine:

- **macOS (local):** Run `make all` directly.
- **Linux (via Docker):** Use `make make-all-in-container` to test inside a Linux Go container.
- **Windows (via SSH):** Use `make make-all-run-windows` to execute tests on a remote Windows machine with SSH access.

This setup allows developers to catch platform-specific issues before pushing to CI.

## Enforcement

1. **CI Gate:** All pull requests must pass the full `make all` target on all three platforms before merging.
2. **Platform-Specific Bugs:** Failures that only appear on one platform are still considered blocking bugs—fix them rather than skip tests.
3. **Build Tag Usage:** Reviewers should ensure that platform-specific code properly uses idiomatic Go build tags and doesn't introduce unnecessary platform restrictions.
