package eventloop

import (
	"errors"
	"io"
	"unsafe"
)

func (x *Loop) storeTerminalError(err error) {
	if err != nil {
		x.terminalErr.Store(&terminalErrorBox{err: err})
	}
}

func (x *Loop) terminalError() error {
	var terminalErr error
	value := x.terminalErr.Load()
	if box, ok := value.(*terminalErrorBox); ok && box != nil {
		terminalErr = box.err
	}
	return joinErrors(terminalErr, x.fdResourceCloseError())
}

func joinErrors(primary, secondary error) error {
	if primary == nil {
		return secondary
	}
	if secondary == nil {
		return primary
	}
	return errors.Join(primary, secondary)
}

func (x *Loop) fdResourceCloseError() error {
	value := x.fdCloseErr.Load()
	if box, ok := value.(*terminalErrorBox); ok && box != nil {
		return box.err
	}
	return nil
}

// drainWakeUpPipe drains the wake-up pipe and resets the wakeup pending flag.
// This is called when the physical wake descriptor is reported ready.
func (x *Loop) drainWakeUpPipe() {
	x.wakeMu.Lock()
	if !x.pollerReady.Load() || x.wakePipe < 0 {
		// Task-only targets and closed or unpublished readiness resources have
		// no physical wake descriptor to drain.
		x.wakeUpSignalPending.Store(wakeSignalIdle)
		x.wakeMu.Unlock()
		return
	}
	read := readFD
	if x.testHooks != nil && x.testHooks.ReadWakeFD != nil {
		read = x.testHooks.ReadWakeFD
	}
	var reportErr error
	var reportMsg string
	for {
		n, err := read(x.wakePipe, x.wakeBuf[:])
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
	x.wakeUpSignalPending.Store(wakeSignalIdle)
	x.wakeMu.Unlock()
	if x.testHooks != nil && x.testHooks.AfterWakeDrain != nil {
		x.testHooks.AfterWakeDrain()
	}
	if reportErr != nil {
		x.logError("eventloop: "+reportMsg, reportErr)
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
func (x *Loop) submitWakeup() error {
	// Test hook: allow tests to observe submitWakeup calls
	if x.testHooks != nil && x.testHooks.OnSubmitWakeup != nil {
		x.testHooks.OnSubmitWakeup()
	}
	// Check state and reject once terminal admission is closed.
	// We MUST allow wake-up during StateTerminating so the loop can
	// drain queued tasks and complete shutdown
	state := x.state.Load()
	if state == StateTerminated {
		// Terminal admission is closed, so no lifecycle wake is needed.
		return ErrLoopTerminated
	}

	return x.submitWakeupPhysical()
}

func (x *Loop) submitWakeupPhysical() error {
	x.wakeMu.Lock()
	defer x.wakeMu.Unlock()
	return x.submitWakeupPhysicalLocked()
}

func (x *Loop) submitWakeupPhysicalLocked() error {
	if !fdPollingSupported {
		return nil
	}
	if !x.pollerReady.Load() {
		return nil
	}
	if x.testHooks != nil && x.testHooks.BeforePhysicalWake != nil {
		x.testHooks.BeforePhysicalWake()
	}
	// Platform-specific eventfd or self-pipe wake mechanism.
	// Internal optimization: Native endianness, no binary.LittleEndian overhead
	var one uint64 = 1
	buf := (*[8]byte)(unsafe.Pointer(&one))[:]

	write := writeFD
	if x.testHooks != nil && x.testHooks.WriteWakeFD != nil {
		write = x.testHooks.WriteWakeFD
	}
	for {
		n, err := write(x.wakePipeWrite, buf)
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

func (x *Loop) submitPendingWakeup() error {
	if !fdPollingSupported {
		return nil
	}
	if x.wakeUpSignalPending.Load() == wakeSignalPending {
		return nil
	}
	if x.testHooks != nil && x.testHooks.BeforePendingWakeLock != nil {
		x.testHooks.BeforePendingWakeLock()
	}
	x.wakeMu.Lock()
	defer x.wakeMu.Unlock()
	if x.wakeUpSignalPending.Load() == wakeSignalPending {
		return nil
	}
	if !x.pollerReady.Load() {
		return nil
	}
	if x.testHooks != nil && x.testHooks.OnSubmitWakeup != nil {
		x.testHooks.OnSubmitWakeup()
	}
	if x.state.Load() == StateTerminated {
		x.wakeUpSignalPending.Store(wakeSignalIdle)
		return ErrLoopTerminated
	}
	x.wakeUpSignalPending.Store(wakeSignalSubmitting)
	if err := x.submitWakeupPhysicalLocked(); err != nil {
		// No physical wake represents this claim. Re-open admission so a later
		// producer can retry rather than stranding accepted work in PollIO.
		x.wakeUpSignalPending.Store(wakeSignalIdle)
		return err
	}
	x.wakeUpSignalPending.Store(wakeSignalPending)
	return nil
}
