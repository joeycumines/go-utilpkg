# Review Group C: Goja Integration & Combinators

**Status:** ❌ CRITICAL ISSUES FOUND - NOT PRODUCTION READY
**Reviewer:** Automated Deep Analysis
**Date:** 2026-01-21
**Analysis Depth:** Maximum (Line-by-line verification, all edge cases)

---

## SUCCINCT SUMMARY

The Goja adapter implementation contains **FIVE CRITICAL SAFETY VIOLATIONS** that MUST be fixed:

1. **DEADLOCK RISK** - `ensureLoopThread()` holds mutex across Goja calls; Goja is not thread-safe and mutex does NOT ensure thread safety. Actual thread safety via `loop.Run()` is missing.

2. **PANIC INFRASTRUCTURE** - `panics` propagate through Goja runtime without recovery, potentially crashing the entire event loop and corrupting state.

3. **UNSAFE TYPE ASSERTIONS** - Multiple unchecked type assertions (`internal.Export().(*goeventloop.ChainedPromise)`) can panic if `_internalPromise` is corrupted or wrong type.

4. **TEST COVERAGE GAPS** - Promise chaining (`TestPromiseChain`), combinators (`All`, `Race`, etc.), and executor error handling are **COMPLETELY SKIPPED** with `t.Skip()`.

5. **CROSSTHREAD EXECUTION** - Timer/interval callbacks execute Goja functions from event loop thread, but there's no verification this is safe under all race conditions.

**Verdict:** This code is **NOT safe for production use**.

---

## DETAILED ANALYSIS

---

### 1. THREAD SAFETY CRITICAL FAILURES

#### 1.1 FAKE "Safety" in `ensureLoopThread()`

**Location:** `adapter.go:75-79`

```go
func (a *Adapter) ensureLoopThread() {
    a.mu.Lock()
    defer a.mu.Unlock()
}
```

**CRITICAL ISSUE:** This function provides **zero thread safety**. The mutex does NOT ensure we're on the event loop thread - it just provides a memory barrier.

**The Problem:**
- Goja runtime is **not thread-safe** and must be accessed from a single thread
- `ensureLoopThread()` uses a mutex, which creates a synchronization point
- This does NOTHING to prevent concurrent access from different goroutines
- The actual safety comes from `loop.Run()` executing all callbacks on one thread
- The mutex is at **best useless** and at **worst harmful** (confusion, subtle race conditions)

**Expected Implementation:**
```go
func (a *Adapter) ensureLoopThread() {
    // This should either:
    // 1. Panic if not on the loop thread (runtime/trace assertions)
    // 2. Be removed entirely and rely on loop.Run() guarantee
    // 3. Schedule work to loop thread if called from wrong thread

    // Current implementation is MISLEADING and DANGEROUS
}
```

**Recommendation:** Remove `ensureLoopThread()` entirely from `setTimeout`, `clearTimeout`, `setInterval`, `clearInterval`, `queueMicrotask`. The actual thread safety is guaranteed by the fact that these functions are called from JavaScript running in the Goja runtime, which should be on the event loop thread.

**VERIFICATION NEEDED:** Add a test that attempts concurrent Goja access to verify panic behavior.

---

#### 1.2 Promise Constructor Thread Safety

**Location:** `adapter.go:156-247` (`promiseConstructorWrapper`)

**CRITICAL ISSUE:** The Promise constructor assumes it's called on the event loop thread, but this is never verified.

```go
func (a *Adapter) promiseConstructorWrapper(call goja.ConstructorCall) *goja.Object {
    a.ensureLoopThread()  // <-- FAKE safety!
    // ... creates internal promise
    // ... defines methods
    // ... calls executor
}
```

**The Problem:**
1. `new Promise()` is called from JavaScript running in Goja
2. We assume Goja is being called from the event loop thread
3. If JavaScript code is called from a wrong thread, we corrupt internal state
4. `_internalPromise` field is set on `call.This`, creating a reference
5. Goja objects are accessed from `then()`, `catch()`, `finally()` methods
6. These methods all call `ensureLoopThread()` - which is **useless**

**Expected Race Scenario:**
```
Thread A (event loop):  new Promise((resolve) => resolve(42))
Thread B (wrong thread): promise.then(x => x + 1)
```

This can happen if:
- JavaScript code spawns goroutines
- Goja runtime is called from multiple threads
- User incorrectly uses the adapter

