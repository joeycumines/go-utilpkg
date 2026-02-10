# Eventloop Benchmark Tournament — 2026-02-10

## Tournament Overview

| Field | Value |
|-------|-------|
| **Date** | 2026-02-10 |
| **Scope** | Full eventloop module benchmark suite |
| **Platforms** | 3 (Darwin ARM64, Linux ARM64, Windows AMD64) |
| **Total Benchmarks** | 108 unique benchmarks per platform |
| **Total Runs** | 1,620 (108 × 5 runs × 3 platforms) |
| **Methodology** | `go test -bench=. -benchmem -count=5 -run=^$ -benchtime=1s -timeout=10m` |

---

## Platform Specifications

### Darwin (macOS ARM64)

| Spec | Value |
|------|-------|
| OS | macOS (Darwin) |
| Architecture | ARM64 (Apple Silicon) |
| GOMAXPROCS | 10 |
| Go Version | Local installation |
| Environment | Native macOS |

### Linux (ARM64 Container)

| Spec | Value |
|------|-------|
| OS | Linux |
| Architecture | ARM64 |
| GOMAXPROCS | 10 |
| Go Version | 1.25.7 |
| Environment | Docker container (`golang:1.25.7`) |

### Windows (AMD64)

| Spec | Value |
|------|-------|
| OS | Windows |
| Architecture | AMD64 (x86_64) |
| CPU | Intel Core i9-9900K |
| GOMAXPROCS | 16 |
| Environment | Native Windows |

---

## Benchmark Methodology

All benchmarks were executed with identical parameters across platforms:

```
go test -bench=. -benchmem -count=5 -run=^$ -benchtime=1s -timeout=10m .
```

| Parameter | Value | Purpose |
|-----------|-------|---------|
| `-bench=.` | All benchmarks | Run the complete benchmark suite |
| `-benchmem` | Enabled | Collect memory allocation statistics |
| `-count=5` | 5 iterations | Statistical significance (5 runs per benchmark) |
| `-run=^$` | No tests | Skip unit tests, only run benchmarks |
| `-benchtime=1s` | 1 second | Minimum benchmark duration per run |
| `-timeout=10m` | 10 minutes | Overall timeout |

### Data Collection

For each benchmark run, the following metrics are captured:
- **ns/op** — Nanoseconds per operation (primary performance metric)
- **B/op** — Bytes allocated per operation
- **allocs/op** — Number of allocations per operation

### Statistical Analysis

For each benchmark, the following statistics are computed from the 5 runs:
- **Mean** — Average across runs
- **Min / Max** — Range
- **StdDev** — Sample standard deviation
- **CV%** — Coefficient of variation (stddev / mean × 100)

Cross-platform comparisons use Welch's t-test (p < 0.05) to identify statistically significant differences.

---

## File Manifest

### Raw Benchmark Data

| File | Description |
|------|-------------|
| [`darwin.json`](darwin.json) | Parsed Darwin benchmark results (108 benchmarks, full statistics) |
| [`linux.json`](linux.json) | Parsed Linux benchmark results (108 benchmarks, full statistics) |
| [`windows.json`](windows.json) | Parsed Windows benchmark results (108 benchmarks, full statistics) |

### Analysis Reports

| File | Description |
|------|-------------|
| [`comparison.md`](comparison.md) | **2-platform comparison** — Darwin vs Linux (both ARM64). Detailed statistical analysis, category breakdowns, allocation comparison |
| [`comparison-3platform.md`](comparison-3platform.md) | **3-platform comparison** — Darwin vs Linux vs Windows. Cross-platform triangulation, architecture comparison, platform rankings |
| [`SUMMARY.md`](SUMMARY.md) | Executive summary with quick results overview and recommendations |
| [`COMPLETE.md`](COMPLETE.md) | Tournament completion checklist |
| [`FINAL.md`](FINAL.md) | Final tournament report with key findings |

### Analysis Tools

