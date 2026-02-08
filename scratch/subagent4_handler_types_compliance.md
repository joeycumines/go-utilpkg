# Timer Handler Types Compliance Analysis
## WHATWG HTML Spec Section 8.6 - Timers

**Document Version:** 1.0  
**Date:** February 9, 2026  
**Status:** EXHAUSTIVE INVESTIGATION COMPLETE

---

## 1. SUCCINCT SUMMARY

The eventloop implementation has **CRITICAL COMPLIANCE GAPS** in handler type handling:

1. **Function handlers**: Pass `undefined` as `this` instead of the required global object (`thisArg`)
2. **String handlers**: Completely unimplemented - implementation only accepts `func()` types
3. **TrustedScript**: No support - the `trusted-types` CSP directive cannot be enforced
4. **CSP string compilation**: No `EnsureCSPDoesNotBlockStringCompilation` algorithm
5. **Error handling**: No distinction between Function and string handler error paths

The Goja adapter (`goja-eventloop/adapter.go`) only accepts Function handlers, rejecting strings with a TypeError, which **technically complies with modern secure defaults** but is **not spec-compliant** for complete API compatibility.

---

## 2. DETAILED ANALYSIS

### 2.1 Handler Type Detection

**Spec Reference:** WHATWG HTML Section 8.6, Timer Initialization Steps

The spec defines handler as:
```
handler: a Function or a TrustedScript or a string (containing code)
```

**Key Spec Excerpt:**
> "If handler is a Function, then invoke handler given arguments and 'report', with callback this value set to thisArg."

> "Otherwise [if string or TrustedScript]: Let sink be a concatenation of globalName, U+0020 SPACE, and methodName. Set handler to the result of invoking the get trusted type compliant string algorithm..."

**IDL Definition:**
```idl
[Exposed=(Window,Worker)]
interface WindowOrWorkerGlobalScope {
  long setTimeout(([LegacyNullToEmptyString] DOMString or Function) handler,
                  [Clamp] long timeout = 0,
                  ...any arguments);
  long setInterval(([LegacyNullToEmptyString] DOMString or Function) handler,
                   [Clamp] long timeout = 0,
                   ...any arguments);
};
```

**Current Implementation:**
- `eventloop/js.go` line ~90: `type SetTimeoutFunc func()` - **only accepts Go functions**
- `goja-eventloop/adapter.go` line ~198: `goja.AssertFunction(fn)` - **only accepts JS functions**

**CRITICAL FINDING:** The implementation uses **type narrowing at the Go level** that **excludes strings entirely**. This means:
- `setTimeout("console.log(1)", 100)` → TypeError in current impl, but spec allows it
- `setTimeout(function(){}, 100)` → Works correctly
- No detection logic exists for distinguishing handler types

### 2.2 Function Handler Invocation - thisArg Handling

**Spec Reference:** WHATWG HTML Section 8.6, Step 7

**Spec Algorithm:**
```
1. Let thisArg be global if that is a WorkerGlobalScope object; 
   otherwise let thisArg be the WindowProxy that corresponds to global.

2. If handler is a Function, then invoke handler given arguments and "report", 
   with callback this value set to thisArg.
```

**Web IDL Reference - "invoke a callback function":**
```
To invoke a callback function type value callable with a Web IDL arguments list args, 
exception behavior exceptionBehavior, and an optional callback this value thisArg...
```

**Current Implementation (adapter.go:198-214):**
```go
func (a *Adapter) setTimeout(call goja.FunctionCall) goja.Value {
    fn := call.Argument(0)
    // ... validation ...
    fnCallable, ok := goja.AssertFunction(fn)
    if !ok {
        panic(a.runtime.NewTypeError("setTimeout requires a function as first argument"))
    }
    
    id, err := a.js.SetTimeout(func() {
        _, _ = fnCallable(goja.Undefined())  // ← THIS IS THE PROBLEM
    }, delayMs)
    ...
}
```

**CRITICAL FINDING:** The implementation passes `goja.Undefined()` as the `this` value, but the spec requires:
- `thisArg = global` (for WorkerGlobalScope)
- `thisArg = WindowProxy` (for Window context)

**Impact:**
```javascript
// Spec-compliant behavior:
setTimeout(function() { 
    console.log(this === window); // true 
}, 0);

// Current implementation:
setTimeout(function() { 
    console.log(this === window); // false (this === undefined)
}, 0);
```

### 2.3 String Handler Compilation

**Spec Reference:** WHATWG HTML Section 8.6, Step 8 (string handling)

