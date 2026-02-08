# SUBAGENT 5: IDLE CALLBACKS

**Investigative Focus**: "requestIdleCallback" and "idle callback" compliance

---

## SUCCINCT SUMMARY

The `requestIdleCallback` API is **completely absent** from both `./eventloop` and `./goja-eventloop`. Neither module implements any idle callback data structures (list of idle request callbacks, list of runnable idle callbacks), algorithms (start idle period, invoke idle callbacks), or interfaces (`IdleDeadline` with `timeRemaining()`/`didTimeout`). This is a **critical compliance gap** since idle callbacks are part of the modern scheduling ecosystem and referenced by the WHATWG HTML spec's microtask queuing section.

---

## DETAILED ANALYSIS

### 1. IDLE CALLBACK ALGORITHM (W3C Spec Section 5)

The spec defines three core algorithms:

#### a) Start an Idle Period Algorithm

Invoked by event loop when idle. Required steps:
- Optionally delay if user agent determines idle period should be throttled
- Move entries from `list of idle request callbacks` → `list of runnable idle callbacks` (preserving order)
- Clear the pending list
- Queue a task to invoke idle callbacks via the "idle-task" task source
- The time between now and the deadline is the "idle period"
- Only ONE idle period can be active at a time per Window
- Idle period can end early if higher priority work becomes runnable

#### b) Invoke Idle Callbacks Algorithm

Runs individual callbacks:
- Check if user agent believes idle period should end early
- If now < deadline AND runnable callbacks list not empty:
  - Pop top callback
  - Create new IdleDeadline with getDeadline algorithm
  - Invoke callback with deadlineArg
  - If more callbacks exist, queue another task to continue

#### c) Invoke Idle Callback Timeout Algorithm

Handles timeout expiration:
- Find callback by handle in BOTH request and runnable lists
- Remove from both lists
- Create IdleDeadline with deadline = now AND timeout = true
- Invoke callback (outside idle period)

---

### 2. DEADLINE CALCULATION & IDLE PERIOD CONCEPT

Per spec Section 2:
- Idle periods are UA-defined times of quiescence
- **Two scenarios for idle period**:
  a) **Inter-frame idle period**: Between frame commit and next frame start (typically <16ms for 60Hz)
  b) **No pending updates idle period**: When no screen updates occurring (capped at **max 50ms**)
- 50ms cap derived from human perception studies (100ms response = instantaneous; 50ms leaves 50ms buffer)
- Deadline is `DOMHighResTimeStamp` (absolute time in milliseconds)
- `timeRemaining()` returns `deadline - now` (clamped to 0 if negative)
- Time estimates SHOULD be coarsened for security (prevent timing attacks)

---

### 3. TIMEOUT PARAMETER HANDLING

Per spec Section 4.1:
- `requestIdleCallback(callback, { timeout: X })`
- If timeout property present and positive:
  1. Wait for timeout milliseconds
  2. Wait until all invocations with same timeout orderingIdentifier that started before have completed
  3. Optionally wait additional UA-defined padding
  4. Queue task to invoke idle callback timeout algorithm
- Timeout and idle callbacks are **raced** - if idle callback runs first, it cancels timeout; vice versa
- Timeout callback gets `didTimeout = true` on its IdleDeadline

---

### 4. DATA STRUCTURES REQUIRED (Spec Section 4)

Per Window object:
- `list of idle request callbacks` - ordered map, initially empty, entries identified by unique number
- `list of runnable idle callbacks` - ordered map, initially empty
- `idle callback identifier` - number, initially 0
- `currently running idle callback` - tracks re-entrancy prevention

---

### 5. IDLE CALLBACK DISPATCH TIMING (Spec Section 5.2)

Critical execution ordering:
- Idle callbacks are queued as **tasks** on the "idle-task" task source
- Dispatched **after** the current task completes
- Dispatched **before** microtasks (unlike setTimeout/setInterval which run after microtasks)
- However: newly posted idle callbacks (via requestIdleCallback FROM within an idle callback) are **NOT** run in current idle period - they go to pending list for NEXT idle period (round-robin fairness)

