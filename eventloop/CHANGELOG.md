# Changelog

All notable changes to the `go-eventloop` package will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- `Loop.Done` exposes the stable terminal-cleanup barrier to integrations. It
  closes only after no callback accepted by the loop can still execute.

- `ScheduleControlTimer` and `ScheduleControlTimerUnrefed` give host adapters
  timer ordering, liveness, cancellation, and panic containment without
  misreporting internal wake plumbing as user callback latency or throughput.
  `RunCallback` lets that control callback synchronously observe each actual
  user callback and its complete microtask checkpoint. `YieldMicrotasks` lets
  the current logical owner defer the remainder of a handled-exception
  checkpoint until the next task or check-phase boundary, while
  `RunMicrotaskCheckpoint` lets a host drain such a boundary without inventing
  a user callback or metric. `RunCallbackDeferredCheckpoint` records one real
  user callback while allowing host task-selection bookkeeping to precede that
  explicit checkpoint. `AdvanceMicrotaskCheckpoint` consumes exactly one
  in-phase yielded checkpoint, while `ResumeMicrotaskCheckpoint`
  force-completes it after an explicit host phase publishes its task-selection
  and liveness state.

- `BindJS` gives runtime integrations one lifecycle-locked installation
  transaction while the loop remains awake, then atomically commits the Go-loop
  adapter and its quiescence callback. A concurrent `Run` or terminal transition
  therefore observes either no binding or the complete installation. Its
  integration callback composes with the public host quiescence handler, and a
  second binding is rejected for the loop's lifetime with `ErrJSBindConflict`.

- `ValidateJSOptions` checks JavaScript-adapter options without constructing or
  registering an adapter, allowing runtime integrations to validate before an
  atomic ownership or installation commit.

- **Role-preserving diagnostic integration** — `Loop.Log` lets adapters emit
  through the configured instance logger while retaining nil-receiver behavior,
  synchronous backpressure, abnormal-exit isolation, and logical lifecycle
  ownership. The outward logger accessor was removed before release.

- **Transferable nonjoining loop requests** — `Loop.Requests` exposes lifecycle
  acknowledgements and FIFO timer-mutation admission for dependency goroutines
  that must return before their parent callback or Promisify worker. Ordinary
  external Loop lifecycle callers retain their join contract, and post-Run
  timer mutations normally wait for owner application.

- **JS.Timeout(delay time.Duration)**: New convenience method that returns a promise
  rejecting with `TimeoutError` after the specified delay. Companion to `JS.Sleep()`.
  Useful with `JS.Race()` for implementing operation timeouts.

- Retained and extended the eventloop tournament research corpus with
  historical scheduler and Promise material, current product modes, Goja and
  optional libuv adapters, manifests, validators, and comparison tooling. The
  corpus remains incomplete and unqualified and may omit major variants; its
  current artifacts do not establish a correctness baseline, winner, or
  longitudinal performance claim.

### Changed

- **Bounded scheduler backing retention** — valid task, phase, timer, and JS
  handle bursts remain unrestricted, while oversized retired slices and map
  generations are released at ownership-safe geometric low-water boundaries.
  Weak JS-adapter registrations retire through GC cleanup plus an amortized
  fallback sweep. Terminal cleanup also releases dead queue-pressure and
  quiescence captures, Promise registry backing, adapter metadata, and the
  stopped fast-sleep timer. Successful JS handle cancellation releases callbacks
  that have not already won entry.

- **Truthful pre-v1 construction and read-only results** — `New` now returns
  only `*Loop`; option failures remain documented panics, while lazy poller
  failures are returned by the operation that first needs native readiness.
  `Promise` is now an opaque read-only value rather than an externally mutable
  output interface, and its invalid zero value panics on observation.

- **Loop-origin performance without loop retention** —
  `NewLoopPerformance` now returns `*Performance` directly after selecting the
  loop tick anchor when available, or its own construction time otherwise. The
  methodless `LoopPerformance` wrapper and its unused strong loop reference
  were removed.

