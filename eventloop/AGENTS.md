# eventloop agent rules

## first, read; do not outsource discovery to this file

Hard rule: **do not add easily discoverable implementation facts to this file.** If a capable agent can find it by opening nearby source, tests, Make targets, or module docs, it does not belong here. Pasting file inventories, function lists, queue layouts, benchmark numbers, or line-level bug snippets into `AGENTS.md` is laziness with extra steps. Put durable architecture facts in `docs/architecture.md`, public API facts in `doc.go` / `README.md`, and executable expectations in tests.

`AGENTS.md` is a behavioral contract. It is not a cheat sheet for avoiding grep.

## preserve the package boundary

Your "product direction" avoids blurring boundaries:

- `eventloop` must remain a fast, Go-native scheduling substrate that is useful without Goja.
- `goja-eventloop` owns JavaScript-visible Node-profile behavior.
- Goja integration should stay minimal: the loop schedules host work; the adapter decides JavaScript semantics.

Do not smuggle Node object models, DOM/EventTarget semantics, process exception policy, or Goja runtime assumptions into core loop APIs unless the source, docs, and tests make that boundary explicit. If a Go-facing helper is only inspired by JavaScript, describe it as Go-native unless it is proven compatible at the JavaScript boundary.

Corollary: the combination must support the exact declared Node.js v26.5.0
profile. This is not a claim to implement the entire Node.js runtime.

## verify the high-pressure invariants

When touching areas such as these, write tests that force the bad interleavings, instead of relying on calm-path examples:

- lifecycle, auto-exit, quiescence, shutdown, and terminal drain;
- accepted work versus terminal-state work;
- owner-local scheduling versus foreign-goroutine ingress;
- microtask, nextTick, checkpoint, and Goja Promise-job handoff;
- internal runtime plumbing versus user-visible callbacks and metrics;
- timer cancel/ref/unref/repeat behavior across phase boundaries;
- FD readiness contracts and platform-specific unsupported behavior;
- benchmark parsing, evidence capture, and claim wording.

Within one queue or timer-state domain, a foreign operation that returns before
a later owner-local mutation or liveness observation begins must be observed
first. Preserve the intentional priority between different event-loop phases
rather than flattening them into a global FIFO. Changes at this boundary need
forced tests in both directions, including result-bearing timer mutations, plus
allocation-sensitive owner-path measurements.

Internal runtime work must not accidentally become a JavaScript-visible callback boundary or inflate user callback metrics. Conversely, Go functions that are currently executing as JavaScript through Goja are semantic JavaScript participants; do not force them through a macrotask hop when owner-local scheduling is required.

Treat configured diagnostic backends as synchronous privileged callbacks. Contain panic and `runtime.Goexit` without discarding the caller's logical lifecycle role, force lifecycle re-entry in tests, and leave nonblocking delivery to an asynchronous backend; the loop cannot safely cancel a writer that never returns.

Invoke `logiface.Logger.Log` through the isolated boundary even when the logger
receiver is nil. Nil receivers are supported by `logiface`; do not add nil,
`Enabled`, builder, or capability preflight.

Memory retention policy applies to retired storage, never active privileged
work. Preserve unrestricted admission and exact order, clear callback-bearing
slots, trim only at an ownership-safe drain/release or registry low-water
boundary, and measure both the shrink cost and the re-warmed steady path.

Test hooks mark exact linearization points and may execute with ownership or locks held. Unless a hook explicitly documents otherwise, it must return normally and must not call `FailNow`, panic, `runtime.Goexit`, or re-enter the loop; make assertions after the hooked goroutine releases the boundary.

## claims require evidence

Do not claim Node 26 behavior from a Go unit test alone. If the claim is JavaScript-visible, prove it with `goja-eventloop` coverage or a Node oracle/differential fixture, or document it as out of scope.

Do not claim performance wins from generated summaries alone. Preserve auditable raw evidence or hash-addressed retrieval instructions, keep package identity intact, avoid lossy benchmark names or rounded-away allocation differences, and regenerate reports after the final diff.

Do not infer Windows runtime behavior from Darwin or Linux runs. Separate compile support, public FD-readiness support, private completion plumbing, and host-run verification.

## tournament corpus status

The tournament corpus is committed in this branch for future same-hardware and longitudinal comparisons. It is currently incomplete and unqualified, may omit major variants, and does not establish a correctness winner, baseline, or performance claim.

When the corpus changes, preserve existing variants and exact live-path identity, update the manifest rather than relying on directory discovery, retain raw benchmark evidence, and mark missing or non-equivalent rows explicitly. Qualification belongs to the complete compared surface; a generated summary is not evidence by itself.

## when adding tests

Prefer tests that encode invariants rather than implementation trivia. A good regression test makes the broken interleaving unavoidable and states the observable contract. If replacing old tests, map the old invariant to the new code path and the new test before deleting coverage.
