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
func (x *Loop) SetQuiescenceHandler(fn func() bool) {
	if x == nil {
		return
	}
	if x.testHooks != nil && x.testHooks.BeforeSetQuiescenceHandlerLock != nil {
		x.testHooks.BeforeSetQuiescenceHandlerLock()
	}
	x.livenessMu.Lock()
	defer x.livenessMu.Unlock()
	state := x.state.Load()
	if state == StateTerminating || state == StateTerminated {
		return
	}
	x.quiescenceMu.Lock()
	x.quiescenceHandler = fn
	x.quiescenceMu.Unlock()
}

func (x *Loop) runQuiescenceHandlers() (resume bool) {
	x.quiescenceMu.Lock()
	host := x.quiescenceHandler
	js := x.jsQuiescenceHandler
	x.quiescenceMu.Unlock()
	if x.runQuiescenceCallback("quiescence handler", host) {
		resume = true
	}
	if x.runQuiescenceCallback("JS quiescence handler", js) {
		resume = true
	}
	return resume
}

func (x *Loop) runQuiescenceCallback(label string, fn func() bool) (resume bool) {
	if fn == nil {
		return false
	}
	if !x.beginCallbackExecution() {
		return false
	}
	outcome := x.executeCallback(func() { resume = fn() }, true)
	x.logCallbackOutcome(label, outcome)
	if outcome.panicked || !outcome.returned {
		return false
	}
	return resume
}

func (x *Loop) hasTimersPending() bool {
	return len(x.timers) > 0
}

func (x *Loop) snapshotCheckJobsLocked() []checkJob {
	jobs := make([]checkJob, 0, len(x.checkJobs)+len(x.closeJobs))
	jobs = append(jobs, x.checkJobs...)
	jobs = append(jobs, x.closeJobs...)
	return jobs
}

func (x *Loop) hasLiveCheckJob(jobs []checkJob) bool {
	return slices.ContainsFunc(jobs, x.checkJobAlive)
}

func (x *Loop) checkJobAlive(job checkJob) (alive bool) {
	if job.fn == nil {
		return false
	}
	if job.refed == nil {
		return true
	}
	if !x.ownsLocalQueues() {
		// Dynamic liveness predicates are user code and may be goroutine-affine or
		// re-enter lifecycle APIs. External observers conservatively retain the job;
		// only the loop owner may evaluate the predicate exactly.
		return true
	}
	if x.testHooks != nil && x.testHooks.BeforeCheckPredicateAdmission != nil {
		x.testHooks.BeforeCheckPredicateAdmission()
	}
	if !x.beginCallbackExecution() {
		return false
	}
	outcome := x.executeCallback(func() { alive = job.refed() }, true)
	x.logCallbackOutcome("check liveness predicate", outcome)
	if outcome.panicked || !outcome.returned {
		return false
	}
	return alive
}