- **Safe zero-value rejection fallback** —
  `UnhandledRejectionFallbackDisabled` is now the zero value and
  `UnhandledRejectionFallbackIsolated` is nonzero, so explicit zero-initialized
  configuration matches the documented default instead of enabling off-owner
  user callbacks. Symbolic callers are unchanged; callers persisting the raw
  integer representation must migrate from `0=Isolated, 1=Disabled` to
  `0=Disabled, 1=Isolated`.

- **Concurrent nextTick checkpoint priority** — a foreign `ScheduleNextTick`
  admission acknowledged during a nextTick callback now joins the nextTick side
  of that checkpoint before the Promise-microtask batch begins. Foreign nextTick
  admissions made during an active Promise batch still wait until that batch
  exhausts.

- **Coherent foreign/owner operation order** — a foreign-goroutine operation
  that returns before a later owner-local queue/timer mutation or liveness
  observation begins is now observed first. This preserves microtask,
  next-tick, checkpoint, `Promisify`, and timer lifecycle order without
  flattening intentional phase priority or adding allocations to uncontended
  owner scheduling paths.

- **Terminal dependency publication** — graceful Shutdown and immediate Close
  release callback dependencies on timer promises, settled Promise adoptions,
  synchronous timer-command results, and reentrant rejection diagnostics before
  waiting on callback or worker barriers. Exact cancellation results that cannot
  be published after terminal commitment return `ErrLoopTerminated`; graceful
  drain still applies their admitted command in FIFO order. Result-bearing
  ordinary ref/unref commands released before owner application return nil and
  are discarded as terminal no-ops so they cannot override a later owner-local
  mutation; admission-only `LoopRequests` remain queued.

- **Complete diagnostic abnormal-exit isolation** — Loop diagnostics call the
  configured `logiface.Logger.Log` directly, including nil logger receivers, and
  contain panic or `runtime.Goexit` across factory, modifier, Event methods,
  writer, and releaser without losing the caller's lifecycle role.

- **Coherent metric snapshots**: `Loop` now owns one private queue, duration,
  and TPS sampler. A serialized metric epoch prevents `Metrics()` from mixing
  fields across an in-progress update, and the returned `Metrics`,
  `LatencyMetrics`, and `QueueMetrics` values are detached snapshot-only types.
  `LatencyMetrics.Count` reports scheduled callbacks admitted to execution,
  including each microtask, while its percentiles, maximum, and arithmetic mean
  cover callback execution-path duration rather than queue residence. Within
  the five-observation exact window, even-sample medians use the midpoint of
  the two middle observations. TPS counts callbacks that returned successfully.
  Queue depths include materialized immediate and close work and are sampled
  consistently on startup, task-only fast-path, and polling turns.

- **Single scheduler ingress topology** — external goroutines now publish only
  typed commands, which the loop owner transfers into unsynchronized local
  phase queues. The unreachable locked chunk queues, separate fast-path
  auxiliary queue, their implementation-only tests and benchmarks, and the
  no-op `WithIngressChunkSize` option were removed. Task-only and physical-poll
  execution now consume the same accepted-work topology.

- **Callback abnormal-exit containment** — normal tasks, timers, next ticks,
  microtasks, phase callbacks, quiescence handlers, liveness predicates, and
  queue-pressure callbacks now execute serially through a logical callback owner while
  the physical loop owner waits. Panics, including `panic(nil)`, are recovered
  and logged. `runtime.Goexit` is logged distinctly, retires only that logical
  owner, and cannot strand `Run`, timer retirement, or later callbacks.

- **Explicit callback and promise error contracts** — exported scheduling,
  timer, immediate, microtask, next-tick, `Promisify`, and `Try` entry points now
  panic synchronously when their required callback is nil. Promise callback
  panics reject with `PanicError`, and `runtime.Goexit` rejects with
  `ErrGoexit`, including reactions and `Try`; `Finally` preserves its original
  settlement after either abnormal exit. Promise self-resolution and typed-nil
  promise adoption reject with the stable `ErrPromiseSelfResolution` and
  `ErrPromiseNilAdoption` identities. `AggregateError` preserves arbitrary
  rejection reasons in order while unwrapping only genuine non-nil errors, and
  retained error types exclude typed-nil causes from their unwrap chains.
  Concurrent context cancellation joins the loop's stored terminal failure,
  including poll and descriptor-close failures.

