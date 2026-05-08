#!/usr/bin/env python3
"""Generate a descriptive Darwin/Linux/Windows tournament report.

The report preserves stable benchmark identities and coverage differences. It
does not claim statistical significance; use repository-pinned benchstat over
raw same-platform logs for longitudinal inference.
"""

import argparse
import json
import re
from pathlib import Path


SCRIPT_DIR = Path(__file__).parent
GO_RELEASE_RE = re.compile(r'^go version (go\S+) ')


def load_benchmark_data(filepath):
    with open(filepath, encoding='utf-8') as source:
        return json.load(source)


def platform_label(data, fallback):
    goos = data.get('goos') or data.get('platform') or fallback.lower()
    goarch = data.get('goarch') or 'unknown-arch'
    return f'{fallback} ({goos}/{goarch})'


def normalize_benchmark_name(name):
    return re.sub(r'-\d+$', '', name)


def extract_benchmark_summary(data):
    """Extract rows by package and stable longitudinal benchmark identity."""
    summary = {}
    for benchmark in data['benchmarks']:
        stable_name = benchmark.get('stable_name') or normalize_benchmark_name(
            benchmark['name']
        )
        name = f"{benchmark.get('package') or '<unknown-package>'}::{stable_name}"
        statistics = benchmark['statistics']
        summary[name] = {
            'ns_op_mean': statistics['ns_op']['mean'],
            'ns_op_stddev': statistics['ns_op']['stddev'],
            'ns_op_min': statistics['ns_op']['min'],
            'ns_op_max': statistics['ns_op']['max'],
            'b_op_mean': statistics['b_op']['mean'],
            'allocs_op_mean': statistics['allocs_op']['mean'],
            'ns_per_op': statistics['ns_op']['mean'],
            'ns_op_runs': [run['ns_op'] for run in benchmark['runs']],
        }
    return summary


def calculate_coefficient_of_variation(mean, stddev):
    return (stddev / mean * 100.0) if mean else 0.0


def require_validated(data, label):
    if not data.get('validated'):
        raise ValueError(f'{label} is not a validated schema-v2 tournament result')
    if data.get('evidence_class') != 'canonical':
        raise ValueError(f'{label} is not canonical tournament evidence')


def go_release(data):
    """Return the toolchain release without its platform-specific suffix."""
    match = GO_RELEASE_RE.match(data.get('source', {}).get('go-version', ''))
    return match.group(1) if match else None


def cross_platform_issues(data_by_platform):
    """Return source defects that invalidate a three-platform evidence join."""
    expected = {'Darwin': 'darwin', 'Linux': 'linux', 'Windows': 'windows'}
    if set(data_by_platform) != set(expected):
        return ['platform set is not exactly Darwin, Linux, and Windows']
    issues = []
    for platform, goos in expected.items():
        if data_by_platform[platform].get('goos') != goos:
            issues.append(f'{platform} GOOS is not {goos}')

    def compare(label, values):
        if any(value is None or value == '' for value in values):
            issues.append(f'{label} is missing')
        elif len(set(values)) != 1:
            issues.append(f'{label} differs')

    datasets = [data_by_platform[platform] for platform in expected]
    compare('effective sample count', [data.get('effective_sample_count') for data in datasets])
    compare('Go release', [go_release(data) for data in datasets])
    for key, label in (
        ('head', 'source HEAD'),
        ('source-state', 'source state'),
        ('source-fingerprint', 'source fingerprint'),
        ('sample-count', 'source sample count'),
        ('goja-fork-version', 'Goja fork version'),
        ('goja-nodejs-version', 'goja_nodejs version'),
    ):
        compare(label, [data.get('source', {}).get(key) for data in datasets])
    for key, label in (
        ('schema_version', 'manifest schema'),
        ('sha256', 'manifest SHA-256'),
        ('git_blob', 'manifest Git blob'),
        ('sample_count', 'manifest sample count'),
    ):
        compare(label, [data.get('manifest', {}).get(key) for data in datasets])
    return issues


def source_summary(data):
    source = data.get('source', {})
    cpu = ', '.join(data.get('cpu') or ['unknown'])
    manifest = data.get('manifest', {})
    return (
        f"head `{source.get('head', 'unknown')}`, state "
        f"`{source.get('source-state', 'unknown')}`, fingerprint "
        f"`{source.get('source-fingerprint', 'unknown')}`, Go "
        f"`{source.get('go-version', 'unknown')}`, Goja "
        f"`{source.get('goja-fork-version', 'unknown')}`, goja_nodejs "
        f"`{source.get('goja-nodejs-version', 'unknown')}`, CPU `{cpu}`, "
        f"manifest `{manifest.get('sha256', 'unknown')}`"
    )


def compatibility_issues(current, past):
    issues = []
    if current.get('goos') != past.get('goos'):
        issues.append('GOOS')
    if current.get('goarch') != past.get('goarch'):
        issues.append('goarch')
    if current.get('cpu') != past.get('cpu'):
        issues.append('CPU identity')
    if current.get('source', {}).get('go-version') != past.get('source', {}).get('go-version'):
        issues.append('Go version')
    for key, label in (
        ('goja-fork-version', 'Goja fork version'),
        ('goja-nodejs-version', 'goja_nodejs version'),
    ):
        if current.get('source', {}).get(key) != past.get('source', {}).get(key):
            issues.append(label)
    if current.get('manifest', {}).get('sha256') != past.get('manifest', {}).get('sha256'):
        issues.append('manifest')
    if current.get('effective_sample_count') != past.get('effective_sample_count'):
        issues.append('effective sample count')
    return issues


