# 04 — Gap Analysis

## Severity-Ranked Gaps

| ID | Severity | Gap | Evidence |
|---|---|---|---|
| GAP-001 | **HIGH** | GOMAXPROCS differs: Windows uses 16 while Darwin/Linux use 10 | Platform specs in tournament data |
| GAP-002 | **HIGH** | Linux high variance on AutoExit benchmarks (CV > 100%) | Linux `BenchmarkAutoExit_FastPathExit` (CV=106.3%), `BenchmarkAutoExit_ImmediateExit` (CV=107.7%) |
| GAP-003 | **MEDIUM** | Architecture confound: Windows AMD64 vs ARM64 | Darwin and Linux are both ARM64; Windows is AMD64 |
| GAP-004 | **MEDIUM** | Environment heterogeneity: Darwin native vs Linux Docker container | Docker containerization introduces confounds |
| GAP-005 | **MEDIUM** | Cross-tournament comparison only available for 2 platforms | Windows had no comparable baseline (0 significant changes reported) |
| GAP-006 | **LOW** | Windows WSL path translation errors mentioned in prior autopsy | Historical artifact from 2026-04-19, may have been resolved |
| GAP-007 | **LOW** | Linux Docker image (golang:1.26.1) may differ from Darwin Go version | Go version could affect runtime behavior |

---

### GAP-001 — GOMAXPROCS Disparity

**Status**: Confirmed — Windows uses GOMAXPROCS=16 while Darwin and Linux use GOMAXPROCS=10.

**Production impact**: Thread-parallel benchmarks on Windows may benefit from 60% more P goroutines. This confounds cross-platform comparisons, particularly for concurrent workloads.

**What this looks like in practice**:
- `BenchmarkHighContention` — Windows wins (1.57x faster than Linux)
- `BenchmarkSubmit_Parallel` — Windows wins (1.55x faster than Linux)
- `Benchmark_microtaskRing_Parallel` — Windows wins (1.81x faster than Linux)

**What a fix looks like**:
- Normalize GOMAXPROCS across all platforms before comparison
- Or explicitly acknowledge GOMAXPROCS as a factor in the report

---

### GAP-002 — Linux AutoExit Variance Instability

**Status**: Confirmed — Linux shows extreme variance (CV > 100%) on multiple AutoExit benchmarks:

| Benchmark | Linux CV% | Darwin CV% | Windows CV% |
|-----------|-----------|------------|-------------|
| BenchmarkAutoExit_FastPathExit | 106.3% | 5.7% | 6.0% |
| BenchmarkAutoExit_ImmediateExit | 107.7% | 1.4% | 5.1% |
| BenchmarkSentinelDrain_NoWork | 136.1% | 57.3% | 1.3% |

**Production impact**: AutoExit-family conclusions on Linux are unreliable. Rankings can flip under noise, making platform comparisons meaningless for these benchmarks.

**What a fix looks like**:
- Increase run count for high-variance benches (10+ runs instead of 5)
- Isolate noisy host effects before comparing means
- Exclude high-variance benchmarks from summary statistics

---

### GAP-003 — Architecture Confound

**Status**: Confirmed — Windows runs on AMD64 while both Darwin and Linux run on ARM64.

**Production impact**: Any Windows-vs-Darwin or Windows-vs-Linux comparison conflates:
1. Architecture differences (AMD64 vs ARM64)
2. OS differences (Windows vs macOS/Linux)
3. Compiler backend differences (Goamd64 vs Goarm64)

**What this means for conclusions**:
- "Darwin is faster than Windows" cannot be disentangled from "ARM64 is faster than AMD64" or "macOS scheduler is better than Windows"
- ARM64 vs ARM64 comparisons (Darwin vs Linux) are the only clean OS-level comparisons

**What a fix looks like**:
- Add ARM64 Windows runners (if hardware is available)
- Or explicitly frame Windows comparisons as "AMD64 + Windows" not pure OS

---

### GAP-004 — Environment Heterogeneity

**Status**: Confirmed — Darwin runs natively on Apple Silicon while Linux runs in a Docker container.