- **Static `NewJS` construction contract** — `NewJS` now returns only
  `*JS`: nil or non-constructor Loop dependencies, nil or typed-nil options,
  nil rejection handlers, and invalid option values panic synchronously instead
  of being ignored or returned as configuration errors. Post-termination
  unhandled-rejection callbacks are disabled by default; callers must explicitly
  select the isolated off-owner fallback when it is safe.

- **Terminal rejection diagnostic ownership** — already-determined diagnostics
  in a graceful terminal drain retain its logical callback owner and complete
  before terminal publication. Only genuinely ownerless post-terminal fallback
  uses the explicitly selected isolated goroutine path.

- **Graceful terminal backlog order** — the final drain uses a regular tick's
  non-timer phase order for its queued check, close, internal, and external
  callbacks, with microtask checkpoints after every callback and phase. Timers
  remain outside the terminal drain and are discarded during cleanup.

- **Node-style repeating-timer anchoring** — native repeating timers now anchor
  the next deadline immediately before callback entry, matching Node.js v26.5.0
  without accumulating callback-duration drift while retaining the later-turn
  guard for overdue and zero-delay intervals.

- **Stable startup and rollover phase order** — timer callbacks in the startup
  timer pass now defer timers they schedule until a later iteration without
  delaying timers admitted by startup microtasks. Check and close phase merges
  preserve admission order across the internal `uint64` sequence rollover.

- **Reused interval deadline lists** — the loop retains one cleared,
  owner-confined deadline-list object, eliminating the per-repeat allocation
  for steady native intervals without retaining callback or timer references.

- **JS handle publication before callback entry** — `SetTimeout`, `SetInterval`,
  and `SetImmediate` now hold their native wrappers behind a once-only
  publication gate until the adapter handle and native timer identity are fully
  visible. A terminal transition that wins final publication invalidates the
  wrapper and returns `ErrLoopTerminated`; successful scheduling cannot expose a
  callback that races an unpublished or unusable handle.

- **Observable public wake failure** — `Loop.Wake` now returns an exact physical
  wake submission failure instead of silently reporting success. The failed
  pending claim is reopened for retry; already accepted ingress retains its
  bounded native-poll recovery contract.

- **Go-native cancellation and event dispatch** — `AbortSignal` now preserves
  non-nil error reasons by identity, wraps other reasons (including typed-nil
  errors) once, releases handlers before
  callback delivery, and continues cleanup and FIFO delivery when a handler
  panics before re-raising the first panic. A later `runtime.Goexit` ends the
  remaining delivery without suppressing an earlier captured panic. `AbortAny`
  deduplicates sources and detaches its internal propagation links after either
  settlement or runtime cleanup of an unreachable composite. Nil panics are
  canonicalized to `runtime.PanicNilError`
  even under legacy `GODEBUG=panicnil=1`. `AbortTimeout` uses `TimeoutError`, validates nil,
  negative, and overflowing inputs, and cancels its referenced timer after a
  manual winning abort. The timer callback or one manual call atomically claims
  settlement; every losing manual call joins stable reason publication without
  waiting for cleanup or handler delivery. Terminal timer discard also releases
  its Loop reference.
  `AbortSignal.OnAbort` is the sole Go callback-registration API; the
  compatibility-shaped `AddEventListener` alias was removed.
  Timeout-triggered handlers run through a delegated-owner boundary so
  `runtime.Goexit` cannot abandon the loop or current timer; callback-local Loop
  lifecycle and scheduling calls retain owner behavior.
  `EventTarget` now observes listener removal until the
  callback-start claim, atomically claims once listeners before invocation, and
  releases removed callback storage. Reused events clear completed dispatch
  state. Active dispatch identity is stored outside the mutable event, so
  whole-value overwrite cannot bypass same-pointer recursion rejection and a
  copied event establishes an independent address identity. Dispatch-owned
  target, cancellation, and propagation outcome also survive whole-value
  overwrite until return. Internal dispatch bookkeeping retains an active event
  only for that dispatch and does not retain it afterward; ordinary copied
  fields retain their referenced values normally. EventTarget contains
  synchronization state and must not be copied;
  callers use a distinct zero value or constructor result for an independent
  target. Listener IDs are unique among live registrations and may reuse
  released values after a full counter wrap, with documented stale-ID ABA
  implications. Documentation labels these APIs as Go-native rather than
  claiming DOM conformance.

