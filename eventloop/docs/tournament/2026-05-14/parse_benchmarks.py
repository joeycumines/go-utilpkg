#!/usr/bin/env python3
"""Parse and validate an eventloop tournament raw log.

Usage:
    python3 parse_benchmarks.py LOG PLATFORM FALLBACK_GOOS FALLBACK_GOARCH

Current logs are accepted only when every manifest lane completed, every Go
benchmark command passed, every declared workload and variant was observed,
and every emitted result has the manifest sample count. Use ``--legacy`` only
to inspect an older unmarked log; legacy output is explicitly unvalidated.
"""

import argparse
import hashlib
import json
import math
import os
import re
import sys
from collections import OrderedDict
from datetime import datetime
from pathlib import Path


BENCH_LINE_RE = re.compile(r'^(Benchmark\S+)\s+(\d+)\s+(.+)$')
BENCH_NAME_RE = re.compile(r'^(Benchmark\S+)(?:\s+.*)?$')
METRIC_NUMBER_RE = r'[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?'
METRIC_RE = re.compile(rf'({METRIC_NUMBER_RE})\s+(\S+)')
CONTINUATION_LINE_RE = re.compile(r'^(\d+)\s+(.+)$')
GOOS_RE = re.compile(r'^goos:\s*(\S+)')
GOARCH_RE = re.compile(r'^goarch:\s*(\S+)')
CPU_RE = re.compile(r'^cpu:\s*(.+)')
PACKAGE_RE = re.compile(r'^pkg:\s*(\S+)')
TOURNAMENT_RE = re.compile(r'^tournament:\s*([a-z-]+)(?:=(.*))?$')
META_RE = re.compile(r'^([^=]+)=(.*)$')

METRIC_KEYS = {
    'ns/op': 'ns_op',
    'B/op': 'b_op',
    'allocs/op': 'allocs_op',
}
REQUIRED_METRICS = {'ns_op', 'b_op', 'allocs_op'}


class ValidationError(Exception):
    """Raised when a raw log cannot support a tournament comparison."""


def strict_object(pairs):
    """Decode a JSON object while rejecting duplicate keys."""
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValidationError(f'duplicate manifest key {key!r}')
        result[key] = value
    return result


def require_manifest_keys(value, required, allowed, description):
    """Reject missing, unknown, or structurally misplaced manifest fields."""
    if not isinstance(value, dict):
        raise ValidationError(f'{description} must be an object')
    missing = required - set(value)
    unknown = set(value) - allowed
    if missing:
        raise ValidationError(f'{description} misses fields {sorted(missing)}')
    if unknown:
        raise ValidationError(f'{description} has unknown fields {sorted(unknown)}')


def default_manifest_path():
    """Find the repository manifest from this script or a dated copy."""
    for parent in (Path(__file__).resolve().parent, *Path(__file__).resolve().parents):
        candidate = parent / 'internal' / 'tournament' / 'manifest.json'
        if candidate.is_file():
            return candidate
    return Path(__file__).resolve().parent / 'manifest.json'