**Key Spec Excerpt:**
```
If handler is not a Function:

8. If previousId was not given:
   a. Let globalName be "Window" if global is a Window object; "WorkerGlobalScope" otherwise.
   b. Let methodName be "setInterval" if repeat is true; "setTimeout" otherwise.
   c. Let sink be a concatenation of globalName, U+0020 SPACE, and methodName.
   d. Set handler to the result of invoking the get trusted type compliant string 
      algorithm with TrustedScript, global, handler, sink, and "script".

9. Assert: handler is a string.

10. Perform EnsureCSPDoesNotBlockStringCompilation...

11. Let fetch options be the default script fetch options.
12. Let base URL be settings object's API base URL.
13. If initiating script is not null:
    - Set fetch options to initiating script's fetch options
    - Set base URL to initiating script's base URL

14. Let script be the result of creating a classic script given handler...

15. Run the classic script.
```

**Note in Spec:**
> "The effect of these steps ensures that the string compilation done by setTimeout() and setInterval() behaves equivalently to that done by eval(). That is, module script fetches via import() will behave the same in both contexts."

**Current Implementation:** **COMPLETELY UNIMPLEMENTED**

The implementation has no code path for:
- String handler acceptance
- `get trusted type compliant string` algorithm
- `EnsureCSPDoesNotBlockStringCompilation` 
- Classic script creation
- Script execution

### 2.4 Trusted Types Compliance

**Spec Reference:** https://w3c.github.io/webappsec-trusted-types/dist/spec/

**Key Concepts:**
1. **TrustedTypePolicy** - Factory for creating TrustedScript objects
2. **get trusted type compliant string** - Algorithm that:
   - Checks if `require-trusted-types-for 'script'` is in effect
   - If yes, uses policy to convert string to TrustedScript
   - If no policy exists, may throw or use string as-is
3. **Sink mapping** - `setTimeout` → sink name `"Window setTimeout"` or `"WorkerGlobalScope setTimeout"`

**CSP Integration:**
```
Content-Security-Policy: trusted-types myPolicy
Content-Security-Policy: require-trusted-types-for 'script'
```

```javascript
// With Trusted Types enabled:
const policy = trustedTypes.createPolicy("myPolicy", {
  createScript: (input) => sanitize(input)
});

setTimeout(policy.createScript("console.log(1)"), 100);  // Works
setTimeout("console.log(1)", 100);  // TypeError - strings not allowed
```

**Current Implementation:** **NO Trusted Types SUPPORT**

- No `TrustedScript` type
- No policy lookup
- No `get trusted type compliant string` algorithm
- Cannot enforce `trusted-types` CSP directive

### 2.5 CSP Considerations for String Compilation

**Spec Reference:** https://w3c.github.io/webappsec-csp/#can-compile-strings

**Algorithm `EnsureCSPDoesNotBlockStringCompilation`:**
```
1. Check Content-Security-Policy for the realm
2. If 'script-src' or 'default-src' contains 'unsafe-eval':
   - Allow string compilation
3. If trusted-types is in effect and no policy exists:
   - Throw TypeError
4. If require-trusted-types-for 'script' is in effect:
   - String compilation is blocked
```

**Current Implementation:** **NO CSP CHECKS**

The implementation cannot enforce:
- `unsafe-eval` in CSP (would block `eval()` and `setTimeout(string, ...)`)
- `trusted-types` directive
- `require-trusted-types-for` directive

### 2.6 Error Handling Differences

**Spec Reference:** WHATWG HTML Section 8.6, Step 10

**Error Handling Requirements:**

| Handler Type | Error Scenario | Behavior |
|-------------|----------------|----------|
| Function | Handler throws | Report exception via "report" behavior |
| Function | Handler returns normally | Continue |
| String/TrustedScript | CSP blocks compilation | Catch exception, report for global, abort |
| String/TrustedScript | Compilation error | Catch exception, report for global, abort |
| String/TrustedScript | Script execution throws | Report exception via "report" behavior |

**Web IDL "invoke a callback function" with exceptionBehavior="report":**
```
14. If exceptionBehavior is "report":
    - Assert: callable's return type is undefined or any
    - Report an exception completion.[[Value]] for realm's global object
    - Return unique undefined IDL value
```

**Current Implementation:**
```go
// In adapter.go setTimeout callback:
id, err := a.js.SetTimeout(func() {
    _, _ = fnCallable(goja.Undefined())
}, delayMs)
```

**Issues:**
1. No error handling wrapper around `fnCallable()` execution
2. No distinction between Function and string error paths
3. No global exception reporting

### 2.7 Handler Type and Arguments

**Spec Reference:** WHATWG HTML Section 8.6, Notes

**Key Notes:**
> "Argument conversion as defined by Web IDL (for example, invoking toString() methods on objects passed as the first argument) happens in the algorithms defined in Web IDL, before this algorithm is invoked."