func (x *Loop) ownerPhaseWorkAlive() bool {
	if x.ownerCheckCount.Load() == 0 && x.ownerCloseCount.Load() == 0 {
		return false
	}
	if !x.ownsLocalQueues() {
		// External observers cannot safely inspect owner-local phase queues without
		// racing the loop owner. Be conservative; the loop owner performs the exact
		// predicate evaluation during auto-exit decisions.
		return true
	}
	return x.hasLiveCheckJob(x.snapshotOwnerCheckJobs())
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
func (x *Loop) Alive() bool {
	state := x.state.Load()
	if (state == StateTerminating || state == StateTerminated) && x.immediateCloseWon() {
		return false
	}
	// A privileged owner observation is ordered after every foreign operation
	// that returned before this call. External observers retain the conservative
	// snapshot path and never mutate owner-only state.
	if x.commandIngressPending.Load() && x.ownsLocalQueues() {
		x.drainCommandIngress()
	}
	const maxRetries = 3
	for range maxRetries {
		epoch := x.submissionEpoch.Load()

		// Fast path: check atomic counters and owner-local queues first, avoiding
		// command-ingress locking unless every cheap liveness signal is empty.
		if x.refedTimerCount.Load() > 0 {
			return true
		}
		if x.promisifyCount.Load() > 0 {
			return true
		}
		if x.userIOFDCount.Load() > 0 {
			return true
		}
		if x.microtaskYield.Load() {
			return true
		}
		if !x.microtaskQueuesEmpty() {
			return true
		}
		if x.ownerInternalCount.Load() > 0 || x.ownerExternalCount.Load() > 0 || x.activePhaseJobCount.Load() > 0 {
			return true
		}
		if x.ownerPhaseWorkAlive() {
			return true
		}

		x.externalMu.Lock()
		commands := x.snapshotCommandsLocked()
		checkJobs := x.snapshotCheckJobsLocked()
		x.externalMu.Unlock()
		if x.hasLiveCheckJob(checkJobs) || x.hasLiveCommand(commands) {
			return true
		}
		if x.refedTimerCount.Load() > 0 || x.promisifyCount.Load() > 0 || x.userIOFDCount.Load() > 0 || x.microtaskYield.Load() || !x.microtaskQueuesEmpty() || x.ownerInternalCount.Load() > 0 || x.ownerExternalCount.Load() > 0 || x.activePhaseJobCount.Load() > 0 || x.ownerPhaseWorkAlive() {
			return true
		}

		if x.testHooks != nil && x.testHooks.BeforeAliveEpochValidation != nil {
			x.testHooks.BeforeAliveEpochValidation()
		}
		// Validate epoch: if unchanged, no concurrent work was added during checks
		if x.submissionEpoch.Load() == epoch {
			return false
		}
		// Epoch changed — concurrent work was added. Retry.
	}
	// Max retries exhausted — conservatively return true (safer to say alive when unsure)
	return true
}

// HasMacrotaskWork reports whether the loop has liveness or queued phase work
// outside the nextTick / promise-microtask queues.
// A loop terminated by immediate Close reports false even while a user function
// outlives Close.
//
// This method is safe to call from any goroutine. Detached check, close, and
// auxiliary phase batches remain visible until the whole accepted batch
// completes. External callers conservatively retain dynamic check work without
// invoking its owner-affine liveness predicate.
func (x *Loop) HasMacrotaskWork() bool {
	state := x.state.Load()
	if (state == StateTerminating || state == StateTerminated) && x.immediateCloseWon() {
		return false
	}
	if x.commandIngressPending.Load() && x.ownsLocalQueues() {
		x.drainCommandIngress()
	}
	const maxRetries = 3
	for range maxRetries {
		epoch := x.submissionEpoch.Load()

		if x.refedTimerCount.Load() > 0 || x.promisifyCount.Load() > 0 || x.userIOFDCount.Load() > 0 || x.microtaskYield.Load() {
			return true
		}
		if x.ownerInternalCount.Load() > 0 || x.ownerExternalCount.Load() > 0 || x.activePhaseJobCount.Load() > 0 {
			return true
		}
		if x.ownerPhaseWorkAlive() {
			return true
		}

		x.externalMu.Lock()
		commands := x.snapshotCommandsLocked()
		checkJobs := x.snapshotCheckJobsLocked()
		x.externalMu.Unlock()
		if x.hasLiveCheckJob(checkJobs) || x.hasMacrotaskCommand(commands) {
			return true
		}
		if x.refedTimerCount.Load() > 0 || x.promisifyCount.Load() > 0 || x.userIOFDCount.Load() > 0 || x.microtaskYield.Load() || x.ownerInternalCount.Load() > 0 || x.ownerExternalCount.Load() > 0 || x.activePhaseJobCount.Load() > 0 || x.ownerPhaseWorkAlive() {
			return true
		}

		if x.submissionEpoch.Load() == epoch {
			return false
		}
	}
	return true
}
