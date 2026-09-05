package eventloop

import (
	"errors"
	"io"
	"unsafe"
)

// drainWakeUpPipe drains the wake-up pipe and resets the wakeup pending flag.
// This is called when the physical wake descriptor is reported ready.
func (l *Loop) drainWakeUpPipe() {
	l.wakeMu.Lock()
	if !l.pollerReady.Load() || l.wakePipe < 0 {
		// Task-only targets and closed or unpublished readiness resources have
		// no physical wake descriptor to drain.
		l.wakeUpSignalPending.Store(wakeSignalIdle)
		l.wakeMu.Unlock()
		return
	}
	read := readFD
	if l.testHooks != nil && l.testHooks.ReadWakeFD != nil {
		read = l.testHooks.ReadWakeFD
	}
	var reportErr error
	var reportMsg string
	for {
		n, err := read(l.wakePipe, l.wakeBuf[:])
		switch {
		case err == nil && n > 0:
			continue
		case err == nil:
			reportErr = io.ErrUnexpectedEOF
			reportMsg = "wake descriptor returned zero bytes"
		case wakeIOInterrupted(err):
			continue
		case wakeReadComplete(err):
			// The nonblocking descriptor is empty; the physical wake epoch is
			// fully consumed and a later producer may establish a new one.
		case errors.Is(err, io.EOF):
			reportErr = err
			reportMsg = "wake descriptor reached EOF"
		default:
			reportErr = err
			reportMsg = "wake descriptor read failed"
		}
		break
	}
	// Reset the wakeup pending flag so future Submit/SubmitInternal can wake again
	l.wakeUpSignalPending.Store(wakeSignalIdle)
	l.wakeMu.Unlock()
	if l.testHooks != nil && l.testHooks.AfterWakeDrain != nil {
		l.testHooks.AfterWakeDrain()
	}
	if reportErr != nil {
		l.logError("eventloop: "+reportMsg, reportErr)
	}
}

// submitWakeup writes to the Unix wake descriptor when native FD polling is
// supported. Platforms without public FD polling wake through fastWakeupCh and
// deliberately avoid posting notifications to an inactive native poller.
//
// Wake-up Policy:
//   - REJECTS: StateTerminated (terminal admission is closed; no lifecycle wake is needed)
//   - ALLOWS: StateTerminating (loop needs to wake and drain remaining tasks)
//   - ALLOWS: StateSleeping, StateRunning, StateAwake
//
// Safe to call concurrently during shutdown - pipe write errors during shutdown are
// gracefully handled by callers.
//
// IMPLEMENTATION NOTES:
//   - readiness targets write to an eventfd or self-pipe
//   - task-only targets do nothing because the loop cannot enter native FD polling
func (l *Loop) submitWakeup() error {
	// Test hook: allow tests to observe submitWakeup calls
	if l.testHooks != nil && l.testHooks.OnSubmitWakeup != nil {
		l.testHooks.OnSubmitWakeup()
	}
	// Check state and reject once terminal admission is closed.
	// We MUST allow wake-up during StateTerminating so the loop can
	// drain queued tasks and complete shutdown
	state := l.state.Load()
	if state == StateTerminated {
		// Terminal admission is closed, so no lifecycle wake is needed.
		return ErrLoopTerminated
	}

	return l.submitWakeupPhysical()
}

func (l *Loop) submitWakeupPhysical() error {
	l.wakeMu.Lock()
	defer l.wakeMu.Unlock()
	return l.submitWakeupPhysicalLocked()
}

func (l *Loop) submitWakeupPhysicalLocked() error {
	if !fdPollingSupported {
		return nil
	}
	if !l.pollerReady.Load() {
		return nil
	}
	if l.testHooks != nil && l.testHooks.BeforePhysicalWake != nil {
		l.testHooks.BeforePhysicalWake()
	}
	// Platform-specific eventfd or self-pipe wake mechanism.
	// Internal optimization: Native endianness, no binary.LittleEndian overhead
	var one uint64 = 1
	buf := (*[8]byte)(unsafe.Pointer(&one))[:]

	write := writeFD
	if l.testHooks != nil && l.testHooks.WriteWakeFD != nil {
		write = l.testHooks.WriteWakeFD
	}
	for {
		n, err := write(l.wakePipeWrite, buf)
		if err == nil {
			if n != len(buf) {
				return io.ErrShortWrite
			}
			return nil
		}
		if wakeIOInterrupted(err) {
			continue
		}
		if wakeWritePending(err) {
			return nil
		}
		return err
	}
}