def load_manifest(path):
    raw = Path(path).read_bytes()
    manifest = json.loads(raw, object_pairs_hook=strict_object)
    require_manifest_keys(
        manifest,
        {
            'schema_version', 'sample_count', 'variants', 'variant_groups',
            'lanes', 'concepts', 'revision_variants', 'revision_checkpoints',
        },
        {
            'schema_version', 'sample_count', 'variants', 'variant_groups',
            'lanes', 'concepts', 'revision_variants', 'revision_checkpoints',
        },
        'manifest',
    )
    if manifest.get('schema_version') != 1:
        raise ValidationError(
            f"unsupported manifest schema {manifest.get('schema_version')!r}"
        )
    if not isinstance(manifest.get('sample_count'), int) or manifest['sample_count'] < 2:
        raise ValidationError('manifest sample_count must be an integer of at least 2')
    if not isinstance(manifest['variants'], list) or not manifest['variants']:
        raise ValidationError('manifest variants must be a nonempty array')
    variant_ids = set()
    aliases = set()
    for index, variant in enumerate(manifest['variants']):
        require_manifest_keys(
            variant,
            {
                'kind', 'id', 'name', 'source_package', 'aliases',
                'origin_commit', 'origin_tree', 'capabilities',
            },
            {
                'kind', 'id', 'name', 'source_package', 'aliases',
                'origin_commit', 'origin_tree', 'capabilities',
            },
            f'manifest variant {index}',
        )
        if variant['id'] in variant_ids:
            raise ValidationError(f"duplicate manifest variant ID {variant['id']!r}")
        variant_ids.add(variant['id'])
        if not isinstance(variant['aliases'], list):
            raise ValidationError(f"variant {variant['id']!r} aliases must be an array")
        for alias in variant['aliases']:
            if alias in aliases:
                raise ValidationError(f'duplicate manifest alias {alias!r}')
            aliases.add(alias)
    if not isinstance(manifest['variant_groups'], dict) or not manifest['variant_groups']:
        raise ValidationError('manifest variant_groups must be a nonempty object')
    for group, members in manifest['variant_groups'].items():
        if not isinstance(members, list) or not members:
            raise ValidationError(f'variant group {group!r} must be a nonempty array')
        if len(members) != len(set(members)) or not set(members) <= variant_ids:
            raise ValidationError(f'variant group {group!r} has duplicate or unknown IDs')
    if not isinstance(manifest['lanes'], list) or not manifest['lanes']:
        raise ValidationError('manifest lanes must be a nonempty array')
    lane_ids = set()
    for index, lane in enumerate(manifest['lanes']):
        require_manifest_keys(
            lane,
            {'id', 'package', 'required', 'benchmarks', 'variant_ids'},
            {
                'id', 'package', 'required', 'benchmarks', 'benchmark_variant_groups',
                'benchmark_goos', 'benchmark_leaves',
                'benchmark_variant_extra_leaves', 'variant_ids', 'default_variant_id',
            },
            f'manifest lane {index}',
        )
        if lane['id'] in lane_ids:
            raise ValidationError(f"duplicate manifest lane ID {lane['id']!r}")
        lane_ids.add(lane['id'])
        if not isinstance(lane['benchmarks'], list) or not lane['benchmarks']:
            raise ValidationError(f"lane {lane['id']!r} benchmarks must be nonempty")
        if len(lane['benchmarks']) != len(set(lane['benchmarks'])):
            raise ValidationError(f"lane {lane['id']!r} repeats a benchmark")
        lane_variants = lane['variant_ids']
        if (
            not isinstance(lane_variants, list)
            or len(lane_variants) != len(set(lane_variants))
            or not set(lane_variants) <= variant_ids
        ):
            raise ValidationError(f"lane {lane['id']!r} has duplicate or unknown variants")
        mappings = lane.get('benchmark_variant_groups', {})
        if set(mappings) - set(lane['benchmarks']):
            raise ValidationError(f"lane {lane['id']!r} maps an unknown benchmark")
        if set(mappings.values()) - set(manifest['variant_groups']):
            raise ValidationError(f"lane {lane['id']!r} maps an unknown variant group")
        if lane_variants and set(mappings) != set(lane['benchmarks']):
            raise ValidationError(f"lane {lane['id']!r} lacks per-benchmark variant mapping")
        mapped_variants = {
            variant_id
            for group in mappings.values()
            for variant_id in manifest['variant_groups'][group]
        }
        if mapped_variants != set(lane_variants):
            raise ValidationError(f"lane {lane['id']!r} mapped variants differ from lane variants")
        goos_policy = lane.get('benchmark_goos', {})
        if set(goos_policy) - set(lane['benchmarks']):
            raise ValidationError(f"lane {lane['id']!r} has GOOS policy for an unknown benchmark")
        for benchmark, goos_values in goos_policy.items():
            if (
                not isinstance(goos_values, list)
                or not goos_values
                or len(goos_values) != len(set(goos_values))
                or not set(goos_values) <= {'darwin', 'linux', 'windows'}
            ):
                raise ValidationError(
                    f"lane {lane['id']!r} benchmark {benchmark!r} has invalid GOOS policy"
                )
        benchmark_leaves = lane.get('benchmark_leaves', {})
        if set(benchmark_leaves) - set(lane['benchmarks']):
            raise ValidationError(f"lane {lane['id']!r} has leaves for an unknown benchmark")
        for benchmark, leaves in benchmark_leaves.items():
            if (
                not isinstance(leaves, list)
                or not leaves
                or len(leaves) != len(set(leaves))
                or any(not leaf for leaf in leaves)
            ):
                raise ValidationError(
                    f"lane {lane['id']!r} benchmark {benchmark!r} has invalid leaves"
                )
        variant_extra_leaves = lane.get('benchmark_variant_extra_leaves', {})
        if set(variant_extra_leaves) - set(lane['benchmarks']):
            raise ValidationError(
                f"lane {lane['id']!r} has variant leaves for an unknown benchmark"
            )
        for benchmark, by_variant in variant_extra_leaves.items():
            group = mappings.get(benchmark)
            applicable_variants = set(manifest['variant_groups'].get(group, []))
            if not isinstance(by_variant, dict) or set(by_variant) - applicable_variants:
                raise ValidationError(
                    f"lane {lane['id']!r} benchmark {benchmark!r} has invalid leaf variants"
                )
            for variant_id, leaves in by_variant.items():
                if (
                    not isinstance(leaves, list)
                    or not leaves
                    or len(leaves) != len(set(leaves))
                    or any(not leaf for leaf in leaves)
                    or set(leaves) & set(benchmark_leaves.get(benchmark, []))
                ):
                    raise ValidationError(
                        f"lane {lane['id']!r} benchmark {benchmark!r} has invalid "
                        f"extra leaves for {variant_id!r}"
                    )
        default = lane.get('default_variant_id')
        if default is not None and default not in lane_variants:
            raise ValidationError(f"lane {lane['id']!r} default is not a lane variant")
    for index, concept in enumerate(manifest['concepts']):
        require_manifest_keys(
            concept,
            {'id', 'name', 'status', 'benchmarkable', 'source_document', 'disposition'},
            {'id', 'name', 'status', 'benchmarkable', 'source_document', 'disposition'},
            f'manifest concept {index}',
        )
        if concept['status'] != 'concept-only' or concept['benchmarkable'] is not False:
            raise ValidationError(f"manifest concept {concept['id']!r} is not concept-only")
    for index, revision in enumerate(manifest['revision_variants']):
        require_manifest_keys(
            revision,
            {'id', 'commit', 'name'},
            {'id', 'commit', 'name'},
            f'manifest revision {index}',
        )
    git_header = f'blob {len(raw)}\0'.encode('ascii')
    return (
        manifest,
        hashlib.sha256(raw).hexdigest(),
        hashlib.sha1(git_header + raw).hexdigest(),
    )