---

### 6. IMPLEMENTATION GAP ANALYSIS

| Component | Status | Notes |
|-----------|--------|-------|
| `requestIdleCallback()` method | **MISSING** | Not in js.go or adapter.go |
| `cancelIdleCallback()` method | **MISSING** | Not in js.go or adapter.go |
| `IdleDeadline` interface | **MISSING** | No timeRemaining() or didTimeout |
| `list of idle request callbacks` | **MISSING** | No data structure |
| `list of runnable idle callbacks` | **MISSING** | No data structure |
| `idle callback identifier` counter | **MISSING** | No ID generation |
| `start idle period` algorithm | **MISSING** | No event loop integration |
| `invoke idle callbacks` algorithm | **MISSING** | No callback dispatch |
| `invoke idle callback timeout` | **MISSING** | No timeout handling |
| `idle-task` task source | **MISSING** | No separate task queue |
| Visibility-based throttling | **MISSING** | No { visibilityState: "hidden" } handling |
| Timeout/idle race mechanism | **MISSING** | No callback cancellation between paths |

---

### 7. ADVERSARIAL GAPS NOT IMMEDIATELY OBVIOUS

#### a) No idle period detection

The event loop has no hook to detect when it would be "idle" (no pending timers, I/O, microtasks). The loop's `runFastPath` and `tick()` have no idle callback integration points.

#### b) Deadline calculation missing

No mechanism to compute deadline based on vsync, next frame time, or 50ms maximum cap.

#### c) Promise rejection handling

The unhandled rejection detection in js.go uses microtask checkpoints per the spec, but idle callbacks are an alternative trigger point browsers may use - this integration is missing.

#### d) Cross-platform idle detection

The spec mentions UA-defined idle determination. On browsers this is tied to rendering pipeline; the Go implementation has no rendering context to make this determination.

#### e) Timeout granularity

Spec allows optional UA padding for power optimization (low-power mode reduced timer granularity). No equivalent in current timer implementation.

#### f) Theoretical conflict with runFastPath

The fast path mode assumes "no user I/O FDs = fast path" but idle callbacks need to run even when no FDs are registered. Integration would require modifications to fast path entry/exit logic.

#### g) No pending/runnable list distinction

Current JS adapter has no separation between "requested but not yet eligible" and "eligible to run now" states - critical for FIFO ordering and round-robin fairness.

#### h) Re-entrancy protection missing

The spec has "currently running idle callback" to prevent nested execution. Current implementation lacks this tracking.

---

### 8. COMPARISON WITH SIMILAR FEATURES

| Feature | Implemented | Notes |
|---------|-------------|-------|
| setTimeout/setInterval | ✅ Complete | Full HTML5 spec compliance including nesting clamp |
| queueMicrotask | ✅ Complete | Standard microtask queue |
| setImmediate | ✅ Complete | Non-standard but implemented |
| process.nextTick | ✅ Complete | Higher priority than microtasks |
| requestIdleCallback | ❌ **MISSING** | Core scheduling API absent |
| requestAnimationFrame | ❌ Not in scope | Rendering API |

---

### 9. VERIFICATION

```bash
# No files match "*idle*" pattern
file_search query="**/*idle*" → No matches

# No references to idle concepts
grep_search query="IdleCallback|idleCallback|requestIdleCallback" → No matches in entire codebase

# js.go confirms timer-only implementation (lines 1-2000 reviewed)
# adapter.go confirms bindings only for setTimeout/setInterval/setImmediate/queueMicrotask
```

---

### 10. THREAT MODEL

- Applications requiring background task scheduling must use `setTimeout(fn, 0)` with manual deadline tracking
- Cannot leverage UA-determined idle periods for power optimization
- Cannot use timeout parameter to guarantee execution within time bound
- No visibility-state throttling for background tabs
- Incompatible with web code expecting `requestIdleCallback` (e.g., React Scheduler, browser-based libraries)

