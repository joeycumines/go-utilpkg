#!/bin/bash
# Tournament Benchmark Script for 2026-02-03
# Captures comprehensive benchmark results for performance analysis

cd /Users/joeyc/dev/go-utilpkg/eventloop

echo "========================================="
echo "Eventloop Tournament Benchmark - 2026-02-03"
echo "========================================="
echo "Date: $(date)"
echo "Platform: $(uname -s) $(uname -m)"
echo "Go Version: $(go version)"
echo "========================================="
echo ""

# Run benchmarks with full instrumentation
echo "Running comprehensive benchmarks..."
echo "Parameters: -bench=. -benchmem -benchtime=2s -timeout=15m"
echo "Log file: /Users/joeyc/dev/go-utilpkg/eventloop/docs/tournament/2026-02-03/benchmark_raw.log"
echo ""

# Capture all output including stderr
go test -bench=. -benchmem -benchtime=2s -timeout=15m ./internal/tournament/... 2>&1 | tee /Users/joeyc/dev/go-utilpkg/eventloop/docs/tournament/2026-02-03/benchmark_raw.log

BENCHMARK_EXIT_CODE=${PIPESTATUS[0]}

echo ""
echo "========================================="
echo "Benchmark run completed with exit code: $BENCHMARK_EXIT_CODE"
echo "========================================="
echo "Raw results saved to: /Users/joeyc/dev/go-utilpkg/eventloop/docs/tournament/2026-02-03/benchmark_raw.log"
echo ""

# Extract and format key metrics
echo "Extracting key metrics..."
echo ""

# Extract PingPong results
echo "=== PINGPONG RESULTS ==="
grep -E "^BenchmarkPingPong" /Users/joeyc/dev/go-utilpkg/eventloop/docs/tournament/2026-02-03/benchmark_raw.log
echo ""

# Extract MultiProducer results  
echo "=== MULTIPRODUCER RESULTS ==="
grep -E "^BenchmarkMultiProducer" /Users/joeyc/dev/go-utilpkg/eventloop/docs/tournament/2026-02-03/benchmark_raw.log
echo ""

# Extract Memory allocation results
echo "=== MEMORY ALLOCATION RESULTS ===
grep -E "B/op|allocs/op" /Users/joeyc/dev/go-utilpkg/eventloop/docs/tournament/2026-02-03/benchmark_raw.log | head -30
echo ""

echo "Benchmark script completed."
