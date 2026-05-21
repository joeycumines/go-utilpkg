# Eventloop Performance Tournament

> **Status:** This branch retains an incomplete, unqualified corpus that may
> omit major variants. It does not establish correctness, a winner, a live
> baseline, or longitudinal-performance conclusions.

The tournament is the longitudinal performance laboratory for eventloop. It
keeps current production measurements separate from comparisons among every
retained scheduler and Promise design. Historical alternatives are evidence,
not dead code: a design stays runnable even after another design becomes the
product default.

The raw Go benchmark log is the authoritative measurement artifact. Parsed
JSON is an indexed view of that log. Statistical comparisons use the
repository-pinned `benchstat`; the Python reports intentionally provide only
descriptive values and observed deltas.

## Tournament surface

`internal/tournament/manifest.json` is the machine-checked inventory for the
currently governed schema. It gives each admitted design a stable ID, records
the exact source tree for historical designs, records its executable source
package, declares benchmark lanes and workloads, and pins the normal sample
count. A separate active-root census fails closed when a restored or newly
added benchmark has not yet received a manifest binding or typed disposition.
While that census is red, the manifest is a migration input rather than a
complete claim about the live tournament.

The complete tournament has four lanes:

| Lane | Purpose | Required |
|---|---|---|
| `product` | Current eventloop production paths | Yes |
| `scheduler` | Current modes, three historical schedulers, Goja baseline, and all Promise designs | Yes |
| `promise` | Independent end-to-end Promise tournament, including chain depth | Yes |
| `libuv` | Preserved native legacy endpoints plus checked V2 wake and timer endpoints | When an exact `pkg-config libuv` authority is available; otherwise the raw log records an explicit skip |

The stable scheduler IDs cover automatic, forced, and disabled current fast
paths; Alternate One, Two, and Three; the Goja Node.js baseline; and libuv. The
stable Promise IDs cover the current chained Promise plus Alternate One through
Five. Manifest and active-root tests jointly expose drift; neither an omitted
binding nor an unclassified historical limitation is silently ignored.

Capability-specific groups prevent a variant from receiving a fabricated row
for an API it never implemented. The scheduler-wide group covers general Go
task admission, the microtask group covers the five implementations with an
executable owner-local microtask adapter, and the readiness group covers the
five implementations with an executable FD-readiness adapter. Readiness
workloads and the four Unix-only product roots run on Darwin and Linux only;
their absence on Windows is declared applicability, not missing evidence.

The following documented designs have no implementation in repository history.
They remain stable concept identities so a future implementation can join the
correct longitudinal record, but they must never produce invented benchmark
rows:

| Stable concept ID | Documented design | Tournament status |
|---|---|---|
| `scheduler.alternate-two-plus.dual-wake` | AlternateTwo-Plus dual wake | Concept only |
| `scheduler.alternate-two-plus.mini-fast-path` | AlternateTwo-Plus mini fast path | Concept only |
| `scheduler.alternate-three.fast-path` | AlternateThree with fast path | Concept only |
| `scheduler.main.task-arena.conservative` | Main with optional TaskArena | Concept only |
| `scheduler.main.task-arena.hybrid` | Main with hybrid TaskArena fallback | Concept only |

Historical descriptions such as obsolete, unsuitable for production, or a
candidate for retirement describe product selection only. They never authorize
removing an implementation, adapter, workload, manifest identity, or raw result
from the tournament.

The old in-process `TournamentResults.BenchmarkData` mechanism is not a timing
authority. Go calibrates benchmarks by calling them repeatedly, so values
recorded from inside a benchmark can mix calibration and final runs. Only the
raw benchmark records emitted by `go test` are used for performance analysis.

## Run the current tournament

Use GNU Make from the monorepo root (`gmake` on macOS):

```bash
# Complete current-host tournament.
gmake eventloop-tournament-bench

# Current product only.
gmake eventloop-product-bench

# Individual longitudinal lanes.
gmake eventloop-tournament-scheduler-bench
gmake eventloop-tournament-promise-bench
gmake eventloop-tournament-libuv-bench
```