**Recommendation:**
```go
func (a *Adapter) promiseConstructorWrapper(call goja.ConstructorCall) *goja.Object {
    // Option 1: Document that all Goja calls must be from loop thread
    // Option 2: Use a channel to queue work to loop thread
    // Option 3: Panic with stack trace if called from wrong thread
    // Option 4: Remove ensureLoopThread() entirely and rely on user discipline

    // Current: MISLEADING fake safety
}
```

---

### 2. PANIC SAFETY INFRASTRUCTURE

#### 2.1 Unprotected Goja Callback Execution

**Location:** `adapter.go:91-127` (setTimeout/setInterval/queueMicrotask)

**CRITICAL ISSUE:** All Goja function calls are wrapped in panic-susceptible code.

```go
id, err := a.js.SetTimeout(func() {
    _, _ = fnCallable(goja.Undefined())  // <-- CAN PANIC
}, delayMs)
```

**The Problem:**
1. `fnCallable(goja.Undefined())` can panic for multiple reasons:
   - JavaScript `throw` statement
   - Invalid `this` binding
   - Type coercion errors
   - Goja internal errors
2. The panic propagates through the event loop's callback wrapper
3. This crashes the event loop thread
4. **All pending work is lost**
5. System state is **corrupted and inconsistent**

**Expected Behavior:**
Per JavaScript spec, errors thrown in callbacks should:
- Be caught by the Promise rejection mechanism (for microtasks)
- Be reported to the global error handler (for timers)
- NOT crash the entire event loop

**Recommendation:**
```go
id, err := a.js.SetTimeout(func() {
    defer func() {
        if r := recover(); r != nil {
            // Report to unhandled rejection callback or error handler
            if a.js.unhandledCallback != nil {
                a.js.unhandledCallback(r)
            } else {
                // Log the error but don't crash
                log.Printf("Uncaught exception in setTimeout callback: %v", r)
            }
        }
    }()
    _, err := fnCallable(goja.Undefined())
    if err != nil {
        panic(err)  // This will be caught by defer above
    }
}, delayMs)
```

**VERIFICATION NEEDED:** Create a test that throws an error in a setTimeout callback and verify:
1. Loop continues running
2. Other timers still fire
3. Error is reported (not silently ignored)

---

#### 2.2 Promise Handler Panics

**Location:** `adapter.go:262-306` (gojaFuncToHandler)

```go
return func(result goeventloop.Result) goeventloop.Result {
    var arg goja.Value
    if result != nil {
        arg = a.runtime.ToValue(result)
    } else {
        arg = goja.Undefined()
    }
    ret, err := fnCallable(goja.Undefined(), arg)
    if err != nil {
        panic(err)  // <-- UNSAFE: propagates to event loop
    }
    // ... return value
}
```

**CRITICAL ISSUE:** Panics in `then()`/`catch()`/`finally()` handlers crash the event loop.

**The Problem:**
- Per Promise/A+ spec, panics in handlers should **reject the returned promise**
- Current code panics instead, crashing the event loop
- This breaks the **entire promise chain ecosystem**

**Recommendation:**
```go
return func(result goeventloop.Result) goeventloop.Result {
    defer func() {
        if r := recover(); r != nil {
            // Convert panic to rejection
            // This is handled by the outer ChainedPromise implementation
            // But we need to ensure this doesn't crash the event loop
        }
    }()
    var arg goja.Value
    if result != nil {
        arg = a.runtime.ToValue(result)
    } else {
        arg = goja.Undefined()
    }
    ret, err := fnCallable(goja.Undefined(), arg)
    if err != nil {
        return err  // Return error as rejection reason
    }
    if ret == goja.Undefined() || ret.Export() == nil {
        return nil
    }
    return ret.Export()
}
```

**VERIFICATION NEEDED:**
- Test: `new Promise((resolve) => resolve(1)).then(() => { throw new Error('test'); })`
- Verify returned promise is rejected (not event loop crash)

---

### 3. TYPE SAFETY VIOLATIONS

#### 3.1 Unchecked Type Assertions in Promise Methods

**Location:** Multiple locations across `adapter.go`

**PATTERN:**
```go
internal := thisObj.Get("_internalPromise")
if internal.Export() == nil || internal == goja.Undefined() {
    panic(a.runtime.NewTypeError("Promise internal state lost"))
}

internalPromise := internal.Export().(*goeventloop.ChainedPromise)  // <-- CAN PANIC
```

**CRITICAL ISSUE:** Type assertion to `*goeventloop.ChainedPromise` can panic even after the nil check.

**The Problem:**
1. `_internalPromise` can be set to a wrong type through:
   - Manual property modification: `promise._internalPromise = "wrong"`
   - Prototype chain pollution
   - Memory corruption
   - Goja object graph manipulation
