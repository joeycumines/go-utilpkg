#!/usr/bin/env python3
"""
2-Platform Benchmark Comparison: Darwin vs Linux
Generates comparison.md from darwin.json and linux.json
Both platforms are ARM64, making this a pure OS-level comparison.
"""

import json
import math
import re
import statistics as stats_mod
from collections import defaultdict
from pathlib import Path
from typing import Dict, List, Tuple, Any

SCRIPT_DIR = Path(__file__).parent

def load(name: str) -> Dict[str, Any]:
    with open(SCRIPT_DIR / name) as f:
        return json.load(f)

def norm(name: str) -> str:
    return re.sub(r'-\d+$', '', name)

def cv(mean: float, sd: float) -> float:
    return (sd / mean * 100.0) if mean != 0 else 0.0

def extract(data: Dict[str, Any]) -> Dict[str, Dict[str, Any]]:
    out = {}
    for b in data['benchmarks']:
        n = norm(b['name'])
        s = b['statistics']
        out[n] = {
            'mean': s['ns_op']['mean'],
            'sd': s['ns_op']['stddev'],
            'min': s['ns_op']['min'],
            'max': s['ns_op']['max'],
            'b_op': s['b_op']['mean'],
            'allocs': s['allocs_op']['mean'],
            'runs': [r['ns_op'] for r in b['runs']],
            'b_runs': [r['b_op'] for r in b['runs']],
            'a_runs': [r['allocs_op'] for r in b['runs']],
        }
    return out

def welch_t(g1: List[float], g2: List[float]) -> Tuple[float, float, bool]:
    """Returns (t_stat, degrees_of_freedom, significant_at_95pct)"""
    if len(g1) < 2 or len(g2) < 2:
        return 0.0, 0.0, False
    m1, m2 = stats_mod.mean(g1), stats_mod.mean(g2)
    v1, v2 = stats_mod.variance(g1), stats_mod.variance(g2)
    n1, n2 = len(g1), len(g2)
    se = math.sqrt(v1/n1 + v2/n2)
    if se == 0:
        return 0.0, 0.0, False
    t = abs(m1 - m2) / se
    num = (v1/n1 + v2/n2)**2
    den = (v1/n1)**2/(n1-1) + (v2/n2)**2/(n2-1)
    df = num / den if den != 0 else 0
    crit = 2.776 if df < 30 else 1.96
    return t, df, t > crit

def fmt_ns(v: float) -> str:
    if v >= 1_000_000:
        return f"{v/1_000_000:,.2f} ms"
    if v >= 1_000:
        return f"{v/1_000:,.2f} µs"
    return f"{v:,.2f} ns"

def categorize(name: str) -> str:
    nl = name.lower()
    if 'latency' in nl or 'directcall' in nl or 'stateload' in nl:
        return 'Latency & Primitives'
    if 'timer' in nl or 'cancel' in nl:
        return 'Timer Operations'
    if 'promise' in nl or 'chain' in nl:
        return 'Promise Operations'
    if 'submit' in nl or 'fastpath' in nl or 'microtask' in nl or 'ingress' in nl or 'chunked' in nl:
        return 'Task Submission'
    if 'contention' in nl or 'parallel' in nl or 'concurrent' in nl:
        return 'Concurrency'
    return 'Other'