def get_timestamp_from_log(log_path):
    """Return the local calendar date represented by the raw log mtime."""
    if os.path.exists(log_path):
        return datetime.fromtimestamp(os.path.getmtime(log_path)).strftime('%Y-%m-%d')
    return datetime.now().strftime('%Y-%m-%d')


def parse_metric_tail(metric_tail):
    """Parse Go benchmark metric pairs after the iteration count."""
    metrics = {}
    for value_raw, unit in METRIC_RE.findall(metric_tail):
        key = METRIC_KEYS.get(unit, unit.replace('/', '_'))
        value = float(value_raw)
        metrics[key] = int(value) if value == int(value) else value
    return metrics


def new_lane_state():
    return {
        'start_count': 0,
        'pass_count': 0,
        'failed': False,
        'skipped': False,
        'skip_reason': None,
        'rows_after_terminal': 0,
    }


def parse_run(filepath):
    """Parse results, lane status, and provenance markers from a raw log."""
    benchmarks = OrderedDict()
    lane_status = OrderedDict()
    metadata = {}
    current_lane = None
    current_package = None
    pending_name = None
    goos_values = set()
    goarch_values = set()
    cpu_values = set()
    schema_version = None
    schema_count = 0
    complete = False
    complete_count = 0
    metadata_duplicates = set()
    global_pass_count = 0
    global_failed = False

    with open(filepath, 'r', encoding='utf-8') as raw_log:
        for raw_line in raw_log:
            line = raw_line.strip()
            marker = TOURNAMENT_RE.match(line)
            if marker:
                key, value = marker.groups()
                if key == 'schema':
                    schema_count += 1
                    try:
                        schema_version = int(value)
                    except (TypeError, ValueError):
                        schema_version = None
                elif key == 'meta' and value:
                    item = META_RE.match(value)
                    if item:
                        metadata_key = item.group(1)
                        if metadata_key in metadata:
                            metadata_duplicates.add(metadata_key)
                        metadata[metadata_key] = item.group(2)
                elif key == 'lane' and value:
                    current_lane = value
                    state = lane_status.setdefault(current_lane, new_lane_state())
                    state['start_count'] += 1
                    current_package = None
                    pending_name = None
                elif key == 'skip' and value:
                    lane, separator, reason = value.partition(':')
                    state = lane_status.setdefault(lane, new_lane_state())
                    state['skipped'] = True
                    state['skip_reason'] = reason if separator and reason else None
                    current_lane = None
                    current_package = None
                    pending_name = None
                elif key == 'complete':
                    complete = True
                    complete_count += 1
                continue

            match = GOOS_RE.match(line)
            if match:
                goos_values.add(match.group(1))
                continue
            match = GOARCH_RE.match(line)
            if match:
                goarch_values.add(match.group(1))
                continue
            match = CPU_RE.match(line)
            if match:
                cpu_values.add(match.group(1))
                continue
            match = PACKAGE_RE.match(line)
            if match:
                current_package = match.group(1)
                pending_name = None
                continue

            if line == 'PASS':
                if current_lane is not None:
                    lane_status.setdefault(current_lane, new_lane_state())['pass_count'] += 1
                else:
                    global_pass_count += 1
                pending_name = None
                continue
            if line == 'FAIL' or line.startswith('FAIL\t') or line.startswith('FAIL '):
                if current_lane is not None:
                    lane_status.setdefault(current_lane, new_lane_state())['failed'] = True
                else:
                    global_failed = True
                pending_name = None
                continue

            match = BENCH_LINE_RE.match(line)
            if match:
                name = match.group(1)
                metrics = parse_metric_tail(match.group(3))
                if REQUIRED_METRICS - set(metrics):
                    pending_name = (current_lane, current_package, name)
                    continue
                key = (current_lane, current_package, name)
                benchmarks.setdefault(key, []).append(metrics)
                if current_lane is not None:
                    state = lane_status.setdefault(current_lane, new_lane_state())
                    if state['pass_count'] or state['failed'] or state['skipped']:
                        state['rows_after_terminal'] += 1
                pending_name = None
                continue

            match = BENCH_NAME_RE.match(line)
            if match:
                pending_name = (current_lane, current_package, match.group(1))
                continue

            if pending_name is None:
                continue
            match = CONTINUATION_LINE_RE.match(line)
            if not match:
                continue
            metrics = parse_metric_tail(match.group(2))
            if REQUIRED_METRICS - set(metrics):
                continue
            benchmarks.setdefault(pending_name, []).append(metrics)
            if pending_name[0] is not None:
                state = lane_status.setdefault(pending_name[0], new_lane_state())
                if state['pass_count'] or state['failed'] or state['skipped']:
                    state['rows_after_terminal'] += 1
            pending_name = None

    return {
        'benchmarks': benchmarks,
        'lanes': lane_status,
        'metadata': metadata,
        'goos_values': goos_values,
        'goarch_values': goarch_values,
        'cpu_values': cpu_values,
        'schema_version': schema_version,
        'schema_count': schema_count,
        'complete': complete,
        'complete_count': complete_count,
        'metadata_duplicates': metadata_duplicates,
        'global_pass_count': global_pass_count,
        'global_failed': global_failed,
    }