2. The nil check passes for non-nil wrong types
3. Type assertion `(*goeventloop.ChainedPromise)` panics
4. Panic crashes event loop

**Recommendation:**
```go
internal := thisObj.Get("_internalPromise")
if internal.Export() == nil || internal == goja.Undefined() {
    panic(a.runtime.NewTypeError("Promise internal state lost"))
}

internalPromise, ok := internal.Export().(*goeventloop.ChainedPromise)
if !ok || internalPromise == nil {
    panic(a.runtime.NewTypeError("Promise internal state corrupted"))
}
```

**AFFECTED LOCATIONS:**
1. `promiseConstructorWrapper` method definitions (lines ~218, 233, 253)
2. `gojaWrapPromise` method implementations (lines ~287, 301, 317)

---

#### 3.2 Export Conversion Edge Cases

**Location:** `adapter.go:347-364` (gojaFuncToHandler)

```go
var arg goja.Value
if result != nil {
    arg = a.runtime.ToValue(result)
} else {
    arg = goja.Undefined()
}
ret, err := fnCallable(goja.Undefined(), arg)
if err != nil {
    panic(err)
}
// Safe export that handles nil
if ret == goja.Undefined() || ret.Export() == nil {
    return nil
}
return ret.Export()
```

**ISSUE:** The condition `ret.Export() == nil` can be true for valid values that legitimately export to `nil` in Go.

**The Problem:**
- Some JavaScript values export to `nil` in Go:
  - `null`
  - ` undefined`
  - Functions (depending on export)
- These distinctions are **important for promise values**
- We lose information by treating all nil exports as `nil`

**Example Scenario:**
```javascript
Promise.resolve(null).then(x => {
    console.log(x);  // Should log "null", not undefined
});
```

**Current Behavior:** `x` becomes `nil` (correct)
**Questionable:** What about `undefined` vs `null` distinction?

**Recommendation:** Preserve the distinction if needed for spec compliance. For now, this is likely acceptable but should be documented.

---

### 4. MEMORY SAFETY AND REFS

#### 4.1 Circular References in Promise Wrappers

**Location:** `adapter.go:314-334` (gojaWrapPromise)

```go
func (a *Adapter) gojaWrapPromise(promise *goeventloop.ChainedPromise) goja.Value {
    wrapped := a.runtime.ToValue(promise)
    wrappedObj := wrapped.ToObject(a.runtime)

    // Store internal promise reference for method access
    _ = wrappedObj.Set("_internalPromise", promise)

    thenFn := a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
        // ... accesses _internalPromise
        internal := internalVal.Export().(*goeventloop.ChainedPromise)
        // ... creates new promise via gojaWrapPromise (recursive)
        chained := internal.Then(onFulfilled, onRejected)
        return a.gojaWrapPromise(chained)  // <-- RECURSIVE CREATION
    })
    // ...
}
```

**POTENTIAL ISSUE:** Infinite object graph growth through promise chaining.

**The Problem:**
1. Each promise chain call creates a new Goja object
2. Each Goja object stores a reference to a `ChainedPromise` via `_internalPromise`
3. The `ChainedPromise` already has internal state
4. What happens with infinite chains like: `p.then(x => x).then(x => x).then(x => x)...`
5. Are old objects garbage collected properly?

**VERIFICATION NEEDED:**
```go
func TestPromiseMemoryLeak(t *testing.T) {
    // Create long chains
    var p goja.Value
    for i := 0; i < 10000; i++ {
        p, _ = runtime.RunString(`Promise.resolve(1).then(x => x)`)
        // Drop old references
    }
    // Force GC and verify minimal memory increase
    runtime.GC()
    // Check memory usage
}
```

---

#### 4.2 Timer Callback Closure Garbage Collection

**Location:** `adapter.go:91-127`

```go
id, err := a.js.SetTimeout(func() {
    _, _ = fnCallable(goja.Undefined())
}, delayMs)
```

**ISSUE:** Closures capture `fnCallable` which is a Goja value.

**The Problem:**
1. `fnCallable` is a Goja function object
2. It's referenced by the Go callback closure
3. This closure is stored in the event loop timer
4. Even after timer fires, the closure might be kept alive by Goja's GC
5. This is likely correct (Goja manages its own object lifetimes)
6. **BUT:** What about timer cleanup before firing (clearTimeout)?

**Verification:** The timer wrapper in `js.go` properly deletes the timer:
```go
wrappedFn := func() {
    defer js.timers.Delete(id)  // <-- Properly cleaned up
    fn()
}
```

**Conclusion:** This is likely correct, but should be documented.

