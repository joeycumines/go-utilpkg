#!/usr/bin/env python3
"""Generate a descriptive Darwin/Linux eventloop tournament report.

Inferential comparison belongs to the pinned ``go tool benchstat`` command over
the raw Go benchmark logs. This script reports coverage, central values,
allocations, and observed deltas without inventing p-values or significance.
"""

import argparse
import json
import re
from pathlib import Path


SCRIPT_DIR = Path(__file__).parent
GO_RELEASE_RE = re.compile(r'^go version (go\S+) ')
METHODOLOGY = (
    'complete `eventloop-tournament-bench` target: current product package, '
    '`internal/tournament` scheduler and Promise matrix, '
    '`internal/promisetournament`, and the explicit libuv lane when available'
)


def load(name):
    with open(SCRIPT_DIR / name, encoding='utf-8') as source:
        return json.load(source)


def load_external(path):
    with open(path, encoding='utf-8') as source:
        return json.load(source)


def platform_label(data, fallback):
    goos = data.get('goos') or data.get('platform') or fallback.lower()
    goarch = data.get('goarch') or 'unknown-arch'
    return f'{fallback} ({goos}/{goarch})'


def norm(name):
    return re.sub(r'-\d+$', '', name)


def cv(mean, stddev):
    return (stddev / mean * 100.0) if mean else 0.0


def extract(data):
    """Return benchmark rows keyed by package and stable benchmark identity."""
    output = {}
    for benchmark in data['benchmarks']:
        stable_name = benchmark.get('stable_name') or norm(benchmark['name'])
        identity = f"{benchmark.get('package') or '<unknown-package>'}::{stable_name}"
        statistics = benchmark['statistics']
        output[identity] = {
            'mean': statistics['ns_op']['mean'],
            'sd': statistics['ns_op']['stddev'],
            'min': statistics['ns_op']['min'],
            'max': statistics['ns_op']['max'],
            'b_op': statistics['b_op']['mean'],
            'allocs': statistics['allocs_op']['mean'],
            'runs': [run['ns_op'] for run in benchmark['runs']],
            'lane': benchmark.get('lane'),
            'variant_ids': benchmark.get('variant_ids', []),
        }
    return output


def require_validated(data, label):
    if not data.get('validated'):
        raise ValueError(f'{label} is not a validated schema-v2 tournament result')
    if data.get('evidence_class') != 'canonical':
        raise ValueError(f'{label} is not canonical tournament evidence')


def go_release(data):
    """Return the Go release while allowing the expected GOOS suffix to differ."""
    match = GO_RELEASE_RE.match(data.get('source', {}).get('go-version', ''))
    return match.group(1) if match else None


def cross_platform_issues(darwin, linux):
    """Return provenance defects that make a Darwin/Linux join inadmissible."""
    issues = []
    if darwin.get('goos') != 'darwin':
        issues.append('Darwin GOOS is not darwin')
    if linux.get('goos') != 'linux':
        issues.append('Linux GOOS is not linux')

    def compare(label, first, second):
        if first is None or first == '' or second is None or second == '':
            issues.append(f'{label} is missing')
        elif first != second:
            issues.append(f'{label} differs')

    compare('architecture', darwin.get('goarch'), linux.get('goarch'))
    compare('CPU identity', darwin.get('cpu'), linux.get('cpu'))
    compare(
        'effective sample count',
        darwin.get('effective_sample_count'),
        linux.get('effective_sample_count'),
    )
    compare('Go release', go_release(darwin), go_release(linux))
    for key, label in (
        ('head', 'source HEAD'),
        ('source-state', 'source state'),
        ('source-fingerprint', 'source fingerprint'),
        ('sample-count', 'source sample count'),
        ('goja-fork-version', 'Goja fork version'),
        ('goja-nodejs-version', 'goja_nodejs version'),
    ):
        compare(
            label,
            darwin.get('source', {}).get(key),
            linux.get('source', {}).get(key),
        )
    for key, label in (
        ('schema_version', 'manifest schema'),
        ('sha256', 'manifest SHA-256'),
        ('git_blob', 'manifest Git blob'),
        ('sample_count', 'manifest sample count'),
    ):
        compare(
            label,
            darwin.get('manifest', {}).get(key),
            linux.get('manifest', {}).get(key),
        )
    return issues