def parse_log(filepath):
    """Legacy helper returning runs keyed only by package and benchmark name."""
    merged = OrderedDict()
    for (_, package, name), runs in parse_run(filepath)['benchmarks'].items():
        merged.setdefault((package, name), []).extend(runs)
    return merged


def _num(value):
    if isinstance(value, float) and value == int(value):
        return int(value)
    return value


def compute_statistics(values):
    """Compute descriptive statistics without claiming inferential significance."""
    count = len(values)
    if count == 0:
        return {'mean': 0, 'min': 0, 'max': 0, 'stddev': 0}
    mean = sum(values) / count
    variance = 0 if count < 2 else sum((item - mean) ** 2 for item in values) / (count - 1)
    return {
        'mean': _num(mean),
        'min': _num(min(values)),
        'max': _num(max(values)),
        'stddev': _num(math.sqrt(variance)),
    }


def normalize_benchmark_name(name):
    return re.sub(r'-\d+$', '', name)


def benchmark_root(name):
    return normalize_benchmark_name(name).split('/', 1)[0]


def manifest_aliases(manifest):
    aliases = {}
    for variant in manifest['variants']:
        for alias in variant.get('aliases', []):
            if alias in aliases:
                raise ValidationError(f'duplicate manifest alias {alias!r}')
            aliases[alias] = variant['id']
    return aliases


