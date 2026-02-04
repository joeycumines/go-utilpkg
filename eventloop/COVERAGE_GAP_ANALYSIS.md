# Comprehensive Coverage Gap Analysis - Eventloop Package

## 1. Current Overall Coverage

**Overall Coverage: 91.0%** (statements)

## 2. Functions with Coverage < 100%

### Critical Gaps (> 20% uncovered)

| Function | File | Coverage | Priority |
|----------|------|----------|----------|
| `thenStandalone` | promise.go:595 | **54.5%** | 🔴 Critical |
| `New` | loop.go:190 | **57.1%** | 🔴 Critical |
| `createWakeFd` | wakeup_darwin.go:21 | **58.8%** | 🔴 Critical |
| `handlePollError` | loop.go:1075 | **66.7%** | 🟠 High |
| `drainWakeUpPipe` | loop.go:1084 | **75.0%** | 🟠 High |
| `SubmitInternal` | loop.go:1219 | **79.3%** | 🟠 High |
| `promisify` | promisify.go:39 | **80.0%** | 🟠 High |
| `percentileIndex` | metrics.go:126 | **75.0%** | 🟠 High |
| `runTimers` | loop.go:1500 | **73.9%** | 🟠 High |
| `pollFastMode` | loop.go:995 | **73.7%** | 🟠 High |
| `ScheduleMicrotask` | loop.go:1317 | **76.5%** | 🟠 High |
| `Finally` | promise.go:670 | **76.6%** | 🟠 High |
| `processExternal` | loop.go:832 | **77.3%** | 🟠 High |
| `UpdateIngress` | metrics.go:160 | **77.8%** | 🟠 High |
| `resolveLoopOptions` | options.go:76 | **85.7%** | 🟡 Medium |
| `NewJS` | js.go:155 | **87.5%** | 🟡 Medium |
| `run` | js.go:511 | **88.9%** | 🟡 Medium |
| `handlePollError` (method) | loop.go:1075 | **66.7%** | 🟠 High |
| `safeExecuteFn` | loop.go:1670 | **83.3%** | 🟡 Medium |
| `Shutdown` | loop.go:355 | **83.3%** | 🟡 Medium |

### Platform-Specific Functions (Darwin)

| Function | File | Coverage | Notes |
|----------|------|----------|-------|
| `Wakeup` | poller_darwin.go:103 | **0.0%** | Stub function (never called on Darwin) |
| `percentileIndex` | metrics.go:126 | **75.0%** | Edge case not tested |
| `Init` | poller_darwin.go:68 | **77.8%** | Partial coverage |
| `Close` | poller_darwin.go:86 | **75.0%** | Partial coverage |
| `PollIO` | poller_darwin.go:227 | **66.7%** | Partial coverage |
| `RegisterFD` | poller_darwin.go:110 | **57.7%** | Low coverage |

## 3. Specific Uncovered Code Paths

### A. `thenStandalone` (promise.go:595) - 54.5%
**Current coverage shows 0.0% in some runs**

This function has multiple uncovered paths:
- Line 608: `id: p.id + 1` - Child promise ID generation
- Lines 616-620: Handler storage when `p.h0.target == nil`
- Lines 621-628: Handler appending to existing handlers slice
- Lines 634-640: Synchronous handler execution for already-settled promises
- Lines 642-644: Return of standalone promise

**Uncovered scenarios:**
1. ❌ Creating a child promise from a pending parent
2. ❌ Adding handlers to a promise that already has handlers (h0 is set)
3. ❌ Synchronous resolution path when promise is already fulfilled
4. ❌ Synchronous rejection path when promise is already rejected

### B. `New` (loop.go:190) - 57.1%
**Current coverage shows 0.0% in some runs**

Uncovered paths:
- Error paths from `createWakeFd()` when pipe creation fails
- Error paths from `poller.Init()` initialization
- Options validation errors in `resolveLoopOptions()`

**Uncovered scenarios:**
1. ❌ `createWakeFd()` failure (pipe creation error)
2. ❌ `poller.Init()` failure (poller initialization error)
3. ❌ Invalid option combinations in `resolveLoopOptions()`

### C. `createWakeFd` (wakeup_darwin.go:21) - 58.8%

Uncovered paths:
- Lines 28-32: Pipe creation failure path
- Lines 38-42: Setting non-blocking on read end failure
- Lines 44-48: Setting non-blocking on write end failure

