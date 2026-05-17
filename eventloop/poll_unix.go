//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

// pollNative performs native readiness polling for loops with registered FDs
// or FastPathDisabled mode. It is only reached on unix targets where
// fdPollingSupported is true.
func (x *Loop) pollNative(timeout int) {
	if err := x.ensurePoller(); err != nil {
		x.handlePollError(err)
		return
	}
	// A wake committed while lazy initialization still had pollerReady=false is
	// represented only by the fast channel. Consume that handoff before entering
	// the newly published native poller. Wakes published after this check observe
	// pollerReady and also submit the physical signal.
	if x.consumeFastWakeup() {
		return
	}
	if x.testHooks != nil && x.testHooks.BeforePollIO != nil {
		x.testHooks.BeforePollIO()
	}
	state := x.state.Load()
	if state == StateTerminating || state == StateTerminated {
		x.state.TryTransition(StateSleeping, StateRunning)
		return
	}
	timeout = boundedPhysicalPollTimeout(timeout)
	pollIO := x.poller.PollIO
	if x.testHooks != nil && x.testHooks.PollIO != nil {
		pollIO = x.testHooks.PollIO
	}
	_, err := pollIO(timeout)
	if x.testHooks != nil && x.testHooks.PollError != nil {
		err = x.testHooks.PollError()
	}
	if err != nil {
		x.poller.clearReadyEvents()
		x.handlePollError(err)
		return
	}
	if x.state.Load() == StateTerminated {
		x.poller.clearReadyEvents()
		return
	}

	if x.testHooks != nil && x.testHooks.PrePollAwake != nil {
		x.testHooks.PrePollAwake()
	}
	if !x.state.TryTransition(StateSleeping, StateRunning) {
		x.poller.clearReadyEvents()
		return
	}
	x.dispatchPollEvents(x.poller.readyEventsSnapshot())
	x.poller.clearReadyEvents()
}

// ensurePoller lazily initializes the native readiness poller. Only reachable
// on unix targets where fdPollingSupported is true.
func (x *Loop) ensurePoller() error {
	x.fdMu.Lock()
	defer x.fdMu.Unlock()
	return x.ensurePollerLocked()
}

func (x *Loop) dispatchPollEvents(events []pollEvent) {
	for _, event := range events {
		if x.testHooks != nil && x.testHooks.BeforeFDPublicationCheck != nil {
			x.testHooks.BeforeFDPublicationCheck(event.fd)
		}
		callback, _, dispatch, ok := x.poller.beginReadyEventDispatch(event)
		if !ok {
			continue
		}
		if x.testHooks != nil && x.testHooks.AfterReadyEventDispatchClaim != nil {
			x.testHooks.AfterReadyEventDispatchClaim(event.fd)
		}
		events, ok := x.poller.startReadyEventDispatch(event, dispatch)
		if !ok {
			continue
		}
		if event.internal {
			x.executePollInternal(func() { callback(events) })
			continue
		}
		x.safeExecute(func() {
			callback(events)
		})
		x.drainMicrotasks()
		if x.hardAbortRequested() {
			return
		}
	}
}
