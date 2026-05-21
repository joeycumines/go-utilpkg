//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

// pollNative performs native readiness polling for loops with registered FDs
// or FastPathDisabled mode. It is only reached on unix targets where
// fdPollingSupported is true.
func (l *Loop) pollNative(timeout int) {
	if err := l.ensurePoller(); err != nil {
		l.handlePollError(err)
		return
	}
	// A wake committed while lazy initialization still had pollerReady=false is
	// represented only by the fast channel. Consume that handoff before entering
	// the newly published native poller. Wakes published after this check observe
	// pollerReady and also submit the physical signal.
	if l.consumeFastWakeup() {
		return
	}
	if l.testHooks != nil && l.testHooks.BeforePollIO != nil {
		l.testHooks.BeforePollIO()
	}
	state := l.state.Load()
	if state == StateTerminating || state == StateTerminated {
		l.state.TryTransition(StateSleeping, StateRunning)
		return
	}
	timeout = boundedPhysicalPollTimeout(timeout)
	pollIO := l.poller.PollIO
	if l.testHooks != nil && l.testHooks.PollIO != nil {
		pollIO = l.testHooks.PollIO
	}
	_, err := pollIO(timeout)
	if l.testHooks != nil && l.testHooks.PollError != nil {
		err = l.testHooks.PollError()
	}
	if err != nil {
		l.poller.clearReadyEvents()
		l.handlePollError(err)
		return
	}
	if l.state.Load() == StateTerminated {
		l.poller.clearReadyEvents()
		return
	}

	if l.testHooks != nil && l.testHooks.PrePollAwake != nil {
		l.testHooks.PrePollAwake()
	}
	if !l.state.TryTransition(StateSleeping, StateRunning) {
		l.poller.clearReadyEvents()
		return
	}
	l.dispatchPollEvents(l.poller.readyEventsSnapshot())
	l.poller.clearReadyEvents()
}

// ensurePoller lazily initializes the native readiness poller. Only reachable
// on unix targets where fdPollingSupported is true.
func (l *Loop) ensurePoller() error {
	l.fdMu.Lock()
	defer l.fdMu.Unlock()
	return l.ensurePollerLocked()
}

func (l *Loop) dispatchPollEvents(events []pollEvent) {
	for _, event := range events {
		if l.testHooks != nil && l.testHooks.BeforeFDPublicationCheck != nil {
			l.testHooks.BeforeFDPublicationCheck(event.fd)
		}
		callback, _, dispatch, ok := l.poller.beginReadyEventDispatch(event)
		if !ok {
			continue
		}
		if l.testHooks != nil && l.testHooks.AfterReadyEventDispatchClaim != nil {
			l.testHooks.AfterReadyEventDispatchClaim(event.fd)
		}
		events, ok := l.poller.startReadyEventDispatch(event, dispatch)
		if !ok {
			continue
		}
		if event.internal {
			l.executePollInternal(func() { callback(events) })
			continue
		}
		l.safeExecute(func() {
			callback(events)
		})
		l.drainMicrotasks()
		if l.hardAbortRequested() {
			return
		}
	}
}