- **Readiness poller lifecycle and platform expansion** — Linux/Android epoll,
  Apple/BSD kqueue, and AIX/Solaris/illumos poll registrations now carry
  monotonic identity through native results, preventing a stale result from
  inheriting a callback after numeric descriptor reuse.
  Descriptor zero is closed correctly, poller close invalidates all local
  ownership, and unregister after user close deletes and releases the poller's
  still-live owned duplicate. Kqueue combines simultaneous read/write filters
  into at most one eligible user callback per committed registration and poll
  batch, and rolls partial filter modifications back to the prior mask when
  possible. Pointer-based kqueue event tags use stable non-Go storage until
  native event resolution joins close, failed unmaps remain owned for a later
  cleanup retry, and NetBSD uses integer tags. Linux verifies remaining
  registrations during epoll-set rebuild so a retired watch cannot attach an
  old callback to a reused numeric descriptor. Poll uses generation-tagged
  snapshots and a private mutation-control pipe; a failed ModifyFD signal
  preserves the previously published interest mask. Provisional registrations
  cannot dispatch callbacks until lifecycle and fast-path ownership commits.
  Retired kqueue tag slots are reused only after in-flight native result
  conversion finishes, bounding mapped storage by peak concurrent registrations.
  Unix registrations use poller-owned descriptor duplicates for native control,
  so caller-side close/reuse cannot redirect modify, rollback, unregister,
  snapshot construction, or epoll rebuild onto a replacement. Registration
  now rejects nil callbacks and masks without a read or write interest;
  `ModifyFD(fd, 0)` remains the supported way to disable interests without
  unregistering, and readiness converted before that modification is filtered
  again at callback-start claim. Descriptor cleanup failures are no longer
  discarded: `FDUnregisterError` identifies whether ownership was released.
  Winning Close calls and external winning Shutdown callers that await terminal
  completion report descriptor failures; loop-callback and Promisify-worker
  Shutdown winners retain their earlier nil request-acknowledgement contract.

- **Physical wake and native-poll recovery** — physical wake submission and
  teardown now share resource ownership, Unix wake epochs distinguish idle,
  submitting, and descriptor-pending states from fast-channel signals, and all
  native waits longer than one second are capped so one failed wake cannot
  strand or excessively delay ingress or terminal cleanup behind a far-future
  timer. The cap costs at most one empty native poll turn per idle Unix I/O loop
  per second; ordinary task-only and non-FD platforms remain on the indefinite
  fast channel and do not post to inactive pollers, while explicit
  `FastPathDisabled` loops on readiness targets receive physical ingress wakes.
  Terminal winners are rechecked at
  the final poll boundary, closed wake descriptors are invalidated before
  release, and drain handling distinguishes interruption, normal would-block
  exhaustion, EOF, zero-byte reads, and unexpected failures.

- **Phase liveness visibility** — `Loop.Alive` and
  `Loop.HasMacrotaskWork` now retain detached check and close phase
  batches until every accepted callback finishes or immediate Close discards
  the remainder. Dynamic immediate liveness predicates remain owner-evaluated;
  external observers are conservative, and a final false owner evaluation is
  the auto-exit linearization point unless loop-visible work invalidates it.

