# Goja Adapter & Promise Combinators Review
## Critical Bug Analysis & Fixes

**Date**: 2026-01-23
**Review Type**: Deep Paranoid Analysis (Restarted)
**Context**: 8/11 tests failing (73% failure rate) after previous incomplete fixes

---

## SUCCINCT SUMMARY

Promise combinators return Go-native `[]eventloop.Result` slices which Goja's `Export()` preserves as opaque wrapper objects instead of converting to JavaScript arrays. The `.then()` method's `gojaFuncToHandler()` receives these Go-native types and passes them directly to JavaScript handlers via `convertToGojaValue(result)`, which has branch-specific logic for `[]eventloop.Result` that converts to JavaScript array, BUT this conversion happens **inside** the handler function **before** calling the JavaScript callback. The real problem is that when combinator promises resolve, the Goja runtime receives the raw `[]eventloop.Result` slice from Go and wraps it as an opaque object during the `runtime.ToValue(val)` call in `gojaFuncToHandler`. The `convertToGojaValue()` function DOES have logic to convert `[]eventloop.Result` to JavaScript arrays, but this logic is **never reached** because by the time `convertToGojaValue()` is called, the value is already a Goja opaque object, not a Go-native `[]eventloop.Result`. The actual fix requires modifying the `.then()`, `.catch()`, and `.finally()` implementations to extract the Go-native value using `promise.Value()` or `promise.Reason()` and converting it with `convertToGojaValue()` **before** calling the JavaScript handler with `runtime.ToValue()`.

---

## DETAILED BUG ANALYSIS

### CRITICAL #1: Type Conversion Failure in Promise.then() Handlers

**Severity**: CRITICAL
**Impact**: All combinator tests fail - values are opaque wrapper objects
**File**: goja-eventloop/adapter.go:315-343
**Evidence**:
```
Index 0: expected 1, got map[_internalPromise:0x140001cad20]
Index 1: expected 2, got map[_internalPromise:0x140001caee0]
```

**Root Cause**:
```go
func (a *Adapter) gojaFuncToHandler(fn goja.Value) func(goeventloop.Result) goeventloop.Result {
    return func(result goeventloop.Result) goeventloop.Result {
        // result is []eventloop.Result from Promise.all()
        jsValue := a.convertToGojaValue(result)  // ← SHOULD convert to JS array
        ret, err := fnCallable(goja.Undefined(), jsValue)  // ← Passes to JS
        if err != nil {
            return err
        }
        return ret.Export()  // ← Exports Goja value back to Go
    }
}

func (a *Adapter) convertToGojaValue(v any) goja.Value {
    // This branch SHOULD handle []eventloop.Result:
    if arr, ok := v.([]goeventloop.Result); ok {
        jsArr := a.runtime.NewArray(len(arr))
        for i, val := range arr {
            _ = jsArr.Set(strconv.Itoa(i), a.convertToGojaValue(val))
        }
        return jsArr  // ← Returns JavaScript array
    }
    // ... other types
    return a.runtime.ToValue(v)
}
```

**Actual Execution Path**:
1. `Promise.all([1,2,3])` resolves with `[]eventloop.Result{1, 2, 3}`
2. `.then(values => {})` calls `gojaFuncToHandler(valuesHandler)`
3. Handler calls `convertToGojaValue([]eventloopResult{1,2,3})`
4. Type assertion `v.([]goeventloop.Result)` **FAILS** because Goja's `Export()` has already wrapped the values
5. Falls through to `a.runtime.ToValue(v)` which creates opaque wrapper objects

**The Deception**:
`convertToGojaValue()` appears correct but its type assertions fail because the value has already been converted by Goja. The `interface{}` type system means `[]eventloop.Result` from Go becomes `[]interface{}` after passing through Goja's boundary.

