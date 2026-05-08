#!/usr/bin/env python3
"""Cross-language regression tests for timer workload digest framing."""

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
    / 'timerworkloaddigestvectors.json'
)
DOMAIN = 'go-utilpkg/eventloop/tournament/timer-parameters/v1'
LOWER_SHA256 = re.compile(r'^[0-9a-f]{64}$')


def framed_sha256(domain, fields):
    digest = hashlib.sha256()
    for field in [domain, *fields]:
        payload = field.encode('utf-8')
        digest.update(struct.pack('>Q', len(payload)))
        digest.update(payload)
    return digest.hexdigest()


class TimerWorkloadDigestTest(unittest.TestCase):
    def test_go_vectors_match_independent_python_framing(self):
        file = json.loads(VECTOR_PATH.read_text(encoding='utf-8'))
        self.assertEqual(
            set(file),
            {'schema_version', 'domain', 'framing_vectors', 'workload_vectors'},
        )
        self.assertEqual(file['schema_version'], 1)
        self.assertEqual(file['domain'], DOMAIN)

        framing_names = set()
        for vector in file['framing_vectors']:
            with self.subTest(framing=vector.get('name')):
                self.assertEqual(set(vector), {'name', 'domain', 'fields', 'sha256'})
                self.assertNotIn(vector['name'], framing_names)
                framing_names.add(vector['name'])
                self.assertRegex(vector['sha256'], LOWER_SHA256)
                self.assertEqual(
                    framed_sha256(vector['domain'], vector['fields']),
                    vector['sha256'],
                )
        self.assertEqual(len(framing_names), 3)

        workload_keys = set()
        for vector in file['workload_vectors']:
            with self.subTest(kind=vector.get('kind'), workload=vector.get('id')):
                self.assertEqual(
                    set(vector),
                    {'kind', 'id', 'parameter_type', 'parameter_json', 'sha256'},
                )
                key = (vector['kind'], vector['id'])
                self.assertNotIn(key, workload_keys)
                workload_keys.add(key)
                self.assertIn(vector['kind'], {'storage', 'qualification'})
                json.loads(vector['parameter_json'])
                self.assertRegex(vector['sha256'], LOWER_SHA256)
                self.assertEqual(
                    framed_sha256(
                        DOMAIN,
                        [
                            vector['kind'],
                            vector['id'],
                            vector['parameter_type'],
                            vector['parameter_json'],
                        ],
                    ),
                    vector['sha256'],
                )
        self.assertEqual(len(workload_keys), 26)


if __name__ == '__main__':
    unittest.main()