def benchmark_variant_ids(name, lane, manifest):
    aliases = manifest_aliases(manifest)
    normalized = normalize_benchmark_name(name)
    found = []
    for segment in normalized.split('/')[1:]:
        variant_id = aliases.get(segment)
        if variant_id is not None and variant_id not in found:
            found.append(variant_id)
    if found:
        return found
    default = lane.get('default_variant_id')
    return [default] if default else []


def stable_benchmark_name(name, lane, manifest):
    """Replace display aliases with stable variant IDs for longitudinal joins."""
    aliases = manifest_aliases(manifest)
    segments = normalize_benchmark_name(name).split('/')
    replaced = [segments[0]]
    matched = False
    for segment in segments[1:]:
        variant_id = aliases.get(segment)
        if variant_id is None:
            replaced.append(segment)
        else:
            replaced.append(variant_id)
            matched = True
    default = lane.get('default_variant_id')
    if default and not matched:
        replaced.append(default)
    return '/'.join(replaced)


def applicable_benchmarks(lane, goos):
    """Return the lane's benchmark roots that exist on the observed GOOS."""
    policies = lane.get('benchmark_goos', {})
    return {
        benchmark
        for benchmark in lane['benchmarks']
        if benchmark not in policies or goos in policies[benchmark]
    }


def expected_benchmark_variants(lane, benchmark, manifest):
    """Return the exact stable variants required for one benchmark root."""
    group = lane.get('benchmark_variant_groups', {}).get(benchmark)
    if group is None:
        return set()
    return set(manifest['variant_groups'][group])


def benchmark_leaf(name, manifest):
    """Return a sub-benchmark leaf after removing its one stable variant alias."""
    aliases = manifest_aliases(manifest)
    segments = normalize_benchmark_name(name).split('/')
    alias_indexes = [
        index for index, segment in enumerate(segments[1:], start=1)
        if segment in aliases
    ]
    if len(alias_indexes) == 1:
        del segments[alias_indexes[0]]
    return '/'.join(segments[1:])


def expected_benchmark_leaves(lane, benchmark, variant_id=None):
    """Return exact required leaf identities for one workload and variant."""
    common = set(lane.get('benchmark_leaves', {}).get(benchmark, ['']))
    extras = lane.get('benchmark_variant_extra_leaves', {}).get(
        benchmark, {}
    ).get(variant_id, [])
    return common | set(extras)