The libuv lane keeps its four original benchmark roots and source files
byte-frozen for longitudinal reruns. Those legacy endpoints have unchecked
failure paths and can block on invalid preconditions, so they are diagnostic
history rather than robustness references. Four additive V2 roots use checked
construction, bounded condition-variable generations, explicit Go/native
ownership, exact callback cardinality, and verified teardown. The threaded V2
endpoint finishes at the prepare phase immediately before the next I/O poll;
it is a checked cross-thread round trip, not proof that the sending goroutine
woke an already-blocked kernel poll. The synchronous V2 timer endpoints drain
naturally, unlike the legacy dummy-async plus `uv_stop` topology, so the two
generations have distinct workload identities rather than fabricated numeric
equivalence. The benchmark target first runs the no-performance native
correctness gate, then runs V2 and legacy groups in separate processes, V2
first:

```bash
gmake eventloop-tournament-libuv-test
```

Availability through ambient `pkg-config` is sufficient for local correctness
work, not canonical cross-host evidence. A governed run must additionally bind
the package metadata, headers, linked library, compiler/linker inputs, and
captured source. In particular, a plain Linux container that lacks that
authority records libuv as unavailable; it does not inherit a Darwin result.
The direct Make benchmark target is a local diagnostic surface: roots within
each generation still share one `go test` process. Canonical evidence requires
the schema-5 runner to put every exact root in its own watchdog-controlled
process so a legacy semaphore hang cannot suppress unrelated results.

The local wrappers in `config.mk` capture complete logs:

```bash
# Darwin and Linux Docker, writing eventloop-tournament-{darwin,linux}.log.
gmake eventloop-tournament

# Windows is explicit and may only be claimed after a real host ran it.
# See "Run the tournament on Windows" below for host setup and the libuv build.
WINDOWS_HOST=your-host gmake eventloop-tournament-windows
```

## Run the tournament on Windows

A Windows run is genuine only when it executes the real `windows/amd64 go.exe`,
never Go under WSL. The harness transfers the repository as a clean tarball
(`hack/run-on-windows.sh`), so the remote workspace has no `.git`; the source
HEAD and dirty/clean state are captured on the originating checkout and
forwarded as positional make variables (`EVENTLOOP_TOURNAMENT_HEAD`,
`EVENTLOOP_TOURNAMENT_SOURCE_STATE`), which `eventloop-tournament-bench`
honors in its metadata lines. The libuv lane must not be skipped: the run
records `tournament: meta=libuv-version=<ver>` and a `lane=libuv` marker only
when pkg-config resolves libuv on the host.

Prerequisites on the Windows host:

- Git for Windows (provides `bash.exe`; the launcher must invoke this, never
  bare `bash`, which `run-on-windows.sh` resolves to WSL bash and Linux tools).
- Go for Windows matching the module's `go` directive (`go1.26.5` or later).
- GNU Make and pkg-config on the Windows `PATH`.
- A MinGW-w64 C toolchain whose runtime ABI matches Go's `runtime/cgo`, and a
  libuv built against that toolchain.

The host-specific paths are overridable make variables (defaults match the
`moo` host; set them for a different machine):

```bash
WINDOWS_HOST=moo                              # SSH alias / host
GIT_BASH='C:/Program Files/Git/bin/bash.exe'  # Git-for-Windows bash
# forwarded into hack/run-eventloop-tournament-windows.sh:
MINGW_BIN=...                                  # mingw-w64 gcc bin dir
LIBUV_PKG_CONFIG_PATH=...                      # dir holding libuv.pc
CGO_LDFLAGS='-static-libgcc -static-libstdc++ -static'
```

Build libuv from source with the matching toolchain using
`hack/build-libuv-mingw8.sh`. It downloads libuv 1.52.0, builds it as a static
library with MinGW-w64 8.1.0, installs it into a prefix, and writes a
pkg-config `.pc` file the tournament consumes via `PKG_CONFIG_PATH`. The script
passes the small set of C defines (`PROCESSOR_ARCHITECTURE_ARM64`,
`WSA_FLAG_NO_HANDLE_INHERIT`, `FILE_DEVICE_CONSOLE`) that the older mingw 8.1.0
headers lack but the libuv 1.52.0 source references. Run it through Git bash on
the host (not WSL bash):

```bash
# On the Windows host, via Git bash:
bash hack/build-libuv-mingw8.sh
```

Then run the tournament. The `eventloop-tournament-windows` config target
captures the local HEAD/state, routes through the launcher (which sets up the
libuv/mingw environment and fails fast if pkg-config cannot find libuv), and
forwards the provenance overrides:

```bash
gmake eventloop-tournament-windows
```

The libuv lane semantics (byte-frozen legacy endpoints, checked V2 roots,
distinct workload identities) are described above under "Run the current
tournament"; they apply unchanged on Windows.