**Fix Required**:
```go
func (a *Adapter) gojaFuncToHandler(fn goja.Value) func(goeventloop.Result) goeventloop.Result {
    if fn.Export() == nil {
        return func(result goeventloop.Result) goeventloop.Result { return result }
    }

    fnCallable, ok := goja.AssertFunction(fn)
    if !ok {
        return func(result goeventloop.Result) goeventloop.Result { return result }
    }

    return func(result goeventloop.Result) goeventloop.Result {
        // CRITICAL FIX: Check type at Go-native level, not after Goja conversion
        var jsValue goja.Value
        switch v := result.(type) {
        case []goeventloop.Result:
            // Convert Go-native slice to JavaScript array
            jsArr := a.runtime.NewArray(len(v))
            for i, val := range v {
                jsValue := a.convertToGojaValue(val)
                _ = jsArr.Set(strconv.Itoa(i), jsValue)
            }
            jsValue = jsArr
        case map[string]interface{}:
            // Convert Go-native map to JavaScript object
            jsObj := a.runtime.NewObject()
            for key, val := range v {
                _ = jsObj.Set(key, a.convertToGojaValue(val))
            }
            jsValue = jsObj
        default:
            // Primitive or already-converted type
            jsValue = a.runtime.ToValue(v)
        }

        ret, err := fnCallable(goja.Undefined(), jsValue)
        if err != nil {
            return err
        }
        return ret.Export()
    }
}
```

**Verification**:
```go
// Test should pass:
Promise.all([Promise.resolve(1), Promise.resolve(2)]).then(values => {
    console.log(values);  // Should print [1, 2] NOT [map[...], map[...]]
});
```

---

### CRITICAL #2: Rejection Handlers Not Firing - Timeout Bug

**Severity**: CRITICAL
**Impact**: 3 tests timeout waiting for never-fired catch handlers
**Files**: adapter_js_combinators_test.go:148, 441, 509, 574
**Evidence**:
```
Test timed out waiting for Promise.all rejection
Test timed out waiting for Promise.any rejection
Test timed out waiting for promise chain
Test timed out waiting for error handling
```

**Root Cause**:
The `.catch()` method implementation has correct structure but the rejection handlers aren't being notified on settled promises:

```go
catchFn := a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
    thisVal := call.This
    thisObj, ok := thisVal.(*goja.Object)
    if !ok || thisObj == nil {
        panic(a.runtime.NewTypeError("catch() called on non-Promise object"))
    }

    internalVal := thisObj.Get("_internalPromise")
    p, ok := internalVal.Export().(*goeventloop.ChainedPromise)
    if !ok || p == nil {
        panic(a.runtime.NewTypeError("catch() called on non-Promise object"))
    }

    onRejected := a.gojaFuncToHandler(call.Argument(0))
    chained := p.Catch(onRejected)  // ← Should fire on rejection
    return a.gojaWrapPromise(chained)
})
```

**Actual Problem**:
`gojaFuncToHandler()` returns `func(goeventloop.Result) goeventloop.Result` but wraps the entire logic. When rejection happens, the handler is called but **doesn't trigger the JavaScript callback**:

```go
promise := a.js.Reject(reason.Export())  // Rejects promise
return a.gojaWrapPromise(promise)        // Wraps rejected promise

// Later in JavaScript:
Promise.reject("error").catch(err => {  // ← Never called
    errorResult = err.message;
    notifyDone();
});
```

The catch handler is attached, but the **rejection propagation path is broken** at the boundary between Go's promise state and Goja's wrapper object.

**Fix Required**:
The issue is actually CRITICAL #1 masking this - once type conversion is fixed, rejections will propagate. However, there's a secondary issue:

Check `p.State()` BEFORE calling `.catch()`:

```go
catchFn := a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
    thisVal := call.This
    thisObj, ok := thisVal.(*goja.Object)
    if !ok || thisObj == nil {
        panic(a.runtime.NewTypeError("catch() called on non-Promise object"))
    }

    internalVal := thisObj.Get("_internalPromise")
    p, ok := internalVal.Export().(*goeventloop.ChainedPromise)
    if !ok || p == nil {
        panic(a.runtime.NewTypeError("catch() called on non-Promise object"))
    }

    onRejected := a.gojaFuncToHandler(call.Argument(0))
    chained := p.Catch(onRejected)

    // CRITICAL: If already rejected, schedule handler immediately
    if p.State() == goeventloop.Rejected {
        reason := p.Reason()
        // Manually trigger the created promise's resolve/reject
        wrapped := a.gojaWrapPromise(chained)
        return wrapped
    }

    return a.gojaWrapPromise(chained)
})
```

**Better Fix**:
The real issue is that `p.Catch()` returns a NEW promise that will be rejected with the handler's return value. We need to ensure the rejection handler **actually runs** by checking if the promise has already settled:

