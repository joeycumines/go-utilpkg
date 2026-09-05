package eventloop

import (
	"slices"
)

// SetQuiescenceHandler installs fn as the callback invoked by auto-exit when
// the loop first observes no ref'd liveness. The callback runs on the logical
// callback-owner goroutine before the terminal quiescing gate is committed, so
// it may schedule additional work through normal loop APIs. Returning true asks
// Run to resume the loop immediately; scheduled work is also detected by the
// subsequent Alive check. Passing nil clears the handler. Calls linearized after
// terminal transition have no effect because the callback can no longer run.
// A JavaScript integration registered by [BindJS] has an independent callback;
// replacing or clearing this host callback does not disturb that integration.
func (l *Loop) SetQuiescenceHandler(fn func() bool) {
	if l == nil {
		return
	}
	if l.testHooks != nil && l.testHooks.BeforeSetQuiescenceHandlerLock != nil {
		l.testHooks.BeforeSetQuiescenceHandlerLock()
	}
	l.livenessMu.Lock()
	defer l.livenessMu.Unlock()
	state := l.state.Load()
	if state == StateTerminating || state == StateTerminated {
		return
	}
	l.quiescenceMu.Lock()
	l.quiescenceHandler = fn
	l.quiescenceMu.Unlock()
}

func (l *Loop) runQuiescenceHandlers() (resume bool) {
	l.quiescenceMu.Lock()
	host := l.quiescenceHandler
	js := l.jsQuiescenceHandler
	l.quiescenceMu.Unlock()
	if l.runQuiescenceCallback("quiescence handler", host) {
		resume = true
	}
	if l.runQuiescenceCallback("JS quiescence handler", js) {
		resume = true
	}
	return resume
}

func (l *Loop) runQuiescenceCallback(label string, fn func() bool) (resume bool) {
	if fn == nil {
		return false
	}
	if !l.beginCallbackExecution() {
		return false
	}
	outcome := l.executeCallback(func() { resume = fn() }, true)
	l.logCallbackOutcome(label, outcome)
	if outcome.panicked || !outcome.returned {
		return false
	}
	return resume
}

func (l *Loop) hasTimersPending() bool {
	return len(l.timers) > 0
}

func (l *Loop) snapshotCheckJobsLocked() []checkJob {
	jobs := make([]checkJob, 0, len(l.checkJobs)+len(l.closeJobs))
	jobs = append(jobs, l.checkJobs...)
	jobs = append(jobs, l.closeJobs...)
	return jobs
}

func (l *Loop) hasLiveCheckJob(jobs []checkJob) bool {
	return slices.ContainsFunc(jobs, l.checkJobAlive)
}

func (l *Loop) checkJobAlive(job checkJob) (alive bool) {
	if job.fn == nil {
		return false
	}
	if job.refed == nil {
		return true
	}
	if !l.ownsLocalQueues() {
		// Dynamic liveness predicates are user code and may be goroutine-affine or
		// re-enter lifecycle APIs. External observers conservatively retain the job;
		// only the loop owner may evaluate the predicate exactly.
		return true
	}
	if l.testHooks != nil && l.testHooks.BeforeCheckPredicateAdmission != nil {
		l.testHooks.BeforeCheckPredicateAdmission()
	}
	if !l.beginCallbackExecution() {
		return false
	}
	outcome := l.executeCallback(func() { alive = job.refed() }, true)
	l.logCallbackOutcome("check liveness predicate", outcome)
	if outcome.panicked || !outcome.returned {
		return false
	}
	return alive
}

func (l *Loop) ownerPhaseWorkAlive() bool {
	if l.ownerCheckCount.Load() == 0 && l.ownerCloseCount.Load() == 0 {
		return false
	}
	if !l.ownsLocalQueues() {
		// External observers cannot safely inspect owner-local phase queues without
		// racing the loop owner. Be conservative; the loop owner performs the exact
		// predicate evaluation during auto-exit decisions.
		return true
	}
	return l.hasLiveCheckJob(l.snapshotOwnerCheckJobs())
}