def validate_run(parsed, manifest, expected_samples=None, manifest_git_blob=None):
    """Reject any log whose coverage or terminal state is incomplete."""
    issues = []
    if parsed['schema_version'] != manifest['schema_version']:
        issues.append(
            f"raw schema {parsed['schema_version']!r} != manifest schema "
            f"{manifest['schema_version']}"
        )
    if parsed['schema_count'] != 1:
        issues.append(
            f"raw log has {parsed['schema_count']} schema markers, want exactly 1"
        )
    if not parsed['complete']:
        issues.append('raw log lacks the terminal tournament: complete marker')
    if parsed['complete_count'] != 1:
        issues.append(
            f"raw log has {parsed['complete_count']} completion markers, want exactly 1"
        )
    if parsed['metadata_duplicates']:
        issues.append(
            f"raw log repeats metadata keys: {sorted(parsed['metadata_duplicates'])}"
        )
    if parsed['global_pass_count']:
        issues.append(
            f"raw log has {parsed['global_pass_count']} PASS markers outside a lane"
        )
    if parsed['global_failed']:
        issues.append('raw log emitted FAIL outside a lane')

    required_metadata = {
        'head',
        'source-state',
        'go-version',
        'sample-count',
        'manifest-git-blob',
        'goja-fork-version',
        'goja-nodejs-version',
        'source-fingerprint',
    }
    missing_metadata = required_metadata - set(parsed['metadata'])
    if missing_metadata:
        issues.append(f"raw log misses metadata: {sorted(missing_metadata)}")
    empty_metadata = sorted(
        key for key in required_metadata if key in parsed['metadata'] and not parsed['metadata'][key]
    )
    if empty_metadata:
        issues.append(f"raw log has empty metadata: {empty_metadata}")
    if not re.fullmatch(r'[0-9a-f]{40}', parsed['metadata'].get('head', '')):
        issues.append(f"raw head is invalid: {parsed['metadata'].get('head')!r}")
    if parsed['metadata'].get('source-state') not in {'clean', 'dirty'}:
        issues.append(
            f"raw source state is invalid: {parsed['metadata'].get('source-state')!r}"
        )
    if not re.fullmatch(
        r'[0-9a-f]{40}', parsed['metadata'].get('source-fingerprint', '')
    ):
        issues.append(
            f"raw source fingerprint is invalid: "
            f"{parsed['metadata'].get('source-fingerprint')!r}"
        )
    if manifest_git_blob is not None:
        observed_blob = parsed['metadata'].get('manifest-git-blob')
        if observed_blob != manifest_git_blob:
            issues.append(
                f"raw manifest blob {observed_blob!r} != parsed manifest blob "
                f"{manifest_git_blob!r}"
            )
    if len(parsed['goos_values']) != 1:
        issues.append(f"raw log has GOOS values {sorted(parsed['goos_values'])}, want exactly 1")
    if len(parsed['goarch_values']) != 1:
        issues.append(
            f"raw log has GOARCH values {sorted(parsed['goarch_values'])}, want exactly 1"
        )
    if len(parsed['cpu_values']) != 1:
        issues.append(f"raw log has CPU values {sorted(parsed['cpu_values'])}, want exactly 1")

    benchmark_rows_outside_lanes = [
        name
        for (lane_id, _, name) in parsed['benchmarks']
        if lane_id is None
    ]
    if benchmark_rows_outside_lanes:
        issues.append(
            "raw log has benchmark rows outside a lane: "
            f"{sorted(benchmark_rows_outside_lanes)}"
        )

    lanes = {lane['id']: lane for lane in manifest['lanes']}
    unknown_lanes = set(parsed['lanes']) - set(lanes)
    if unknown_lanes:
        issues.append(f"raw log contains unknown lanes: {sorted(unknown_lanes)}")

    sample_count = manifest['sample_count'] if expected_samples is None else expected_samples
    if not isinstance(sample_count, int) or sample_count < 1:
        issues.append(f"effective sample count {sample_count!r} must be a positive integer")
    try:
        metadata_sample_count = int(parsed['metadata'].get('sample-count', ''))
    except ValueError:
        metadata_sample_count = None
    if metadata_sample_count != sample_count:
        issues.append(
            f"raw sample-count metadata {metadata_sample_count!r} != expected {sample_count}"
        )
    observed_goos = next(iter(parsed['goos_values']), None)
    for lane_id, lane in lanes.items():
        status = parsed['lanes'].get(lane_id)
        if status is None:
            if lane['required']:
                issues.append(f"required lane {lane_id!r} is absent")
            else:
                issues.append(f"optional lane {lane_id!r} was neither run nor explicitly skipped")
            continue
        if status['skipped']:
            if lane['required']:
                issues.append(f"required lane {lane_id!r} was skipped")
            if not status['skip_reason']:
                issues.append(f"lane {lane_id!r} skip has no reason")
            if status['start_count'] or status['pass_count'] or status['failed']:
                issues.append(f"lane {lane_id!r} was both skipped and executed")
            continue
        if status['start_count'] != 1:
            issues.append(
                f"lane {lane_id!r} started {status['start_count']} times, want exactly 1"
            )
        if status['failed']:
            issues.append(f"lane {lane_id!r} emitted FAIL")
        if status['pass_count'] != 1:
            issues.append(
                f"lane {lane_id!r} emitted {status['pass_count']} PASS markers, want exactly 1"
            )
        if status['rows_after_terminal']:
            issues.append(
                f"lane {lane_id!r} has {status['rows_after_terminal']} benchmark rows "
                "after PASS, FAIL, or skip"
            )

        lane_rows = [
            (package, name, runs)
            for (row_lane, package, name), runs in parsed['benchmarks'].items()
            if row_lane == lane_id
        ]
        packages = {package for package, _, _ in lane_rows}
        if packages != {lane['package']}:
            issues.append(
                f"lane {lane_id!r} packages {sorted(str(item) for item in packages)} "
                f"!= {[lane['package']]}"
            )
        observed_benchmarks = {benchmark_root(name) for _, name, _ in lane_rows}
        expected_benchmarks = applicable_benchmarks(lane, observed_goos)
        missing_benchmarks = expected_benchmarks - observed_benchmarks
        unexpected_benchmarks = observed_benchmarks - expected_benchmarks
        if missing_benchmarks:
            issues.append(
                f"lane {lane_id!r} misses benchmarks: {sorted(missing_benchmarks)}"
            )
        if unexpected_benchmarks:
            issues.append(
                f"lane {lane_id!r} has unmanifested benchmarks: {sorted(unexpected_benchmarks)}"
            )

        observed_variants = set()
        variants_by_benchmark = {benchmark: set() for benchmark in expected_benchmarks}
        leaves_by_cell = {}
        for _, name, runs in lane_rows:
            if len(runs) != sample_count:
                issues.append(
                    f"lane {lane_id!r} row {name!r} has {len(runs)} samples, "
                    f"want {sample_count}"
                )
            variant_ids = benchmark_variant_ids(name, lane, manifest)
            root = benchmark_root(name)
            expected_row_variants = expected_benchmark_variants(lane, root, manifest)
            if expected_row_variants and len(variant_ids) != 1:
                issues.append(
                    f"lane {lane_id!r} row {name!r} identifies variants "
                    f"{variant_ids}, want exactly one"
                )
            if not expected_row_variants and variant_ids:
                issues.append(
                    f"lane {lane_id!r} row {name!r} unexpectedly identifies "
                    f"variants {variant_ids}"
                )
            observed_variants.update(variant_ids)
            if root in variants_by_benchmark:
                variants_by_benchmark[root].update(variant_ids)
                cell_variant = variant_ids[0] if len(variant_ids) == 1 else None
                leaves_by_cell.setdefault((root, cell_variant), set()).add(
                    benchmark_leaf(name, manifest)
                )
        expected_variants = set(lane.get('variant_ids', []))
        if observed_variants != expected_variants:
            issues.append(
                f"lane {lane_id!r} variants {sorted(observed_variants)} "
                f"!= {sorted(expected_variants)}"
            )
        for benchmark in sorted(expected_benchmarks):
            expected = expected_benchmark_variants(lane, benchmark, manifest)
            observed = variants_by_benchmark[benchmark]
            if observed != expected:
                issues.append(
                    f"lane {lane_id!r} benchmark {benchmark!r} variants "
                    f"{sorted(observed)} != {sorted(expected)}"
                )
            cell_variants = expected or {None}
            for variant_id in sorted(cell_variants, key=lambda value: value or ''):
                expected_leaves = expected_benchmark_leaves(
                    lane, benchmark, variant_id
                )
                observed_leaves = leaves_by_cell.get((benchmark, variant_id), set())
                if observed_leaves != expected_leaves:
                    issues.append(
                        f"lane {lane_id!r} benchmark {benchmark!r} variant "
                        f"{variant_id!r} leaves {sorted(observed_leaves)} != "
                        f"{sorted(expected_leaves)}"
                    )

    libuv_status = parsed['lanes'].get('libuv')
    if libuv_status is not None and not libuv_status['skipped']:
        if not parsed['metadata'].get('libuv-version'):
            issues.append('executed libuv lane lacks libuv-version metadata')

    if issues:
        raise ValidationError('\n'.join(issues))