| File | Description |
|------|-------------|
| [`parse_benchmarks.py`](parse_benchmarks.py) | Go benchmark log parser — converts raw log output to structured JSON |
| [`analyze_2platform.py`](analyze_2platform.py) | Generates `comparison.md` from `darwin.json` and `linux.json` |
| [`analyze_3platform.py`](analyze_3platform.py) | Generates `comparison-3platform.md` from all three JSON files |
| [`Makefile`](Makefile) | Build targets for parsing and analysis |

### Build System

| File | Description |
|------|-------------|
| [`Makefile`](Makefile) | Tournament Makefile with `parse`, `analyze`, `clean`, and `all` targets |

---

## How to Reproduce

### 1. Run Benchmarks

From the project root (`go-utilpkg/`):

```bash
# Darwin (local macOS)
cd eventloop && go test -bench=. -benchmem -count=5 -run=^$ -benchtime=1s -timeout=10m . > ../build.benchmark.darwin.log 2>&1

# Linux (Docker container)
docker run --rm -v $(pwd):/work -w /work/eventloop golang:1.25.7 \
  go test -bench=. -benchmem -count=5 -run=^$ -benchtime=1s -timeout=10m . > build.benchmark.linux.log 2>&1

# Windows (native)
go test -bench=. -benchmem -count=5 -run=^$ -benchtime=1s -timeout=10m ./eventloop > build.benchmark.windows.log 2>&1
```

### 2. Parse Benchmark Logs to JSON

```bash
cd eventloop/docs/tournament/2026-02-10

# Parse each platform
python3 parse_benchmarks.py ../../../../build.benchmark.darwin.log darwin darwin arm64 > darwin.json
python3 parse_benchmarks.py ../../../../build.benchmark.linux.log linux linux arm64 > linux.json
python3 parse_benchmarks.py ../../../../build.benchmark.windows.log windows windows amd64 > windows.json

# Or use Make:
make parse
```

### 3. Generate Analysis Reports

```bash
cd eventloop/docs/tournament/2026-02-10

# Generate 3-platform comparison
python3 analyze_3platform.py

# Generate 2-platform comparison (Darwin vs Linux)
python3 analyze_2platform.py

# Or use Make:
make analyze
```

---

## Summary of Key Findings

### Platform Win Rates

| Platform | Wins (of 108) | Win Rate | Best Category |
|----------|---------------|----------|---------------|
| 🐧 Linux | 42 | 38.9% | Task submission, concurrency |
| 🍎 Darwin | 37 | 34.3% | Timer operations, promises |
| 🪟 Windows | 29 | 26.9% | Low-level latency primitives |

### Performance Highlights

- **Fastest single operation:** `BenchmarkLatencyDirectCall` — 0.21 ns/op (Windows)
- **Lowest latency:** `BenchmarkLatencyStateLoad` — 0.26 ns/op (Windows)
- **Zero-allocation hot paths:** Submit, FastPathSubmit, Microtask (all platforms)
- **Allocation consistency:** 89.8% allocs/op match between Darwin and Linux

### Architecture Insights

- **ARM64 vs ARM64 (Darwin vs Linux):** Nearly identical mean performance (ratio 0.980x), with Darwin excelling at timer operations and Linux at parallel task submission
- **ARM64 vs AMD64:** Windows AMD64 competitive on low-level primitives but slower on timer-heavy workloads
- **OS-level impact:** Linux CFS scheduler benefits parallel/concurrent workloads; macOS Mach scheduler benefits synchronization-heavy timer operations

### Stability

- 24 benchmarks achieve CV < 2% on both Darwin and Linux
- Sub-nanosecond precision achieved for baseline operations (DirectCall, StateLoad)
- Timer operations show highest variance across platforms due to OS scheduling differences

---

## Related Links

- [2-Platform Comparison (Darwin vs Linux)](comparison.md)
- [3-Platform Comparison (All Platforms)](comparison-3platform.md)
- [Executive Summary](SUMMARY.md)
- [Tournament Completion Status](COMPLETE.md)
