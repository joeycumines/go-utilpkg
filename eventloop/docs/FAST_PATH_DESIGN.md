# Fast Path Mode Design

## Overview

This document describes the intelligent fast path mode selection system for the eventloop package.

## Problem Statement

The current implementation requires users to manually call `SetFastPathEnabled(true)` to enable fast path mode, even though the conditions for using fast path are already checked internally:
- `userIOFDCount == 0` (no I/O file descriptors registered)
- `!hasTimersPending()` (no timers scheduled)
- `!hasInternalTasks()` (no priority tasks)

This creates unnecessary friction and potential for suboptimal performance when users forget to enable fast path.

## Design Goals

1. **Auto-detection**: Fast path should automatically enable when conditions are met
2. **Zero overhead**: Auto-detection should add negligible overhead (<5ns per check)
3. **Override capability**: Users should be able to force specific modes for testing or edge cases
4. **Error handling**: Pinning to incompatible modes should return errors
5. **Observability**: Debug builds should warn when fast path could help but isn't used

## Mode Types

### FastPathMode Enum

```go
type FastPathMode int

const (
    // FastPathAuto automatically selects mode based on conditions
    FastPathAuto FastPathMode = iota
    
    // FastPathForced always uses fast path (error if I/O FDs registered)
    FastPathForced
    
    // FastPathDisabled always uses poll path (for debugging/testing)
    FastPathDisabled
)
```

## Condition Checks

### canUseFastPath() Method

Consolidate all fast path condition checks into a single method:

```go
func (l *Loop) canUseFastPath() bool {
    mode := l.fastPathMode.Load()
    
    switch FastPathMode(mode) {
    case FastPathForced:
        return true
    case FastPathDisabled:
        return false
    case FastPathAuto:
        fallthrough
    default:
        // Auto-detect: use fast path when no I/O features are active
        return l.userIOFDCount.Load() == 0
    }
}
```

### Usage Contexts

The three current check locations have different requirements:

1. **Line 420 (run loop)**: Full check including timers and internal tasks
   - `canUseFastPath() && !hasTimersPending() && !hasInternalTasks()`
   
2. **Line 946 (Submit)**: Only check if fast mode is possible
   - `canUseFastPath()` - determines queue to use
   
3. **Line 1017 (SubmitInternal)**: Check if on loop thread
   - `canUseFastPath() && isLoopThread()` - for direct execution

## API Changes

### Remove SetFastPathEnabled

```go
// DEPRECATED: Use SetFastPathMode instead
// func (l *Loop) SetFastPathEnabled(enabled bool)
```

### Add SetFastPathMode

```go
// SetFastPathMode sets the fast path mode selection.
// 
// Modes:
//   - FastPathAuto: Automatically select based on I/O FD count (default)
//   - FastPathForced: Always use fast path (returns error if I/O FDs registered)
//   - FastPathDisabled: Always use poll path (for debugging)
//
// Returns ErrFastPathIncompatible if FastPathForced is set but I/O FDs are registered.
func (l *Loop) SetFastPathMode(mode FastPathMode) error {
    if mode == FastPathForced && l.userIOFDCount.Load() > 0 {
        return ErrFastPathIncompatible
    }
    l.fastPathMode.Store(int32(mode))
    return nil
}

// FastPathMode returns the current fast path mode.
func (l *Loop) FastPathMode() FastPathMode {
    return FastPathMode(l.fastPathMode.Load())
}
```

### New Error

```go
var ErrFastPathIncompatible = errors.New("eventloop: fast path incompatible with registered I/O FDs")
```

## Backward Compatibility

For backward compatibility during migration:

```go
// SetFastPathEnabled is deprecated. Use SetFastPathMode instead.
// 
// Deprecated: This method will be removed in a future version.
func (l *Loop) SetFastPathEnabled(enabled bool) {
    if enabled {
        l.fastPathMode.Store(int32(FastPathForced))
    } else {
        l.fastPathMode.Store(int32(FastPathDisabled))
    }
}
```

## Implementation Plan

1. Add `FastPathMode` type and constants
2. Add `fastPathMode atomic.Int32` to Loop struct
3. Implement `canUseFastPath()` method
4. Implement `SetFastPathMode()` with error handling
5. Update all three check locations to use `canUseFastPath()`
6. Deprecate `SetFastPathEnabled()` (keep for compatibility)
7. Update tests to use new API
8. Benchmark auto-detection overhead

## Performance Considerations

The `canUseFastPath()` method adds:
- One atomic load (`fastPathMode`)
- One switch statement
- For Auto mode: one additional atomic load (`userIOFDCount`)

Expected overhead: < 5ns per call.

## Testing Strategy

1. **TestFastPath_AutoDetection**: Verify fast path auto-enables when no I/O FDs
2. **TestFastPath_AutoDisable**: Verify fast path auto-disables when I/O FD registered
3. **TestFastPath_Forced_Success**: Verify forced mode works when compatible
4. **TestFastPath_Forced_Error**: Verify error when forced but incompatible
5. **TestFastPath_Disabled**: Verify disabled mode uses poll path
6. **BenchmarkFastPath_AutoDetectionOverhead**: Measure overhead vs manual flag