---

## SUBAGENT 6: USER PROMPTS & LIFECYCLE

**Investigative Focus**: "user prompts" (alert, confirm, prompt, beforeunload) compliance

---

## SUCCINCT SUMMARY

The eventloop and goja-eventloop implementations have **ZERO user prompt support** (alert, confirm, prompt, beforeunload), violating HTML spec Section 8.8 compliance. No pause mechanism exists for blocking event loop during dialogs, no beforeunload event handling, no termination nesting level, no unload counter, and no prompt-allowed-in-unload checks. The implementations only cover timer/Promise APIs, not the modal dialog lifecycle requirements.

---

## DETAILED ANALYSIS

### 1. CRITICAL GAP: No User Prompt APIs Implemented

Per HTML Spec Section 8.8.1 (Simple Dialogs):
- `window.alert(message)` - NOT IMPLEMENTED
- `result = window.confirm(message)` - NOT IMPLEMENTED  
- `result = window.prompt(message, default)` - NOT IMPLEMENTED

Search across both implementations confirms: grep_search for "alert|confirm|prompt|beforeunload|dialog" returns ONLY log level references in logiface package (logging "Alert" level), NOT browser dialog APIs.

The goja-eventloop adapter.go Bind() method only binds:
- setTimeout, setInterval, clearTimeout, clearInterval
- queueMicrotask, setImmediate
- Promise constructors/combinators
- AbortController, AbortSignal
- performance, console
- EventTarget, CustomEvent
- structuredClone
- URL, Headers, FormData, Blob
- TextEncoder, TextDecoder
- localStorage, sessionStorage
- DOMException, Symbol
- atob, btoa
- delay(), crypto.randomUUID

**No alert/confirm/prompt**.

---

### 2. CRITICAL GAP: No "Pause" Algorithm (Section 8.1.7.3)

The spec requires a "pause" mechanism for user prompts:
```
To pause while waiting for the user's response:
1. Let global be the current global object
2. Let timeBeforePause be the current high resolution time
3. If necessary, update the rendering
4. Wait until the condition goal is met
5. Record pause duration
```

The eventloop loop.go has NO equivalent pause mechanism. The poll() and runFastPath() methods block on channels/polling but do NOT:
- Record pause duration
- Update rendering before blocking
- Handle the "while paused, event loop must not run further tasks" semantic
- Maintain the "currently running task" invariant during pause

---

### 3. CRITICAL GAP: No beforeunload Event Handling

Per spec Section 8.1.8 and 7.4.2.4:
- `window.onbeforeunload` handler must fire when page is unloading
- Must show confirmation dialog if handler returns non-null/non-undefined value
- Must "pause" while waiting for user confirmation
- Must respect "prompt-allowed-in-unload" checks

The abort.go file implements AbortController/AbortSignal (which is different), but there is NO:
- BeforeUnloadEvent type
- WindowEventHandlers mixin with onbeforeunload binding
- "fire beforeunload" algorithm
- Unload event dispatch
- Document's "to be unloaded" flag tracking
- Unload counter for tracking nested unloads

---

### 4. CRITICAL GAP: No Termination Nesting Level

Per HTML Spec Section 8.8.1 (Simple Dialogs) "we cannot show simple dialogs" check:
```
If window's relevant agent's event loop's termination nesting level is nonzero,
then optionally return true (block dialogs during termination)
```

The eventloop state.go implements LoopState (Awake, Running, Sleeping, Terminating, Terminated) but has NO:
- `termination nesting level` field in Loop struct
- Integration with "cannot show simple dialogs" checks
- Automatic dialog blocking during shutdown sequence

---

### 5. CRITICAL GAP: No Document Lifecycle Integration

Per spec:
- Documents have "fully active" state affecting timers
- "unload counter" tracks documents being unloaded  
- "map of active timers" per document/global
- "suspended timer handles" for bfcache

