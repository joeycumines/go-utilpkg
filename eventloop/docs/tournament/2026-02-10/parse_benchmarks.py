#!/usr/bin/env python3
"""Parse Go benchmark output into structured JSON with statistics.

Usage:
    python3 parse_benchmarks.py <log_file> <platform> <goos> <goarch>

Reads a Go benchmark log file, groups runs by benchmark name, calculates
statistics (mean, min, max, sample stddev) for ns/op, B/op, and allocs/op,
and writes a JSON document to stdout.
"""

import json
import math
import re
import sys
from collections import OrderedDict


# Matches Go benchmark output lines:
#   BenchmarkName-N  <iterations>  <ns_op> ns/op  <b_op> B/op  <allocs_op> allocs/op
BENCH_RE = re.compile(
    r'^(Benchmark\S+)'       # benchmark name (including -N suffix)
    r'\s+'                   # whitespace
    r'(\d+)'                 # iterations
    r'\s+'                   # whitespace
    r'([\d.]+)\s+ns/op'     # ns/op (float)
    r'\s+'                   # whitespace
    r'(\d+)\s+B/op'         # B/op (int)
    r'\s+'                   # whitespace
    r'(\d+)\s+allocs/op'    # allocs/op (int)
)


def parse_log(filepath):
    """Parse benchmark log file and return dict mapping name -> list of runs."""
    benchmarks = OrderedDict()
    with open(filepath, 'r') as f:
        for line in f:
            line = line.strip()
            m = BENCH_RE.match(line)
            if not m:
                continue
            name = m.group(1)
            ns_op_raw = float(m.group(3))
            # Use int when value is a whole number to match Go benchmark format
            ns_op = int(ns_op_raw) if ns_op_raw == int(ns_op_raw) else ns_op_raw
            b_op = int(m.group(4))
            allocs_op = int(m.group(5))
            benchmarks.setdefault(name, []).append({
                'ns_op': ns_op,
                'b_op': b_op,
                'allocs_op': allocs_op,
            })
    return benchmarks


def _num(v):
    """Return int if value is a whole number, float otherwise."""
    if isinstance(v, float) and v == int(v):
        return int(v)
    return v


def compute_statistics(values):
    """Compute mean, min, max, and sample stddev for a list of numbers."""
    n = len(values)
    if n == 0:
        return {'mean': 0, 'min': 0, 'max': 0, 'stddev': 0}
    mean = sum(values) / n
    min_val = min(values)
    max_val = max(values)
    if n < 2:
        stddev = 0.0
    else:
        variance = sum((x - mean) ** 2 for x in values) / (n - 1)
        stddev = math.sqrt(variance)
    return {
        'mean': _num(mean),
        'min': _num(min_val),
        'max': _num(max_val),
        'stddev': _num(stddev),
    }


def build_output(benchmarks, platform, goos, goarch):
    """Build the output JSON structure."""
    # Sort benchmarks alphabetically by name
    sorted_names = sorted(benchmarks.keys())

    bench_list = []
    for name in sorted_names:
        runs = benchmarks[name]
        stats = {
            'allocs_op': compute_statistics([r['allocs_op'] for r in runs]),
            'b_op': compute_statistics([r['b_op'] for r in runs]),
            'ns_op': compute_statistics([r['ns_op'] for r in runs]),
        }
        bench_list.append({
            'name': name,
            'runs': runs,
            'statistics': stats,
        })

    return {
        'platform': platform,
        'goos': goos,
        'goarch': goarch,
        'timestamp': '2026-02-10',
        'benchmarks': bench_list,
    }


def main():
    if len(sys.argv) != 5:
        print(f'Usage: {sys.argv[0]} <log_file> <platform> <goos> <goarch>',
              file=sys.stderr)
        sys.exit(1)

    log_file = sys.argv[1]
    platform = sys.argv[2]
    goos = sys.argv[3]
    goarch = sys.argv[4]

    benchmarks = parse_log(log_file)
    output = build_output(benchmarks, platform, goos, goarch)
    print(json.dumps(output, indent=2))


if __name__ == '__main__':
    main()