- **Nonblocking timer lifecycle** — `JS.ClearTimeout`, `JS.ClearInterval`,
  `JS.RefTimeout`, `JS.UnrefTimeout`, `JS.RefInterval`, and `JS.UnrefInterval`
  no longer wait for the loop owner after accepting their relevant handles. A
  successful one-shot timeout clear suppresses callback entry across the
  detached-timer dispatch window, while core cancel/ref/unref mutations retain
  FIFO ordering. Interval ref changes target one stable native repeating timer
  across every tick.

- **Saturating timer identity and exact lifecycle boundaries** — core
  `TimerID` allocation now spans the full `uint64` namespace, while shared JS
  timeout/interval handles and separate immediate handles stop at JavaScript's
  maximum safe integer. All three allocators saturate without wrapping. Native
  timer callbacks wait for successful-call ID publication; timers scheduled
  during an active turn remain deferred across tick-counter wrap; referenced-timer
  accounting is 64-bit; and terminal cleanup scrubs deadline-list and heap backing
  references before pooling. Pre-Run cancellation
  now reports the same exact sequential success/not-found results as active-loop
  cancellation, while pre-Run ref changes queue without a speculative timeout.
  `ClearImmediate` now reports the distinct `ErrImmediateNotFound`, and
  non-negative Go `JS` timer delays that overflow `time.Duration` panic before
  consuming an ID or publishing a handle.

- **Terminal timer-promise settlement** — accepted `JS.Sleep` promises now
  resolve with nil and accepted `JS.Timeout` promises reject with their normal
  `TimeoutError` when graceful or immediate terminal cleanup discards the native
  timer. Calls whose native timer admission fails instead reject with the exact
  admission error. Terminal settlement is registered atomically with lifecycle
  cleanup and occurs exactly once outside the lifecycle lock.

- **Promisify worker shutdown ownership** — a `Promisify` worker that wins
  `Loop.Shutdown` now returns nil after an independent finisher owns the
  graceful request instead of waiting on terminal completion that includes its
  own worker. External winning callers still wait for completion or their
  context. The request acknowledgement is distinct from completed cleanup.

- **Immediate Close worker lifetime** — a winning `Loop.Close` now closes normal callback
  admission, rejects registered pending promises before waiting for the loop
  owner, lets only an owner callback whose execution was already claimed finish,
  and returns after resources are cleaned instead of waiting indefinitely for
  in-flight `Promisify` user functions. Their promises receive
  `ErrLoopTerminated`; the functions retain their
  caller-provided contexts and may continue, but terminal admission rejects new
  loop work. A committed worker that has not claimed user-function entry when
  Close wins skips the function; one that already claimed entry may continue and
  may execute its first user-function instruction even after Close has returned.
  JS timeout, interval, and immediate handle registries are invalidated during
  terminal cleanup, and publication that loses the lifecycle race returns
  `ErrLoopTerminated` rather than exposing a stale handle.
  External lifecycle callers that overlap an open terminal transition join its
  complete-cleanup barrier and receive one aggregate terminal result; Shutdown
  retains each caller's context bound. Promisify workers use exact worker
  identity to avoid joining a barrier that can depend on themselves. Matching
  graceful Shutdown and immediate Close requests return nil acknowledgements;
  conflicting terminal modes return `ErrLoopTerminated`. Exact finisher
  identity applies the same matrix to synchronous terminal-cleanup callbacks,
  preventing a logger writer from joining the completion it interrupted. A
  logger running synchronously on the logical Run owner also keeps the
  non-immediate Shutdown acknowledgement after the active drain has ended. Close
  acquires lifecycle ownership before closing callback admission, avoiding a cycle with FD
  unregistration's pending-dispatch start guarantee; terminal rejection fallback
  handoff cannot be lost or run user code on the exiting loop owner.

- **Descriptor readiness contract completed** — epoll, kqueue, and poll
  registrations use bounded-growth dense storage with sparse fallback,
  saturating non-reused
  registration identities, return-boundary callback publication, typed-nil-safe
  immutable cleanup errors, and Linux `EPOLLRDHUP` peer-half-close reporting.
  Static misuse and uninitialized Loops panic consistently before platform
  checks. Windows, Plan 9, js/wasm, and wasip1/wasm share one task-only
  implementation and return `ErrReadinessUnsupported`; speculative Windows
  IOCP plumbing was removed.

