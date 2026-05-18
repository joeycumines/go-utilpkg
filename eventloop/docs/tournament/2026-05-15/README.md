# Tournament 2026-05-15

## Status

- **Darwin**: COMPLETE — all tests pass, all benchmarks captured.
- **Linux**: FAILED — tournament tool test phase failed due to Docker
  `GOPROXY=off` preventing module downloads for source identity checks.
  Benchmarks not captured. Failed log pruned.
- **Windows**: FAILED — tournament test phase failed due to Windows WSL
  Go 1.20.2 not supporting `go -C` flag. Benchmarks not captured. Failed
  log pruned.

## Environment

- **Darwin**: go1.26.5 darwin/arm64 (Apple M2 Pro)

## Context

This tournament was captured after staticcheck error-propagation changes
(U1000 restructuring, ST1005/ST1003/SA9003/ST1000/ST1023/ST1020 fixes)
and the associated tournament infrastructure fixes (root_dispositions
validation, component registry hash updates, TPS timing fix, entry size
portability fix).