def source_notes(data):
    source = data.get('source', {})
    notes = [
        f"head `{source.get('head', 'unknown')}`",
        f"source state `{source.get('source-state', 'unknown')}`",
        f"source fingerprint `{source.get('source-fingerprint', 'unknown')}`",
        f"Go `{source.get('go-version', 'unknown')}`",
        f"Goja `{source.get('goja-fork-version', 'unknown')}`",
        f"goja_nodejs `{source.get('goja-nodejs-version', 'unknown')}`",
    ]
    cpus = data.get('cpu') or ['unknown']
    notes.append(f"CPU `{', '.join(cpus)}`")
    manifest = data.get('manifest', {})
    notes.append(f"manifest `{manifest.get('sha256', 'unknown')}`")
    return '; '.join(notes)


def compatibility_issues(current, past):
    issues = []
    if current.get('goos') != past.get('goos'):
        issues.append('goos differs')
    if current.get('goarch') != past.get('goarch'):
        issues.append('goarch differs')
    if current.get('cpu') != past.get('cpu'):
        issues.append('reported CPU identity differs')
    if current.get('source', {}).get('go-version') != past.get('source', {}).get('go-version'):
        issues.append('Go version differs')
    for key, label in (
        ('goja-fork-version', 'Goja fork version'),
        ('goja-nodejs-version', 'goja_nodejs version'),
    ):
        if current.get('source', {}).get(key) != past.get('source', {}).get(key):
            issues.append(f'{label} differs')
    if current.get('manifest', {}).get('sha256') != past.get('manifest', {}).get('sha256'):
        issues.append('manifest differs')
    if current.get('effective_sample_count') != past.get('effective_sample_count'):
        issues.append('effective sample count differs')
    return issues


def delta_rows(current, past):
    rows = []
    for name in sorted(set(current) & set(past)):
        current_mean = current[name]['mean']
        past_mean = past[name]['mean']
        percent = ((current_mean - past_mean) / past_mean * 100.0) if past_mean else 0.0
        rows.append((abs(percent), name, past_mean, current_mean, percent))
    return sorted(rows, reverse=True)


def write_delta_section(lines, label, current_data, past_data, past_label):
    lines.extend(['', f'## {label}: current vs {past_label}', ''])
    issues = compatibility_issues(current_data, past_data)
    if issues:
        lines.append(
            'Direct longitudinal deltas are refused because ' + ', '.join(issues) + '. '
            'Re-run both revisions on the same machine and Go toolchain.'
        )
        return

    current = extract(current_data)
    past = extract(past_data)
    rows = delta_rows(current, past)
    lines.append(
        f'{len(rows)} stable benchmark identities overlap. These are arithmetic '
        'mean deltas, not statistical findings; use `go tool benchstat OLD.log NEW.log` '
        'for inference from raw samples.'
    )
    lines.extend([
        '',
        '| Benchmark | Previous ns/op | Current ns/op | Observed delta |',
        '|---|---:|---:|---:|',
    ])
    for _, name, previous, current_mean, percent in rows[:40]:
        lines.append(f'| `{name}` | {previous:,.2f} | {current_mean:,.2f} | {percent:+.1f}% |')
    current_only = sorted(set(current) - set(past))
    past_only = sorted(set(past) - set(current))
    lines.extend([
        '',
        f'- Current-only stable identities: {len(current_only)}',
        f'- Previous-only stable identities: {len(past_only)}',
    ])


