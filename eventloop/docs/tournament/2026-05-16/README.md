# Tournament Results 2026-05-16

Fresh tournament runs after error-propagation changes, staticcheck fixes, and
Windows platform fixes. The Windows run is genuine native `windows/amd64`
(real `go.exe`, never WSL) with the libuv lane executed; it replaces an earlier
log that had been produced under WSL `linux/amd64` with libuv skipped. Windows
host setup and the libuv build are documented in the parent
[`../README.md`](../README.md) (see "Run the tournament on Windows").

The three logs use the legacy aggregate format, so they parse only with
`--legacy` and are stamped `validated: false`; the raw `.log` is the
authoritative artifact for this date.

## Source provenance

| Platform | HEAD | Source state | Source fingerprint |
|---|---|---|---|
| Darwin  | `d1702446` | clean | `38a08f0ec3a516cefe4fdac5673b624a0750b76294b8135d1150570e5a683aa3` |
| Linux   | `c6e3fb4e` | clean | `e00b4b9a1221050f6157d539d413d090bcd10ba761aed5f02fa79da77b00196e` |
| Windows | `1ec46105` | clean | `46e1a3c3ab9fd8a28a9f57d2d2e2a8bfa1e4fa0e14785d9718eca6866cd287b3` |

Linux was captured first at `c6e3fb4e` (the eventloop-core rework), early in the
2026-05-16 session while the Docker daemon was healthy. Darwin and Windows were
re-run later at the adjacent trees `d1702446` and `1ec46105` after the staticcheck
and portable-entry fixes landed; the one- to two-commit gaps are documentation-
and wiring-only and do not change any benchmark source. The fingerprints differ
across the three because the governed source surface (notably this tournament
documentation) changed between captures; do not treat these three as a single
longitudinal snapshot.

## Darwin
- Platform: darwin/arm64 (Apple M2 Pro)
- Go version: go1.26.5
- Status: complete
- Lanes: scheduler, promise, libuv
- Source fingerprint: 38a08f0ec3a516cefe4fdac5673b624a0750b76294b8135d1150570e5a683aa3
- Benchmark rows: 2055 (40 libuv)
- libuv version: 1.52.1

## Linux
- Platform: linux/arm64 (Docker `golang:1.26.2`)
- Go version: go1.26.2
- Status: complete
- Lanes: scheduler, promise (libuv lane absent — see note)
- Source fingerprint: e00b4b9a1221050f6157d539d413d090bcd10ba761aed5f02fa79da77b00196e
- Benchmark rows: 2015 (0 libuv)
- Note: libuv benchmarks skipped (pkg-config-libuv-unavailable in the Docker
  image). The log records no libuv lane, not a skipped-but-present lane.
- Note: captured at `c6e3fb4e`; not re-run because the Docker daemon went down
  later in the session.

## Windows
- Platform: windows/amd64 (native `go.exe` on host `moo`, not WSL)
- Go version: go1.26.5
- Status: complete
- Lanes: scheduler, promise, libuv
- Source fingerprint: 46e1a3c3ab9fd8a28a9f57d2d2e2a8bfa1e4fa0e14785d9718eca6866cd287b3
- Benchmark rows: 2005 (40 libuv)
- libuv version: 1.52.0 (built from source with mingw-w64 8.1.0; statically
  linked so test binaries need no runtime DLLs)
- Note: libuv lane is executed, not skipped. The four Unix-only product roots
  (FD-readiness workloads) are inapplicable on Windows by declaration, not
  missing evidence.