- **Scheduler owner topology** — logical-owner microtask, nextTick, checkpoint,
  internal, external, check, and close work now uses owner-local queues, while
  external goroutines enter through typed command ingress. Fast path and normal
  tick phase ordering now share the same Node-shaped priority model.

- **Timer scheduler rewrite** — timers now use monotonic millisecond deadline
  buckets with exact-deadline ordering inside buckets. Intervals are native
  repeating timer nodes with stable `TimerID` values and preserved ref/unref
  liveness.

- **Poller and FD storage rewrite** — epoll, kqueue, and poll resources are lazy
  and FD registrations use hybrid dense/sparse storage. Ready events are dispatched by
  the loop after generation revalidation, panic recovery, and per-callback
  microtask checkpoints.

- **Metrics hot path** — loop-owned runtime samplers avoid public metric locks
  on scheduled callbacks while preserving coherent `Metrics()` snapshots.

- **BREAKING: ChainedPromise compact layout** — On 64-bit targets the struct
  shrank from 120B to 64B; its 32-bit layout is 40B. `toChannels`,
  `creationStack`, and `id` moved to side tables on `JS`, keyed by
  `weak.Pointer[ChainedPromise]` for GC-safe automatic cleanup.

- **API cleanup: unexported internal-only types** — Tightened the public API surface by
	unexporting symbols that were only used internally: `FastState` → `fastState`,
	`FastPoller` → `fastPoller`,
	`IOCallback` → `ioCallback`, `TPSCounter` → `tpsCounter`, and others. The
	test-only `MaxFDLimit` and `ErrorWrapper` surfaces were removed entirely. The
	old main-loop microtask ring implementation has been removed in favor of
	owner-local queues.

### Removed

- **Hidden HTML timer policy and compatibility entry points**: Core
  `Loop.ScheduleTimer` now preserves the supplied Go duration and repeating
  timers preserve their configured interval. Loop-wide nesting state,
  `ScheduleTimerUnclamped`, `JS.SetTimeoutUnclamped`, and
  `JS.SetIntervalUnclamped` were removed; JavaScript hosts own their delay
  normalization policy.

- **Compatibility-only aliases and rejection state**: Removed the
  `PromiseState.Resolved` alias, `AbortSignal.AddEventListener`, and the
  strongly retaining JS-level handled-rejection side map. Callers use
  `Fulfilled`, `AbortSignal.OnAbort`, and promise-local handled state directly.

- **Metric mutation API**: Removed `LatencyMetrics.Record`,
  `LatencyMetrics.Sample`, the `QueueMetrics.Update*` methods, and
  `LatencyMetrics.Sum`. These fields and methods mutated detached public values
  rather than the loop's runtime sampler. `LatencyMetrics.Count` and the
  all-observation `Mean` now expose the corresponding snapshot state directly.

### Fixed

- **Bounded rejection-check ownership** — a `JS` adapter that schedules an
  unhandled-rejection checkpoint no longer starts a goroutine retained until
  the shared Loop terminates. The Loop now holds the adapter strongly only
  while a check, reschedule, or terminal fallback handoff is outstanding, then
  releases both the adapter and empty high-water storage. If a reporting
  callback exits via `runtime.Goexit`, residual rejection records are
  rescheduled before that ownership barrier can close.

- **Terminal Promise combinators** — `All`, `Race`, `AllSettled`, and `Any`
  reject a still-unclaimed aggregate with `ErrLoopTerminated` when a required
  input reaction can no longer execute, including an accepted reaction denied
  by immediate-Close callback admission, instead of leaving the aggregate
  pending and rejecting an inaccessible internal child. Normal and terminal
  outcomes share the aggregate's atomic settlement claim, so the failure never
  replaces an earlier claim. The internal observer path preserves the
  compact 64-byte Promise layout. The shared reaction path likewise rejects a
  normal non-nil-handler child when immediate Close discards its accepted
  microtask; nil-handler pass-through retains the parent settlement. A
  common-case inline owner plus bounded reusable overflow caps retained map
  high-water after small bursts and releases it after larger drains. Large
  microtask buffers are likewise released after drain, and the existing command
  envelope size is preserved for unrelated ingress.

