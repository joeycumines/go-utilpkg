# Eventloop Tournament Evidence — 2026-05-14

This directory preserves the raw, parsed, and comparison artifacts for the
restored longitudinal tournament. Raw logs are authoritative; parsed JSON is
an indexed view and must pass the strict current manifest.

`darwin.failed-random-deadlines.log` is the rejected first canonical attempt.
It has no aggregate completion marker: the product lane exposed that
`BenchmarkTimerScheduleRandomDeadlines` allowed calibrated near-deadline timers
to fire before untimed cleanup. The benchmark now holds loop ownership through
cleanup, and a deterministic due-timer regression plus a focused five-sample
benchmark gate protect that boundary. The failed log is retained as defect
evidence and must never be used for performance analysis.

Rejected artifact SHA-256:

- `darwin.failed-random-deadlines.log`:
  `c47d642b642fe405e8e7711b2043db79049634d99955affb8345991bb027d9ff`

Final Darwin and Linux artifact hashes and validation results will be recorded
here only after both canonical runs complete.

## Superseded Darwin canonical

The strict parser accepts `darwin.pre-source-fingerprint-fix.log` as canonical
evidence for its exact bytes, with five samples, four lane `PASS` markers, one
aggregate completion marker, 1,940 raw benchmark rows, 388 stable benchmark
records, and all 14 executable variants. It is superseded for cross-platform
comparison because preserving its dated JSON and README changed the old
over-broad source fingerprint. The live fingerprint now excludes dated result
archives, and final Darwin/Linux evidence must be collected under that stable
boundary.

Its provenance is Go 1.26.5 on darwin/arm64, Apple M3 Max, libuv 1.52.1,
source fingerprint `aeb5829e6f5ad205d58f11df1639f687cbd211a3`, and manifest
Git blob `d3527250806642fab6eab5a0a177121e2d23e52a`.

- `darwin.pre-source-fingerprint-fix.log` SHA-256:
  `bd565d6ec2ea788f839d87d02b8ff20b14af59229c9c27a4d05d39f21351af12`
- `darwin.pre-source-fingerprint-fix.json` SHA-256:
  `21f8119937d9ab9defbd51b3a4d32aba759843a1dbb651fd7c7f3a3288efd04e`
- `manifest.json` SHA-256:
  `1bb9c1f3ce395a8486a632e6bf56f5457a8bace601f20cff4cadd03e72829ad6`

## Archived v1 tooling

The live tournament moved to protocol v2 after the hostile pre-canonical audit.
The exact final v1 parser, tests, and analyzers are preserved here so the
superseded artifact remains reproducible. Because the archived v1 parser's
historical default lookup prefers a live ancestor manifest, invoke it with the
explicit sibling snapshot:

```bash
python3 parse_benchmarks.py \
  darwin.pre-source-fingerprint-fix.log darwin darwin arm64 \
  --manifest manifest.json > /tmp/darwin-v1.json
```

- `parse_benchmarks.py` SHA-256:
  `8af000871b0c6293184d75af6ba2caafcf93bf96cbd3334fd8e088e62dc70451`
- `parse_benchmarks_test.py` SHA-256:
  `1de6b4ad0376bac417214618e59e55fa06da976c2135ea5db3392f683cc0bf2e`
- `analyze_2platform.py` SHA-256:
  `d9b7c6423c078023c2beae9c586f11f87bf9af209b5ee7f3e32a085c2f597f0f`
- `analyze_3platform.py` SHA-256:
  `8a1d52428ba481290adc60ffb55b3eea808758c1690361e63bb2be119f8386c5`
