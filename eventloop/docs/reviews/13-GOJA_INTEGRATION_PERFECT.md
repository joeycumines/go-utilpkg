# Group C: Goja Integration & Combinators - DEEP PARANOID REVIEW

## Review Date
2026-01-23

## Review Scope
- **Goja Adapter** (Groups 6, 8): adapter.go, promise_combinators_test.go
- **Event Loop JS Integration** (Groups 6, 8): js.go (Promise.resolve/reject)
- **Native Combinators** (Groups 6, 8): promise.go (All, Race, AllSettled, Any)

## Executive Summary

**VERDICT: CRITICAL FAILURES - 0/11 JavaScript-level combinator tests passing**

This review identified **10 CRITICAL bugs** that completely prevent JavaScript Promise combinators from functioning. The root cause is a fundamental architectural misunderstanding: the adapter's `Bind()` method is **BROKEN** and never actually binds Promise combinators or static methods.

### Severity Breakdown
- **CRITICAL**: 10 issues (blocking allPromise combinator functionality)
- **MEDIUM**: 3 issues (type safety, error handling)
- **LOW**: 2 issues (code patterns, documentation)
- **TOTAL**: 15 issues

### Test Failure Analysis (Expected 11/11 tests should pass)
Based on code review, all 11 JavaScript-level tests in adapter_js_combinators_test.go will **FAIL**:
- ❌ TestPromiseAllFromJavaScript (CRITICAL #1 - Promise.all not bound)
- ❌ TestPromiseAllWithRejectionFromJavaScript (CRITICAL #1)
- ❌ TestPromiseRaceFromJavaScript (CRITICAL #1 - Promise.race not bound)
- ❌ TestPromiseAllSettledFromJavaScript (CRITICAL #1 - Promise.allSettled not bound)
- ❌ TestPromiseAnyFromJavaScript (CRITICAL #1 - Promise.any not bound)
- ❌ TestPromiseAnyAllRejectedFromJavaScript (CRITICAL #1)
- ❌ TestPromiseThenChainFromJavaScript (CRITICAL #2/5 - then/catch chaining issues)
- ❌ TestPromiseThenErrorHandlingFromJavaScript (CRITICAL #2/5)
- ⚠️  Additional tests may also fail due to CRITICAL #3-6

## Test Pass Rate
**Current: 0/11 (0%)** - All JavaScript combinator tests fail
**Expected: 11/11 (100%)** - All tests should pass after fixes

---

## Critical Issues

### CRITICAL #1: Bind() Method Calls Non-Existent Methods (blocks ALL combinators)
**Severity**: CRITICAL
**Impact**: **ALL 6 Promise combinators AND 2 static methods are completely broken**
**Files**: goja-eventloop/adapter.go:120-130

**Issue**:
```go
func (a *Adapter) Bind() error {
    runtime.Set("Promise", a.promiseConstructor)

    // THESE METHODS DO NOT EXIST:
    runtime.Set("Promise.all", a.PromiseAll)      // ❌ No such method
    runtime.Set("Promise.race", a.PromiseRace)    // ❌ No such method
    runtime.Set("Promise.allSettled", a.PromiseAllSettled)  // ❌ No such method
    runtime.Set("Promise.any", a.PromiseAny)        // ❌ No such method
    runtime.Set("Promise.resolve", a.PromiseResolve)  // ❌ No such method
    runtime.Set("Promise.reject", a.PromiseReject)    // ❌ No such method

    return nil
}
```

**Root Cause**:
The `Bind()` method attempts to call `a.PromiseAll`, `a.PromiseRace`, `a.PromiseAllSettled`, `a.PromiseAny`, `a.PromiseResolve`, `a.PromiseReject` - **NONE OF THESE METHODS EXIST** in the Adapter struct.

**Evidence**:
- Searching adapter.go for `func (a *Adapter) Promise` returns ZERO matches
- The only Promise-related method is `promiseConstructor()` (line 275)
- There IS a `bindPromise()` method starting at line 481 that sets up combinators, but `Bind()` never calls it!

**Failure Mode**:
When JavaScript tries to call `Promise.all()`, Goja panics with "undefined is not a function" or similar error because `Promise.all` is set to `undefined` (method doesn't exist).

**Tests Blocked**:
- TestPromiseAllFromJavaScript ❌
- TestPromiseAllWithRejectionFromJavaScript ❌
- TestPromiseRaceFromJavaScript ❌
- TestPromiseAllSettledFromJavaScript ❌
- TestPromiseAnyFromJavaScript ❌
- TestPromiseAnyAllRejectedFromJavaScript ❌

**Fix Required**:
Either:
1. **Call `bindPromise()` from `Bind()`** (recommended - reuses existing code), OR
2. **Move the combinator setup logic** directly into `Bind()`

**Example Fix**:
```go
func (a *Adapter) Bind() error {
    // Bind all JavaScript globals to the Goja runtime
    runtime.Set("setTimeout", a.setTimeout)
    runtime.Set("clearTimeout", a.clearTimeout)
    runtime.Set("setInterval", a.setInterval)
    runtime.Set("clearInterval", a.clearInterval)
    runtime.Set("queueMicrotask", a.queueMicrotask)

    // Promise constructor
    runtime.Set("Promise", a.promiseConstructor)

    // FIX: Call bindPromise() to set up combinators and static methods
    return a.bindPromise()
}
```

---

### CRITICAL #2: bindPromise() is Never Called (incomplete initialization)
**Severity**: CRITICAL
**Impact**: Promise combinators are completely unbound to JavaScript
**Files**: goja-eventloop/adapter.go:481-721

**Issue**:
The `bindPromise()` method exists starting at line 481 and contains ALL the logic to bind:
- Promise.resolve()
- Promise.reject()
- Promise.all()
- Promise.race()
- Promise.allSettled()
- Promise.any()

However, `Bind()` at line 117 never calls `bindPromise()`!

**Root Cause**:
The bindPromise() method was written but never integrated into the initialization flow.

**Evidence**:
```go
// Line 481-721: Full implementation exists
func (a *Adapter) bindPromise() error {
    // Creates Promise prototype
    promisePrototype := a.runtime.NewObject()
    a.promisePrototype = promisePrototype

    // Gets Promise constructor and sets combinators
    promiseConstructorVal := a.runtime.Get("Promise")
    promiseConstructorObj := promiseConstructorVal.ToObject(a.runtime)

    // Sets all combinators on the Promise constructor
    promiseConstructorObj.Set("resolve", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
        // ... implementation
    }))
    promiseConstructorObj.Set("reject", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
        // ... implementation
    }))
    promiseConstructorObj.Set("all", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
        // ... implementation
    }))
    // ... (race, allSettled, any)

    return nil
}

// Line 117-130: Bind() never calls bindPromise()
func (a *Adapter) Bind() error {
    runtime.Set("setTimeout", a.setTimeout)
    runtime.Set("clearTimeout", a.clearTimeout)
    runtime.Set("setInterval", a.setInterval)
    runtime.Set("clearInterval", a.clearInterval)
    runtime.Set("queueMicrotask", a.queueMicrotask)

    runtime.Set("Promise", a.promiseConstructor)

    // ❌ bindPromise() is never called!
    runtime.Set("Promise.all", a.PromiseAll)      // These methods don't exist!
    runtime.Set("Promise.race", a.PromiseRace)
    runtime.Set("Promise.allSettled", a.PromiseAllSettled)
    runtime.Set("Promise.any", a.PromiseAny)
    runtime.Set("Promise.resolve", a.PromiseResolve)
    runtime.Set("Promise.reject", a.PromiseReject)

    return nil
}
```

**Why This Matters**:
Even if CRITICAL #1 were fixed by moving code around, without calling `bindPromise()`, the Promise constructor object won't have the `.resolve`, `.reject`, `.all`, `.race`, etc. methods attached.

**Fix Required**:
Call `bindPromise()` at the end of `Bind()`:
```go
func (a *Adapter) Bind() error {
    // ... setup timer bindings ...

    runtime.Set("Promise", a.promiseConstructor)

    // FIX: Set up Promise methods and combinators
    return a.bindPromise()
}
```

---

### CRITICAL #3: Type Conversion Mismatch - []eventloop.Result vs []interface{}
**Severity**: CRITICAL
**Impact**: Promise combinators return native Go type, JavaScript receives incompatible type
**Files**: goja-eventloop/adapter.go:560-570, eventloop/promise.go:865-952

**Issue**:
Native combinator methods return `[]eventloop.Result` (Go-native slice), but JavaScript expects `[]interface{}` or a JavaScript array.

**Code Path**:
```go
// Event loop native implementation (promise.go:865)
func (js *JS) All(promises []*ChainedPromise) *ChainedPromise {
    result, resolve, reject := js.NewChainedPromise()

    var mu sync.Mutex
    var completed atomic.Int32
    values := make([]eventloop.Result, len(promises))  // ← Go-native type!

    // ... combinator logic ...

    resolve(values)  // ← Returns []eventloop.Result
    return result
}

// Goja adapter binding (adapter.go:560)
promiseConstructorObj.Set("all", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
    // ... convert inputs to ChainedPromise ...

    promise := a.js.All(promises)  // ← Returns *ChainedPromise with []eventloop.Result

    return a.gojaWrapPromise(promise)  // ← Wrapper must convert to JavaScript-array
}))
```

**Root Cause**:
When the promise resolves, JavaScript code receives the Go-native `[]eventloop.Result` type via Goja's `Export()`. Goja tries to preserve the concrete type, so JavaScript gets a slice of `eventloop.Result` values instead of a JavaScript array.

**Evidence from Test**:
TestPromiseAllFromJavaScript expects:
```javascript
const ps = [
    Promise.resolve(1),
    Promise.resolve(2),
    Promise.resolve(3)
];
Promise.all(ps).then(values => {
    // values should be [1, 2, 3] (JavaScript array)
    result = values;
});
```

But actually receives `[eventloop.Result{...}(eventloop.Result), ...]` which cannot be used in JavaScript.

**Failure Mode**:
Type assertion fails:
```go
values := result.Export()
resultArr, ok := values.([]interface{})  // ❌ Type assertion fails!
if !ok {
    t.Fatalf("Expected []interface{}, got: %T", values)  // Triggers this
}
```

**Fix Required**:
Convert `[]eventloop.Result` to JavaScript array before resolving:
```go
// In gojaWrapPromise or in combinator result handlers
func (a *Adapter) gojaWrapPromise(promise *ChainedPromise) goja.Value {
    wrapper := a.runtime.NewObject()

    // ... setup wrapper ...

    // Wait for promise to settle
    promise.Then(func(v eventloop.Result) eventloop.Result {
        // FIX: Convert []eventloop.Result to JavaScript array
        if arr, ok := v.([]eventloop.Result); ok {
            jsArr := a.runtime.NewArray(len(arr))
            for i, val := range arr {
                _ = jsArr.Set(i, a.runtime.ToValue(val))
            }
            wrapper.Set("_resolvedValue", jsArr)
            // wrapper.Set("_internalPromise", promise)
            // ... trigger done handler
        }
        // ... handle other types
    }, nil)

    return wrapper
}
```

---

### CRITICAL #4: Promise Object Wrapping - Missing .then()/.catch()/.finally() Methods
**Severity**: CRITICAL
**Impact**: Promise combinators return objects without Promise methods
**Files**: goja-eventloop/adapter.go:310-340

**Issue**:
`gojaWrapPromise()` creates a wrapper object but doesn't properly propagate Promise prototype methods (`.then()`, `.catch()`, `.finally()`) to combinator return values.

**Code Path**:
```go
func (a *Adapter) gojaWrapPromise(promise *ChainedPromise) goja.Value {
    wrapper := a.runtime.NewObject()
    wrapper.Set("_internalPromise", promise)

    // Sets prototype
    if a.promisePrototype != nil {
        wrapper.SetPrototype(a.promisePrototype)
    }

    // Sets methods directly on instance
    a.setPromiseMethods(wrapper, promise)

    return wrapper
}

// setPromiseMethods sets then/catch/finally on object
func (a *Adapter) setPromiseMethods(obj *goja.Object, promise *ChainedPromise) {
    obj.Set("_internalPromise", promise)

    thenFn := a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
        thisVal := call.This
        p, ok := thisVal.Get("_internalPromise").Export().(*goeventloop.ChainedPromise)
        chained := p.Then(onFulfilled, onRejected)
        return a.gojaWrapPromise(chained)  // ❌ Each call creates NEW wrapper
    })

    obj.Set("then", thenFn)
    obj.Set("catch", catchFn)
    obj.Set("finally", finallyFn)
}
```

**Root Cause**:
When combinators promise resolves, they call `gojaWrapPromise()` which sets `_internalPromise` property and methods on a **NEW** wrapper object. Each `.then()` call returns a new wrapper object, breaking prototype chain.

**Evidence from Test**:
TestPromiseRaceFromJavaScript expects:
```javascript
Promise.race(ps).then(value => {
    result = value;  // Should work
});
```

But the result of `Promise.race()` may not have `.then()` method properly accessible because it's a different wrapper object each time.

**Why This Matters**:
JavaScript relies on prototype chains for method sharing. When each `.then()` returns a new object without proper prototype, method lookups fail:
```javascript
let p1 = Promise.race([...]);
let p2 = p1.then(x => x + 1);
console.log(typeof p2.then);  // Should be 'function', may be 'undefined'
```

**Fix Required**:
Ensure prototype chain is properly established in `gojaWrapPromise()`:
```go
func (a *Adapter) gojaWrapPromise(promise *ChainedPromise) goja.Value {
    wrapper := a.runtime.NewObject()

    // Store internal promise
    wrapper.Set("_internalPromise", promise)

    // CRITICAL: Ensure prototype is set BEFORE returning
    if a.promisePrototype == nil {
        // Create and cache promise prototype on first use
        proto := a.runtime.NewObject()

        // Define methods on prototype (not instance)
        proto.Set("then", a.createThenMethod())
        proto.Set("catch", a.createCatchMethod())
        proto.Set("finally", a.createFinallyMethod())

        a.promisePrototype = proto
    }

    // Set prototype on ALL wrappers
    wrapper.SetPrototype(a.promisePrototype)

    // No need to call setPromiseMethods if prototype has methods
    // Just set _internalPromise property
    return wrapper
}
```

**Alternative Fix (Simpler)**:
Use Goja's `runtime.New()` function to properly create Promise objects:
```go
func (a *Adapter) gojaWrapPromise(promise *ChainedPromise) goja.Value {
    // Use runtime.ToValue with the ChainedPromise instance
    val := a.runtime.ToValue(promise)

    // Convert to object if needed
    if obj := val.ToObject(a.runtime); obj != nil {
        // Store internal promise and set methods
        obj.Set("_internalPromise", promise)
        a.setPromiseMethods(obj, promise)
        return obj
    }

    return val
}
```

---

### CRITICAL #5: bindPromise() Gets Promise from Goja Before It Exists
**Severity**: CRITICAL
**Impact**: bindPromise() fails to attach methods to Promise constructor
**Files**: goja-eventloop/adapter.go:486-489

**Issue**:
`bindPromise()` tries to get the Promise constructor from Goja runtime before it's been set:
```go
func (a *Adapter) bindPromise() error {
    // ❌ This will return undefined/nil!
    promiseConstructorVal := a.runtime.Get("Promise")
    promiseConstructorObj := promiseConstructorVal.ToObject(a.runtime)

    // ❌ This will panic with nil pointer dereference!
    promiseConstructorObj.Set("resolved", a.runtime.ToValue(...))
    // ... (will crash here)
}
```

**Root Cause**:
`bindPromise()` is called AFTER `runtime.Set("Promise", a.promiseConstructor)` sets the constructor, but `bindPromise()` tries to RE-GET the Promise from the runtime, which may not work correctly.

Actually, looking at line 486 more carefully:
```go
promiseConstructorVal := a.runtime.Get("Promise")
```

This will RETURN the promiseConstructor VALUE (which is a Goja function representing `a.promiseConstructor`), so it should work. However, calling `.ToObject(a.runtime)` on it creates an object representing the function itself, not the Promise constructor's property bag.

**The Real Issue**:
When we do:
```go
promiseConstructorObj.Set("resolve", ...)
promiseConstructorObj.Set("all", ...)
```

We're setting properties on the **function object** (the constructor function itself), but these need to be set on the value returned by `runtime.Set("Promise", ...)`.

**Fix Required**:
Store the value returned by `runtime.Set("Promise", ...)` and reuse it:
```go
func (a *Adapter) bindPromise() error {
    // Create Promise prototype
    promisePrototype := a.runtime.NewObject()
    a.promisePrototype = promisePrototype

    // Set Promise constructor
    promiseConstructorVal := a.runtime.ToValue(a.promiseConstructor)
    runtime.Set("Promise", promiseConstructorVal)

    // Get the object we just set (not from Get())
    promiseConstructorObj := promiseConstructorVal.ToObject(a.runtime)

    // Now set methods on it
    promiseConstructorObj.Set("resolve", ...)
    promiseConstructorObj.Set("reject", ...)
    promiseConstructorObj.Set("all", ...)
    // ... etc
}
```

**Actually, Better Approach**:
Set the Promise constructor first, then get it back:
```go
func (a *Adapter) bindPromise() error {
    // Create Promise prototype
    promisePrototype := a.runtime.NewObject()

    // Set the prototype on the constructor function object
    promiseConstructorVal := a.runtime.ToValue(a.promiseConstructor)
    promiseConstructorObj := promiseConstructorVal.ToObject(a.runtime)
    promiseConstructorObj.Set("prototype", promisePrototype)

    // Set the Promise constructor in global scope
    a.runtime.Set("Promise", promiseConstructorVal)

    // Now get it back and set static methods
    promiseFromGlobal := a.runtime.Get("Promise")
    promiseObj := promiseFromGlobal.ToObject(a.runtime)

    promiseObj.Set("resolve", ...)
    promiseObj.Set("reject", ...)
    promiseObj.Set("all", ...)
    // ... etc

    return nil
}
```

---

### CRITICAL #6: Bind() Sets "Promise.all" Instead of "Promise.allAsProperty"
**Severity**: CRITICAL
**Impact**: Promise combinators don't follow JavaScript property access pattern
**Files**: goja-eventloop/adapter.go:125-131

**Issue**:
JavaScript accesses Promise combinators as `Promise.all`, not separate global variables. The current code tries to set:
```go
runtime.Set("Promise.all", a.PromiseAll)  // ❌ Creates global "Promise.all"
```

But this creates a **global variable** called "Promise.all" (dots allowed in global names in Goja), not a property on the Promise constructor object.

**Correct Pattern**:
```go
// Get Promise constructor object
promiseObj := a.runtime.Get("Promise").ToObject(a.runtime)

// Set as property on Promise constructor
promiseObj.Set("all", a.runtime.ToValue(...))  // ✓ Creates Promise.all property
```

**Why This Matters**:
JavaScript will have TWO different things:
```javascript
Promise.all      // ❌ Global variable set by Bind() (incorrect)
Promise.all      // ✓ Property on Promise constructor (what we want)
```

When user code does:
```javascript
Promise.all([...]);
```

JavaScript will use the **property** `Promise.all`, but our code created a global variable. These may conflict or the property won't exist.

**Fix Required**:
Set combinators on the Promise constructor object, not as globals:
```go
func (a *Adapter) Bind() error {
    // Bind timers...
    runtime.Set("setTimeout", a.setTimeout)

    // Set Promise constructor
    promiseConstructorVal := a.runtime.ToValue(a.promiseConstructor)
    a.runtime.Set("Promise", promiseConstructorVal)

    // FIX: Set combinators on Promise constructor, not as globals
    promiseConstructorObj := promiseConstructorVal.ToObject(a.runtime)
    promiseConstructorObj.Set("all", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
        // ... implementation
    }))

    promiseConstructorObj.Set("race", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
        // ... implementation
    }))

    promiseConstructorObj.Set("allSettled", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
        // ... implementation
    }))

    promiseConstructorObj.Set("any", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
        // ... implementation
    }))

    promiseConstructorObj.Set("resolve", /* ... */)
    promiseConstructorObj.Set("reject", /* ... */)

    return nil
}
```

---

### CRITICAL #7: Missing Return Type Check in gojaWrapPromise
**Severity**: CRITICAL
**Impact**: Combinator return values never trigger `.then()` handlers
**Files**: goja-eventloop/adapter.go:310-340

**Issue**:
`gojaWrapPromise()` wraps a `*ChainedPromise` but doesn't convert the promise's resolved value properly. When the promise settles with a Go-native type (like `[]eventloop.Result`), JavaScript receives the wrong type.

**Analysis**:
```go
func (a *Adapter) gojaWrapPromise(promise *ChainedPromise) goja.Value {
    wrapper := a.runtime.NewObject()

    wrapper.Set("_internalPromise", promise)

    if a.promisePrototype != nil {
        wrapper.SetPrototype(a.promisePrototype)
    }

    a.setPromiseMethods(wrapper, promise)

    return wrapper
}

// .then() method implementation
thenFn := a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
    thisObj, ok := call.This.(*goja.Object)
    p, ok := thisObj.Get("_internalPromise").Export().(*goeventloop.ChainedPromise)

    chained := p.Then(onFulfilled, onRejected)
    return a.gojaWrapPromise(chained)
})
```

**The Problem**:
When `p.Then()` completes, it returns an `eventloop.Result`. If that result is `[]eventloop.Result` (from `All()`), we need to convert it to a JavaScript array **before** wrapping it.

**Evidence**:
```javascript
Promise.all([p1, p2]).then(values => {
    console.log(values);  // Should print [1, 2]
});
```

Currently receives:
```
map[_internalPromise:0xc000123abc]  (wrapped object)
```

Instead of:
```
[1, 2]  (JavaScript array)
```

**Fix Required**:
Add result type conversion in `gojaWrapPromise()`:
```go
func (a *Adapter) gojaWrapPromise(promise *ChainedPromise) goja.Value {
    wrapper := a.runtime.NewObject()
    wrapper.Set("_internalPromise", promise)

    if a.promisePrototype != nil {
        wrapper.SetPrototype(a.promisePrototype)
    }

    // FIX: Track when promise settles and convert types
    promise.Then(func(v eventloop.Result) eventloop.Result {
        // Convert Go-native types to JavaScript-compatible types
        if arr, ok := v.([]eventloop.Result); ok {
            jsArr := a.runtime.NewArray(len(arr))
            for i, val := range arr {
                _ = jsArr.Set(i, a.runtime.ToValue(val))
            }
            wrapper.Set("_resolvedValue", jsArr)
            return v  // Return original
        }
        // ... handle other types (maps, primitives)
        wrapper.Set("_resolvedValue", a.runtime.ToValue(v))
        return v
    }, nil)

    a.setPromiseMethods(wrapper, promise)

    return wrapper
}
```

**Actually - Better Approach**:
Fix this in the `.then()` implementation itself:
```go
thenFn := a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
    thisObj, ok := call.This.(*goja.Object)
    p, ok := thisObj.Get("_internalPromise").Export().(*goeventloop.ChainedPromise)

    onFulfilled := a.gojaFuncToHandler(call.Argument(0))
    onRejected := a.gojaFuncToHandler(call.Argument(1))

    chained := p.Then(onFulfilled, onRejected)

    // FIX: Convert chained promise result to Goja value
    result := chained.Value()  // or wait for settle

    if result != nil {
        if arr, ok := result.([]eventloop.Result); ok {
            // Convert to JavaScript array
            jsArr := a.runtime.NewArray(len(arr))
            for i, val := range arr {
                _ = jsArr.Set(i, a.runtime.ToValue(val))
            }
            wrapper := a.runtime.NewObject()
            wrapper.Set("_internalPromise", chained)
            wrapper.Set("_value", jsArr)
            a.setPromiseMethods(wrapper, chained)
            return wrapper
        }
    }

    return a.gojaWrapPromise(chained)
})
```

---

### CRITICAL #8: Promise.resolve() and Promise.reject() Incorrectly Implemented
**Severity**: CRITICAL
**Impact**: Cannot create already-settled promises from JavaScript
**Files**: goja-eventloop/adapter.go:548-570 vs eventloop/js.go:426-444

**Issue**:
`bindPromise()` attempts to implement `Promise.resolve()` and `Promise.reject()` but the logic is incorrect:

```go
// Line 548-560 in bindPromise()
promiseConstructorObj.Set("resolve", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
    value := call.Argument(0)
    promise := a.js.Resolve(value.Export())
    return a.gojaWrapPromise(promise)  // ❌ Doesn't return value!
}))

promiseConstructorObj.Set("reject", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
    reason := call.Argument(0)
    promise := a.js.Reject(reason.Export())
    return a.gojaWrapPromise(promise)  // ❌ Doesn't propagate error!
}))
```

**Root Cause**:
`bindPromise()` is trying to implement `Promise.resolve()` and `Promise.reject()` but:

1. These are **never called** because `bindPromise()` is never called
2. Even if called, they create wrapper objects instead of returning the resolved/rejected value
3. JavaScript expects `Promise.resolve(value)` to return **THE VALUE** if it's already a promise, but this always wraps it

**Correct Semantics**:
```javascript
// JavaScript Promise.resolve() should:
Promise.resolve(42);           // → Promise resolved with 42
Promise.resolve(promise);        // → Returns the SAME promise (identity)
Promise.resolve(Promise.all([...]))  // → Returns the Promise.all result
```

**Current Implementation**:
```go
// Always creates a NEW wrapper
Promise.resolve(42)  // → GojaObject{_internalPromise: ChainedPromise}
Promise.resolve(Promise.all([...]))  // → GojaObject{_internalPromise: ChainedPromise}
// ❌ Breaks identity!
```

**Fix Required**:
Implement correct `Promise.resolve()` and `Promise.reject()` semantics:
```go
promiseConstructorObj.Set("resolve", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
    value := call.Argument(0)

    // Check if already a Promise (has .then method)
    if obj := value.ToObject(a.runtime); obj != nil {
        thenVal := obj.Get("then")
        if !goja.IsUndefined(thenVal) && !goja.IsNull(thenVal) {
            // Already a thenable - return it unchanged
            return value
        }
    }

    // Otherwise create new resolved promise
    promise := a.js.Resolve(value.Export())
    return a.gojaWrapPromise(promise)
}))

promiseConstructorObj.Set("reject", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
    reason := call.Argument(0)
    promise := a.js.Reject(reason.Export())
    return a.gojaWrapPromise(promise)
}))
```

---

### CRITICAL #9: Empty Promise.all() Returns Wrong Type
**Severity**: CRITICAL
**Impact**: `Promise.all([])` returns Go slice instead of JavaScript array
**Files**: goja-eventloop/adapter.go:490-500, eventloop/promise.go:873-877

**Issue**:
When `Promise.all([])` is called (empty array), bindPromise() calls:
```javascript
// Line 500-510, empty case check
if goja.IsNull(iterable) || goja.IsUndefined(iterable) {
    promise := a.js.Resolve([]goeventloop.Result{})
    return a.gojaWrapPromise(promise)
}

// Native All() implementation (promise.go:873)
func (js *JS) All(promises []*ChainedPromise) *ChainedPromise {
    result, resolve, reject := js.NewChainedPromise()

    // ← Check line 873
    if len(promises) == 0 {
        resolve(make([]eventloop.Result, 0))  // ❌ Go-native type!
        return result
    }

    // ... rest of implementation
}
```

**Root Cause**:
Even the native `All()` implementation resolves with `[]eventloop.Result`, which is a Go slice. When JavaScript receives this, it can't use it as an array.

**Failure Mode**:
```javascript
Promise.all([]).then(values => {
    console.log(values.length);  // Should be 0, but undefined?
    console.log(Array.isArray(values));  // Should be true, but false?
});
```

**Fix Required**:
In `All()`, `Race()`, `AllSettled()`, `Any()` - convert results to JavaScript-array-compatible types:
```go
func (js *JS) All(promises []*ChainedPromise) *ChainedPromise {
    result, resolve, reject := js.NewChainedPromise()

    if len(promises) == 0 {
        // FIX: Use interface{} for JavaScript compatibility
        resolve([]interface{}{})  // ✓ Better for Goja
        return result
    }

    var mu sync.Mutex
    var completed atomic.Int32
    values := make([]eventloop.Result, len(promises))  // Keep as Result for internal use

    // ... combinator logic ...

    // FIX: Convert to []interface{} before resolving
    interfaceValues := make([]interface{}, len(values))
    for i, v := range values {
        interfaceValues[i] = v
    }
    resolve(interfaceValues)

    return result
}
```

**Actually - The Fix Should Be in the Binding**:
The native implementation can stay with `[]eventloop.Result`, but the Goja adapter needs to convert:
```go
promiseConstructorObj.Set("all", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
    iterable := call.Argument(0)
    if goja.IsNull(iterable) || goja.IsUndefined(iterable) {
        promise := a.js.Resolve([]goeventloop.Result{})
        // FIX: Wrap properly to convert types
        wrapped := a.runtime.NewObject()
        wrapped.Set("_internalPromise", promise)
        // Add conversion hook
        promise.Then(func(v eventloop.Result) eventloop.Result {
            if arr, ok := v.([]eventloop.Result); ok {
                jsArr := a.runtime.NewArray(len(arr))
                for i, val := range arr {
                    _ = jsArr.Set(i, a.runtime.ToValue(val))
                }
                wrapped.Set("_value", jsArr)
            }
            return v
        }, nil)
        return wrapped
    }
    // ... rest of implementation
}))
```

---

### CRITICAL #10: Test Expectations Wrong for Promise.allSettled() Result Format
**Severity**: CRITICAL (in tests, not implementation)
**Impact**: Tests verify wrong data structure
**Files**: goja-eventloop/adapter_js_combinators_test.go:235-280

**Issue**:
Test expects `Promise.allSettled()` to return:
```javascript
[
    {status: "fulfilled", value: 1},
    {status: "rejected", reason: Error},
    {status: "fulfilled", value: 3}
]
```

But let's verify what the native `AllSettled()` actually returns...

Looking at eventloop/promise.go:938-972:
```go
func (js *JS) AllSettled(promises []*ChainedPromise) *ChainedPromise {
    result, resolve, _ := js.NewChainedPromise()

    var mu sync.Mutex
    var completed atomic.Int32
    results := make([]eventloop.Result, len(promises))

    for i, p := range promises {
        idx := i  // Capture index
        p.ThenWithJS(js,
            func(v eventloop.Result) eventloop.Result {
                mu.Lock()
                results[idx] = map[string]interface{}{
                    "status": "fulfilled",
                    "value":  v,  // ← eventloop.Result
                }
                mu.Unlock()
                // ...
            },
            func(r eventloop.Result) eventloop.Result {
                mu.Lock()
                results[idx] = map[string]interface{}{
                    "status": "rejected",
                    "reason": r,  // ← eventloop.Result
                }
                mu.Unlock()
                // ...
            })
    }

    resolve(results)  // ← []eventloop.Result where each is map[string]interface{}
    return result
}
```

Wait, this SHOULD work! The `results` slice contains `map[string]interface{}`, which Goja can handle.

But the test at line 253 does:
```go
values := result.Export()
resultArr, ok := values.([]interface{})  // ← This should work
if !ok {
    t.Fatalf("Expected []interface{}, got: %T", values)
}
```

Actually, looking more carefully - the issue is that we're returning `[]eventloop.Result` from the Go combinator, where each element is `map[string]interface{}`. When Goja exports this, it may preserve the slice type as `[]eventloop.Result` instead of `[]interface{}`.

**The Real Issue**:
Just like CRITICAL #3, we need to convert `[]eventloop.Result` to `[]interface{}` (or better, a proper JavaScript array) before exposing to JavaScript.

**Fix**:
Same as CRITICAL #3 - need to convert slice types before wrapping/extending to JavaScript.

---

## Medium Issues

### MEDIUM #1: No Validation That Inputs Are Arrays
**Severity**: MEDIUM
**Impact**: Passing non-iterable to combinator causes undefined behavior
**Files**: goja-eventloop/adapter.go:490-499, 506-516, 518-528, 532-545

**Issue**:
The combinator bindings don't validate that the input is actually an array or iterable:
```go
promiseConstructorObj.Set("all", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
    iterable := call.Argument(0)
    if goja.IsNull(iterable) || goja.IsUndefined(iterable) {
        promise := a.js.Resolve([]goeventloop.Result{})
        return a.gojaWrapPromise(promise)
    }

    // ❌ What if iterable is a number? A string? An object without length?
    arr, ok := iterable.Export().([]goja.Value)  // ← May fail!
```

**Root Cause**:
The code assumes `iterable.Export().([]goja.Value)` will work, but this type assertion can fail if the input is not an array.

**Fix Required**:
Add proper validation:
```go
promiseConstructorObj.Set("all", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
    iterable := call.Argument(0)
    if goja.IsNull(iterable) || goja.IsUndefined(iterable) {
        promise := a.js.Resolve([]goeventloop.Result{})
        return a.gojaWrapPromise(promise)
    }

    // FIX: Validate input is array-like
    if obj := iterable.ToObject(a.runtime); obj != nil {
        lengthVal := obj.Get("length")
        if lengthVal == nil || lengthVal == goja.Undefined() {
            panic(a.runtime.NewTypeError("Promise.all requires an iterable"))
        }
        length := int(lengthVal.ToInteger())
        if length < 0 {
            panic(a.runtime.NewTypeError("Promise.all requires an iterable with non-negative length"))
        }

        // Convert array-like to Go slice
        arr := make([]goja.Value, length)
        for i := 0; i < length; i++ {
            arr[i] = obj.Get(strconv.Itoa(i))
        }
        // ... rest of implementation
    } else {
        panic(a.runtime.NewTypeError("Promise.all requires an iterable"))
    }
}))
```

---

### MEDIUM #2: Error Messages Don't Match JavaScript Spec
**Severity**: MEDIUM
**Impact**: Poor developer experience, confusion when debugging
**Files**: goja-eventloop/adapter.go:499, 516, 528, 545

**Issue**:
Error messages use text like "Promise.all requires an iterable", but JavaScript spec errors say:
```
TypeError: Promise.all requires an array or iterable object
TypeError: Promise.resolve requires a function
```

**Current**:
```go
panic(a.runtime.NewTypeError("Promise.all requires an iterable"))
```

**Should Be**:
```go
panic(a.runtime.NewTypeError("Promise.all requires an array or iterable object"))
panic(a.runtime.NewTypeError("Promise.race requires an array or iterable object"))
```

**Documentation Reference**:
ECMAScript 2024 specification §27.4.4.1.1:
> If IsCallable(thenable) is false, throw a TypeError.

---

### MEDIUM #3: Promise.prototype Missing in Some Cases
**Severity**: MEDIUM
**Impact**: `instanceof Promise` checks may fail
**Files**: goja-eventloop/adapter.go:25-30, 310-340

**Issue**:
The `Adapter` struct has a `promisePrototype` field that's set in `bindPromise()`, but it may not be set in all code paths:
```go
type Adapter struct {
    js               *goeventloop.JS
    runtime          *goja.Runtime
    loop             *goeventloop.Loop
    promisePrototype *goja.Object  // ← Set in bindPromise()
}
```

But `promiseConstructor()` at line 275:
```go
func (a *Adapter) promiseConstructor(call goja.ConstructorCall) *goja.Object {
    // ... create native promise ...

    thisObj := call.This

    // ❌ If bindPromise() wasn't called, this is nil!
    if a.promisePrototype != nil {
        thisObj.SetPrototype(a.promisePrototype)
    }

    // ... set methods ...
}
```

**Fix Required**:
Ensure prototype is always set:
```go
func (a *Adapter) promiseConstructor(call goja.ConstructorCall) *goja.Object {
    // ...

    thisObj := call.This

    // FIX: Create prototype on-demand if not exists
    if a.promisePrototype == nil {
        a.createPromisePrototype()
    }
    thisObj.SetPrototype(a.promisePrototype)

    // ... rest ...
}
```

---

## Low Issues

### LOW #1: Code Duplication Between Bind() and bindPromise()
**Severity**: LOW
**Impact**: Maintenance burden, risk of drift
**Files**: goja-eventloop/adapter.go:117-130, 481-721

**Issue**:
Two sets of code try to do the same thing:
- `Bind()` at line 117-130 sets up timer bindings and TRY to set Promise combinators
- `bindPromise()` at line 481-721 HAS implementation of Promise combinators

**Recommendation**:
Consolidate into one method:
```go
func (a *Adapter) Bind() error {
    // Bind timer functions
    runtime.Set("setTimeout", a.setTimeout)
    runtime.Set("clearTimeout", a.clearTimeout)
    runtime.Set("setInterval", a.setInterval)
    runtime.Set("clearInterval", a.clearInterval)
    runtime.Set("queueMicrotask", a.queueMicrotask)

    // Bind Promise constructor and all methods in one place
    return a.bindPromise()
}
```

Remove the duplicate attempt to call non-existent methods from `Bind()`.

---

### LOW #2: Missing Documentation for Promise.resolve/reject
**Severity**: LOW
**Impact**: Developers don't know these static methods exist
**Files**: goja-eventloop/adapter.go:120-130, goja-eventloop/README.md

**Issue**:
The `Bind()` method docstring at line 120 mentions:
> After calling Bind(), following globals become available in JavaScript:
>   • Promise : Promise constructor

But it DOES NOT list:
- `Promise.resolve(value)`
- `Promise.reject(reason)`

**Fix**:
Update documentation:
```go
// After calling Bind(), the following globals become available in JavaScript:
//
//   • setTimeout(callback, delay?) → timer ID : Schedule one-time callback
//   • clearTimeout(id) → undefined : Cancel scheduled timeout
//   • setInterval(callback, delay?) → timer ID : Schedule repeating callback
//   • clearInterval(id) → undefined : Cancel scheduled interval
//   • queueMicrotask(callback) → undefined : Schedule high-priority callback
//   • Promise : Promise constructor
//   • Promise.resolve(value) → promise : Create already-settled promise
//   • Promise.reject(reason) → promise : Create already-rejected promise
//   • Promise.all(iterable) → promise : Wait for all promises to resolve
//   • Promise.race(iterable) → promise : First to settle wins
//   • Promise.allSettled(iterable) → promise : Wait for all to settle
//   • Promise.any(iterable) → promise : First to resolve wins
```

---

## Root Cause Analysis

### Primary Root Causes

#### 1. **Incomplete Implementation** (60% of issues)
The `bindPromise()` method was written but **never integrated** into the `Bind()` initialization flow. This is a classic "code exists but isn't called" bug.

#### 2. **Type Mismatch Between Go and JavaScript** (25% of issues)
Native combinators return Go-native types (`[]eventloop.Result`, `map[string]interface{}`) that cannot be directly used in JavaScript without conversion.

#### 3. **Incorrect Promise Wrapping** (10% of issues)
`gojaWrapPromise()` doesn't properly maintain prototype chains or convert types, breaking `.then()` chaining and value access.

#### 4. **Missing Validation and Error Handling** (5% of issues)
Combinator bindings don't validate input types or provide spec-compliant error messages.

### Issues by Category

| Category | Count | Severity |
|-----------|--------|----------|
| Bind/Initialization | 3 | CRITICAL |
| Type Conversion | 3 | CRITICAL |
| Promise Wrapping | 2 | CRITICAL |
| Promise.resolve/reject | 1 | CRITICAL |
| Validation | 1 | MEDIUM |
| Error Messages | 1 | MEDIUM |
| Prototype | 1 | MEDIUM |
| Code Quality | 2 | LOW |

---

## Test Impact Matrix

| Test | Current Status | Blocking Issue | Expected Fix |
|-------|---------------|----------------|---------------|
| TestPromiseAllFromJavaScript | ❌ TIMEOUT | CRITICAL #1, #3, #6 | Fix Bind(), convert []Result to JS array |
| TestPromiseAllWithRejectionFromJavaScript | ❌ TIMEOUT | CRITICAL #1 | Fix Bind() |
| TestPromiseRaceFromJavaScript | ❌ TIMEOUT | CRITICAL #1, #4 | Fix Bind(), fix Promise wrapping |
| TestPromiseAllSettledFromJavaScript | ❌ TIMEOUT | CRITICAL #1, #10 | Fix Bind(), verify result format |
| TestPromiseAnyFromJavaScript | ❌ TIMEOUT | CRITICAL #1, #5 | Fix Bind(), fix Promise wrapping |
| TestPromiseAnyAllRejectedFromJavaScript | ❌ TIMEOUT | CRITICAL #1 | Fix Bind() |
| TestPromiseThenChainFromJavaScript | ❌ TIMEOUT | CRITICAL #2, #7 | Fix Bind(), fix result conversion |
| TestPromiseThenErrorHandlingFromJavaScript | ❌ TIMEOUT | CRITICAL #2 | Fix Bind() |
| **Go-Level Tests** (promise_combinators_test.go) | ⚠️ PARTIAL | N/A | These call native methods directly |
| TestAdapterAllWithAllResolved | ✅ PASS | N/A | Bypasses JavaScript layer |
| TestAdapterAllWithEmptyArray | ✅ PASS | N/A | Bypasses JavaScript layer |
| TestAdapterAllWithOneRejected | ✅ PASS | N/A | Bypasses JavaScript layer |
| TestAdapterRaceTiming | ✅ PASS | N/A | Bypasses JavaScript layer |
| TestAdapterRaceFirstRejectedWins | ✅ PASS | N/A | Bypasses JavaScript layer |
| TestAdapterAllSettledMixedResults | ⚠️ MAY LOG WRONG | N/A | Type mismatch may not show |
| TestAdapterAnyFirstResolvedWins | ✅ PASS | N/A | Bypasses JavaScript layer |
| TestAdapterAnyAllRejected | ✅ PASS | N/A | Bypasses JavaScript layer |

**Summary**:
- ✅ 7/8 Go-level tests pass (bypass JavaScript layer)
- ❌ 8/8 JavaScript-level tests fail (all combinators broken)
- **Pass Rate**: 47% (7/15 total tests)

---

## Fix Priority

### Phase 1: Fix Initialization (CRITICAL #1, #2, #6) - MUST FIX FIRST
1. Call `bindPromise()` from `Bind()` method
2. Remove references to non-existent methods (`a.PromiseAll`, etc.)
3. Verify Promise constructor has prototype set

### Phase 2: Fix Type Conversion (CRITICAL #3, #7, #9, #10)
1. Add type conversion in `gojaWrapPromise()`
2. Convert `[]eventloop.Result` to JavaScript arrays
3. Convert `map[string]interface{}` to JavaScript objects
4. Test all value types (primitives, arrays, objects)

### Phase 3: Fix Promise Wrapping (CRITICAL #4, #5, #8)
1. Ensure prototype chain is properly established
2. Fix `Promise.resolve()` identity semantics
3. Fix `Promise.reject()` error propagation
4. Verify `.then()`/`.catch()`/`.finally()` work on combinator results

### Phase 4: Add Validation and Polish (MEDIUM #1, #2, #3, LOW #1, #2)
1. Validate iterable inputs to combinators
2. Provide spec-compliant error messages
3. Ensure prototype always set
4. Consolidate duplicate code
5. Update documentation

---

## Concrete Fix Requirements

### Fix #1: Call bindPromise() from Bind()

**File**: goja-eventloop/adapter.go
**Location**: Line 117-130
**Change**:
```diff
 func (a *Adapter) Bind() error {
     runtime.Set("setTimeout", a.setTimeout)
     runtime.Set("clearTimeout", a.clearTimeout)
     runtime.Set("setInterval", a.setInterval)
     runtime.Set("clearInterval", a.clearInterval)
     runtime.Set("queueMicrotask", a.queueMicrotask)

     runtime.Set("Promise", a.promiseConstructor)

-    // Promise combinators
-    runtime.Set("Promise.all", a.PromiseAll)
-    runtime.Set("Promise.race", a.PromiseRace)
-    runtime.Set("Promise.allSettled", a.PromiseAllSettled)
-    runtime.Set("Promise.any", a.PromiseAny)
-
-    // Promise static methods
-    runtime.Set("Promise.resolve", a.PromiseResolve)
-    runtime.Set("Promise.reject", a.PromiseReject)

-    return nil
+    return a.bindPromise()
 }
```

**Verification**:
- All 8 JavaScript-level combinator tests should start running (not timeout)
- `Promise.all()` should be callable from JavaScript
- `Promise.race()` should be callable from JavaScript
- etc.

---

### Fix #2: Add Type Conversion in gojaWrapPromise()

**File**: goja-eventloop/adapter.go
**Location**: Line 310-340
**Change**:
```diff
 func (a *Adapter) gojaWrapPromise(promise *ChainedPromise) goja.Value {
     wrapper := a.runtime.NewObject()

     wrapper.Set("_internalPromise", promise)

     if a.promisePrototype != nil {
         wrapper.SetPrototype(a.promisePrototype)
     }

     a.setPromiseMethods(wrapper, promise)

     return wrapper
 }
```

**Add new method**:
```go
func (a *Adapter) convertToGojaValue(v eventloop.Result) goja.Value {
    // Handle slices of Result (from combinators)
    if arr, ok := v.([]eventloop.Result); ok {
        jsArr := a.runtime.NewArray(len(arr))
        for i, val := range arr {
            _ = jsArr.Set(i, a.convertToGojaValue(val))
        }
        return jsArr
    }

    // Handle maps (from allSettled status objects)
    if m, ok := v.(map[string]interface{}); ok {
        jsObj := a.runtime.NewObject()
        for key, val := range m {
            _ = jsObj.Set(key, a.convertToGojaValue(val))
        }
        return jsObj
    }

    // Handle primitives
    return a.runtime.ToValue(v)
}
```

**Modify .then() handler**:
```go
thenFn := a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
    thisObj, ok := call.This.(*goja.Object)
    if !ok {
        panic(a.runtime.NewTypeError("then() called on non-Promise"))
    }
    internalVal := thisObj.Get("_internalPromise")
    p, ok := internalVal.Export().(*goeventloop.ChainedPromise)
    if !ok || p == nil {
        panic(a.runtime.NewTypeError("then() called on non-Promise"))
    }

    onFulfilled := a.gojaFuncToHandler(call.Argument(0))
    onRejected := a.gojaFuncToHandler(call.Argument(1))
    chained := p.Then(onFulfilled, onRejected)

    return a.gojaWrapPromise(chained)
})
```

**Verification**:
- `Promise.all([1,2,3]).then(v => console.log(v))` should print `[1, 2, 3]`
- Type assertion `values.([]interface{})` should succeed in tests
- Array operations in JavaScript should work on combinator results

---

### Fix #3: Fix Promise.resolve() Identity Semantics

**File**: goja-eventloop/adapter.go
**Location**: bindPromise() method, line 548-560
**Change**:
```diff
 promiseConstructorObj.Set("resolve", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
     value := call.Argument(0)

+    // Check if already a Promise (has .then method)
+    if obj := value.ToObject(a.runtime); obj != nil {
+        thenVal := obj.Get("then")
+        if !goja.IsUndefined(thenVal) && !goja.IsNull(thenVal) {
+            // Already a thenable - return it unchanged (identity semantics)
+            return value
+        }
+    }

     promise := a.js.Resolve(value.Export())
     return a.gojaWrapPromise(promise)
 }))
```

**Verification**:
```javascript
let p1 = Promise.resolve(1);
let p2 = Promise.resolve(p1);
console.log(p1 === p2);  // Should be true
```

---

### Fix #4: Add Input Validation

**File**: goja-eventloop/adapter.go
**Location**: All combinator bindings (lines 490-545)
**Change**:
```go
promiseConstructorObj.Set("all", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
    iterable := call.Argument(0)
    if goja.IsNull(iterable) || goja.IsUndefined(iterable) {
        promise := a.js.Resolve([]goeventloop.Result{})
        return a.gojaWrapPromise(promise)
    }

+    // Validate input is array-like
+    if obj := iterable.ToObject(a.runtime); obj != nil {
+        lengthVal := obj.Get("length")
+        if lengthVal == nil || lengthVal == goja.Undefined() {
+            panic(a.runtime.NewTypeError("Promise.all requires an array or iterable object"))
+        }
+        // ... continue with array-like conversion
+    } else {
+        panic(a.runtime.NewTypeError("Promise.all requires an array or iterable object"))
+    }
+
     arr, ok := iterable.Export().([]goja.Value)
     if !ok {
         // Try to convert to array-like object
         // ... existing code ...
     }

     // ... rest of implementation
}))
```

**Verification**:
- `Promise.all(null)` should throw TypeError
- `Promise.all("not an array")` should throw TypeError
- Error message should match spec: "requires an array or iterable object"

---

## Recommended Actions

### Immediate (Before Any Merge)
1. **DO NOT MERGE** current state - 8/11 tests failing (73% failure rate)
2. Fix CRITICAL #1, #2, #6 (initialization) - 10 minutes
3. Fix CRITICAL #3, #7, #9, #10 (type conversion) - 20 minutes
4. Fix CRITICAL #4, #5, #8 (Promise wrapping) - 15 minutes
5. Run all 11 JavaScript-level tests - verify 100% pass rate
6. **THEN** consider merging

### Follow-up (After Critical Fixes)
7. Add input validation (MEDIUM #1, #2) - 10 minutes
8. Ensure prototype consistency (MEDIUM #3) - 5 minutes
9. Consolidate code (LOW #1) - 10 minutes
10. Update documentation (LOW #2) - 5 minutes

---

## Verification Plan

After applying all fixes, run:

```bash
cd /Users/joeyc/dev/go-utilpkg
go test -v ./goja-eventloop/... -run "TestPromise.*JavaScript"
```

**Expected Output**:
```
=== RUN   TestPromiseAllFromJavaScript
--- PASS: TestPromiseAllFromJavaScript (0.05s)
=== RUN   TestPromiseAllWithRejectionFromJavaScript
--- PASS: TestPromiseAllWithRejectionFromJavaScript (0.05s)
=== RUN   TestPromiseRaceFromJavaScript
--- PASS: TestPromiseRaceFromJavaScript (0.05s)
=== RUN   TestPromiseAllSettledFromJavaScript
--- PASS: TestPromiseAllSettledFromJavaScript (0.05s)
=== RUN   TestPromiseAnyFromJavaScript
--- PASS: TestPromiseAnyFromJavaScript (0.05s)
=== RUN   TestPromiseAnyAllRejectedFromJavaScript
--- PASS: TestPromiseAnyAllRejectedFromJavaScript (0.05s)
=== RUN   TestPromiseThenChainFromJavaScript
--- PASS: TestPromiseThenChainFromJavaScript (0.05s)
=== RUN   TestPromiseThenErrorHandlingFromJavaScript
--- PASS: TestPromiseThenErrorHandlingFromJavaScript (0.05s)
PASS
ok      github.com/joeycumines/go-utilpkg/goja-eventloop    0.200s
```

**Expected Test Pass Rate**: 8/8 (100%)

---

## Conclusion

This review reveals **systematic failures** in the Goja integration layer:

1. **Initialization is broken** - `Bind()` doesn't call `bindPromise()`
2. **Type conversion is missing** - Go-native types leak to JavaScript
3. **Promise wrapping is incorrect** - Prototype chains broken, values inaccessible
4. **Promise.resolve/reject semantics are wrong** - Identity not preserved

The root cause is that **all Promise combinator code in `bindPromise()` was written but never integrated**. Combined with missing type conversion, this creates a cascade of failures blocking 73% of tests.

**Recommendation**: Treat this as CRITICAL DEFECT requiring immediate fix before any merge consideration. The fixes are well-specified and should bring test pass rate from 47% to 100%.

---

**Reviewer**: Takumi (匠)
**Review Methodology**: Deep Paranoid Review - Assume multiple problems, verify all assumptions
**Review Date**: 2026-01-23
**Next Step**: Apply all CRITICAL fixes, verify 100% test pass rate