- **Terminal cleanup callback ownership** — settled Promise adoptions collected
  during JS cleanup now propagate only after lifecycle ownership is released, so
  a synchronous diagnostic logger can re-enter public Loop APIs and receive
  their terminal result instead of blocking on the cleanup lock. Owner callback
  workers created by final JS or descriptor-cleanup diagnostics are retired
  after those diagnostics return, including when the configured logger is nil.

- **Auto-exit cancellation and admission linearization** — cancellation visible
  at the final auto-exit terminal-admission boundary now wins over clean exit,
  including cancellation from a quiescence callback. Every aborted auto-exit
  commit lowers its provisional quiescing gate before normal loop admission
  resumes, so a later quiescence callback cannot spuriously reject valid work.

- **Physical wake resource races** — Unix eventfd and self-pipe submission now
  joins terminal resource release, preventing a late write from targeting a
  reused descriptor. Interrupted Unix wake I/O is retried and
  a full nonblocking wake descriptor is treated as an already-pending wake.
  Native I/O polling periodically rechecks loop state as a safety net after an
  unexpected wake failure; fast-channel waits remain indefinite.

- **Run() returns ErrLoopTerminated for StateTerminating**: Fixed a race condition where
  `Run()` would return `ErrLoopAlreadyRunning` instead of `ErrLoopTerminated` when
  `Shutdown()` won the scheduling race on Windows.

- **`debugStacks` now uses `weak.Pointer` for GC-safe cleanup** — Creation stack traces
  (debug mode only) stored in a side table with automatic cleanup when promises are
  garbage collected.

- **Promise-local rejection handling** — Handler registration publishes handled
  state on the promise itself. Unhandled-rejection tracking no longer maintains
  a compatibility side map that could strongly retain pending promises.

- **Terminal rejection diagnostics** — A post-termination rejection with
  callback fallback disabled emits exactly one instance-scoped error event with
  the original reason. The expected handoff to terminal fallback no longer also
  reports an internal microtask-scheduling failure.

- **Timer delay semantics clarified across profiles**: Core `eventloop.JS`
  `SetTimeout` / `SetInterval` normalize negative millisecond delays to `0`
  before scheduling, while low-level `Loop.ScheduleTimer(time.Duration)` remains
  a Go primitive that schedules against the supplied duration. The separate
  `goja-eventloop` Node profile coerces invalid, too-small, and overflowing
  delays to `1ms` and emits Node-style timer warnings.

- **ChainedPromise rejection propagation**: `Then(onFulfilled, nil)` no longer
  marks the source rejection as handled; the propagated child is the reportable
  unhandled rejection unless a downstream rejection handler observes it.

- **I/O and lifecycle races**: stale FD ready events are skipped after
  unregister/re-register, `UnregisterFD` waits for claimed dispatch starts,
  `ModifyFD` rolls back tracked event masks on syscall failure, fast-path context
  cancellation is honored during continuous ready work, and auto-exit no longer
  races accepted lifecycle/terminal-drain work.

---

## [Previous] - 2026-02-07

### Added

- Windows platform support (IOCP poller, event handle wakeup)
- `crypto.getRandomValues()` (Web Crypto API subset)
- Go-native AbortController/AbortSignal APIs with browser-inspired names
- Performance API (`performance.now()`, marks, measures)
- `Promise.withResolvers()` (ES2024)
- Console timer API (`console.time`, `console.timeEnd`, `console.timeLog`)
- O(1) streaming percentile estimation (P-Square algorithm)

### Changed

- Unexported internal-only types (`FastState` → `fastState`, etc.)

### Fixed

- Race conditions in timer callbacks, leak tests, workload tests, fuzz tests
- Restructured goja-eventloop tests to prevent concurrent `goja.Runtime` access