def append_longitudinal(lines, platform, current_data, past_data, past_name):
    lines.extend(['', f'## {platform} longitudinal observations vs {past_name}', ''])
    issues = compatibility_issues(current_data, past_data)
    if issues:
        lines.append(
            f"Comparison refused because {', '.join(issues)} differs. Re-run both "
            'revisions under one controlled environment.'
        )
        return
    current = extract_benchmark_summary(current_data)
    past = extract_benchmark_summary(past_data)
    rows = []
    for name in set(current) & set(past):
        before = past[name]['ns_op_mean']
        after = current[name]['ns_op_mean']
        percent = ((after - before) / before * 100.0) if before else 0.0
        rows.append((abs(percent), name, before, after, percent))
    rows.sort(reverse=True)
    lines.extend([
        f'{len(rows)} stable identities overlap. Deltas are descriptive means only.',
        '',
        '| Benchmark | Previous ns/op | Current ns/op | Observed delta |',
        '|---|---:|---:|---:|',
    ])
    for _, name, before, after, percent in rows[:30]:
        lines.append(f'| `{name}` | {before:,.2f} | {after:,.2f} | {percent:+.1f}% |')


def generate(data_by_platform, compare_to=None):
    for platform, data in data_by_platform.items():
        require_validated(data, f'{platform.lower()}.json')
    issues = cross_platform_issues(data_by_platform)
    if issues:
        raise ValueError(
            'three-platform evidence has incompatible provenance: ' + ', '.join(issues)
        )

    summaries = {
        platform: extract_benchmark_summary(data)
        for platform, data in data_by_platform.items()
    }
    identity_sets = [set(summary) for summary in summaries.values()]
    common = sorted(set.intersection(*identity_sets))
    union = set.union(*identity_sets)
    labels = {
        platform: platform_label(data_by_platform[platform], platform)
        for platform in data_by_platform
    }
    lines = [
        '# Eventloop Tournament: Three-Platform Observations',
        '',
        'This is a descriptive report. Absolute cross-platform ratios combine '
        'operating-system, architecture, runtime, and machine effects. They are not '
        'regression findings or statistical significance claims.',
        '',
    ]
    for platform in ('Darwin', 'Linux', 'Windows'):
        lines.append(
            f'- **{labels[platform]}:** {source_summary(data_by_platform[platform])}'
        )

    lines.extend([
        '',
        '## Coverage',
        '',
        f'- Stable identities present on all three platforms: {len(common)}',
        f'- Stable identities present on at least one platform: {len(union)}',
        '',
        '| Platform | Result identities | Missing from union |',
        '|---|---:|---:|',
    ])
    for platform in ('Darwin', 'Linux', 'Windows'):
        lines.append(
            f'| {labels[platform]} | {len(summaries[platform])} | '
            f'{len(union - set(summaries[platform]))} |'
        )

    lines.extend([
        '',
        '## Shared benchmark observations',
        '',
        '| Benchmark | Darwin ns/op | Linux ns/op | Windows ns/op | Fastest observed |',
        '|---|---:|---:|---:|---|',
    ])
    for name in common:
        values = {
            platform: summaries[platform][name]['ns_op_mean']
            for platform in ('Darwin', 'Linux', 'Windows')
        }
        fastest = min(values, key=values.get)
        lines.append(
            f"| `{name}` | {values['Darwin']:,.2f} | {values['Linux']:,.2f} | "
            f"{values['Windows']:,.2f} | {fastest} |"
        )

    lines.extend([
        '',
        '## Platform-only coverage',
        '',
        'Rows absent on one platform are reported as missing coverage, never as an '
        'infinite slowdown or a win for another platform.',
    ])
    for platform in ('Darwin', 'Linux', 'Windows'):
        only = sorted(set(summaries[platform]) - set.union(*[
            set(summaries[other])
            for other in summaries if other != platform
        ]))
        lines.extend(['', f'### {platform}-only', ''])
        if only:
            lines.extend(f'- `{name}`' for name in only)
        else:
            lines.append('- None')

    lines.extend([
        '',
        '## Statistical comparison',
        '',
        'Run old and new code on the same platform and hardware, preserve both raw '
        'logs, and invoke:',
        '',
        '```bash',
        'gmake eventloop-tournament-compare OLD_LOG=old.log NEW_LOG=new.log',
        '```',
        '',
        'That target uses the pinned `benchstat`; this report intentionally does not '
        'approximate its statistical implementation.',
    ])

    if compare_to is not None:
        for platform in ('Darwin', 'Linux', 'Windows'):
            past_path = compare_to / f'{platform.lower()}.json'
            past = load_benchmark_data(past_path)
            require_validated(past, str(past_path))
            append_longitudinal(
                lines, platform, data_by_platform[platform], past, compare_to.name
            )
    return '\n'.join(lines) + '\n'


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--compare-to', type=Path)
    args = parser.parse_args()
    try:
        data = {
            platform: load_benchmark_data(SCRIPT_DIR / f'{platform.lower()}.json')
            for platform in ('Darwin', 'Linux', 'Windows')
        }
        report = generate(data, args.compare_to)
    except (OSError, json.JSONDecodeError, ValueError) as error:
        parser.error(str(error))
    output = SCRIPT_DIR / 'comparison-3platform.md'
    output.write_text(report, encoding='utf-8')
    print(f'Generated {output}')


if __name__ == '__main__':
    main()