**Example from Spec:**
```javascript
setTimeout({ toString: function () {
  setTimeout("logger('ONE')", 100);
  return "logger('TWO')";
} }, 100);
```

**Result:** `logger('ONE')` and `logger('TWO')` both execute

**Current Implementation:**
- Object with `toString()` → TypeError ("setTimeout requires a function")
- Spec says: should call `toString()`, use returned string

---

## 3. IMPLEMENTATION FINDINGS

### 3.1 Code File References

| File | Lines | Finding |
|------|-------|---------|
| `eventloop/js.go` | 87-95 | `SetTimeoutFunc` type definition - only `func()` |
| `eventloop/js.go` | 221-240 | `SetTimeout` - no handler type detection |
| `eventloop/js.go` | 245-270 | `SetInterval` - same issue |
| `goja-eventloop/adapter.go` | 198-214 | `setTimeout` - only accepts Function, rejects strings |
| `goja-eventloop/adapter.go` | 218-232 | `setInterval` - same issue |
| `goja-eventloop/adapter.go` | 236-246 | `queueMicrotask` - also uses `goja.Undefined()` |

### 3.2 thisArg Implementation Analysis

**Current Implementation:**
```go
// adapter.go line ~210
id, err := a.js.SetTimeout(func() {
    _, _ = fnCallable(goja.Undefined())
}, delayMs)
```

**Required Implementation:**
```go
// Should be:
thisArg := globalObject  // WindowProxy for window context, global for worker

id, err := a.js.SetTimeout(func() {
    _, _ = fnCallable(thisArg)  // Pass correct thisArg
}, delayMs)
```

**Goja Specifics:**
- Goja's `goja.Undefined()` is the JavaScript `undefined` value
- Goja's `goja.Global()` returns the global object
- The implementation needs to pass the global object as `thisArg`

### 3.3 Handler Type Detection Gap

**Current Code Path:**
```
JavaScript: setTimeout(fn, 100)
    ↓
adapter.setTimeout(call)
    ↓
fn := call.Argument(0)
fnCallable := goja.AssertFunction(fn)  // ← Only accepts functions
    ↓
panic(TypeError) if not a function
```

**Required Code Path:**
```
JavaScript: setTimeout(fnOrString, 100)
    ↓
adapter.setTimeout(call)
    ↓
handler := call.Argument(0)
    ↓
if IsCallable(handler):
    // Function handler path
    // Invoke with correct thisArg
else:
    // String handler path
    // Get trusted type compliant string
    // EnsureCSPDoesNotBlockStringCompilation
    // Create and run classic script
```

---

## 4. IDENTIFIED COMPLIANCE GAPS

### Critical (Must Fix)

| Gap ID | Description | Spec Section | Severity |
|--------|-------------|--------------|----------|
| G1 | `thisArg` handling - passes `undefined` instead of global | 8.6 Step 1, 7 | CRITICAL |
| G2 | String handlers completely unimplemented | 8.6 Step 8-15 | CRITICAL |
| G3 | TrustedScript support missing | Trusted Types Spec | CRITICAL |
| G4 | CSP string compilation checks missing | CSP Spec | HIGH |
| G5 | `get trusted type compliant string` algorithm missing | TT Spec | HIGH |

### Major (Should Fix)

| Gap ID | Description | Spec Section | Severity |
|--------|-------------|--------------|----------|
| G6 | Error handling differs between Function/string | 8.6 Step 10 | HIGH |
| G7 | Object.toString() conversion not implemented | 8.6 Notes | HIGH |
| G8 | No reporting of exceptions to global | WebIDL 3.12 | MEDIUM |

### Minor (Nice to Have)

| Gap ID | Description | Spec Section | Severity |
|--------|-------------|--------------|----------|
| G9 | Module script fetch options not applied | 8.6 Step 11-13 | MEDIUM |
| G10 | Initiating script base URL not used | 8.6 Step 12 | LOW |

---

## 5. RECOMMENDATIONS

### 5.1 Immediate Actions (Priority 1)

**Fix G1 - thisArg Handling:**

```go
// In adapter.go, setTimeout/setInterval:

func (a *Adapter) setTimeout(call goja.FunctionCall) goja.Value {
    fn := call.Argument(0)
    // ...
    
    // Get the global object for this context
    var thisArg goja.Value
    if runtime, ok := a.runtime.(*goja.Runtime); ok {
        thisArg = runtime.Global()  // Get the global object
    } else {
        thisArg = goja.Undefined()
    }
    
    id, err := a.js.SetTimeout(func() {
        _, _ = fnCallable(thisArg)  // Pass global as this
    }, delayMs)
    // ...
}
```