func (l *Loop) submitPendingWakeup() error {
	if !fdPollingSupported {
		return nil
	}
	if l.wakeUpSignalPending.Load() == wakeSignalPending {
		return nil
	}
	if l.testHooks != nil && l.testHooks.BeforePendingWakeLock != nil {
		l.testHooks.BeforePendingWakeLock()
	}
	l.wakeMu.Lock()
	defer l.wakeMu.Unlock()
	if l.wakeUpSignalPending.Load() == wakeSignalPending {
		return nil
	}
	if !l.pollerReady.Load() {
		return nil
	}
	if l.testHooks != nil && l.testHooks.OnSubmitWakeup != nil {
		l.testHooks.OnSubmitWakeup()
	}
	if l.state.Load() == StateTerminated {
		l.wakeUpSignalPending.Store(wakeSignalIdle)
		return ErrLoopTerminated
	}
	l.wakeUpSignalPending.Store(wakeSignalSubmitting)
	if err := l.submitWakeupPhysicalLocked(); err != nil {
		// No physical wake represents this claim. Re-open admission so a later
		// producer can retry rather than stranding accepted work in PollIO.
		l.wakeUpSignalPending.Store(wakeSignalIdle)
		return err
	}
	l.wakeUpSignalPending.Store(wakeSignalPending)
	return nil
}

// doWakeup attempts both the channel and physical wake signals. This handles
// the race where canUseFastPath()
// disagrees with the loop's actual poll path (e.g., mode=Forced + count>0 due
// to concurrent RegisterFD/SetFastPathMode). On platforms without public FD
// polling, submitWakeup is a no-op and the channel is the sole wait primitive.
func (l *Loop) doWakeup() {
	// Always try channel wakeup (covers fast path mode)
	select {
	case l.fastWakeupCh <- struct{}{}:
	default:
		// Channel already has pending wakeup
	}

	// Always try pipe/eventfd wakeup (covers I/O poll mode)
	// This is unconditional to prevent lost wakeups when mode and count
	// are transiently inconsistent due to concurrent SetFastPathMode/RegisterFD.
	_ = l.submitWakeup()
}

// wakeAfterIngress wakes whichever wait primitive the loop is currently using
// after work has already been admitted to a queue. The decision is deliberately
// post-admission: mode and FD counts may have changed while the caller was
// waiting to commit the task. A buffered fast wake covers fast-channel waiters;
// a physical wake is added whenever the loop can be blocked in PollIO, either
// because a user FD is registered or FastPathDisabled explicitly selects native
// polling for a task-only loop. FD-count and mode transitions perform their own
// physical wakeups, so ordinary task-only submissions do not pay the pipe/eventfd
// cost.
func (l *Loop) wakeAfterIngress() {
	select {
	case l.fastWakeupCh <- struct{}{}:
	default:
	}

	if l.state.Load() == StateSleeping && l.nativePollSelected() {
		_ = l.submitPendingWakeup()
	}
}

func (l *Loop) nativePollSelected() bool {
	return l.userIOFDCount.Load() > 0 || FastPathMode(l.fastPathMode.Load()) == FastPathDisabled
}

func (l *Loop) forceWakeup() {
	select {
	case l.fastWakeupCh <- struct{}{}:
	default:
	}
	_ = l.submitWakeupPhysical()
}

// Wake attempts to wake up the loop from a suspended state.
//
// State Policy:
//   - StateSleeping: performs wake-up (if not already pending)
//   - StateTerminated: returns nil (no-op on terminated loop)
//   - StateRunning: publishes both selected wait signals so a concurrent
//     Running-to-Sleeping transition cannot overtake the wake
//   - StateTerminating/StateAwake: returns nil
//
// A physical wake submission failure is returned to the caller. The pending
// claim is reopened so a later Wake or ingress operation can retry.
func (l *Loop) Wake() error {
	state := l.state.Load()
	if state != StateRunning && state != StateSleeping {
		return nil
	}

	select {
	case l.fastWakeupCh <- struct{}{}:
	default:
	}
	if !l.nativePollSelected() {
		return nil
	}
	err := l.submitPendingWakeup()
	if err == ErrLoopTerminated {
		return nil
	}
	return err
}
