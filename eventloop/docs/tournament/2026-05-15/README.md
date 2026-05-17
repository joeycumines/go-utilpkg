# Tournament 2026-05-15

## Status

- **Darwin**: COMPLETE — all tests pass, all benchmarks captured.
- **Linux**: PARTIAL — tournament tool test phase failed due to Docker
  `GOPROXY=off` preventing module downloads for source identity checks.
  Component cross-compilation and core tests pass. Benchmarks not captured.
- **Windows**: PARTIAL — tournament test phase failed due to Windows WSL
  Go 1.20.2 not supporting `go -C` flag. Component tests pass independently.
  Benchmarks not captured.

## Environment

- **Darwin**: go1.26.5 darwin/arm64 (Apple M3 Max)
- **Linux**: go1.26.5 linux/amd64 (Docker golang:1.26.5, GOPROXY=off)
- **Windows**: go1.20.2 linux/amd64 (WSL on Windows host `moo`)

## Context

This tournament was captured after staticcheck error-propagation changes
(U1000 restructuring, ST1005/ST1003/SA9003/ST1000/ST1023/ST1020 fixes)
and the associated tournament infrastructure fixes (root_dispositions
validation, component registry hash updates, TPS timing fix, entry size
portability fix).
