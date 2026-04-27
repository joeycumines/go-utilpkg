# Eventloop Performance Tournament

A comprehensive guide to running full platform-to-platform performance benchmarks across all three supported platforms: Darwin (macOS), Linux, and Windows.

## Tournament Definition & Goal

The overarching goal of the tournament is to act as a **reliable, robust anti-performance-regression tool** where relative performance characteristics and multi-faceted meta-analysis are recorded over time. 

A **full tournament** means:

- **All benchmarks** run, not a subset. The eventloop module currently runs **108 benchmarks** covering timers, promises, microtasks, I/O polling, and integration scenarios.
- **All 3 platforms**: Darwin (macOS), Linux, Windows — each run independently.
- **Promises benchmarks included**: Promise resolution latency, concurrent promise handling, promise chain throughput.
- **Statistical rigor**: 5 runs per benchmark (via `-count=5`), with Welch's t-test significance testing (p < 0.05).

A tournament produces comparable, statistically-validated performance data across platforms.

## Pre-Tournament Checklist

Before running a tournament, verify your environment:

```bash
# 1. Verify Go installation (1.23+ required)
go version   # Should be 1.23 or higher

# 2. Verify Docker access (required for Linux benchmarks)
docker --version
docker run --rm golang:1.25.7 go version  # Confirm container execution works

# 3. Verify Windows access (required for Windows benchmarks)
# This requires the hack/run-on-windows.sh script and appropriate access
hack/run-on-windows.sh echo "windows accessible" 2>/dev/null && echo "OK" || echo "Windows access unavailable"

# 4. Check benchmark count matches reference (108 benchmarks)
go test -bench=. -list='^$' -run=^$ . 2>/dev/null | grep -c "Benchmark"  # Should report 108
```

If any check fails, resolve before proceeding. Linux benchmarks can be skipped if Docker is unavailable, but this breaks the 3-platform comparison.

## Execution Contexts: Monorepo vs. Isolated

Before executing the tournament, identify your current repository context, as this alters paths and make commands:

### Context A: Running from `go-utilpkg` (Monolithic Root Repo)
* **Location:** You are at the root of the `go-utilpkg` repository.
* **Tournament Path:** `eventloop/docs/tournament/`
* **Log Output Location:** `go-utilpkg/eventloop-tournament-*.log`
* **Log Relative Path from Parsers:** `../../../../eventloop-tournament-*.log`

### Context B: Running from `go-eventloop` (Isolated Module Repo)
* **Location:** You are at the root of the isolated `go-eventloop` repository.
* **Tournament Path:** `docs/tournament/`
* **Log Output Location:** `go-eventloop/eventloop-tournament-*.log`
* **Log Relative Path from Parsers:** `../../../eventloop-tournament-*.log`

## Step-by-Step Process

### Phase 1: Benchmark Execution

The tournament uses `make` (or `gmake` on macOS) to orchestrate cross-platform builds and testing. All executions rely on standard targets defined in your configuration.

**Step 1: Bootstrap your Make configuration**

If a local `config.mk` file does not exist in your root directory, you must bootstrap it from the example:
```bash
cp example.config.mk config.mk
```

**Step 2: Run the Tournament via Make**

You can run the entire 3-platform suite at once, or run them individually. 

```bash
# Run the full 3-platform tournament
make eventloop-tournament

# OR run individual targets if needed:
make eventloop-tournament-darwin
make eventloop-tournament-linux
make eventloop-tournament-windows
```

**Execution Outputs:**

Executing these targets will output the raw logs directly to the root of your current repository context:
* `eventloop-tournament-darwin.log`
* `eventloop-tournament-linux.log`
* `eventloop-tournament-windows.log`

**Notes on flags utilized under the hood by the Make targets**:

- `-bench=.` runs all benchmarks (matches all benchmark functions)
- `-benchmem` enables memory allocation reporting (B/op, allocs/op)
- `-count=5` runs each benchmark 5 times for statistical validity
- `-run=^$` ensures no unit tests run (only benchmarks)
- `-benchtime=1s` provides consistent timing across runs
- `-timeout=10m` prevents hanging on slow platforms