---

### 5. PROMISE CONSTRUCTOR CORRECTNESS

#### 5.1 Executor Error Handling

**Location:** `adapter.go:238-245`

```go
_, err := executorCallable(goja.Undefined(),
    a.runtime.ToValue(func(result goja.Value) {
        var val any
        if result != goja.Undefined() && result.Export() != nil {
            val = result.Export()
        }
        resolve(val)
    }),
    a.runtime.ToValue(func(reason goja.Value) {
        var val any
        if reason != goja.Undefined() && reason.Export() != nil {
            val = reason.Export()
        }
        reject(val)
    }),
)
if err != nil {
    // If executor throws, reject the promise
    reject(err)
}
```

**CRITICAL ISSUE:** The executor error handling is **correct but INCOMPLETE**.

**The Problem:**
1. If executor throws, we call `reject(err)`
2. This is correct per Promise spec
3. **BUT:** What if executor throws after calling resolve/reject?
4. What if executor calls resolve then throws?
5. Current behavior: **resolve wins, error is ignored**
6. Expected per spec: **once resolved/rejected, subsequent transitions are ignored**

**Example Scenario:**
```javascript
new Promise((resolve) => {
    resolve(1);
    throw new Error('ignored');  // Per spec, this error should not reject
})
```

**Current Code:** The spec-compliant behavior is in `ChainedPromise.resolve()`:
```go
func (p *ChainedPromise) resolve(value Result, js *JS) {
    if !p.state.CompareAndSwap(int32(Pending), int32(Fulfilled)) {
        // Already settled
        return
    }
    // ...
}
```

**Conclusion:** This is **CORRECT** - the underlying promise implementation handles the spec compliance. The adapter correctly propagates the error.

---

#### 5.2 Resolve/Reject Callback Closure Scope

**Location:** `adapter.go:241-245`

```go
a.runtime.ToValue(func(result goja.Value) {
    var val any
    if result != goja.Undefined() && result.Export() != nil {
        val = result.Export()
    }
    resolve(val)
}),
```

**POTENTIAL ISSUE:** The closures capture `resolve` and `reject` from the outer scope.

**Verification:**
1. `promise, resolve, reject := a.js.NewChainedPromise()` - creates functions
2. These can be called from any goroutine (per `ChainedPromise` API)
3. The Goja callbacks call them from the executor call
4. The executor call happens on the event loop thread (correct)

**Conclusion:** This is **CORRECT** - the closures capture the right functions and are called from the right thread.

---

### 6. TEST COVERAGE GAPS

#### 6.1 SKIPPED TESTS

**Location:** `adapter_test.go`

**CRITICAL:** Multiple tests are skipped with `t.Skip()`:

```go
func TestPromiseChain(t *testing.T) {
    t.Skip("Promise chaining requires additional work - deferred to Phase 3")
}

func TestMixedTimersAndPromises(t *testing.T) {
    t.Skip("Timer/microtask/Promise interaction tests require Promise chaining - deferred to Phase 3")
}

func TestConcurrentJSOperations(t *testing.T) {
    t.Skip("Concurrent JS operations require Promise chaining - deferred to Phase 3")
}
```

**ISSUE:** Promise chaining **IS IMPLEMENTED** but not tested.

**Analysis of Current Tests:**
- `TestSetTimeout` - ✅ Basic setTimeout
- `TestClearTimeout` - ✅ clearTimeout works
- `TestSetInterval` - ✅ Basic setInterval (3 fires)
- `TestClearInterval` - ✅ clearInterval works
- `TestQueueMicrotask` - ✅ Basic microtask
- `TestPromiseThen` - ⚠️ EXISTS BUT ONLY CHECKS METHOD EXISTS
- `TestPromiseChain` - ❌ SKIPPED
- `TestMixedTimersAndPromises` - ❌ SKIPPED
- `TestContextCancellation` - ⚠️ WEAK (no verification of behavior)
- `TestConcurrentJSOperations` - ❌ SKIPPED

**MISSING TEST COVERAGE:**

1. **Promise Resolution Values:**
   ```javascript
   Test that promise values are correctly passed:
   - Primitive values (number, string, boolean, null, undefined)
   - Objects (plain objects, arrays)
   - Nested promises (promise resolution)
   ```

2. **Promise Rejection:**
   ```javascript
   Test that promise errors are correctly handled:
   - Thrown errors in executor
   - Explicit reject() calls
   - Error propagation through chains
   ```

3. **Promise Chaining:**
   ```javascript
   Test multi-step chains:
   - Multiple .then() calls
   - .catch() handling errors
   - .finally() cleanup
   - Return value transformation
   ```

