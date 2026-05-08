#!/usr/bin/env python3
"""Regression tests for parse_benchmarks.py."""

import copy
import importlib.util
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


def load_sibling_module(name):
    path = Path(__file__).with_name(f'{name}.py')
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class ParseBenchmarksTest(unittest.TestCase):
    def setUp(self):
        self.parser = load_sibling_module('parse_benchmarks')

    def test_scientific_notation_metrics_are_not_corrupted(self):
        metrics = self.parser.parse_metric_tail('4.24e-05 ns/op 0 B/op 0 allocs/op')
        self.assertEqual(metrics['ns_op'], 4.24e-05)
        self.assertEqual(metrics['b_op'], 0)
        self.assertEqual(metrics['allocs_op'], 0)

    def test_same_benchmark_name_in_distinct_packages_stays_distinct(self):
        log = """\
goos: darwin
goarch: arm64
pkg: example.test/first
BenchmarkShared-8  1  10 ns/op  1 B/op  1 allocs/op
pkg: example.test/second
BenchmarkShared-8
diagnostic output between name and result
1  20 ns/op  2 B/op  2 allocs/op
"""
        with tempfile.NamedTemporaryFile('w', encoding='utf-8') as file:
            file.write(log)
            file.flush()
            benchmarks = self.parser.parse_log(file.name)

        output = self.parser.build_output(
            benchmarks,
            platform='test',
            goos='darwin',
            goarch='arm64',
            timestamp='2026-05-13',
        )
        self.assertEqual(
            [(entry['package'], entry['name']) for entry in output['benchmarks']],
            [
                ('example.test/first', 'BenchmarkShared-8'),
                ('example.test/second', 'BenchmarkShared-8'),
            ],
        )
        self.assertEqual(output['benchmarks'][0]['statistics']['ns_op']['mean'], 10)
        self.assertEqual(output['benchmarks'][1]['statistics']['ns_op']['mean'], 20)

    def test_analyzers_preserve_duplicate_package_identity(self):
        benchmark = {
            'runs': [{'ns_op': 10, 'b_op': 1, 'allocs_op': 1}],
            'statistics': {
                metric: {'mean': value, 'min': value, 'max': value, 'stddev': 0}
                for metric, value in [('ns_op', 10), ('b_op', 1), ('allocs_op', 1)]
            },
        }
        data = {
            'benchmarks': [
                {**benchmark, 'package': 'example.test/first', 'name': 'BenchmarkShared-8'},
                {**benchmark, 'package': 'example.test/second', 'name': 'BenchmarkShared-8'},
            ],
        }

        two_platform = load_sibling_module('analyze_2platform').extract(data)
        three_platform = load_sibling_module('analyze_3platform').extract_benchmark_summary(data)
        expected = {
            'example.test/first::BenchmarkShared',
            'example.test/second::BenchmarkShared',
        }
        self.assertEqual(set(two_platform), expected)
        self.assertEqual(set(three_platform), expected)

    def test_analyzers_preserve_unique_package_identity(self):
        def data(package):
            return {
                'benchmarks': [{
                    'package': package,
                    'name': 'BenchmarkShared-8',
                    'runs': [{'ns_op': 10, 'b_op': 1, 'allocs_op': 1}],
                    'statistics': {
                        metric: {'mean': value, 'min': value, 'max': value, 'stddev': 0}
                        for metric, value in [
                            ('ns_op', 10),
                            ('b_op', 1),
                            ('allocs_op', 1),
                        ]
                    },
                }],
            }

        first = data('example.test/first')
        moved = data('example.test/second')
        for extractor in (
            load_sibling_module('analyze_2platform').extract,
            load_sibling_module('analyze_3platform').extract_benchmark_summary,
        ):
            first_keys = set(extractor(first))
            moved_keys = set(extractor(moved))
            self.assertEqual(first_keys, {'example.test/first::BenchmarkShared'})
            self.assertEqual(moved_keys, {'example.test/second::BenchmarkShared'})
            self.assertFalse(first_keys & moved_keys)

    def test_analyzers_match_asymmetric_package_sets_by_full_identity(self):
        benchmark = {
            'runs': [{'ns_op': 10, 'b_op': 1, 'allocs_op': 1}],
            'statistics': {
                metric: {'mean': value, 'min': value, 'max': value, 'stddev': 0}
                for metric, value in [('ns_op', 10), ('b_op', 1), ('allocs_op', 1)]
            },
        }
        complete = {
            'benchmarks': [
                {**benchmark, 'package': 'example.test/first', 'name': 'BenchmarkShared-8'},
                {**benchmark, 'package': 'example.test/second', 'name': 'BenchmarkShared-8'},
            ],
        }
        partial = {'benchmarks': [complete['benchmarks'][0]]}

        for extractor in (
            load_sibling_module('analyze_2platform').extract,
            load_sibling_module('analyze_3platform').extract_benchmark_summary,
        ):
            common = set(extractor(complete)) & set(extractor(partial))
            self.assertEqual(common, {'example.test/first::BenchmarkShared'})

    def test_validated_run_requires_complete_manifest_coverage(self):
        manifest = self.minimal_manifest()
        parsed = self.parse_run("""\
tournament: schema=1
tournament: meta=head=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
tournament: meta=source-state=dirty
tournament: meta=go-version=go version go1.26.5 darwin/arm64
tournament: meta=sample-count=2
tournament: meta=manifest-git-blob=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
tournament: meta=goja-fork-version=v0.0.0-test
tournament: meta=goja-nodejs-version=v0.0.0-test
tournament: meta=source-fingerprint=cccccccccccccccccccccccccccccccccccccccc
tournament: lane=scheduler
goos: darwin
goarch: arm64
cpu: Example CPU
pkg: example.test/tournament
BenchmarkOne/Main-8  1  10 ns/op  1 B/op  1 allocs/op
BenchmarkOne/Main-8  1  12 ns/op  1 B/op  1 allocs/op
PASS
tournament: skip=libuv:pkg-config-libuv-unavailable
tournament: complete
""")

        self.parser.validate_run(parsed, manifest)
        output = self.parser.build_output(
            parsed,
            platform='test',
            goos='darwin',
            goarch='arm64',
            timestamp='2026-05-14',
            manifest=manifest,
            manifest_digest='digest',
            validated=True,
        )
        self.assertTrue(output['validated'])
        self.assertEqual(output['cpu'], ['Example CPU'])
        self.assertEqual(
            output['benchmarks'][0]['variant_ids'],
            ['scheduler.main.auto'],
        )

    def test_validation_rejects_failed_lane_even_with_samples(self):
        manifest = self.minimal_manifest(include_optional=False)
        parsed = self.parse_run("""\
tournament: schema=1
tournament: lane=scheduler
pkg: example.test/tournament
BenchmarkOne/Main-8  1  10 ns/op  1 B/op  1 allocs/op
BenchmarkOne/Main-8  1  12 ns/op  1 B/op  1 allocs/op
FAIL
tournament: complete
""")

        with self.assertRaisesRegex(self.parser.ValidationError, 'emitted FAIL'):
            self.parser.validate_run(parsed, manifest)

    def test_validation_rejects_partial_sample_set(self):
        manifest = self.minimal_manifest(include_optional=False)
        parsed = self.parse_run("""\
tournament: schema=1
tournament: lane=scheduler
pkg: example.test/tournament
BenchmarkOne/Main-8  1  10 ns/op  1 B/op  1 allocs/op
PASS
tournament: complete
""")

        with self.assertRaisesRegex(self.parser.ValidationError, 'has 1 samples, want 2'):
            self.parser.validate_run(parsed, manifest)

    def test_validation_rejects_missing_terminal_marker(self):
        manifest = self.minimal_manifest(include_optional=False)
        parsed = self.parse_run("""\
tournament: schema=1
tournament: lane=scheduler
pkg: example.test/tournament
BenchmarkOne/Main-8  1  10 ns/op  1 B/op  1 allocs/op
BenchmarkOne/Main-8  1  12 ns/op  1 B/op  1 allocs/op
PASS
""")

        with self.assertRaisesRegex(self.parser.ValidationError, 'terminal'):
            self.parser.validate_run(parsed, manifest)

    def test_two_platform_methodology_matches_product_target(self):
        analyzer = load_sibling_module('analyze_2platform')

        self.assertIn('eventloop-tournament-bench', analyzer.METHODOLOGY)
        self.assertIn('internal/tournament', analyzer.METHODOLOGY)
        self.assertIn('internal/promisetournament', analyzer.METHODOLOGY)
        self.assertNotIn('EVENTLOOP_REVIEW11_BENCH_RE', analyzer.METHODOLOGY)
        self.assertNotIn('goja-eventloop', analyzer.METHODOLOGY)

    def test_two_platform_analysis_requires_matching_provenance(self):
        analyzer = load_sibling_module('analyze_2platform')
        darwin = self.canonical_analysis_result('darwin')
        linux = self.canonical_analysis_result('linux')
        report = analyzer.generate(darwin, linux)
        self.assertIn('Shared stable benchmark identities: 1', report)
        self.assertIn('source fingerprint `' + ('c' * 40) + '`', report)

        mismatches = (
            ('source fingerprint', ('source', 'source-fingerprint'), 'e' * 40),
            ('source HEAD', ('source', 'head'), 'f' * 40),
            ('source state', ('source', 'source-state'), 'clean'),
            ('Go release', ('source', 'go-version'), 'go version go1.27.0 linux/arm64'),
            ('Goja fork version', ('source', 'goja-fork-version'), 'v0.0.0-other'),
            ('manifest SHA-256', ('manifest', 'sha256'), '0' * 64),
            ('effective sample count', ('effective_sample_count',), 4),
            ('architecture', ('goarch',), 'amd64'),
            ('CPU identity', ('cpu',), ['Different CPU']),
        )
        for label, path, value in mismatches:
            with self.subTest(label=label):
                incompatible = copy.deepcopy(linux)
                target = incompatible
                for component in path[:-1]:
                    target = target[component]
                target[path[-1]] = value
                with self.assertRaises(ValueError) as raised:
                    analyzer.generate(darwin, incompatible)
                self.assertIn(label, str(raised.exception))

    def test_three_platform_analysis_requires_matching_source(self):
        analyzer = load_sibling_module('analyze_3platform')
        data = {
            'Darwin': self.canonical_analysis_result('darwin'),
            'Linux': self.canonical_analysis_result('linux'),
            'Windows': self.canonical_analysis_result('windows'),
        }
        data['Windows']['goarch'] = 'amd64'
        data['Windows']['cpu'] = ['Example Windows CPU']
        report = analyzer.generate(data)
        self.assertIn('Stable identities present on all three platforms: 1', report)
        self.assertIn('fingerprint `' + ('c' * 40) + '`', report)

        incompatible = copy.deepcopy(data)
        incompatible['Windows']['source']['source-fingerprint'] = 'e' * 40
        with self.assertRaises(ValueError) as raised:
            analyzer.generate(incompatible)
        self.assertIn('source fingerprint', str(raised.exception))

    def test_comparison_ignores_only_tournament_harness_metadata(self):
        make = shutil.which('gmake') or shutil.which('make')
        self.assertIsNotNone(make, 'GNU Make is required')
        project_root = Path(__file__).resolve().parents[3]

        def benchmark_log(revision, lane, values):
            samples = ''.join(
                f'BenchmarkShared-8  1  {value} ns/op  1 B/op  1 allocs/op\n'
                for value in values
            )
            return f'''\
goos: darwin
goarch: arm64
pkg: example.test/tournament
cpu: Example CPU
tournament-revision: id={revision}
tournament: lane={lane}
{samples}'''

        with tempfile.TemporaryDirectory() as directory:
            old_log = Path(directory, 'old.log')
            new_log = Path(directory, 'new.log')
            old_log.write_text(
                benchmark_log('old', 'historical', [10, 11, 9, 10, 10, 10]),
                encoding='utf-8',
            )
            new_log.write_text(
                benchmark_log('new', 'product', [20, 21, 19, 20, 20, 20]),
                encoding='utf-8',
            )

            result = subprocess.run(
                [
                    make,
                    '-C',
                    str(project_root),
                    '--no-print-directory',
                    'eventloop-tournament-compare',
                    f'OLD_LOG={old_log}',
                    f'NEW_LOG={new_log}',
                ],
                check=False,
                capture_output=True,
                text=True,
            )

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('vs base', result.stdout)
        self.assertIn('+100.00%', result.stdout)
        self.assertEqual(result.stdout.count('pkg: example.test/tournament'), 1)

    def test_source_fingerprint_excludes_dated_evidence_only(self):
        project_root = Path(__file__).resolve().parents[3]
        make = shutil.which('gmake') or shutil.which('make')
        self.assertIsNotNone(make, 'GNU Make is required')

        def fingerprint():
            result = subprocess.run(
                [
                    make,
                    '-C',
                    str(project_root),
                    '--no-print-directory',
                    'eventloop-tournament-source-fingerprint',
                ],
                check=False,
                capture_output=True,
                text=True,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            value = result.stdout.strip()
            self.assertRegex(value, r'^[0-9a-f]{40}$')
            return value

        first = fingerprint()
        evidence_directory = Path(__file__).with_name('2026-05-14')
        with tempfile.NamedTemporaryFile(
            'w',
            dir=evidence_directory,
            prefix='.fingerprint-regression-',
            encoding='utf-8',
        ) as artifact:
            artifact.write('dated evidence must not mutate source identity\n')
            artifact.flush()
            second = fingerprint()
        self.assertEqual(first, second)

        with tempfile.NamedTemporaryFile(
            'w',
            dir=project_root / 'eventloop',
            prefix='.fingerprint-regression-',
            encoding='utf-8',
        ) as artifact:
            artifact.write('live source must mutate source identity\n')
            artifact.flush()
            self.assertNotEqual(first, fingerprint())

    def test_source_fingerprint_rejects_newline_path(self):
        project_root = Path(__file__).resolve().parents[3]
        make = shutil.which('gmake') or shutil.which('make')
        self.assertIsNotNone(make, 'GNU Make is required')

        with tempfile.NamedTemporaryFile(
            'w',
            dir=project_root / 'eventloop',
            prefix='.fingerprint-regression-\n',
            encoding='utf-8',
        ) as artifact:
            artifact.write('ambiguous path framing must fail closed\n')
            artifact.flush()
            result = subprocess.run(
                [
                    make,
                    '-C',
                    str(project_root),
                    '--no-print-directory',
                    'eventloop-tournament-source-fingerprint',
                ],
                check=False,
                capture_output=True,
                text=True,
            )

        self.assertNotEqual(result.returncode, 0)
        self.assertIn('newline-bearing path', result.stderr)

    def test_validation_rejects_missing_workload_variant_cell(self):
        manifest = self.matrix_manifest()
        parsed = self.parse_run(self.valid_prefix(2) + """\
tournament: lane=scheduler
goos: darwin
goarch: arm64
cpu: Example CPU
pkg: example.test/tournament
BenchmarkOne/Main-8  1  10 ns/op  1 B/op  1 allocs/op
BenchmarkOne/Main-8  1  11 ns/op  1 B/op  1 allocs/op
BenchmarkTwo/Alt-8  1  12 ns/op  1 B/op  1 allocs/op
BenchmarkTwo/Alt-8  1  13 ns/op  1 B/op  1 allocs/op
PASS
tournament: complete
""")

        with self.assertRaisesRegex(self.parser.ValidationError, 'benchmark.*variants'):
            self.parser.validate_run(parsed, manifest)

    def test_validation_rejects_row_with_multiple_variant_aliases(self):
        manifest = self.matrix_manifest(benchmarks=['BenchmarkOne'])
        parsed = self.parse_run(self.valid_prefix(2) + """\
tournament: lane=scheduler
goos: darwin
goarch: arm64
cpu: Example CPU
pkg: example.test/tournament
BenchmarkOne/Main/Alt-8  1  10 ns/op  1 B/op  1 allocs/op
BenchmarkOne/Main/Alt-8  1  11 ns/op  1 B/op  1 allocs/op
PASS
tournament: complete
""")

        with self.assertRaisesRegex(self.parser.ValidationError, 'want exactly one'):
            self.parser.validate_run(parsed, manifest)

    def test_validation_rejects_missing_leaf_workload(self):
        manifest = self.minimal_manifest(include_optional=False)
        manifest['lanes'][0]['benchmark_leaves'] = {
            'BenchmarkOne': ['Depth=10', 'Depth=100'],
        }
        parsed = self.parse_run(self.valid_prefix(2) + """\
tournament: lane=scheduler
goos: darwin
goarch: arm64
cpu: Example CPU
pkg: example.test/tournament
BenchmarkOne/Main/Depth=10-8  1  10 ns/op  1 B/op  1 allocs/op
BenchmarkOne/Main/Depth=10-8  1  11 ns/op  1 B/op  1 allocs/op
PASS
tournament: complete
""")

        with self.assertRaisesRegex(self.parser.ValidationError, 'leaves'):
            self.parser.validate_run(parsed, manifest)

    def test_validation_rejects_benchmark_outside_lane(self):
        manifest = self.minimal_manifest(include_optional=False)
        parsed = self.parse_run(self.valid_prefix(2) + """\
goos: darwin
goarch: arm64
cpu: Example CPU
pkg: example.test/tournament
BenchmarkEvil/Main-8  1  1 ns/op  1 B/op  1 allocs/op
BenchmarkEvil/Main-8  1  1 ns/op  1 B/op  1 allocs/op
tournament: lane=scheduler
pkg: example.test/tournament
BenchmarkOne/Main-8  1  10 ns/op  1 B/op  1 allocs/op
BenchmarkOne/Main-8  1  11 ns/op  1 B/op  1 allocs/op
PASS
tournament: complete
""")

        with self.assertRaisesRegex(self.parser.ValidationError, 'outside a lane'):
            self.parser.validate_run(parsed, manifest)

    def test_validation_rejects_missing_provenance_and_platform(self):
        manifest = self.minimal_manifest(include_optional=False)
        parsed = self.parse_run("""\
tournament: schema=1
tournament: lane=scheduler
pkg: example.test/tournament
BenchmarkOne/Main-8  1  10 ns/op  1 B/op  1 allocs/op
BenchmarkOne/Main-8  1  11 ns/op  1 B/op  1 allocs/op
PASS
tournament: complete
""")

        with self.assertRaisesRegex(self.parser.ValidationError, 'misses metadata'):
            self.parser.validate_run(parsed, manifest)

    def test_validation_rejects_metadata_sample_disagreement(self):
        manifest = self.minimal_manifest(include_optional=False)
        parsed = self.parse_run(self.valid_prefix(999) + """\
tournament: lane=scheduler
goos: darwin
goarch: arm64
cpu: Example CPU
pkg: example.test/tournament
BenchmarkOne/Main-8  1  10 ns/op  1 B/op  1 allocs/op
BenchmarkOne/Main-8  1  11 ns/op  1 B/op  1 allocs/op
PASS
tournament: complete
""")

        with self.assertRaisesRegex(self.parser.ValidationError, 'sample-count metadata'):
            self.parser.validate_run(parsed, manifest)

    def test_validation_rejects_optional_skip_without_reason(self):
        manifest = self.minimal_manifest()
        parsed = self.parse_run(self.valid_prefix(2) + """\
tournament: lane=scheduler
goos: darwin
goarch: arm64
cpu: Example CPU
pkg: example.test/tournament
BenchmarkOne/Main-8  1  10 ns/op  1 B/op  1 allocs/op
BenchmarkOne/Main-8  1  11 ns/op  1 B/op  1 allocs/op
PASS
tournament: skip=libuv
tournament: complete
""")

        with self.assertRaisesRegex(self.parser.ValidationError, 'skip has no reason'):
            self.parser.validate_run(parsed, manifest)

    def test_windows_applicability_excludes_unix_benchmark(self):
        manifest = self.minimal_manifest(include_optional=False)
        manifest['lanes'][0]['benchmarks'].append('BenchmarkUnix')
        manifest['lanes'][0]['benchmark_goos'] = {
            'BenchmarkUnix': ['darwin', 'linux'],
        }
        parsed = self.parse_run(self.valid_prefix(2) + """\
tournament: lane=scheduler
goos: windows
goarch: amd64
cpu: Example CPU
pkg: example.test/tournament
BenchmarkOne/Main-8  1  10 ns/op  1 B/op  1 allocs/op
BenchmarkOne/Main-8  1  11 ns/op  1 B/op  1 allocs/op
PASS
tournament: complete
""")

        self.parser.validate_run(parsed, manifest)

    def test_load_manifest_rejects_unknown_field(self):
        manifest = self.parser.default_manifest_path().read_text(encoding='utf-8')
        mutated = manifest.replace('"sample_count": 5,', '"sample_count": 5,\n  "typo": true,', 1)
        with tempfile.NamedTemporaryFile('w', encoding='utf-8') as file:
            file.write(mutated)
            file.flush()
            with self.assertRaisesRegex(self.parser.ValidationError, 'unknown fields'):
                self.parser.load_manifest(file.name)

    def test_analyzers_reject_smoke_result(self):
        smoke = {'validated': True, 'evidence_class': 'smoke'}
        for analyzer in (
            load_sibling_module('analyze_2platform'),
            load_sibling_module('analyze_3platform'),
        ):
            with self.assertRaisesRegex(ValueError, 'canonical'):
                analyzer.require_validated(smoke, 'smoke.json')

    def parse_run(self, log):
        with tempfile.NamedTemporaryFile('w', encoding='utf-8') as file:
            file.write(log)
            file.flush()
            return self.parser.parse_run(file.name)

    @staticmethod
    def canonical_analysis_result(goos):
        statistics = {
            metric: {'mean': value, 'min': value, 'max': value, 'stddev': 0}
            for metric, value in [('ns_op', 10), ('b_op', 1), ('allocs_op', 1)]
        }
        return {
            'validated': True,
            'evidence_class': 'canonical',
            'effective_sample_count': 5,
            'platform': goos.title(),
            'goos': goos,
            'goarch': 'arm64',
            'cpu': ['Example CPU'],
            'source': {
                'head': 'a' * 40,
                'source-state': 'dirty',
                'go-version': f'go version go1.26.5 {goos}/arm64',
                'sample-count': '5',
                'manifest-git-blob': 'b' * 40,
                'goja-fork-version': 'v0.0.0-goja',
                'goja-nodejs-version': 'v0.0.0-goja-nodejs',
                'source-fingerprint': 'c' * 40,
            },
            'manifest': {
                'schema_version': 1,
                'sha256': 'd' * 64,
                'git_blob': 'b' * 40,
                'sample_count': 5,
                'effective_sample_count': 5,
            },
            'benchmarks': [{
                'lane': 'scheduler',
                'package': 'example.test/tournament',
                'name': 'BenchmarkOne/Main-8',
                'stable_name': 'BenchmarkOne/scheduler.main.auto',
                'variant_ids': ['scheduler.main.auto'],
                'runs': [{'ns_op': 10, 'b_op': 1, 'allocs_op': 1}],
                'statistics': statistics,
            }],
        }

    @staticmethod
    def valid_prefix(sample_count):
        return f"""\
tournament: schema=1
tournament: meta=head=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
tournament: meta=source-state=dirty
tournament: meta=go-version=go version go1.26.5 darwin/arm64
tournament: meta=sample-count={sample_count}
tournament: meta=manifest-git-blob=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
tournament: meta=goja-fork-version=v0.0.0-test
tournament: meta=goja-nodejs-version=v0.0.0-test
tournament: meta=source-fingerprint=cccccccccccccccccccccccccccccccccccccccc
"""

    @staticmethod
    def matrix_manifest(benchmarks=None):
        benchmarks = benchmarks or ['BenchmarkOne', 'BenchmarkTwo']
        return {
            'schema_version': 1,
            'sample_count': 2,
            'variants': [
                {'id': 'scheduler.main.auto', 'aliases': ['Main']},
                {'id': 'scheduler.alt', 'aliases': ['Alt']},
            ],
            'variant_groups': {
                'scheduler': ['scheduler.main.auto', 'scheduler.alt'],
            },
            'lanes': [{
                'id': 'scheduler',
                'package': 'example.test/tournament',
                'required': True,
                'benchmarks': benchmarks,
                'benchmark_variant_groups': {
                    benchmark: 'scheduler' for benchmark in benchmarks
                },
                'variant_ids': ['scheduler.main.auto', 'scheduler.alt'],
            }],
        }

    @staticmethod
    def minimal_manifest(include_optional=True):
        lanes = [{
            'id': 'scheduler',
            'package': 'example.test/tournament',
            'required': True,
            'benchmarks': ['BenchmarkOne'],
            'benchmark_variant_groups': {'BenchmarkOne': 'scheduler'},
            'variant_ids': ['scheduler.main.auto'],
        }]
        if include_optional:
            lanes.append({
                'id': 'libuv',
                'package': 'example.test/libuv',
                'required': False,
                'benchmarks': ['BenchmarkLibuv'],
                'benchmark_variant_groups': {'BenchmarkLibuv': 'libuv'},
                'variant_ids': ['scheduler.libuv.native'],
                'default_variant_id': 'scheduler.libuv.native',
            })
        return {
            'schema_version': 1,
            'sample_count': 2,
            'variants': [
                {
                    'id': 'scheduler.main.auto',
                    'aliases': ['Main'],
                },
                {
                    'id': 'scheduler.libuv.native',
                    'aliases': [],
                },
            ],
            'variant_groups': {
                'scheduler': ['scheduler.main.auto'],
                'libuv': ['scheduler.libuv.native'],
            },
            'lanes': lanes,
        }


if __name__ == '__main__':
    unittest.main()