**Verify with Test:**
```javascript
// Test this value in timer callback
setTimeout(function() {
    console.log(this === self);  // Should be true in spec-compliant impl
}, 0);
```

### 5.2 Short-term Actions (Priority 2)

**Implement String Handler Support (G2, G4, G5):**

This requires significant work:

1. Modify handler type detection:
   - Accept `goja.Value` instead of only functions
   - Check `goja.IsFunction(fn)` vs `goja.IsString(fn)` vs TrustedScript

2. Implement `get trusted type compliant string`:
   ```go
   func getTrustedTypeCompliantString(policyType string, global, handler, sink, typeArg string) (string, error)
   ```

3. Implement CSP checks:
   ```go
   func ensureCSPDoesNotBlockStringCompilation(realm, handler string) error
   ```

4. Implement script creation/execution (Goja specific):
   ```go
   script := runtime.RunString(handler)  // Create classic script
   script.Run()                         // Execute script
   ```

**Warning:** This is a significant implementation effort. The spec requires:
- Same behavior as `eval()` for string compilation
- CSP integration
- Trusted Types integration

### 5.3 Long-term Considerations

**Trusted Types Full Implementation:**

```go
// If require-trusted-types-for 'script' is enforced:

type TrustedScript interface {
    // Marker interface for TrustedScript
}

type TrustedTypePolicy struct {
    Name string
    CreateScript func(script string) TrustedScript
    // ... other factories
}

// Policy lookup and creation
policy := trustedTypes.GetPolicy(policyName)
trustedScript := policy.CreateScript(scriptString)
```

### 5.4 Testing Recommendations

**Test Cases Needed:**

1. **thisArg Test:**
```javascript
setTimeout(function() {
    console.log(this === self);  // true
}, 0);
```

2. **String Handler Test:**
```javascript
var executed = false;
setTimeout("executed = true", 10);
// Verify executed becomes true
```

3. **Trusted Types Test:**
```javascript
// Only run if trustedTypes available
if (typeof trustedTypes !== 'undefined') {
    const policy = trustedTypes.createPolicy('test', {});
    setTimeout(policy.createScript('1+1'), 10);
}
```

4. **CSP Test:**
```javascript
// With CSP blocking eval:
setTimeout("1+1", 10);  // Should throw
```

---

## 6. REFERENCES

### Specifications

1. **WHATWG HTML Living Standard** - Section 8.6 Timers
   - URL: https://html.spec.whatwg.org/multipage/timers-and-user-prompts.html
   - Last Updated: February 8, 2026

2. **Web IDL Specification** - Section 3.12 Invoking Callback Functions
   - URL: https://webidl.spec.whatwg.org/
   - Key Concepts: callback this value, exceptionBehavior, invoke algorithm

3. **Trusted Types Specification**
   - URL: https://w3c.github.io/webappsec-trusted-types/dist/spec/
   - Key Concepts: TrustedScript, get trusted type compliant string, sink

4. **Content Security Policy Level 3**
   - URL: https://w3c.github.io/webappsec-csp/
   - Key Concepts: EnsureCSPDoesNotBlockStringCompilation, unsafe-eval

### Implementation Files Analyzed

1. `eventloop/js.go` - Core timer implementation
2. `goja-eventloop/adapter.go` - Goja runtime adapter

### Key Algorithm References

| Algorithm | Purpose | Spec Reference |
|-----------|---------|----------------|
| Timer Initialization | Main timer algorithm | HTML 8.6 |
| get trusted type compliant string | Trusted Types conversion | TT Spec |
| EnsureCSPDoesNotBlockStringCompilation | CSP validation | CSP 3.1 |
| invoke a callback function | Function invocation | WebIDL 3.12 |
| create a classic script | Script creation | HTML |
| Run the classic script | Script execution | HTML |

---

## 7. CONCLUSION

The eventloop implementation provides a **functional timer API** for the common case (Function handlers), but has **significant compliance gaps** for:

1. **Complete handler type support** (strings not accepted)
2. **Security features** (Trusted Types, CSP string compilation checks)
3. **Spec-accurate `this` binding** (uses `undefined` instead of global)

**For applications that need:**
- **Spec compliance**: String handlers and Trusted Types would need implementation
- **Security hardening**: Current implementation is actually MORE SECURE (no string handlers)
- **Browser compatibility**: May differ from browser behavior for edge cases

**Recommendation:** 
- For new projects: Accept current limitations or implement full spec
- For security-sensitive applications: Current implementation may be preferred (no string handler attack surface)
- For spec compliance requirements: Full implementation of G1-G10 needed

---

**Document prepared by:** Takumi (匠)  
**Investigation Date:** February 9, 2026  
**Next Review:** When handler type implementation changes