4. **Promise Combinators:**
   ```javascript
   Test All, Race, AllSettled, Any:
   - Empty arrays
   - Single promise
   - Multiple promises
   - Mixed resolved/rejected
   - Timeouts
   ```

5. **Timer vs Microtask Ordering:**
   ```javascript
   Test event loop ordering:
   - setTimeout(0) vs queueMicrotask()
   - Microtasks drain before timer callbacks
   - Nested microtasks
   ```

**Recommendation:** Fill all skipped tests BEFORE merging.

---

#### 6.2 Weak Test: TestContextCancellation

**Location:** `adapter_test.go:239-256`

```go
func TestContextCancellation(t *testing.T) {
    _, cancel := context.WithCancel(context.Background())
    loop, err := goeventloop.New()
    // ... setup adapter ...

    _, err = runtime.RunString(`
        setTimeout(() => {});
        setTimeout(() => {});
        setTimeout(() => {});
    `)
    if err != nil {
        t.Fatalf("Failed to run JavaScript: %v", err)
    }

    // Cancel immediately
    cancel()

    // The loop should shutdown cleanly
    t.Log("Context cancellation handled cleanly")
}
```

**CRITICAL ISSUE:** Test **does not verify anything** - just logs a message.

**The Problem:**
1. Test creates 3 setTimeouts
2. Cancels context immediately
3. **Does NOT verify:** timers are canceled
4. **Does NOT verify:** loop shuts down without error
5. **Does NOT verify:** no goroutine leaks
6. **Does NOT verify:** memory is cleaned up

**Recommendation:**
```go
func TestContextCancellation(t *testing.T) {
    ctx, cancel := context.WithCancel(context.Background())
    loop, err := goeventloop.New()
    if err != nil {
        t.Fatalf("Failed to create loop: %v", err)
    }
    defer loop.Shutdown(ctx)

    runtime := goja.New()
    adapter, err := New(loop, runtime)
    if err != nil {
        t.Fatalf("Failed to create adapter: %v", err)
    }

    if err := adapter.Bind(); err != nil {
        t.Fatalf("Failed to bind adapter: %v", err)
    }

    // Run loop in background
    done := make(chan error, 1)
    go func() {
        done <- loop.Run(ctx)
    }()

    // Schedule timers that SHOULD NOT execute
    called := make(chan bool, 3)
    _, err = runtime.RunString(`
        setTimeout(() => {
            called = 'timer1';
        }, 100);
        setTimeout(() => {
            called = 'timer2';
        }, 200);
        setTimeout(() => {
            called = 'timer3';
        }, 300);
    `)
    if err != nil {
        t.Fatalf("Failed to run JavaScript: %v", err)
    }

    // Cancel immediately
    cancel()

    // Wait for loop to exit
    err = <-done
    if err != nil {
        t.Errorf("Loop should exit cleanly, got error: %v", err)
    }

    // Verify timers did NOT fire
    result := runtime.Get("called")
    if result.Export() != nil {
        t.Errorf("Timers should have been cancelled, got called: %v", result.Export())
    }
}
```

---

### 7. PROMISE COMBINATORS CORRECTNESS

#### 7.1 Type Conversion in Combinators

**Location:** `adapter.go:387-410` (All, Race, AllSettled, Any)

```go
func (a *Adapter) All(promises []*goeventloop.ChainedPromise) *goeventloop.ChainedPromise {
    return a.js.All(promises)
}

func (a *Adapter) Race(promises []*goeventloop.ChainedPromise) *goeventloop.ChainedPromise {
    return a.js.Race(promises)
}

func (a *Adapter) AllSettled(promises []*goeventloop.ChainedPromise) *goeventloop.ChainedPromise {
    return a.js.AllSettled(promises)
}

func (a *Adapter) Any(promises []*goeventloop.ChainedPromise) *goeventloop.ChainedPromise {
    return a.js.Any(promises)
}
```

**CRITICAL ISSUE:** These methods **expose Go internal types** to the public API.

**The Problem:**
1. `All/Race/etc.` accept `[]*goeventloop.ChainedPromise`
2. These are **Go types**, not JavaScript promise objects
3. Users must manually extract `_internalPromise` from Goja objects
4. This defeats the purpose of the adapter (transparent Go interoperability)