### Phase 2: Data Parsing

Create a timestamped directory for the current tournament, copy the required python scripts into it, and execute the parsers against the `.log` files generated in Phase 1.

```bash
# Step 1: Create directory for this tournament (Adjust path based on Context A vs B)
TOURNAMENT_DATE=$(date +%Y-%m-%d)
mkdir -p eventloop/docs/tournament/$TOURNAMENT_DATE

# Step 2: Copy the necessary Python scripts from a previous run or template
# (Assuming copying from the directory root into the new date dir)
cp eventloop/docs/tournament/parse_benchmarks.py eventloop/docs/tournament/$TOURNAMENT_DATE/
cp eventloop/docs/tournament/analyze_2platform.py eventloop/docs/tournament/$TOURNAMENT_DATE/
cp eventloop/docs/tournament/analyze_3platform.py eventloop/docs/tournament/$TOURNAMENT_DATE/

# Step 3: Enter the directory and parse the logs
cd eventloop/docs/tournament/$TOURNAMENT_DATE

# NOTE: Adjust the `../` depth depending on your Execution Context (A vs B)
python3 parse_benchmarks.py ../../../../eventloop-tournament-darwin.log darwin darwin arm64 > darwin.json
python3 parse_benchmarks.py ../../../../eventloop-tournament-linux.log linux linux arm64 > linux.json
python3 parse_benchmarks.py ../../../../eventloop-tournament-windows.log windows windows amd64 > windows.json
```

The parser produces structured JSON with per-benchmark metrics: ns/op (median, mean, stddev), B/op, allocs/op, and coefficient of variation.

### Phase 3: Analysis & Meta-Analysis

Run the analysis scripts to generate markdown reports (`comparison.md`, `comparison-3platform.md`):

```bash
# 3-platform analysis (Darwin vs Linux vs Windows)
python3 analyze_3platform.py

# 2-platform analysis (Darwin vs Linux only)
python3 analyze_2platform.py

# With cross-tournament comparison
python3 analyze_3platform.py --compare-to ../2026-04-19

# Cross-tournament 2-platform comparison
python3 analyze_2platform.py --compare-to ../2026-04-19
```

#### Leveraging the `adversarial-codebase-autopsy` Skill
To ensure this tournament operates as a robust anti-regression tool, you must leverage the **`adversarial-codebase-autopsy`** Skill to perform a meaningful meta-analysis on the generated data. This skill should be used to scrutinize the results across multiple distinct levels and angles:

1. **Simple Past vs. Current Performance:** Compare the current run against previous tournament data *on the exact same platform* to detect algorithmic or allocation regressions.
2. **Cross-OS Comparisons:** Evaluate how identical hardware handles the runtime across different operating systems (e.g., native macOS vs. Linux via Docker on the same machine).
3. **Cross-Machine Comparisons:** Compare results across physically different test benches if available in historical data.
4. **Cross-Architecture Comparisons:** Evaluate ARM64 (Darwin/Linux) versus AMD64/x86_64 (Windows).

> **CRITICAL SCIENTIFIC NOTE FOR PAST COMPARISONS:**
> If the underlying hardware or the Go version differs *at all* from when a past tournament was executed, comparing historical data directly to current data is scientifically invalid. In these cases, the correct approach to check for regressions is to **check out the old code** and re-run the benchmark tournament on the **current hardware** with the **current Go version**.

## File Structure

```
eventloop/docs/tournament/
├── README.md                          # This file
├── parse_benchmarks.py                # Source benchmark log parser
├── analyze_2platform.py               # Source 2-platform comparison script
├── analyze_3platform.py               # Source 3-platform comparison script
└── YYYY-MM-DD/                        # Tournament date directory
    ├── README.md                      # Per-tournament documentation (optional)
    ├── darwin.json                    # Darwin benchmark results
    ├── linux.json                     # Linux benchmark results
    ├── windows.json                   # Windows benchmark results
    ├── comparison.md                  # 2-platform analysis (Darwin vs Linux)
    ├── comparison-3platform.md        # 3-platform analysis
    ├── parse_benchmarks.py            # Copied parser script for execution
    ├── analyze_2platform.py           # Copied 2-platform script for execution
    └── analyze_3platform.py           # Copied 3-platform script for execution
```