def build_output(parsed_or_benchmarks, platform, goos, goarch, timestamp,
                 manifest=None, manifest_digest=None, manifest_git_blob=None,
                 validated=False, effective_sample_count=None):
    """Build structured JSON while retaining raw samples for benchstat."""
    if 'benchmarks' in parsed_or_benchmarks:
        parsed = parsed_or_benchmarks
        benchmarks = parsed['benchmarks']
    else:
        parsed = None
        benchmarks = OrderedDict(
            ((None, package, name), runs)
            for (package, name), runs in parsed_or_benchmarks.items()
        )

    lane_map = {} if manifest is None else {lane['id']: lane for lane in manifest['lanes']}
    bench_list = []
    for lane_id, package, name in sorted(
        benchmarks,
        key=lambda key: ((key[0] or ''), (key[1] or ''), key[2]),
    ):
        runs = benchmarks[(lane_id, package, name)]
        metric_names = sorted({key for run in runs for key in run})
        entry = {
            'lane': lane_id,
            'package': package,
            'name': name,
            'stable_name': (
                stable_benchmark_name(name, lane_map[lane_id], manifest)
                if manifest is not None and lane_id in lane_map
                else normalize_benchmark_name(name)
            ),
            'variant_ids': (
                benchmark_variant_ids(name, lane_map[lane_id], manifest)
                if manifest is not None and lane_id in lane_map else []
            ),
            'runs': runs,
            'statistics': {
                key: compute_statistics([run[key] for run in runs if key in run])
                for key in metric_names
            },
        }
        bench_list.append(entry)

    canonical_sample_count = None if manifest is None else manifest['sample_count']
    if effective_sample_count is None:
        effective_sample_count = canonical_sample_count
    evidence_class = (
        'legacy'
        if not validated
        else 'canonical'
        if effective_sample_count == canonical_sample_count
        else 'smoke'
    )
    output = {
        'schema_version': 2,
        'validated': validated,
        'evidence_class': evidence_class,
        'effective_sample_count': effective_sample_count,
        'platform': platform,
        'goos': goos,
        'goarch': goarch,
        'timestamp': timestamp,
        'benchmarks': bench_list,
    }
    if parsed is not None:
        output['cpu'] = sorted(parsed['cpu_values'])
        output['source'] = parsed['metadata']
        output['lanes'] = parsed['lanes']
    if manifest is not None:
        output['manifest'] = {
            'schema_version': manifest['schema_version'],
            'sha256': manifest_digest,
            'git_blob': manifest_git_blob,
            'sample_count': manifest['sample_count'],
            'effective_sample_count': effective_sample_count,
        }
    return output