Normal runs use five one-second samples with allocation reporting. For a smoke
check, override the count and benchmark time without changing the manifest:

```bash
gmake eventloop-tournament-bench \
  EVENTLOOP_TOURNAMENT_COUNT=1 \
  EVENTLOOP_TOURNAMENT_BENCHTIME=1x
```

A smoke log is not a valid normal tournament. The parser accepts its different
sample count only when explicitly passed `--expected-samples 1`, stamps the
result with the `smoke` evidence class, and the analyzers refuse to use it as
canonical evidence. Five samples produce the `canonical` evidence class.

Every aggregate raw log includes:

- the source HEAD and whether governed working bytes were dirty;
- the exact Go version and requested sample count;
- Go's `goos`, `goarch`, package, and CPU headers;
- an explicit marker for every lane;
- one `PASS` for every executed lane or an explicit optional-lane skip; and
- a terminal completion marker.

The source fingerprint covers every physical tracked or untracked file in the
eventloop, workspace, and tournament build surface, including restored files
that are currently staged for deletion. Dated result archives under
`docs/tournament/YYYY-MM-DD/` are excluded: preserving a raw or parsed result
must not mutate the source identity of the next platform run. The live
GNU Make fingerprint target and its tests remain inside the governed surface.
It hashes bounded file batches, frames the final path/blob stream with NUL
delimiters, and rejects newline-bearing paths rather than accepting ambiguous
input.

Dependency metadata names the actual executable Goja forks,
`github.com/joeycumines/goja` and `github.com/joeycumines/goja_nodejs`, with
their resolved versions. A log naming a different module is not equivalent
provenance for the Goja baseline.

If a command fails, the completion marker is absent. The parser rejects that
log rather than returning a partial comparison.

## Parse and preserve a run

Create a dated evidence directory and preserve the raw logs before analysis:

```bash
run_date=$(date +%Y-%m-%d)
run_dir="eventloop/docs/tournament/$run_date"
mkdir -p "$run_dir"
cp eventloop-tournament-darwin.log "$run_dir/darwin.log"
cp eventloop-tournament-linux.log "$run_dir/linux.log"

python3 eventloop/docs/tournament/parse_benchmarks.py \
  "$run_dir/darwin.log" darwin unknown unknown > "$run_dir/darwin.json"
python3 eventloop/docs/tournament/parse_benchmarks.py \
  "$run_dir/linux.log" linux unknown unknown > "$run_dir/linux.json"
```

Also copy the exact manifest into the dated directory when publishing results.
The parsed JSON records its SHA-256 digest, and each result row contains both
the display name and a stable name in which variant aliases are replaced by
stable IDs.

The parser fails closed when any of these conditions holds:

- a required lane is missing, skipped, failed, or lacks exactly one `PASS`;
- an optional lane is neither executed nor explicitly skipped;
- a declared workload or variant is absent;
- an undeclared workload appears;
- a result row has the wrong sample count;
- multiple GOOS or GOARCH values are mixed; or
- the terminal completion marker is absent.

Coverage is the exact Cartesian workload-by-variant matrix declared by the
manifest, including declared leaf subworkloads. One row cannot satisfy two
variant aliases, and a lane-level union cannot hide a missing cell.

Old unmarked raw logs can be inspected with `--legacy`. Their JSON is stamped
`"validated": false`; current analysis scripts refuse to treat it as a valid
tournament result.

## Analyze results

Copy the current analyzer scripts into a dated directory or run them from the
tournament root after placing the JSON files beside them:

```bash
python3 analyze_2platform.py

# Only after a real Windows run produced validated windows.json.
python3 analyze_3platform.py
```

The reports retain package identity and stable variant identity, show missing
coverage explicitly, and calculate descriptive means, sample standard
deviation, coefficient of variation, allocations, and observed ratios. They do
not label a delta significant.

Both analyzers require canonical evidence. Before joining Darwin and Linux,
the two-platform analyzer requires the expected GOOS roles and identical source
fingerprint, HEAD/state, architecture, CPU identity, Go release, Goja dependency
versions, manifest identities, and sample count. The three-platform report
requires identical governed source and tool provenance but permits architecture
and CPU differences because it explicitly describes machine effects. For
longitudinal old/new sections, each platform still requires matching GOOS,
architecture, CPU, Go version, Goja dependencies, manifest, and sample count.
Re-run incompatible evidence in one controlled environment instead of
overriding those checks.