**Uncovered scenarios:**
1. ❌ `syscall.Pipe()` fails
2. ❌ `syscall.SetNonblock()` fails on read end
3. ❌ `syscall.SetNonblock()` fails on write end

### D. `handlePollError` (loop.go:1075) - 66.7%

This function has a single branch that's never exercised:
- Line 1078: The condition `l.state.TryTransition(StateSleeping, StateTerminating)`

**Uncovered scenario:**
- ❌ Polling error when loop is in StateSleeping (triggers transition to StateTerminating)

### E. `drainWakeUpPipe` (loop.go:1084) - 75.0%

Uncovered paths:
- Lines 1090-1094: Error from `readFD()` breaking the loop
- Lines 1100-1102: The reset of `wakeUpSignalPending`

**Uncovered scenario:**
- ❌ `readFD()` returning an error (indicates EOF or actual error)

### F. `promisify` (promisify.go:39) - 80.0%

Uncovered paths:
- Lines 45-52: Loop already terminated state check
- Lines 72-82: Context cancellation path
- Lines 85-95: Panic recovery path
- Lines 97-106: `runtime.Goexit()` detection
- Lines 111-120: SubmitInternal failure during resolution
- Lines 125-132: SubmitInternal failure during rejection

**Uncovered scenarios:**
1. ❌ Context already cancelled before goroutine starts
2. ❌ Function panics with actual value (not nil)
3. ❌ Function exits via `runtime.Goexit()`
4. ❌ `SubmitInternal()` fails during promise resolution
5. ❌ `SubmitInternal()` fails during promise rejection

### G. `percentileIndex` (metrics.go:126) - 75.0%

This function has two branches:
- Line 128: `if index >= n { return n - 1 }`
- Line 130: `return index`

**Uncovered scenario:**
- ❌ Percentile calculation where result index >= n (edge case when p=100 or n=1)

### H. `runTimers` (loop.go:1500) - 73.9%

Uncovered paths:
- Lines 1515-1528: Timer callback execution with nesting depth management
- Lines 1530-1535: Timer cleanup and return to pool

**Uncovered scenarios:**
1. ❌ Timer callback that panics (tests always pass)
2. ❌ Timer with `canceled.Load() == true` path
3. ❌ Multiple nested timer calls

### I. `pollFastMode` (loop.go:995) - 73.7%

Uncovered paths:
- Lines 1010-1028: Timeout handling with `fastWakeupCh`
- Lines 1040-1060: Timeout with auxJobs check
- Lines 1065-1080: Return with state transition

**Uncovered scenarios:**
1. ❌ Fast mode wakeup via channel
2. ❌ Timeout expiration while in fast mode
3. ❌ Termination check during fast mode

## 4. Recommendations for Additional Tests

### Priority 1: Critical Functions (Must Cover)

#### Test 1.1: `thenStandalone` Coverage
```go
// Test creating standalone promise without event loop
func TestThenStandalone(t *testing.T) {
    p := &ChainedPromise{
        id:    1,
        state: NewFastState(),
    }
    p.state.Store(int32(Pending))
    
    // Test 1: Then on pending promise
    child := p.thenStandalone(func(r Result) Result { return r }, nil)
    if child.id != 2 {
        t.Errorf("Expected child ID 2, got %d", child.id)
    }
    
    // Test 2: Then on already fulfilled promise
    p2 := &ChainedPromise{
        id:    10,
        state: NewFastState(),
    }
    p2.state.Store(int32(Fulfilled))
    p2.result = "test-value"
    
    child2 := p2.thenStandalone(func(r Result) Result { 
        return "modified-" + r.String() 
    }, nil)
    if child2.state.Load() != int32(Fulfilled) {
        t.Error("Expected immediate resolution")
    }
    
    // Test 3: Then on already rejected promise
    p3 := &ChainedPromise{
        id:    20,
        state: NewFastState(),
    }
    p3.state.Store(int32(Rejected))
    p3.result = errors.New("test error")
    
    child3 := p3.thenStandalone(nil, func(r Result) Result {
        return Wrap(r, "wrapped")
    })
    if child3.state.Load() != int32(Rejected) {
        t.Error("Expected immediate rejection")
    }
}
```