The loop.go implements timerMap and timers heap but:
- Has NO document association for timers
- Has NO "fully active" document check before firing timers
- Has NO unload counter tracking
- Has NO timer suspension/resumption for bfcache

---

### 6. CRITICAL GAP: No WebDriver BiDi User Prompt Integration

Spec Section 8.8.1 specifies WebDriver BiDi hooks:
- `WebDriver BiDi user prompt opened` - notify automation
- `WebDriver BiDi user prompt closed` - report result
- `userPromptHandler` responses: "accept", "dismiss", "none"

No implementation exists for these hooks in either eventloop or goja-eventloop.

---

### 7. Implementation Analysis: What IS Properly Implemented

#### The eventloop correctly implements:

- Timer initialization with nesting level clamping (4ms minimum after depth > 5)
- Timer ID unique internal values per spec Section 2.3.11
- Microtask queue with checkpoint semantics
- Promise/A+ with HostEnqueuePromiseJob hooks
- FastState machine for loop lifecycle
- Chunked ingress for concurrent task submission
- Platform poller abstraction (kqueue/epoll/IOCP)

#### The goja-eventloop correctly binds:

- All Promise combinators (All, Race, AllSettled, Any)
- AbortController/AbortSignal with statics (any(), timeout())
- Performance.now() with timeOrigin
- EventTarget and CustomEvent dispatch

---

### 8. Specification Compliance Matrix

| Requirement | Spec Ref | Status |
|-------------|----------|--------|
| alert/confirm/prompt APIs | 8.8.1 | NOT IMPLEMENTED |
| "pause" mechanism | 8.1.7.3 | NOT IMPLEMENTED |
| beforeunload event | 8.1.8 | NOT IMPLEMENTED |
| termination nesting level | 8.8.1 | NOT IMPLEMENTED |
| unload counter | 7.4.2.4 | NOT IMPLEMENTED |
| "to be unloaded" flag | spec | NOT IMPLEMENTED |
| prompt-allowed-in-unload | spec | NOT IMPLEMENTED |
| WebDriver BiDi prompts | 8.8.1 | NOT IMPLEMENTED |
| Timer nesting clamping | 8.6 | ✅ IMPLEMENTED |
| Unique internal values | 2.3.11 | ✅ IMPLEMENTED |
| Microtask checkpoint | 8.1.7.3 | ✅ IMPLEMENTED |
| Promise combinators | ES2021 | ✅ IMPLEMENTED |

---

### 9. Adversarial Concerns

Given the complete absence of user prompt support:

- Any JavaScript code calling `alert()`/`confirm()`/`prompt()` will throw ReferenceError
- `window.onbeforeunload` binding doesn't exist, so assigning handlers has no effect
- Page lifecycle events cannot block navigation
- Tests for prompt compliance are entirely missing
- No mechanism exists for headless browser automation (WebDriver BiDi) to intercept prompts
- The implementation cannot pass any W3C DOM conformance tests requiring prompts

---

### 10. Impact Assessment

This is a **complete feature gap** preventing browser compatibility. The eventloop is suitable for server-side JavaScript execution (Node.js compatible) but:

- Cannot implement the Window object fully per HTML spec
- Cannot support SPA frameworks that use beforeunload for confirmation dialogs
- Cannot integrate with testing tools that rely on WebDriver BiDi prompt interception
- Would fail any browser compatibility test suite (e.g., test262, web-platform-tests)

---

## CONCLUSION

The eventloop/goja-eventloop implementations are **HTML5 timer/Promise compliant but NOT HTML5 user prompt compliant**.

Full Section 8.8 (User prompts) compliance requires implementing:
- Dialog APIs (alert/confirm/prompt)
- Pause mechanism
- beforeunload event handling
- Termination nesting level
- Unload counter
- WebDriver BiDi hooks
- Document lifecycle integration for timers