def singleton_or_fallback(values, fallback):
    return next(iter(values)) if len(values) == 1 else fallback


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('log_file')
    parser.add_argument('platform')
    parser.add_argument('fallback_goos')
    parser.add_argument('fallback_goarch')
    parser.add_argument('--manifest', type=Path, default=default_manifest_path())
    parser.add_argument('--expected-samples', type=int)
    parser.add_argument(
        '--legacy', action='store_true',
        help='parse an old unmarked log without granting validation credit',
    )
    args = parser.parse_args()

    try:
        manifest, manifest_digest, manifest_git_blob = load_manifest(args.manifest)
        parsed = parse_run(args.log_file)
        effective_sample_count = (
            manifest['sample_count']
            if args.expected_samples is None
            else args.expected_samples
        )
        if not args.legacy:
            validate_run(parsed, manifest, effective_sample_count, manifest_git_blob)
    except (OSError, json.JSONDecodeError, ValidationError) as error:
        print(f'tournament log rejected: {error}', file=sys.stderr)
        return 2

    goos = singleton_or_fallback(parsed['goos_values'], args.fallback_goos)
    goarch = singleton_or_fallback(parsed['goarch_values'], args.fallback_goarch)
    output = build_output(
        parsed,
        args.platform,
        goos,
        goarch,
        get_timestamp_from_log(args.log_file),
        manifest,
        manifest_digest,
        manifest_git_blob,
        validated=not args.legacy,
        effective_sample_count=effective_sample_count,
    )
    print(json.dumps(output, indent=2))
    return 0


if __name__ == '__main__':
    sys.exit(main())