For statistical comparison, use raw logs from two controlled runs:

```bash
gmake eventloop-tournament-compare \
  OLD_LOG=/path/to/old.log \
  NEW_LOG=/path/to/new.log
```

That target invokes the version of `golang.org/x/perf/cmd/benchstat` pinned in
the root Go tool set. It ignores only the tournament harness metadata keys so
revision IDs and lane markers cannot split otherwise compatible rows into
separate tables; Go version, platform, package, CPU, and benchmark dimensions
remain intact. Do not replace its model with a hand-written t threshold.

## Compare revisions on current hardware

Historical JSON from another machine or Go version cannot establish a code
regression. Re-run the old source on the same hardware and current Go toolchain.
The revision target uses `git archive` into a private temporary directory, runs
every benchmark present in that revision (including deleted benchmark files and
that revision's nested tournament when present), and removes the archive on
exit. It does not switch or modify the active worktree.

Pass a stable revision ID from the manifest, not an arbitrary revision. The
accepted longitudinal checkpoints are:

| Stable revision ID | Governed source |
|---|---|
| `scheduler.revision.initial-go-native` | `506d6643cc1d45b1da156096870991ecb30b8847` |
| `scheduler.revision.auto-exit-liveness` | `cc005d72b329fd91eee03aac62ba7188df7c91b9` |
| `scheduler.revision.node26-refactor` | `0def02e2ff987be01a38d237a5d84dae256a85ac` |
| `scheduler.revision.tournament-snapshot` | `27b93ec32938ca838e1519bc8e17b6852d7df449` |
| `scheduler.revision.unix-poller-hardened` | `986e2378c1484aa917a1bb0fd13aef914bdce50f` |
| `scheduler.revision.current-candidate` | Live governed worktree (`current`) |

```bash
gmake eventloop-tournament-revision-bench \
  REVISION=scheduler.revision.initial-go-native \
  > revision-initial-go-native.log 2>&1
```

Repeat for the checkpoints in `manifest.json`, then compare overlapping raw
benchmark identities with `eventloop-tournament-compare`. Exact-revision logs
are intentionally not forced into the current manifest: old revisions had
different workloads and variants, and pretending they have current coverage
would be false. `benchstat` reports the real intersection.

`scheduler.revision.current-candidate` runs the governed bytes in the live
worktree so staged or committed snapshots cannot silently replace the candidate
being evaluated. Historical IDs run fail-fast archives and emit a completion
marker only after every selected package succeeds. When that revision contains
a `go.work`, the archive uses it so local modules that historically supplied
undeclared workspace dependencies remain reproducible; otherwise it runs with
workspace mode disabled. The selected mode is written into the raw log. An
unknown revision or a failed historical package makes the target fail.

## Interpretation rules

1. Correctness gates every performance result. A faster design that fails the
   tournament's lifecycle, conservation, panic, or ordering tests is not a
   candidate.
2. Compare revisions on the same hardware, OS, architecture, Go version,
   benchmark flags, and dependency versions. Otherwise re-run them.
3. A dirty source state is suitable for local development measurements only.
   Preserve the exact source bytes before citing it later.
4. Cross-OS ratios are descriptive. They combine OS, container, runtime, and
   toolchain effects and do not prove a regression.
5. Missing rows are coverage gaps, never infinite slowdowns or implicit wins.
6. Keep raw logs. Parsed means alone are insufficient for `benchstat` or later
   forensic checks.
7. Preserve the manifest digest and stable IDs. Human-readable names may evolve;
   the stable identity is the longitudinal join key.
8. Dated reports preserve the methodology and conclusions of their recorded run.
   They are historical evidence, not current execution instructions; use this
   guide and the live manifest for a new run.

## Durable files

```text
eventloop/docs/tournament/
├── README.md
├── parse_benchmarks.py
├── parse_benchmarks_test.py
├── analyze_2platform.py
├── analyze_3platform.py
└── YYYY-MM-DD/
    ├── manifest.json
    ├── darwin.log
    ├── darwin.json
    ├── linux.log
    ├── linux.json
    ├── windows.log        # only after a real Windows run
    ├── windows.json       # only after a real Windows run
    ├── comparison.md
    └── comparison-3platform.md
```

Temporary smoke logs can remain outside the durable documentation tree. Any
result cited in a design or regression decision must retain its raw log,
validated JSON, exact manifest, comparison output, and source provenance.