**Docker-specific confounds**:
- Container cgroup resource limits
- Different memory pressure characteristics
- Host kernel vs container kernel
- Networking stack differences

**Production impact**: Some Linux performance characteristics may be Docker-specific rather than Linux-OS-specific.

**What a fix looks like**:
- Run Linux natively on bare metal for production-representative comparisons
- Or explicitly document Docker as a factor

---

### GAP-005 — Windows Cross-Tournament Comparison Missing

**Status**: Confirmed — Windows shows "0 improvements, 0 regressions" in cross-tournament comparison because the 2026-04-19 Windows baseline had only 25 benchmarks, none of which are in the 2026-04-22 set.

**Production impact**: Cannot evaluate whether Windows performance has improved or regressed since 2026-04-19. The new Windows benchmarks (`Alive_Epoch_*` family) have no prior baseline.

**What a fix looks like**:
- Maintain benchmark manifest parity across tournaments
- Add historical baseline preservation

---

### GAP-006 — WSL Path Translation (Historical)

**Status**: Uncertain — The 2026-04-19 autopsy noted Windows log showed "WSL path translation errors before benchmark output." This may have been resolved in 2026-04-22.

**Production impact**: If unresolved, WSL path translation could introduce noise in Windows benchmark runs.

**What a fix looks like**:
- Verify Windows log cleanliness in 2026-04-22 raw logs

---

### GAP-007 — Go Version Differences

**Status**: Partially confirmed — `eventloop/go.mod` records `go 1.25.7` at the February-2026 tournament baseline commit (`506d664`) and `go 1.26.1` at the April-2026 tournament commit (`ba73276`); these are the only verified facts about the Go version difference.  The mechanism by which each Linux run selected its Docker image cannot be determined from current repository history: the historical tournament commits do not contain `config.mk`, and the current `config.mk` derives its Go version from the root module's `go.mod`, not from `eventloop/go.mod`.  Darwin native Go version was not recorded and may have differed from the `go.mod` directive on both platforms.

**Production impact**: Go version differences can affect:
- Runtime scheduler behavior
- Compiler optimizations
- Memory management improvements

Cross-tournament comparisons (Feb-2026 vs Apr-2026) conflate code changes with runtime version changes. This makes it impossible to attribute all performance differences solely to `eventloop` code commits.

**What a fix looks like**:
- Print the `go` directive from `eventloop/go.mod` at tournament parse time (now implemented in the 2026-04-22 tournament Makefile's `parse` target; accurate only when run at tournament time on the correct checkout)
- Record Go version in tournament metadata JSON or a companion `.env` file

---

## Gap Analysis: 2026-04-22 vs 2026-04-19

| Gap from 2026-04-19 | Status in 2026-04-22 |
|---------------------|----------------------|
| GAP-001: JSON not reproducible | **RESOLVED** — All 158 benchmarks present and consistent |
| GAP-002: Missing analysis scripts | **RESOLVED** — Scripts exist and produce output |
| GAP-003: Coverage collapse (96/45/25) | **RESOLVED** — Now 158/158/158 |
| GAP-004: Non-uniform sample counts | **RESOLVED** — All benchmarks have 5 runs |
| GAP-005: Linux high variance | **PARTIALLY RESOLVED** — AutoExit still has extreme variance |
| GAP-006: Environment confounding | **PERSISTS** — Still heterogeneous hosts |

## What Was Fixed

The 2026-04-22 tournament addressed most critical gaps from 2026-04-19:
1. Complete benchmark parity (158 each)
2. Functional pipeline (all Makefile targets work)
3. Statistical significance testing (Welch t-test applied)
4. Cross-tournament comparison (16-17 improvements/regressions detected)

## What Persists

1. GOMAXPROCS difference (16 on Windows, 10 on Darwin/Linux)
2. Architecture difference (AMD64 vs ARM64 for Windows)
3. Environment heterogeneity (Docker container vs native)
4. Linux AutoExit high variance

## Production Impact

The persistent gaps mean:
- Windows comparison is confounded by GOMAXPROCS and architecture
- Linux AutoExit conclusions are unreliable due to CV > 100%
- Cross-platform conclusions must be qualified by these confounds
