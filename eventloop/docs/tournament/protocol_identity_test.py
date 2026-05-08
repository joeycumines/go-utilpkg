#!/usr/bin/env python3
"""Independent verification of tournament protocol identity vectors."""

import hashlib
import json
from pathlib import Path
import re
import struct
import unittest


VECTOR_PATH = (
    Path(__file__).resolve().parents[2]
    / 'internal'
    / 'tournament'
    / 'testdata'
    / 'protocolidentityvectors.json'
)
LOWER_SHA256 = re.compile(r'^[0-9a-f]{64}$')


def framed_sha256(domain, fields):
    digest = hashlib.sha256()
    for field in [domain, *fields]:
        payload = field.encode('utf-8')
        digest.update(struct.pack('>Q', len(payload)))
        digest.update(payload)
    return digest.hexdigest()


def strict_json_loads(payload):
    def object_pairs(pairs):
        result = {}
        for key, value in pairs:
            if key in result:
                raise ValueError(f'duplicate JSON key: {key}')
            result[key] = value
        return result

    def reject_constant(value):
        raise ValueError(f'non-finite JSON number: {value}')

    return json.loads(
        payload,
        object_pairs_hook=object_pairs,
        parse_constant=reject_constant,
    )


def settings_fields(settings):
    fields = ['configuration-count', str(len(settings))]
    for setting in settings:
        fields.extend(['configuration', setting['key'], setting['value']])
    return fields


def comparison_fields(value):
    fields = [
        'implementation-id', value['implementation_id'],
        'workload-id', value['workload_id'],
        'semantic-harness-id', value['semantic_harness_id'],
        'measurement-contract-sha256', value['measurement_contract_sha256'],
        'stable-leaf', value['stable_leaf'],
    ]
    return fields + settings_fields(value['configuration'])


def unit_fields(value):
    fields = [
        'lane-id', value['lane_id'],
        'build-cell-id', value['build_cell_id'],
        'module-id', value['module_id'],
        'package', value['package'],
        'raw-root-id', value['raw_root_id'],
        'harness-id', value['harness_id'],
        'applicability', value['applicability'],
        'measurement-contract-sha256', value['measurement_contract_sha256'],
        'binding-count', str(len(value['bindings'])),
    ]
    for binding in value['bindings']:
        fields.extend([
            'binding',
            'binding-id', binding['binding_id'],
            'benchmark', binding['benchmark'],
            'implementation-id', binding['implementation_id'],
            'snapshot-count', str(len(binding['snapshot_ids'])),
        ])
        for snapshot_id in binding['snapshot_ids']:
            fields.extend(['snapshot-id', snapshot_id])
        fields.extend([
            'workload-id', binding['workload_id'],
            'semantic-harness-id', binding['semantic_harness_id'],
        ])
        fields.extend(settings_fields(binding['configuration']))
        fields.extend(['result-count', str(len(binding['results']))])
        for result in binding['results']:
            fields.extend([
                'result',
                'emitted-leaf', result['emitted_leaf'],
                'stable-leaf', result['stable_leaf'],
                'comparison-id', result['comparison_id'],
            ])
    return fields


def selection_plan_fields(value):
    fields = [
        'manifest-sha256', value['manifest_sha256'],
        'lineage-sha256', value['lineage_sha256'],
        'lineage-floor-sha256', value['lineage_floor_sha256'],
        'shared-source-id', value['shared_source_id'],
        'source-capture-id', value['source_capture_id'],
        'host-authority-id', value['host_authority_id'],
        'measurement-contract-sha256', value['measurement_contract_sha256'],
        'build-cell-count', str(len(value['build_cell_ids'])),
    ]
    for build_cell_id in value['build_cell_ids']:
        fields.extend(['build-cell-id', build_cell_id])
    fields.extend(['unit-count', str(len(value['unit_ids']))])
    for unit_id in value['unit_ids']:
        fields.extend(['unit-id', unit_id])
    fields.extend(['disposition-count', str(len(value['dispositions']))])
    for disposition in value['dispositions']:
        fields.extend([
            'disposition',
            'kind', disposition['kind'],
            'subject-id', disposition['subject_id'],
            'authority-id', disposition['authority_id'],
        ])
    return fields


def execution_fields(value):
    fields = [
        'plan-id', value['plan_id'],
        'unit-id', value['unit_id'],
        'shared-source-id', value['shared_source_id'],
        'source-capture-id', value['source_capture_id'],
        'binary-sha256', value['binary_sha256'],
        'toolchain-authority-sha256', value['toolchain_authority_sha256'],
        'module-graph-sha256', value['module_graph_sha256'],
        'native-authority-sha256', value['native_authority_sha256'],
        'host-authority-id', value['host_authority_id'],
        'measurement-profile-sha256', value['measurement_profile_sha256'],
        'execution-profile-sha256', value['execution_profile_sha256'],
        'argv-count', str(len(value['argv'])),
    ]
    for argument in value['argv']:
        fields.extend(['argv', argument])
    fields.extend(['environment-count', str(len(value['environment']))])
    for environment in value['environment']:
        fields.extend(['environment', environment])
    return fields


class ProtocolIdentityTest(unittest.TestCase):
    def test_go_vectors_match_independent_python_framing(self):
        data = strict_json_loads(VECTOR_PATH.read_text(encoding='utf-8'))
        self.assertEqual(
            set(data),
            {
                'schema_version', 'algorithm', 'domains', 'framing_vectors',
                'comparison', 'unit', 'selection_plan', 'execution',
            },
        )
        self.assertEqual(data['schema_version'], 1)
        self.assertEqual(data['algorithm'], 'sha256-domain-length-framed-v1')
        self.assertEqual(
            set(data['domains']),
            {'comparison', 'unit', 'selection_plan', 'execution'},
        )
        for vector in data['framing_vectors']:
            self.assertEqual(
                framed_sha256(vector['domain'], vector['fields']),
                vector['sha256'],
            )

        cases = [
            ('comparison', comparison_fields),
            ('unit', unit_fields),
            ('selection_plan', selection_plan_fields),
            ('execution', execution_fields),
        ]
        for name, field_builder in cases:
            with self.subTest(identity=name):
                vector = data[name]
                self.assertRegex(vector['sha256'], LOWER_SHA256)
                self.assertEqual(
                    framed_sha256(data['domains'][name], field_builder(vector['input'])),
                    vector['sha256'],
                )


if __name__ == '__main__':
    unittest.main()
