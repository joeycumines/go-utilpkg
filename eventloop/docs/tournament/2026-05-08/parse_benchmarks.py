#!/usr/bin/env python3
"""Parse Go benchmark output into structured JSON with statistics.

Usage:
    python3 parse_benchmarks.py <log_file> <platform> <fallback-goos> <fallback-goarch>

Reads a Go benchmark log file, groups runs by benchmark name, detects `goos:`
and `goarch:` headers when present, calculates
statistics (mean, min, max, sample stddev) for ns/op, B/op, allocs/op, and
any additional numeric metrics reported by b.ReportMetric, and writes a JSON
document to stdout.
"""

import json
import math
import os
import re
import sys
from collections import OrderedDict
from datetime import datetime
from pathlib import Path


# Matches the benchmark name, iteration count, and the remaining metric pairs.
BENCH_LINE_RE = re.compile(
    r'^(Benchmark\S+)'       # benchmark name (including -N suffix)
    r'\s+'                   # whitespace
    r'(\d+)'                 # iterations
    r'\s+'
    r'(.+)$'                 # one or more '<number> <unit>' pairs
)

BENCH_NAME_RE = re.compile(r'^(Benchmark\S+)(?:\s+.*)?$')
METRIC_NUMBER_RE = r'[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?'
METRIC_RE = re.compile(rf'({METRIC_NUMBER_RE})\s+(\S+)')
CONTINUATION_LINE_RE = re.compile(
    r'^(\d+)'                 # iterations
    r'\s+'
    r'(.+)$'                 # one or more '<number> <unit>' pairs
)

METRIC_KEYS = {
    'ns/op': 'ns_op',
    'B/op': 'b_op',
    'allocs/op': 'allocs_op',
}

GOOS_RE = re.compile(r'^goos:\s*(\S+)')
GOARCH_RE = re.compile(r'^goarch:\s*(\S+)')


def get_timestamp_from_log(log_path: str) -> str:
    """
    Auto-detect timestamp from log file modification time.
    Falls back to current date if file doesn't exist.
    """
    if os.path.exists(log_path):
        mtime = os.path.getmtime(log_path)
        dt = datetime.fromtimestamp(mtime)
        return dt.strftime('%Y-%m-%d')
    return datetime.now().strftime('%Y-%m-%d')


def parse_metric_tail(metric_tail):
    """Parse Go benchmark metric pairs from the text after the iteration count."""
    metrics = {}
    for value_raw, unit in METRIC_RE.findall(metric_tail):
        key = METRIC_KEYS.get(unit, unit.replace('/', '_'))
        value = float(value_raw)
        metrics[key] = int(value) if value == int(value) else value
    return metrics


def parse_log(filepath):
    """Parse benchmark log file and return dict mapping name -> list of runs.

    Go's benchmark runner writes result records to stdout, while benchmarked
    code may log diagnostics to stderr. When those streams are merged, a log
    line can be interleaved between the benchmark name and the numeric result,
    leaving the result line as only '<iterations> <metrics...>'. Keep track of
    the last benchmark name whose result was split this way so parsed JSON stays
    faithful to the raw log instead of silently dropping those samples.
    """
    benchmarks = OrderedDict()
    pending_name = None
    with open(filepath, 'r') as f:
        for line in f:
            line = line.strip()
            m = BENCH_LINE_RE.match(line)
            if m:
                name = m.group(1)
                metrics = parse_metric_tail(m.group(3))
                if {'ns_op', 'b_op', 'allocs_op'} - set(metrics):
                    pending_name = name
                    continue
                benchmarks.setdefault(name, []).append(metrics)
                pending_name = None
                continue

            m = BENCH_NAME_RE.match(line)
            if m:
                pending_name = m.group(1)
                continue

            if pending_name is None:
                continue
            m = CONTINUATION_LINE_RE.match(line)
            if not m:
                continue
            metrics = parse_metric_tail(m.group(2))
            if {'ns_op', 'b_op', 'allocs_op'} - set(metrics):
                continue
            benchmarks.setdefault(pending_name, []).append(metrics)
            pending_name = None
    return benchmarks


def detect_log_metadata(filepath):
    """Detect goos/goarch from Go benchmark output headers when available."""
    goos = None
    goarch = None
    with open(filepath, 'r') as f:
        for line in f:
            line = line.strip()
            if goos is None:
                m = GOOS_RE.match(line)
                if m:
                    goos = m.group(1)
            if goarch is None:
                m = GOARCH_RE.match(line)
                if m:
                    goarch = m.group(1)
            if goos is not None and goarch is not None:
                break
    return goos, goarch


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


def build_output(benchmarks, platform, goos, goarch, timestamp):
    """Build the output JSON structure."""
    # Sort benchmarks alphabetically by name
    sorted_names = sorted(benchmarks.keys())

    bench_list = []
    for name in sorted_names:
        runs = benchmarks[name]
        metric_names = sorted({key for run in runs for key in run})
        stats = {
            key: compute_statistics([r[key] for r in runs if key in r])
            for key in metric_names
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
        'timestamp': timestamp,
        'benchmarks': bench_list,
    }


def main():
    if len(sys.argv) != 5:
        print(f'Usage: {sys.argv[0]} <log_file> <platform> <fallback-goos> <fallback-goarch>',
              file=sys.stderr)
        sys.exit(1)

    log_file = sys.argv[1]
    platform = sys.argv[2]
    goos = sys.argv[3]
    goarch = sys.argv[4]
    detected_goos, detected_goarch = detect_log_metadata(log_file)
    if detected_goos:
        goos = detected_goos
    if detected_goarch:
        goarch = detected_goarch

    # Auto-detect timestamp from log file modification time
    timestamp = get_timestamp_from_log(log_file)

    benchmarks = parse_log(log_file)
    output = build_output(benchmarks, platform, goos, goarch, timestamp)
    print(json.dumps(output, indent=2))


if __name__ == '__main__':
    main()