#### Test 1.2: `New` Error Paths
```go
// Test New() with pipe creation failure (mock)
func TestNew_PipeCreationFailure(t *testing.T) {
    // This requires mocking or modifying createWakeFd
    // Consider using a build tag to inject failure
    if testing.Short() {
        t.Skip("Requires system call mocking")
    }
    
    // Test with invalid file descriptors
    // This would require modifying the code to accept injected errors
}

// Test New() with invalid options
func TestNew_InvalidOptions(t *testing.T) {
    _, err := New(WithFastPathMode(-1)) // Invalid mode
    if err == nil {
        t.Error("Expected error for invalid fast path mode")
    }
}
```

#### Test 1.3: `createWakeFd` Error Paths
```go
// Test pipe creation failure
// Note: This requires modifying the code to be testable with injected errors
func TestCreateWakeFd_Errors(t *testing.T) {
    t.Run("pipe_creation_fails", func(t *testing.T) {
        // Would require injecting syscall.Pipe failure
    })
    
    t.Run("set_nonblock_fails_read", func(t *testing.T) {
        // Would require injecting syscall.SetNonblock failure
    })
    
    t.Run("set_nonblock_fails_write", func(t *testing.T) {
        // Would require injecting syscall.SetNonblock failure
    })
}
```

#### Test 1.4: `handlePollError` Path
```go
// Test poll error handling when in StateSleeping
func TestHandlePollError_SleepingState(t *testing.T) {
    loop, _ := New()
    
    // Transition to StateSleeping
    loop.state.Store(StateSleeping)
    
    // Simulate poll error
    pollErr := errors.New("poll failed: too many open files")
    loop.handlePollError(pollErr)
    
    // Verify transition to terminating
    if loop.state.Load() != StateTerminating {
        t.Errorf("Expected StateTerminating, got %v", loop.state.Load())
    }
}
```

### Priority 2: High Priority Functions

#### Test 2.1: `drainWakeUpPipe` Error Path
```go
// Test drainWakeUpPipe with readFD error
func TestDrainWakeUpPipe_ReadError(t *testing.T) {
    loop, _ := New()
    
    // Manually set up a wake pipe that will error
    // This requires modifying the code to inject the error
}
```

#### Test 2.2: `promisify` Error Paths
```go
// Test promisify with context cancellation
func TestPromisify_ContextCancellation(t *testing.T) {
    loop, _ := New()
    
    ctx, cancel := context.WithCancel(context.Background())
    cancel() // Cancel immediately
    
    p := loop.Promisify(ctx, func(ctx context.Context) (Result, error) {
        return nil, errors.New("should not run")
    })
    
    // Should be rejected immediately
    if p.State() != Rejected {
        t.Error("Expected promise to be rejected")
    }
}

// Test promisify with panic recovery
func TestPromisify_PanicRecovery(t *testing.T) {
    loop, _ := New()
    
    p := loop.Promisify(context.Background(), func(ctx context.Context) (Result, error) {
        panic("test panic")
    })
    
    // Should reject with PanicError
    if p.State() != Rejected {
        t.Error("Expected promise to be rejected")
    }
}

// Test promisify with Goexit
func TestPromisify_Goexit(t *testing.T) {
    loop, _ := New()
    
    p := loop.Promisify(context.Background(), func(ctx context.Context) (Result, error) {
        runtime.Goexit()
        return nil, nil // Unreachable
    })
    
    // Should reject with ErrGoexit
    if p.State() != Rejected {
        t.Error("Expected promise to be rejected")
    }
}

// Test promisify with SubmitInternal failure
func TestPromisify_SubmitInternalFailure(t *testing.T) {
    loop, _ := New()
    
    // Shutdown the loop first
    loop.Shutdown(context.Background())
    
    p := loop.Promisify(context.Background(), func(ctx context.Context) (Result, error) {
        return "success", nil
    })
    
    // Should still resolve via fallback
    // This tests the fallback path
}
```

#### Test 2.3: `percentileIndex` Edge Cases
```go
// Test percentileIndex with edge cases
func TestPercentileIndex(t *testing.T) {
    tests := []struct {
        n, p   int
        expect int
    }{
        {1, 0, 0},      // Single element, 0th percentile
        {1, 50, 0},     // Single element, 50th percentile
        {1, 100, 0},    // Single element, 100th percentile (edge)
        {10, 100, 9},   // 100th percentile returns n-1
        {100, 50, 50},  // Normal case
    }
    
    for _, tt := range tests {
        got := percentileIndex(tt.n, tt.p)
        if got != tt.expect {
            t.Errorf("percentileIndex(%d, %d) = %d, want %d", tt.n, tt.p, got, tt.expect)
        }
    }
}
```