**Expected API:**
```go
// Should accept Goja promise objects
func (a *Adapter) All(promises []goja.Value) goja.Value {
    // Convert Goja objects to ChainedPromise
    goPromises := make([]*goeventloop.ChainedPromise, len(promises))
    for i, p := range promises {
        pObj := p.ToObject(a.runtime)
        internal := pObj.Get("_internalPromise")
        if internal.Export() == nil {
            panic(a.runtime.NewTypeError("Not a promise"))
        }
        goPromises[i] = internal.Export().(*goeventloop.ChainedPromise)
    }
    result := a.js.All(goPromises)
    return a.gojaWrapPromise(result)
}
```

**Verification:** These methods are **NOT TESTED AT ALL**. No test invokes them.

**Recommendation:**
1. Update method signatures to accept/goja.Value types
2. Add JavaScript bindings for `Promise.all`, `Promise.race`, etc.
3. Add comprehensive tests

---

#### 7.2 JavaScript Promise.all Compatibility

**CRITICAL:** `Promise.all` is **NOT BOUND** to the JavaScript runtime.

**Location:** `adapter.go:124-127` (Bind method)

```go
// Bind installs JavaScript global functions into the Goja runtime.
// This adds the following globals:
//   - setTimeout(fn, delay) → number
//   - clearTimeout(id)
//   - setInterval(fn, delay) → number
//   - clearInterval(id)
//   - queueMicrotask(fn)
//   - Promise(executor) with then/catch/finally  <-- BUT NOT all/race/any!
func (a *Adapter) Bind() error {
    // ...
    // Define Promise constructor using native constructor signature
    promiseConstructor := a.runtime.ToValue(func(call goja.ConstructorCall) *goja.Object {
        return a.promiseConstructorWrapper(call)
    })
    if err := a.runtime.Set("Promise", promiseConstructor); err != nil {
        return fmt.Errorf("failed to bind Promise: %w", err)
    }

    return nil
}
```

**MISSING BINDINGS:**
- `Promise.all()` - NOT BOUND
- `Promise.race()` - NOT BOUND
- `Promise.allSettled()` - NOT BOUND
- `Promise.any()` - NOT BOUND

**Expected Implementation:**
```go
// Bind Promise constructor with static methods
promiseConstructor := a.runtime.ToValue(func(call goja.ConstructorCall) *goja.Object {
    return a.promiseConstructorWrapper(call)
})
promiseObj := promiseConstructor.ToObject(a.runtime)

// Add static methods
if err := promiseObj.Set("all", a.jsAll); err != nil {
    return fmt.Errorf("failed to bind Promise.all: %w", err)
}
if err := promiseObj.Set("race", a.jsRace); err != nil {
    return fmt.Errorf("failed to bind Promise.race: %w", err)
}
// ...

if err := a.runtime.Set("Promise", promiseConstructor); err != nil {
    return fmt.Errorf("failed to bind Promise: %w", err)
}
```

**Verification:** Attempt to call `Promise.all()` from JavaScript - it will fail.

---

### 8. EDGE CASES AND SPECIAL BEHAVIORS

#### 8.1 Negative Delay Values

**Location:** `adapter.go:82-84`

```go
if delayMs < 0 {
    panic(a.runtime.NewTypeError("delay cannot be negative"))
}
```

**ISSUE:** This is **overly strict** compared to JavaScript spec.

**JavaScript Behavior:**
```javascript
setTimeout(fn, -100);  // Valid: treated as 0 delay
setInterval(fn, -10); // Valid: treated as 0 delay
```

**Current Behavior:** Panics

**Recommendation:**
```go
if delayMs < 0 {
    delayMs = 0  // Clamp to 0 per JavaScript spec
}
```

**Verification:** Run test with negative delay - should NOT panic.

---

#### 8.2 Non-Function Callbacks

**Location:** `adapter.go:79-82`

```go
fn := call.Argument(0)
if fn.Export() == nil {
    panic(a.runtime.NewTypeError("setTimeout requires a function as first argument"))
}

fnCallable, ok := goja.AssertFunction(fn)
if !ok {
    panic(a.runtime.NewTypeError("setTimeout requires a function as first argument"))
}
```

**ISSUE:** Panic-based error handling is **not user-friendly**.

**The Problem:**
1. Invalid input types throw TypeError immediately
2. This crashes the JavaScript execution context
3. User cannot catch or handle this error
4. Better: Return error value or use Goja error mechanism

**Current:** `setTimeout("not a function", 100)` → **PANICS**
**Expected:** `setTimeout("not a function", 100)` → **Throws in JavaScript context**

But this is actually correct for JavaScript!

**Conclusion:** This behavior is CORRECT for JavaScript compatibility. Goja panics in callbacks translate to JavaScript errors.

---

#### 8.3 Timer ID Overflow

**Location:** `adapter.go:99`