## Interpretation Guide

### Metrics

| Metric | Meaning |
|--------|---------|
| `ns/op` | Nanoseconds per operation — primary latency measure |
| `B/op` | Bytes allocated per operation — memory efficiency |
| `allocs/op` | Allocation count per operation — GC pressure indicator |
| `median` | 50th percentile of 5 runs |
| `mean` | Arithmetic mean of 5 runs |
| `stddev` | Standard deviation |
| `cv%` | Coefficient of variation (stddev/mean × 100) — stability indicator |

### Statistical Significance

- **Welch's t-test** is used to compare benchmark runs
- **p < 0.05** means the difference is statistically significant (95%+ confidence)
- Results marked with `*` or `significant` have p < 0.05
- **nsd** (no significant difference) means p >= 0.05 — the platforms perform similarly

### Coefficient of Variation (CV%)

| CV% Range | Interpretation |
|-----------|----------------|
| < 5% | Excellent stability — benchmark is highly repeatable |
| 5–15% | Normal variation — typical for system-level benchmarks |
| 15–30% | High variation — results should be viewed cautiously |
| > 30% | Unstable — investigate for background processes or thermal throttling |

Benchmarks with CV% > 30% are flagged in the analysis output.

### Platform Comparison Caveats

- **Different hardware**: Darwin (Apple Silicon or Intel Mac), Linux (Docker container), Windows (likely different CPU) have different microarchitectures
- **Different schedulers**: Go's goroutine scheduler behaves differently across platforms
- **System load**: Benchmarks should be run on quiescent systems
- **Thermal throttling**: Sustained benchmark runs may trigger throttling on mobile chips
- **Container overhead**: Linux benchmarks run in Docker may have ~1-2% overhead vs bare metal

Direct ns/op comparison across platforms is only meaningful when hardware is similar. Focus on relative ordering and significant deltas, not absolute numbers.

## Troubleshooting

### Missing Benchmarks

If the benchmark count is less than 108:

```bash
# Check what benchmarks exist
go test -bench=. -list='^$' -run=^$ . 2>/dev/null | grep Benchmark

# Common causes:
# - New benchmark code not yet committed
# - Build tags exclude some benchmarks on certain platforms
# - Test file not imported in benchmark_test.go
```

### Parse Failures

If `parse_benchmarks.py` fails:

```bash
# Verify log format — should contain BenchmarkXxx lines
head -50 build.benchmark.darwin.log

# Check for incomplete runs (no "PASS" at end)
tail -20 build.benchmark.darwin.log
```

The parser expects the standard Go test output format. Incomplete runs (killed, timed out) produce partial output that causes parse errors.

### Cross-Platform Benchmark Mismatch

Different platforms may report slightly different benchmark counts due to:

- Platform-specific build tags (`//go:build darwin`, `//go:build linux`, `//go:build windows`)
- Conditional compilation (`#ifdef _WIN32` etc.)
- Timers that are unavailable on certain platforms

A mismatch of 1-3 benchmarks is normal. Larger gaps require investigation.

### Windows Access Failures

```bash
# Test Windows access directly
hack/run-on-windows.sh echo "test"  # Should print "test"

# If permission denied, verify:
# - SSH key configured for Windows host
# - Windows host is reachable
# - go/test executables exist on Windows path
```

### High CV% on All Platforms

If all benchmarks show high variation (>15% CV):

- System under load — close other applications
- Thermal throttling — ensure adequate cooling
- Run on a quieter system or at cooler ambient temperature
- Increase `-benchtime=1s` to `-benchtime=3s` for longer sampling