#### Test 2.4: `runTimers` Edge Cases
```go
// Test runTimers with panic in callback
func TestRunTimers_PanicRecovery(t *testing.T) {
    loop, _ := New()
    
    loop.ScheduleTimer(10*time.Millisecond, func() {
        panic("timer panic")
    })
    
    // Run the loop briefly
    done := make(chan struct{})
    go func() {
        loop.Run()
        close(done)
    }()
    
    time.Sleep(50 * time.Millisecond)
    loop.Shutdown(context.Background())
    <-done
    
    // Loop should still be functional
    // Timer should be cleaned up
}

// Test runTimers with canceled timer
func TestRunTimers_CanceledTimer(t *testing.T) {
    loop, _ := New()
    
    timerID := loop.ScheduleTimer(10*time.Millisecond, func() {
        t.Error("Canceled timer should not run")
    })
    
    loop.CancelTimer(timerID)
    
    // Run the loop briefly
    done := make(chan struct{})
    go func() {
        loop.Run()
        close(done)
    }()
    
    time.Sleep(50 * time.Millisecond)
    loop.Shutdown(context.Background())
    <-done
}
```

### Priority 3: Medium Priority Functions

#### Test 3.1: `pollFastMode` Coverage
```go
// Test pollFastMode with timeout
func TestPollFastMode_Timeout(t *testing.T) {
    loop, _ := New(
        WithFastPathMode(FastPathEnabled),
    )
    
    // Schedule a microtask
    called := false
    loop.ScheduleMicrotask(func() {
        called = true
    })
    
    // Let it run briefly
    done := make(chan struct{})
    go func() {
        loop.Run()
        close(done)
    }()
    
    time.Sleep(20 * time.Millisecond)
    loop.Shutdown(context.Background())
    <-done
    
    if !called {
        t.Error("Microtask should have been called")
    }
}

// Test pollFastMode wakeup via channel
func TestPollFastMode_Wakeup(t *testing.T) {
    loop, _ := New(
        WithFastPathMode(FastPathEnabled),
    )
    
    // Wake up the loop
    loop.Wake()
    
    // Run the loop briefly
    done := make(chan struct{})
    go func() {
        loop.Run()
        close(done)
    }()
    
    time.Sleep(20 * time.Millisecond)
    loop.Shutdown(context.Background())
    <-done
}
```

### Platform-Specific Tests

#### Test 4.1: Darwin-specific `Wakeup` (0.0% coverage)
```go
// Test the stub Wakeup function on Darwin
// This function is never called on Darwin, but we should verify it exists
func TestWakeup_Stub(t *testing.T) {
    poller := &FastPoller{}
    err := poller.Wakeup()
    if err != nil {
        t.Errorf("Expected nil error from stub, got %v", err)
    }
}
```

## 5. Summary

To achieve **100% coverage**, the following tests are **essential**:

1. **thenStandalone**: 3 test cases (pending, fulfilled, rejected parent states)
2. **New**: 2-3 test cases (pipe failure, poller failure, invalid options)
3. **createWakeFd**: 3 test cases (pipe error, read FD error, write FD error)
4. **handlePollError**: 1 test case (error when StateSleeping)
5. **drainWakeUpPipe**: 1 test case (readFD error)
6. **promisify**: 5 test cases (context cancel, panic, Goexit, SubmitInternal failure x2)
7. **percentileIndex**: 1 test case (p >= 100 edge case)
8. **runTimers**: 2 test cases (panic recovery, canceled timer)
9. **pollFastMode**: 2 test cases (timeout, channel wakeup)

**Estimated additional test code**: ~200-300 lines
**Estimated additional test functions**: 15-20 new test functions

## 6. Implementation Strategy

1. **Week 1**: Focus on Priority 1 functions (critical)
2. **Week 2**: Focus on Priority 2 functions (high priority)
3. **Week 3**: Focus on Priority 3 functions (medium priority)
4. **Week 4**: Platform-specific tests and integration tests

## 7. Testing Infrastructure Needs

To properly test some error paths, consider:

1. **System call mocking**: Use a interface for `syscall.Pipe`, `syscall.SetNonblock`, etc.
2. **Dependency injection**: Allow injecting errors in `createWakeFd`
3. **Build tags**: Use build tags to enable error injection in tests
4. **Test hooks**: Add test hooks for internal state inspection

---
*Generated: 4 February 2026*
*Data source: go test -coverprofile analysis*