```go
id, err := a.js.SetTimeout(func() {
    _, _ = fnCallable(goja.Undefined())
}, delayMs)
if err != nil {
    panic(a.runtime.NewGoError(err))
}

return a.runtime.ToValue(id)
```

**POTENTIAL ISSUE:** Timer ID is `uint64` - what happens when it overflows?

**Analysis:**
1. `js.nextTimerID.Add(1)` in `js.go` uses `atomic.Uint64`
2. After 2⁶⁴ timers, it wraps to 0
3. Old timer with ID 0 doesn't exist (it would have fired already)
4. **BUT:** Recent timers might still be active
5. Timer recycling can cause false positives in `clearTimeout(id)`

**Recommendation:** Document the limitation or use a different ID scheme.

---

### 9. PERFORMANCE CONCERNS

#### 9.1 Mutex Overhead in ensureLoopThread

**Location:** `adapter.go:75-79`

```go
func (a *Adapter) ensureLoopThread() {
    a.mu.Lock()
    defer a.mu.Unlock()
}
```

**PERFORMANCE ISSUE:** Every timeout/interval/promise call acquires a mutex.

**The Problem:**
1. `setTimeout` calls `ensureLoopThread()` - mutex lock/unlock
2. `setInterval` calls `ensureLoopThread()` - mutex lock/unlock
3. `clearTimeout` calls `ensureLoopThread()` - mutex lock/unlock
4. `queueMicrotask` calls `ensureLoopThread()` - mutex lock/unlock
5. `promiseConstructorWrapper` calls `ensureLoopThread()` - mutex lock/unlock
6. Then/catch/finally methods do NOT call `ensureLoopThread()` - **INCONSISTENT**

**Impact:**
- Unnecessary synchronization overhead
- The mutex does NOT provide actual safety (as documented in section 1.1)

**Recommendation:** Remove `ensureLoopThread()` entirely, or document why it's needed.

---

#### 9.2 Object Creation in Promise Methods

**Location:** `adapter.go:287-326` (gojaWrapPromise)

```go
func (a *Adapter) gojaWrapPromise(promise *goeventloop.ChainedPromise) goja.Value {
    wrapped := a.runtime.ToValue(promise)
    wrappedObj := wrapped.ToObject(a.runtime)

    // Then method
    thenFn := a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
        // ...
    })
    // ... create catchFn, finallyFn
}
```

**PERFORMANCE ISSUE:** Every `.then()` call creates 3 new closures.

**The Problem:**
1. Long promise chains: `p.then(x).then(y).then(z).then(w)...`
2. Each `.then()` creates new closures for then/catch/finally
3. These closures capture runtime state
4. High GC pressure

**Potential Optimization:**
- Cache shared method implementations
- Use prototype-based inheritance (like JavaScript)
- Only create methods when actually called

**Recommendation:** Profile performance on long chains. Only optimize if actually slow.

---

### 10. DOCUMENTATION AND API DESIGN

#### 10.1 Deprecated NewChainedPromise Function

**Location:** `adapter.go:368-384`

```go
// NewChainedPromise creates a new promise with Goja-compatible resolve/reject.
//
// This is a convenience function for creating promises from Go code that
// will be used in JavaScript. The returned resolve and reject functions
// accept goja.Value arguments.
//
// Deprecated: Prefer using [Adapter.JS().NewChainedPromise()] and converting
// values manually for more control.
func NewChainedPromise(loop *goeventloop.Loop, runtime *goja.Runtime) (*goeventloop.ChainedPromise, goja.Value, goja.Value) {
    js, err := goeventloop.NewJS(loop)
    if err != nil {
        panic(err)
    }

    promise, resolve, reject := js.NewChainedPromise()

    resolveVal := runtime.ToValue(func(result goja.Value) {
        resolve(result.Export())
    })

    rejectVal := runtime.ToValue(func(reason goja.Value) {
        reject(reason.Export())
    })

    return promise, resolveVal, rejectVal
}
```

**ISSUE:** Deprecated function exists without deprecation tag.

**Recommendation:**
```go
// Deprecated: Prefer using Adapter.JS().NewChainedPromise() and converting
// values manually for more control. Will be removed in v2.0.0.
//lint:ignore U1000  // Kept for backward compatibility
func NewChainedPromise(...) {
```

---

#### 10.2 Missing Documentation on Thread Safety

**CRITICAL:** Package-level documentation does NOT explain actual thread safety model.

**Current doc:**
```go
// Thread Safety:
//
// The adapter coordinates between Goja (which is not thread-safe) and the
// event loop. JavaScript callbacks are always executed on the event loop
// thread. The Goja runtime should only be accessed from the event loop
// thread after binding.
```