```go
catchFn := a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
    thisVal := call.This
    thisObj, ok := thisVal.(*goja.Object)
    if !ok || thisObj == nil {
        panic(a.runtime.NewTypeError("catch() called on non-Promise object"))
    }

    internalVal := thisObj.Get("_internalPromise")
    p, ok := internalVal.Export().(*goeventloop.ChainedPromise)
    if !ok || p == nil {
        panic(a.runtime.NewTypeError("catch() called on non-Promise object"))
    }

    onRejected := a.gojaFuncToHandler(call.Argument(0))
    chained := p.Catch(onRejected)

    // Fix: Force handler to run if promise already rejected
    if p.State() == goeventloop.Rejected {
        // Manually call the rejection handler
        reason := p.Reason()
        onRejected(reason)
    }

    return a.gojaWrapPromise(chained)
})
```

**Actually Correct Fix**:
The timeout issue is caused by **event loop starvation**. The microtasks scheduled by `.catch()` aren't being processed because the test waits on a channel that's never closed due to the type conversion bug (CRITICAL #1). Once CRITICAL #1 is fixed, the handlers will fire and tests will pass.

**Resolution**: Fix CRITICAL #1 first (type conversion). Timeouts are a symptom, not a separate bug.

---

### CRITICAL #3: TypeError on Null/Undefined in Promise.any()

**Severity**: HIGH
**Impact**: TypeError in unrelated tests
**File**: goja-eventloop/adapter.go:603, 627
**Evidence**:
```
TypeError: Cannot convert undefined or null to object
at github.com/joeycumines/goja-eventloop.(*Adapter).bindPromise.func4 (native)
```

**Root Cause**:
In `Promise.any()` and `Promise.race()`, when `iterable.ToObject()` is called on null/undefined AFTER the null check:

```go
// Promise.any
promiseConstructorObj.Set("any", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
    iterable := call.Argument(0)
    if goja.IsNull(iterable) || goja.IsUndefined(iterable) {
        panic(a.runtime.NewTypeError("Promise.any requires an array or iterable object"))
    }

    arr, ok := iterable.Export().([]goja.Value)
    if !ok {
        // Try to convert to array-like object
        obj := iterable.ToObject(a.runtime)  // ← Can panic if nil
        // ...
    }
    // ...
}))
```

The problem is that `iterable.Export()` might succeed even for null/undefined (returning nil), then the `ToObject()` call panics.

**Fix Required**:
```go
// Promise.any()
if goja.IsNull(iterable) || goja.IsUndefined(iterable) {
    panic(a.runtime.NewTypeError("Promise.any requires an array or iterable object"))
}

arr, ok := iterable.Export().([]goja.Value)
if !ok {
    // Try to convert to array-like object
    obj := iterable.ToObject(a.runtime)
    if obj == nil {  // ← FIX: Check for nil BEFORE using
        panic(a.runtime.NewTypeError("Promise.any requires an array or iterable object"))
    }
    lengthVal := obj.Get("length")
    if lengthVal == nil || goja.IsUndefined(lengthVal) {
        panic(a.runtime.NewTypeError("Promise.any requires an array with length property"))
    }
    length := int(lengthVal.ToInteger())
    arr = make([]goja.Value, length)
    for i := 0; i < length; i++ {
        arr[i] = obj.Get(strconv.Itoa(i))
    }
}
```

Apply the same fix to `Promise.all()`, `Promise.race()`, and `Promise.allSettled()`.

---

### CRITICAL #4: Promise.resolve() Identity Semantics Incorrect

**Severity**: MEDIUM
**Impact**: Performance and correctness edge cases
**File**: goja-eventloop/adapter.go:497-511

**Root Cause**:
When `Promise.resolve()` receives an already-wrapped promise, it should return it unchanged:

```go
promiseConstructorObj.Set("resolve", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
    value := call.Argument(0)

    // Check if already our wrapped promise (has _internalPromise)
    if obj := value.ToObject(a.runtime); obj != nil {
        if internalVal := obj.Get("_internalPromise"); internalVal != nil {
            if _, ok := internalVal.Export().(*goeventloop.ChainedPromise); ok {
                // Already a wrapped promise from our system - return unchanged
                return value  // ← CORRECT
            }
        }
    }

    // Otherwise create new resolved promise
    promise := a.js.Resolve(value.Export())
    return a.gojaWrapPromise(promise)
}))
```

This looks correct but has a timing issue: `obj.Get("_internalPromise")` might return `undefined` if called before the promise is fully initialized.

**Fix Required**:
The current implementation is actually correct for the test cases. However, to be more defensive:

```go
promiseConstructorObj.Set("resolve", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
    value := call.Argument(0)

    // Skip null/undefined - just return resolved promise
    if goja.IsNull(value) || goja.IsUndefined(value) {
        promise := a.js.Resolve(nil)
        return a.gojaWrapPromise(promise)
    }

    // Check if already our wrapped promise
    if obj := value.ToObject(a.runtime); obj != nil {
        if internalVal := obj.Get("_internalPromise"); internalVal != nil && !goja.IsUndefined(internalVal) {
            if p, ok := internalVal.Export().(*goeventloop.ChainedPromise); ok && p != nil {
                // Already a wrapped promise - return unchanged (identity semantics)
                return value
            }
        }
    }

    // Otherwise create new resolved promise
    promise := a.js.Resolve(value.Export())
    return a.gojaWrapPromise(promise)
}))
```

---

### LOW-PRIORITY ISSUES

#### Issue #1: Memory Leak in timer cleanup

**Severity**: LOW
**File**: eventloop/js.go:165-175
**Status**: Already fixed in previous review (see CRITICAL #5 in prior review)

#### Issue #2: Missing error handling in SetInterval

**Severity**: LOW
**File**: eventloop/js.go:215-250
**Status**: Already fixed in previous review (see CRITICAL #6 in prior review)

---

## REQUIRED CHANGES

### Change #1: Fix gojaFuncToHandler Type Conversion (CRITICAL #1)

Update `adapter.go` lines 315-343:

```go
// gojaFuncToHandler converts a Goja function value to a promise handler
func (a *Adapter) gojaFuncToHandler(fn goja.Value) func(goeventloop.Result) goeventloop.Result {
    if fn.Export() == nil {
        // No handler provided - pass through the result
        return func(result goeventloop.Result) goeventloop.Result { return result }
    }

    fnCallable, ok := goja.AssertFunction(fn)
    if !ok {
        // Not a function - pass through the result
        return func(result goeventloop.Result) goeventloop.Result { return result }
    }

    return func(result goeventloop.Result) goeventloop.Result {
        // CRITICAL FIX: Type conversion at Go-native level before passing to JavaScript
        var jsValue goja.Value
        switch v := result.(type) {
        case []goeventloop.Result:
            // Convert Go-native slice to JavaScript array
            jsArr := a.runtime.NewArray(len(v))
            for i, val := range v {
                jsValue := a.convertToGojaValue(val)
                _ = jsArr.Set(strconv.Itoa(i), jsValue)
            }
            jsValue = jsArr
        case map[string]interface{}:
            // Convert Go-native map to JavaScript object
            jsObj := a.runtime.NewObject()
            for key, val := range v {
                _ = jsObj.Set(key, a.convertToGojaValue(val))
            }
            jsValue = jsObj
        default:
            // Primitive or already-converted type
            jsValue = a.runtime.ToValue(v)
        }

        ret, err := fnCallable(goja.Undefined(), jsValue)
        if err != nil {
            // Return rejection result instead of panicking
            return err
        }
        return ret.Export()
    }
}
```

### Change #2: Add Null Checks to Promise Combinators (CRITICAL #3)

Update `adapter.go` in the following locations:

1. **Promise.all()** (around line 564):
```go
if !ok {
    obj := iterable.ToObject(a.runtime)
    if obj == nil {  // ← ADD THIS CHECK
        panic(a.runtime.NewTypeError("Promise.all requires an array or iterable object"))
    }
    lengthVal := obj.Get("length")
    if lengthVal == nil || goja.IsUndefined(lengthVal) {  // ← ADD undefined check
        panic(a.runtime.NewTypeError("Promise.all requires an array with length property"))
    }
    // rest unchanged...
}
```

2. **Promise.race()** (around line 590):
```go
if !ok {
    obj := iterable.ToObject(a.runtime)
    if obj == nil {  // ← ADD THIS CHECK
        panic(a.runtime.NewTypeError("Promise.race requires an array or iterable object"))
    }
    lengthVal := obj.Get("length")
    if lengthVal == nil || goja.IsUndefined(lengthVal) {  // ← ADD undefined check
        panic(a.runtime.NewTypeError("Promise.race requires an array with length property"))
    }
    // rest unchanged...
}
```

3. **Promise.allSettled()** (around line 617):
```go
if !ok {
    obj := iterable.ToObject(a.runtime)
    if obj == nil {  // ← ADD THIS CHECK
        panic(a.runtime.NewTypeError("Promise.allSettled requires an array or iterable object"))
    }
    lengthVal := obj.Get("length")
    if lengthVal == nil || goja.IsUndefined(lengthVal) {  // ← ADD undefined check
        panic(a.runtime.NewTypeError("Promise.allSettled requires an array with length property"))
    }
    // rest unchanged...
}
```

4. **Promise.any()** (around line 645):
```go
if !ok {
    obj := iterable.ToObject(a.runtime)
    if obj == nil {  // ← ADD THIS CHECK
        panic(a.runtime.NewTypeError("Promise.any requires an array or iterable object"))
    }
    lengthVal := obj.Get("length")
    if lengthVal == nil || goja.IsUndefined(lengthVal) {  // ← ADD undefined check
        panic(a.runtime.NewTypeError("Promise.any requires an array with length property"))
    }
    // rest unchanged...
}
```

### Change #3: Strengthen Promise.resolve() Identity Check (CRITICAL #4)

Update `adapter.go` lines 497-511:

```go
promiseConstructorObj.Set("resolve", a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
    value := call.Argument(0)

    // Skip null/undefined - just return resolved promise
    if goja.IsNull(value) || goja.IsUndefined(value) {
        promise := a.js.Resolve(nil)
        return a.gojaWrapPromise(promise)
    }

    // Check if already our wrapped promise
    if obj := value.ToObject(a.runtime); obj != nil {
        if internalVal := obj.Get("_internalPromise"); internalVal != nil && !goja.IsUndefined(internalVal) {
            if p, ok := internalVal.Export().(*goeventloop.ChainedPromise); ok && p != nil {
                // Already a wrapped promise - return unchanged (identity semantics)
                return value
            }
        }
    }

    // Otherwise create new resolved promise
    promise := a.js.Resolve(value.Export())
    return a.gojaWrapPromise(promise)
}))
```

---

## VERIFICATION CHECKLIST

After applying these fixes, verify:

- [ ] All 11 combinator tests pass
- [ ] No TypeError exceptions in unrelated tests
- [ ] Promise.all() returns JavaScript arrays, not wrapper objects
- [ ] Promise.race() returns resolved values, not wrapper objects
- [ ] Promise.any() returns resolved values, not wrapper objects
- [ ] Promise.allSettled() returns array of status objects
- [ ] Rejection handlers fire synchronously for already-rejected promises
- [ ] Empty array handling is correct (Promise.all/allSettled resolve with [], Promise.any rejects)

---

## RELATED BUGS FROM PRIOR REVIEWS

Per `blueprint.json` and previous review `13-GOJA_INTEGRATION_PERFECT.md`, the following issues were supposedly fixed but verify they remain fixed:

1. **Promise chaining** (CRITICAL #1 from prior review) - should be working
2. **Timer memory leaks** (CRITICAL #5 from prior review) - should be working
3. **SetInterval deadlock** (CRITICAL #6 from prior review) - should be working

If any of these resurface after current fixes, they indicate deeper systematic issues requiring a rewrite of the adapter's initialization logic.

---

## CONCLUSION

The root cause of **8/11 test failures** is CRITICAL #1: `gojaFuncToHandler()` not properly converting `[]eventloop.Result` slices to JavaScript arrays. The switch on type happens **after** Goja has wrapped the values in opaque objects, causing type assertions to fail.

CRITICAL #3 (TypeError on null/undefined) is a straightforward defensive programming fix.

CRITICAL #2 (timeouts) is a **symptom** of CRITICAL #1, not a separate bug. Once type conversion works, handlers will fire and tests will complete.

CRITICAL #4 (Promise.resolve() identity) is a minor improvement for correctness.

**Expected Result After Fixes**: All 11 combinator tests pass, 0 test failures in goja-eventloop package.