def generate(darwin_data, linux_data, compare_to=None):
    require_validated(darwin_data, 'darwin.json')
    require_validated(linux_data, 'linux.json')
    issues = cross_platform_issues(darwin_data, linux_data)
    if issues:
        raise ValueError(
            'Darwin/Linux evidence has incompatible provenance: ' + ', '.join(issues)
        )
    darwin = extract(darwin_data)
    linux = extract(linux_data)
    common = sorted(set(darwin) & set(linux))
    darwin_only = sorted(set(darwin) - set(linux))
    linux_only = sorted(set(linux) - set(darwin))
    darwin_label = platform_label(darwin_data, 'Darwin')
    linux_label = platform_label(linux_data, 'Linux')

    lines = [
        '# Eventloop Tournament: Darwin and Linux',
        '',
        f'**Methodology:** {METHODOLOGY}.',
        '',
        'This report is descriptive. Cross-platform ratios combine OS and runtime '
        'effects and are not regression tests. Statistical comparisons must use the '
        'pinned `go tool benchstat` command over raw logs from controlled runs.',
        '',
        f'- **{darwin_label}:** {source_notes(darwin_data)}',
        f'- **{linux_label}:** {source_notes(linux_data)}',
        '',
        '## Coverage',
        '',
        f'- Shared stable benchmark identities: {len(common)}',
        f'- Darwin-only identities: {len(darwin_only)}',
        f'- Linux-only identities: {len(linux_only)}',
        '',
        'A platform-only identity is a coverage difference, not a performance result.',
        '',
        '## Shared benchmark observations',
        '',
        '| Benchmark | Darwin ns/op | Darwin CV | Linux ns/op | Linux CV | D/L ratio | '
        'Darwin B/op | Linux B/op |',
        '|---|---:|---:|---:|---:|---:|---:|---:|',
    ]
    for name in common:
        darwin_row = darwin[name]
        linux_row = linux[name]
        ratio = darwin_row['mean'] / linux_row['mean'] if linux_row['mean'] else float('inf')
        lines.append(
            f"| `{name}` | {darwin_row['mean']:,.2f} | "
            f"{cv(darwin_row['mean'], darwin_row['sd']):.1f}% | "
            f"{linux_row['mean']:,.2f} | {cv(linux_row['mean'], linux_row['sd']):.1f}% | "
            f"{ratio:.3f}x | {darwin_row['b_op']:,.0f} | {linux_row['b_op']:,.0f} |"
        )

    if darwin_only:
        lines.extend(['', '### Darwin-only identities', ''])
        lines.extend(f'- `{name}`' for name in darwin_only)
    if linux_only:
        lines.extend(['', '### Linux-only identities', ''])
        lines.extend(f'- `{name}`' for name in linux_only)

    lines.extend([
        '',
        '## Correct comparison workflow',
        '',
        'Run the old and new revisions on the same hardware, OS, architecture, Go '
        'toolchain, manifest, and benchmark flags. Preserve both raw logs, then run:',
        '',
        '```bash',
        'gmake eventloop-tournament-compare OLD_LOG=old.log NEW_LOG=new.log',
        '```',
        '',
        'The Make target invokes the repository-pinned `benchstat`; this JSON report '
        'does not duplicate or approximate its statistical model.',
    ])

    if compare_to is not None:
        past_darwin = load_external(compare_to / 'darwin.json')
        past_linux = load_external(compare_to / 'linux.json')
        require_validated(past_darwin, str(compare_to / 'darwin.json'))
        require_validated(past_linux, str(compare_to / 'linux.json'))
        write_delta_section(lines, 'Darwin longitudinal observations', darwin_data,
                            past_darwin, compare_to.name)
        write_delta_section(lines, 'Linux longitudinal observations', linux_data,
                            past_linux, compare_to.name)

    return '\n'.join(lines) + '\n'


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--compare-to', type=Path)
    args = parser.parse_args()
    try:
        report = generate(load('darwin.json'), load('linux.json'), args.compare_to)
    except (OSError, json.JSONDecodeError, ValueError) as error:
        parser.error(str(error))
    output = SCRIPT_DIR / 'comparison.md'
    output.write_text(report, encoding='utf-8')
    print(f'Generated {output}')


if __name__ == '__main__':
    main()