def main():
    darwin_data = load('darwin.json')
    linux_data = load('linux.json')

    darwin = extract(darwin_data)
    linux = extract(linux_data)

    common = sorted(set(darwin.keys()) & set(linux.keys()))
    darwin_only = sorted(set(darwin.keys()) - set(linux.keys()))
    linux_only = sorted(set(linux.keys()) - set(darwin.keys()))

    # --- Compute summary stats ---
    darwin_wins = 0
    linux_wins = 0
    ties = 0
    sig_diffs = []
    all_ratios = []

    for name in common:
        d, l = darwin[name], linux[name]
        if d['mean'] < l['mean']:
            darwin_wins += 1
        elif l['mean'] < d['mean']:
            linux_wins += 1
        else:
            ties += 1
        ratio = d['mean'] / l['mean'] if l['mean'] != 0 else float('inf')
        all_ratios.append(ratio)
        t_stat, df, sig = welch_t(d['runs'], l['runs'])
        if sig:
            faster = 'Darwin' if d['mean'] < l['mean'] else 'Linux'
            speedup = max(d['mean'], l['mean']) / min(d['mean'], l['mean']) if min(d['mean'], l['mean']) > 0 else 0
            sig_diffs.append((name, faster, speedup, t_stat, df, d['mean'], l['mean']))

    darwin_mean_all = stats_mod.mean([darwin[n]['mean'] for n in common])
    linux_mean_all = stats_mod.mean([linux[n]['mean'] for n in common])

    # --- Allocation analysis ---
    alloc_match = 0
    alloc_mismatch = []
    bop_match = 0
    bop_mismatch = []
    zero_alloc_both = []

    for name in common:
        d, l = darwin[name], linux[name]
        if d['allocs'] == l['allocs']:
            alloc_match += 1
        else:
            alloc_mismatch.append((name, d['allocs'], l['allocs']))
        if d['b_op'] == l['b_op']:
            bop_match += 1
        else:
            bop_mismatch.append((name, d['b_op'], l['b_op']))
        if d['allocs'] == 0 and l['allocs'] == 0:
            zero_alloc_both.append(name)

    # --- Categorize ---
    categories = defaultdict(list)
    for name in common:
        cat = categorize(name)
        categories[cat].append(name)

    # --- Build report ---
    lines = []

    def w(s=''):
        lines.append(s)

    w('# Darwin vs Linux Benchmark Comparison')
    w()
    w('**Date:** 2026-02-10')
    w(f'**Platforms:** Darwin ARM64 (macOS, GOMAXPROCS=10) vs Linux ARM64 (container, GOMAXPROCS=10)')
    w(f'**Methodology:** `go test -bench=. -benchmem -count=5 -run=^$ -benchtime=1s -timeout=10m`')
    w(f'**Benchmarks Compared:** {len(common)} common benchmarks')
    w()

    # --- Executive Summary ---
    w('## Executive Summary')
    w()
    w('This report compares eventloop benchmark performance between **Darwin (macOS)** and **Linux**,')
    w('both running on **ARM64** architecture. Since the architecture is identical, performance')
    w('differences reflect OS-level differences: kernel scheduling, memory management, syscall')
    w('overhead, and Go runtime behavior on each OS.')
    w()
    w('### Key Metrics')
    w()
    w('| Metric | Value |')
    w('|--------|-------|')
    w(f'| Common benchmarks | {len(common)} |')
    w(f'| Darwin-only benchmarks | {len(darwin_only)} |')
    w(f'| Linux-only benchmarks | {len(linux_only)} |')
    w(f'| Darwin wins (faster) | **{darwin_wins}** ({darwin_wins/len(common)*100:.1f}%) |')
    w(f'| Linux wins (faster) | **{linux_wins}** ({linux_wins/len(common)*100:.1f}%) |')
    w(f'| Ties | {ties} |')
    w(f'| Statistically significant differences | {len(sig_diffs)} |')
    w(f'| Darwin mean (common benchmarks) | {darwin_mean_all:,.2f} ns/op |')
    w(f'| Linux mean (common benchmarks) | {linux_mean_all:,.2f} ns/op |')
    w(f'| Mean ratio (Darwin/Linux) | {stats_mod.mean(all_ratios):.3f}x |')
    w(f'| Median ratio (Darwin/Linux) | {stats_mod.median(all_ratios):.3f}x |')
    w(f'| Allocation match rate | {alloc_match}/{len(common)} ({alloc_match/len(common)*100:.1f}%) |')
    w(f'| Zero-allocation benchmarks (both) | {len(zero_alloc_both)} |')
    w()

    # --- Full Comparison Table ---
    w('## Full Statistical Comparison Table')
    w()
    w('| # | Benchmark | Darwin (ns/op) | Darwin CV% | Linux (ns/op) | Linux CV% | Faster | Ratio | Sig? |')
    w('|---|-----------|----------------|------------|---------------|-----------|--------|-------|------|')

    for i, name in enumerate(common, 1):
        d, l = darwin[name], linux[name]
        cv_d = cv(d['mean'], d['sd'])
        cv_l = cv(l['mean'], l['sd'])
        faster = '🍎 Darwin' if d['mean'] < l['mean'] else ('🐧 Linux' if l['mean'] < d['mean'] else 'Tie')
        ratio = d['mean'] / l['mean'] if l['mean'] != 0 else 0
        _, _, sig = welch_t(d['runs'], l['runs'])
        sig_mark = '✅' if sig else ''
        short_name = name[:55]
        w(f'| {i} | {short_name} | {d["mean"]:>14,.2f} | {cv_d:>9.1f}% | {l["mean"]:>13,.2f} | {cv_l:>8.1f}% | {faster} | {ratio:>5.2f}x | {sig_mark} |')

    w()

    # --- Category Analysis ---
    w('## Performance by Category')
    w()

    for cat in sorted(categories.keys()):
        names = categories[cat]
        w(f'### {cat} ({len(names)} benchmarks)')
        w()

        cat_d_wins = sum(1 for n in names if darwin[n]['mean'] < linux[n]['mean'])
        cat_l_wins = sum(1 for n in names if linux[n]['mean'] < darwin[n]['mean'])
        cat_d_mean = stats_mod.mean([darwin[n]['mean'] for n in names])
        cat_l_mean = stats_mod.mean([linux[n]['mean'] for n in names])

        w(f'- Darwin wins: {cat_d_wins}/{len(names)}')
        w(f'- Linux wins: {cat_l_wins}/{len(names)}')
        w(f'- Darwin category mean: {cat_d_mean:,.2f} ns/op')
        w(f'- Linux category mean: {cat_l_mean:,.2f} ns/op')
        w()

        w('| Benchmark | Darwin (ns/op) | Linux (ns/op) | Faster | Ratio |')
        w('|-----------|----------------|---------------|--------|-------|')
        for n in sorted(names, key=lambda n: abs(darwin[n]['mean'] - linux[n]['mean']), reverse=True):
            d, l = darwin[n], linux[n]
            faster = '🍎' if d['mean'] < l['mean'] else '🐧'
            ratio = max(d['mean'], l['mean']) / min(d['mean'], l['mean']) if min(d['mean'], l['mean']) > 0 else 0
            w(f'| {n[:55]} | {d["mean"]:>14,.2f} | {l["mean"]:>13,.2f} | {faster} | {ratio:.2f}x |')
        w()

    # --- Statistically Significant Differences ---
    w('## Statistically Significant Differences')
    w()
    w(f'**{len(sig_diffs)}** out of {len(common)} benchmarks show statistically significant')
    w(f'differences (Welch\'s t-test, p < 0.05).')
    w()

    if sig_diffs:
        sig_sorted = sorted(sig_diffs, key=lambda x: x[2], reverse=True)

        darwin_sig_wins = sum(1 for s in sig_diffs if s[1] == 'Darwin')
        linux_sig_wins = sum(1 for s in sig_diffs if s[1] == 'Linux')
        w(f'- Darwin significantly faster: **{darwin_sig_wins}** benchmarks')
        w(f'- Linux significantly faster: **{linux_sig_wins}** benchmarks')
        w()

        w('### Largest Significant Differences')
        w()
        w('| Benchmark | Faster | Speedup | Darwin (ns/op) | Linux (ns/op) | t-stat |')
        w('|-----------|--------|---------|----------------|---------------|--------|')
        for name, faster, speedup, t_stat, df, d_mean, l_mean in sig_sorted[:30]:
            emoji = '🍎' if faster == 'Darwin' else '🐧'
            w(f'| {name[:50]} | {emoji} {faster} | {speedup:.2f}x | {d_mean:>14,.2f} | {l_mean:>13,.2f} | {t_stat:.2f} |')
        w()

    # --- Top 10 lists ---
    w('## Top 10 Fastest Benchmarks')
    w()
    for platform_name, data in [('Darwin', darwin), ('Linux', linux)]:
        w(f'### {platform_name}')
        w()
        w('| Rank | Benchmark | ns/op | B/op | Allocs/op | CV% |')
        w('|------|-----------|-------|------|-----------|-----|')
        sorted_b = sorted([(n, data[n]) for n in common], key=lambda x: x[1]['mean'])
        for i, (n, d) in enumerate(sorted_b[:10], 1):
            c = cv(d['mean'], d['sd'])
            w(f'| {i} | {n[:50]} | {d["mean"]:>10,.2f} | {d["b_op"]:>4,.0f} | {d["allocs"]:>9,.0f} | {c:.1f}% |')
        w()

    # --- Allocation comparison ---
    w('## Allocation Comparison')
    w()
    w('Since both platforms run the same Go code, allocations (allocs/op) and bytes (B/op)')
    w('should be identical. Differences indicate platform-specific runtime behavior.')
    w()
    w(f'- **Allocs/op match:** {alloc_match}/{len(common)} ({alloc_match/len(common)*100:.1f}%)')
    w(f'- **B/op match:** {bop_match}/{len(common)} ({bop_match/len(common)*100:.1f}%)')
    w(f'- **Zero-allocation benchmarks (both platforms):** {len(zero_alloc_both)}')
    w()

    if zero_alloc_both:
        w('### Zero-Allocation Benchmarks')
        w()
        w('These benchmarks achieve zero allocations on both platforms — the gold standard')
        w('for hot-path performance:')
        w()
        for n in sorted(zero_alloc_both):
            d, l = darwin[n], linux[n]
            faster = '🍎' if d['mean'] < l['mean'] else '🐧'
            w(f'- `{n}` — Darwin: {d["mean"]:.2f} ns/op, Linux: {l["mean"]:.2f} ns/op {faster}')
        w()

    if alloc_mismatch:
        w('### Allocation Mismatches')
        w()
        w('| Benchmark | Darwin allocs | Linux allocs | Δ |')
        w('|-----------|---------------|--------------|---|')
        for n, d_a, l_a in sorted(alloc_mismatch):
            w(f'| {n[:55]} | {d_a:,.0f} | {l_a:,.0f} | {abs(d_a - l_a):,.0f} |')
        w()

    if bop_mismatch:
        w('### B/op Mismatches')
        w()
        w('| Benchmark | Darwin B/op | Linux B/op | Δ |')
        w('|-----------|-------------|------------|---|')
        for n, d_b, l_b in sorted(bop_mismatch):
            w(f'| {n[:55]} | {d_b:,.0f} | {l_b:,.0f} | {abs(d_b - l_b):,.0f} |')
        w()

    # --- Stability / Consistency ---
    w('## Measurement Stability')
    w()
    w('Coefficient of variation (CV%) indicates measurement consistency. Lower is better.')
    w()

    high_cv_d = [(n, cv(darwin[n]['mean'], darwin[n]['sd'])) for n in common if cv(darwin[n]['mean'], darwin[n]['sd']) > 5]
    high_cv_l = [(n, cv(linux[n]['mean'], linux[n]['sd'])) for n in common if cv(linux[n]['mean'], linux[n]['sd']) > 5]
    low_cv_both = sum(1 for n in common if cv(darwin[n]['mean'], darwin[n]['sd']) < 2 and cv(linux[n]['mean'], linux[n]['sd']) < 2)

    w(f'- Benchmarks with CV < 2% on both platforms: **{low_cv_both}**')
    w(f'- Darwin benchmarks with CV > 5%: **{len(high_cv_d)}**')
    w(f'- Linux benchmarks with CV > 5%: **{len(high_cv_l)}**')
    w()

    if high_cv_d or high_cv_l:
        w('### High-Variance Benchmarks (CV > 5%)')
        w()
        all_high = set(n for n, _ in high_cv_d) | set(n for n, _ in high_cv_l)
        w('| Benchmark | Darwin CV% | Linux CV% |')
        w('|-----------|------------|-----------|')
        for n in sorted(all_high):
            c_d = cv(darwin[n]['mean'], darwin[n]['sd'])
            c_l = cv(linux[n]['mean'], linux[n]['sd'])
            flag_d = ' ⚠️' if c_d > 5 else ''
            flag_l = ' ⚠️' if c_l > 5 else ''
            w(f'| {n[:55]} | {c_d:.1f}%{flag_d} | {c_l:.1f}%{flag_l} |')
        w()

    # --- Key Findings ---
    w('## Key Findings')
    w()
    w('### 1. Architecture Parity')
    w()
    w('Both platforms run ARM64, eliminating architectural differences. Performance gaps')
    w('are attributable to:')
    w('- **OS kernel scheduling** (macOS Mach scheduler vs Linux CFS)')
    w('- **Memory management** (macOS memory pressure vs Linux cgroups in container)')
    w('- **Syscall overhead** differences')
    w('- **Go runtime behavior** variations between `darwin/arm64` and `linux/arm64`')
    w()

    w('### 2. Performance Distribution')
    w()
    ratio_lt1 = sum(1 for r in all_ratios if r < 0.9)
    ratio_near1 = sum(1 for r in all_ratios if 0.9 <= r <= 1.1)
    ratio_gt1 = sum(1 for r in all_ratios if r > 1.1)
    w(f'- Darwin significantly faster (ratio < 0.9): **{ratio_lt1}** benchmarks')
    w(f'- Roughly equal (0.9–1.1x): **{ratio_near1}** benchmarks')
    w(f'- Linux significantly faster (ratio > 1.1): **{ratio_gt1}** benchmarks')
    w()

    w('### 3. Timer Operations')
    w()
    timer_names = [n for n in common if 'timer' in n.lower() or 'cancel' in n.lower()]
    if timer_names:
        timer_d_wins = sum(1 for n in timer_names if darwin[n]['mean'] < linux[n]['mean'])
        timer_l_wins = len(timer_names) - timer_d_wins
        w(f'- Total timer benchmarks: {len(timer_names)}')
        w(f'- Darwin faster: {timer_d_wins}')
        w(f'- Linux faster: {timer_l_wins}')
        biggest_timer = max(timer_names, key=lambda n: abs(darwin[n]['mean'] - linux[n]['mean']))
        bt_d, bt_l = darwin[biggest_timer]['mean'], linux[biggest_timer]['mean']
        bigger_is = 'Darwin' if bt_d > bt_l else 'Linux'
        bt_ratio = max(bt_d, bt_l) / min(bt_d, bt_l) if min(bt_d, bt_l) > 0 else 0
        w(f'- Biggest difference: `{biggest_timer}` — {bigger_is} is {bt_ratio:.2f}x slower')
    w()

    w('### 4. Concurrency & Contention')
    w()
    conc_names = [n for n in common if any(x in n.lower() for x in ['contention', 'parallel', 'concurrent', 'submit'])]
    if conc_names:
        for n in conc_names:
            d, l = darwin[n], linux[n]
            faster = '🍎 Darwin' if d['mean'] < l['mean'] else '🐧 Linux'
            ratio = max(d['mean'], l['mean']) / min(d['mean'], l['mean']) if min(d['mean'], l['mean']) > 0 else 0
            w(f'- `{n}`: {faster} ({ratio:.2f}x)')
    w()

    w('### 5. Summary')
    w()
    if darwin_wins > linux_wins:
        w(f'**Darwin wins overall** with {darwin_wins}/{len(common)} benchmarks faster.')
    elif linux_wins > darwin_wins:
        w(f'**Linux wins overall** with {linux_wins}/{len(common)} benchmarks faster.')
    else:
        w(f'**Platforms are evenly matched** at {darwin_wins} wins each.')
    w()
    w(f'The mean performance ratio of {stats_mod.mean(all_ratios):.3f}x (Darwin/Linux) indicates')
    if stats_mod.mean(all_ratios) < 0.95:
        w('Darwin is systematically faster across the board.')
    elif stats_mod.mean(all_ratios) > 1.05:
        w('Linux is systematically faster across the board.')
    else:
        w('the platforms are remarkably close in overall performance, with each')
        w('excelling in different workload categories.')
    w()

    # --- Write output ---
    output_path = SCRIPT_DIR / 'comparison.md'
    with open(output_path, 'w') as f:
        f.write('\n'.join(lines))
        f.write('\n')

    print(f'✅ comparison.md written to {output_path}')
    print(f'   Common benchmarks: {len(common)}')
    print(f'   Darwin wins: {darwin_wins}, Linux wins: {linux_wins}')
    print(f'   Significant differences: {len(sig_diffs)}')

if __name__ == '__main__':
    main()
