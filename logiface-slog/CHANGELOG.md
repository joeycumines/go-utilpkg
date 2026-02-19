# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.0.0] - 2026-02-18

### Added

- Event pooling via sync.Pool for zero-allocation overhead in hot paths
- Logger type bridging logiface Writer, EventFactory, and EventReleaser to slog.Handler
- LoggerFactory convenience type with global L instance
- WithSlogHandler option function for slog.Handler integration
- Level mapping from logiface levels to slog levels