**Missing:**
1. How to ensure Goja is on the event loop thread?
2. What happens if you access Goja from wrong thread?
3. How to use multiple adapters with multiple loops?
4. How to use a single adapter with multiple runtimes?

**Recommendation:** Add detailed thread safety usage examples:
```go
// Thread Safety:
//
// The adapter is NOT thread-safe unless properly configured:
//
// Option 1: Single-threaded (recommended)
//   - Create one Loop and one Runtime
//   - Run Loop in dedicated goroutine: go loop.Run(ctx)
//   - All JavaScript code runs on Loop's goroutine
//   - Never access Runtime from other goroutines
//
// Option 2: Multi-threaded (advanced)
//   - Use channels to queue work to Loop
//   - Never call Runtime methods directly from other goroutines
//
// The ensureLoopThread() method provides a memory barrier but does NOT
// guarantee thread safety. Actual safety comes from Loop.Run() executing
// all callbacks on a single thread.
```

---

## RECOMMENDED FIX LIST

### MUST FIX (Blocking Issues):

1. ❌ **Remove or fix `ensureLoopThread()`** - It's misleading and provides no actual safety
2. ❌ **Add panic recovery** in all Goja callbacks (setTimeout, setInterval, queueMicrotask, promise handlers)
3. ❌ **Add type assertion validation** in promise methods (check ok return value)
4. ❌ **Bind Promise.all/race/allSettled/any** to JavaScript runtime
5. ❌ **Update combinator methods** to accept/goja.Value instead of `[]*ChainedPromise`
6. ❌ **Fill all skipped tests** (PromiseChain, MixedTimers, ConcurrentOps)
7. ❌ **Fix TestContextCancellation** to actually verify behavior

### SHOULD FIX (Important Issues):

8. ⚠️ **Clamp negative delays to 0** instead of panicking
9. ⚠️ **Add goroutine leak detection** to tests
10. ⚠️ **Add memory leak tests** for promise chains
11. ⚠️ **Remove or deprecate ensureLoopThread()** mutex
12. ⚠️ **Add deprecation tag** to NewChainedPromise
13. ⚠️ **Improve thread safety documentation**

### COULD FIX (Nice to Have):

14. 💡 Add `Promise.all/race/etc.` static method bindings to JavaScript
15. 💡 Optimize closure creation in promise methods
16. 💡 Add timer overflow handling/documentation
17. 💡 Add nil vs undefined distinction preservation

---

## VERIFICATION CHECKLIST

Before merging, verify:

- [ ] All tests pass (no skipped tests)
- [ ] Add test: `setTimeout` with negative delay (should not panic)
- [ ] Add test: `setTimeout` callback throws error (loop continues, error reported)
- [ ] Add test: `Promise.then` handler throws error (returned promise rejects)
- [ ] Add test: Multi-step promise chain (10+ .then() calls)
- [ ] Add test: `Promise.all` with empty array
- [ ] Add test: `Promise.all` with mixed resolved/rejected
- [ ] Add test: `Promise.race` with timeout
- [ ] Add test: Manual corruption of `_internalPromise` (should panic gracefully)
- [ ] Add test: Context cancellation cancels all pending timers
- [ ] Add test: Goroutine leak detection after 1000 setTimeouts
- [ ] Add test: Memory leak detection after 10000 promise chains
- [ ] Verify: `Promise.all` is callable from JavaScript
- [ ] Verify: `Promise.race` is callable from JavaScript
- [ ] Verify: `Promise.allSettled` is callable from JavaScript
- [ ] Verify: `Promise.any` is callable from JavaScript
- [ ] Document thread safety model with examples
- [ ] Add deprecation tag to NewChainedPromise
- [ ] Remove ensureLoopThread() mutex or document purpose

---

## CONCLUSION

This code represents a **good start** at Goja integration, but has **critical safety issues** that prevent production use:

1. **Fake thread safety** through misleading mutex usage
2. **Panic infrastructure** that crashes the event loop instead of handling errors
3. **Unsafe type assertions** that can crash on corrupted state
4. **Missing test coverage** for critical features
5. **Incomplete API** (Promise combinators not bound)
6. **Weak error handling** (negative delays, context cancellation)

**Time to Fix:** Estimate 2-3 days for critical issues only.

**Recommendation:** **DO NOT MERGE** until all MUST FIX items are addressed. The SHOULD FIX items should also be completed before production release.

---

**Verdict:** ❌ CRITICAL ISSUES - NOT PRODUCTION READY