// Alive reports whether the event loop has ref'd pending work.
// When false, all ref'd timers have fired, all queues and detached phase
// batches are empty, no Promisify goroutines are in-flight, and no I/O FDs are
// registered.
// A terminated loop reports false even if a Promisify user function continues
// after immediate Close, because that function can no longer keep the loop alive.
//
// Analogous to libuv's uv_loop_alive().
// Safe to call from any goroutine, including event loop callbacks.
// Uses epoch-based consistency to prevent false negatives under
// concurrent submission: reads submissionEpoch before and after checks;
// if it changed (concurrent work added), retries up to 3 times.
// After max retries, conservatively returns true.
//
// Check ordering: atomic counters are checked first (no lock acquisition)
// to reduce mutex contention under high load. Queue checks require mutex
// acquisition and are performed only when all atomic checks return zero.
// External callers may observe a conservative true while owner-local or
// detached check, close, or auxiliary phase work is pending; only the loop
// owner evaluates dynamic liveness predicates exactly to avoid racing
// owner-local queues or running user predicate code from an arbitrary goroutine.
func (l *Loop) Alive() bool {
	state := l.state.Load()
	if (state == StateTerminating || state == StateTerminated) && l.immediateCloseWon() {
		return false
	}
	// A privileged owner observation is ordered after every foreign operation
	// that returned before this call. External observers retain the conservative
	// snapshot path and never mutate owner-only state.
	if l.commandIngressPending.Load() && l.ownsLocalQueues() {
		l.drainCommandIngress()
	}
	const maxRetries = 3
	for range maxRetries {
		epoch := l.submissionEpoch.Load()

		// Fast path: check atomic counters and owner-local queues first, avoiding
		// command-ingress locking unless every cheap liveness signal is empty.
		if l.refedTimerCount.Load() > 0 {
			return true
		}
		if l.promisifyCount.Load() > 0 {
			return true
		}
		if l.userIOFDCount.Load() > 0 {
			return true
		}
		if l.microtaskYield.Load() {
			return true
		}
		if !l.microtaskQueuesEmpty() {
			return true
		}
		if l.ownerInternalCount.Load() > 0 || l.ownerExternalCount.Load() > 0 || l.activePhaseJobCount.Load() > 0 {
			return true
		}
		if l.ownerPhaseWorkAlive() {
			return true
		}

		l.externalMu.Lock()
		commands := l.snapshotCommandsLocked()
		checkJobs := l.snapshotCheckJobsLocked()
		l.externalMu.Unlock()
		if l.hasLiveCheckJob(checkJobs) || l.hasLiveCommand(commands) {
			return true
		}
		if l.refedTimerCount.Load() > 0 || l.promisifyCount.Load() > 0 || l.userIOFDCount.Load() > 0 || l.microtaskYield.Load() || !l.microtaskQueuesEmpty() || l.ownerInternalCount.Load() > 0 || l.ownerExternalCount.Load() > 0 || l.activePhaseJobCount.Load() > 0 || l.ownerPhaseWorkAlive() {
			return true
		}

		if l.testHooks != nil && l.testHooks.BeforeAliveEpochValidation != nil {
			l.testHooks.BeforeAliveEpochValidation()
		}
		// Validate epoch: if unchanged, no concurrent work was added during checks
		if l.submissionEpoch.Load() == epoch {
			return false
		}
		// Epoch changed — concurrent work was added. Retry.
	}
	// Max retries exhausted — conservatively return true (safer to say alive when unsure)
	return true
}

// HasMacrotaskWork reports whether the loop has liveness or queued phase work
// outside the nextTick / microtask queues.
// A loop terminated by immediate Close reports false even while a user function
// outlives Close.
//
// This method is safe to call from any goroutine. Detached check, close, and
// auxiliary phase batches remain visible until the whole accepted batch
// completes. External callers conservatively retain dynamic check work without
// invoking its owner-affine liveness predicate.
func (l *Loop) HasMacrotaskWork() bool {
	state := l.state.Load()
	if (state == StateTerminating || state == StateTerminated) && l.immediateCloseWon() {
		return false
	}
	if l.commandIngressPending.Load() && l.ownsLocalQueues() {
		l.drainCommandIngress()
	}
	const maxRetries = 3
	for range maxRetries {
		epoch := l.submissionEpoch.Load()

		if l.refedTimerCount.Load() > 0 || l.promisifyCount.Load() > 0 || l.userIOFDCount.Load() > 0 || l.microtaskYield.Load() {
			return true
		}
		if l.ownerInternalCount.Load() > 0 || l.ownerExternalCount.Load() > 0 || l.activePhaseJobCount.Load() > 0 {
			return true
		}
		if l.ownerPhaseWorkAlive() {
			return true
		}

		l.externalMu.Lock()
		commands := l.snapshotCommandsLocked()
		checkJobs := l.snapshotCheckJobsLocked()
		l.externalMu.Unlock()
		if l.hasLiveCheckJob(checkJobs) || l.hasMacrotaskCommand(commands) {
			return true
		}
		if l.refedTimerCount.Load() > 0 || l.promisifyCount.Load() > 0 || l.userIOFDCount.Load() > 0 || l.microtaskYield.Load() || l.ownerInternalCount.Load() > 0 || l.ownerExternalCount.Load() > 0 || l.activePhaseJobCount.Load() > 0 || l.ownerPhaseWorkAlive() {
			return true
		}

		if l.submissionEpoch.Load() == epoch {
			return false
		}
	}
	return true
}
