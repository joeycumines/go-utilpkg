# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Initial implementation of islog adapter for slog.Handler integration
- Event pooling via sync.Pool for zero-allocation overhead in hot paths
- Comprehensive godoc documentation for all exported types and methods
- 7 example functions demonstrating all adapter features
- 99,501 tests passing across logiface-testsuite adapter
- 99.5% code coverage (628/631 statements)
- Support for all logiface levels with slog level mapping

## [1.0.0] - 2026-02-18

### Added

- Event pooling implementation reducing allocation overhead for high-throughput logging
- Logger type bridging logiface.Writer[*Event], logiface.EventFactory[*Event],
  and logiface.EventReleaser[*Event] to slog.Handler
- LoggerFactory convenience type with global L instance for ergonomic configuration
- WithSlogHandler() option function for slog.Handler integration
- Comprehensive documentation in doc.go covering:
    - Package overview and integration patterns
    - Level mapping table (logiface → slog)
    - Performance characteristics (pool reuse, early filter, inline-friendly)
    - Thread safety guarantees
    - Usage examples and patterns
- Type-level documentation:
    - Event: lifecycle, pool reuse, per-goroutine usage
    - Logger: bridge role, level filtering, error handling, panic behavior
    - LoggerFactory: configuration pattern, aliasing strategy
- Method-level documentation:
    - Event: Level(), AddField(), AddMessage(), AddError(), AddGroup()
    - Logger: NewEvent(), ReleaseEvent(), Write()
- 7 example_test.go functions:
    - Basic usage pattern
    - Level filtering configuration
    - Custom handler with ReplaceAttr
    - Context propagation
    - Error field handling
    - Pool reuse demonstration
    - All level logging
- Coverage analysis documentation (COVERAGE.md) explaining 3 uncovered
  statements (defensive nil guard)
- Static analysis clean (zero issues from go vet, staticcheck)
- Go 1.21+ requirement for slog package support

## Security Notes

No known security vulnerabilities.

- No unsafe package usage
- No reflect package usage in hot paths
- Input validation: nil handler panics on WithSlogHandler()
- Defensive nil guards in ReleaseEvent() prevent potential panics

## Migration Guide

Upgrading from direct slog.Logger to logiface-slog:

```go
// Before (direct slog):
logger := slog.New(handler)
logger.Info("message", "key", "value")

// After (logiface-slog):
logger := islog.L.New(islog.L.WithSlogHandler(handler))
logger.Info().Str("key", "value").Log("message")
```

See example_test.go for complete usage patterns.
