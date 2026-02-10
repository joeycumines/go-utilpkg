//go:build windows

package alternatetwo

// initWakeup sets up the wakeup mechanism on Windows.
// On Windows, IOCP uses PostQueuedCompletionStatus for wakeup — no pipe needed.
func (l *Loop) initWakeup() error {
	return l.poller.Init()
}

// drainWakeUpPipe is a no-op on Windows (wakeup uses IOCP, not pipes).
func (l *Loop) drainWakeUpPipe() {
	l.wakePending.Store(0)
}

// submitWakeup posts a completion to the IOCP handle to wake PollIO.
func (l *Loop) submitWakeup() error {
	return l.poller.Wakeup()
}

// closeFDs closes the poller on Windows (no wake pipe FDs to close).
func (l *Loop) closeFDs() {
	_ = l.poller.Close()
}
