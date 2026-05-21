package eventloop

import (
	"errors"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/joeycumines/goroutineid"
	"github.com/joeycumines/logiface"
)

var (
	errCallbackGoexit       = errors.New("eventloop: callback exited via runtime.Goexit")
	errCallbackRoleTransfer = errors.New("eventloop: callback ownership transfer failed")

	// ErrCallbackOwner reports an attempt to enter a host-adapter user callback
	// boundary without the loop's logical callback ownership.
	ErrCallbackOwner = errors.New("eventloop: callback execution requires the logical owner")
)

const (
	wakeSignalIdle uint32 = iota
	wakeSignalPending
	wakeSignalSubmitting
)

type terminalErrorBox struct {
	err error
}

type callbackGateMode uint8

const (
	callbackGateOpen callbackGateMode = iota
	callbackGateClosed
)

type callbackOutcome struct {
	panicValue any
	panicked   bool
	returned   bool
}

type callbackWorker struct {
	requests chan func()
	outcomes chan callbackOutcome
	done     chan struct{}
	ownerID  atomic.Int64
	active   atomic.Bool
}

type isolatedCallbackRole struct {
	worker                  *callbackWorker
	ownerID                 int64
	loopOwner               bool
	terminalDrainOwner      bool
	terminalCompletionOwner bool
	promisifyWorker         bool
	loggerCallback          bool
}

// Log synchronously attempts an instance-scoped diagnostic through the logger
// configured with [WithLogger]. It calls [logiface.Logger.Log] directly even
// when the configured logger is nil. Logger errors are ignored, and logger
// panic or runtime.Goexit is contained without discarding the caller's logical
// event-loop role. Use an asynchronous logger backend when delivery must not
// apply backpressure.
//
// Log panics if the Loop receiver is nil.
func (l *Loop) Log(level logiface.Level, modifier logiface.Modifier[logiface.Event]) {
	if l == nil {
		panic("eventloop: nil Loop")
	}
	logger := l.logger
	l.executeLoggerCallback(func() {
		_ = logger.Log(level, modifier)
	})
}

// safeExecute executes a task with panic recovery.
func (l *Loop) safeExecute(t func()) {
	l.safeExecuteTask(t, true)
}

// safeExecuteControl preserves callback ownership and panic containment for
// internal host plumbing without reporting it as a user callback.
func (l *Loop) safeExecuteControl(t func()) {
	l.safeExecuteTask(t, false)
}

func (l *Loop) safeExecuteTask(t func(), recordMetrics bool) bool {
	if t == nil {
		return false
	}
	if !l.beginCallbackExecution() {
		return false
	}
	if recordMetrics {
		l.releaseMicrotaskYield()
	}

	// Record callback execution-path duration if metrics are enabled.
	var start time.Time
	var end time.Time
	var outcome callbackOutcome
	if recordMetrics && l.metrics != nil {
		start = l.refreshTickTime()
	}

	defer func() {
		if r := recover(); r != nil {
			l.logCallbackError("eventloop: task panicked", r)
		}
		// Record the duration even when the callback panics or exits abnormally.
		if recordMetrics && l.metrics != nil {
			if end.IsZero() {
				end = l.refreshTickTime()
			}
			duration := end.Sub(start)
			l.metrics.recordCallback(duration, end, outcome.returned && !outcome.panicked)
		}
	}()

	outcome = l.executeCallback(t, true)
	l.logCallbackOutcome("task", outcome)
	return true
}

// RunCallback synchronously executes one user callback from host-adapter
// control plumbing, records its callback metrics, and completes the resulting
// nextTick and Promise-microtask checkpoint before returning. The caller must
// hold the logical loop or terminal-drain owner role; ordinary goroutines get
// [ErrCallbackOwner]. RunCallback is intended for adapters that multiplex
// several user callbacks behind one internal readiness or timer wake.
//
// RunCallback panics if the Loop receiver or fn is nil.
func (l *Loop) RunCallback(fn func()) error {
	if l == nil {
		panic("eventloop: nil Loop")
	}
	if fn == nil {
		panic("eventloop: nil RunCallback callback")
	}
	if err := l.runHostCallback(fn); err != nil {
		return err
	}
	l.drainMicrotasks()
	return nil
}

// RunCallbackDeferredCheckpoint synchronously executes one host-adapter user
// callback and records its callback metrics, but leaves the resulting nextTick
// and Promise-microtask checkpoint pending. It is intended for host algorithms
// that must update their own task-selection state after callback return before
// running the corresponding [Loop.RunMicrotaskCheckpoint].
//
// The caller must hold the logical loop or terminal-drain owner role; ordinary
// goroutines receive [ErrCallbackOwner].
//
// RunCallbackDeferredCheckpoint panics if the Loop receiver or fn is nil.
func (l *Loop) RunCallbackDeferredCheckpoint(fn func()) error {
	if l == nil {
		panic("eventloop: nil Loop")
	}
	if fn == nil {
		panic("eventloop: nil RunCallbackDeferredCheckpoint callback")
	}
	return l.runHostCallback(fn)
}

func (l *Loop) runHostCallback(fn func()) error {
	if !l.ownsLocalQueues() {
		return ErrCallbackOwner
	}
	if !l.safeExecuteTask(fn, true) {
		return ErrLoopTerminated
	}
	return nil
}

// safeExecuteFn preserves a function-shaped callback executor for rejection
// reporting while sharing the scheduled-callback admission and metrics path.
func (l *Loop) safeExecuteFn(fn func()) {
	l.safeExecute(fn)
}

// safeExecuteFnDirect executes a callback that is already inside the isolated
// owner worker. Keeping this boundary separate lets queue drains amortize the
// worker handoff without paying a goroutine-id lookup for every callback.
func (l *Loop) safeExecuteFnDirect(fn func()) bool {
	if fn == nil {
		return false
	}
	if !l.beginCallbackExecution() {
		return false
	}
	var start time.Time
	successful := false
	if l.metrics != nil {
		start = l.refreshTickTime()
		defer func() {
			end := l.refreshTickTime()
			l.metrics.recordCallback(end.Sub(start), end, successful)
		}()
	}
	value, panicked := invokeCallback(fn)
	successful = !panicked
	l.logCallbackOutcome("task", callbackOutcome{
		panicValue: value,
		panicked:   panicked,
		returned:   true,
	})
	return true
}

// safeExecuteFallback executes terminal diagnostics after normal callback
// admission has closed. Exact loop, terminal, and Promisify caller roles pass
// through the isolated boundary so the synchronous callback cannot join a
// completion barrier that depends on its caller. Genuinely post-terminal
// callers retain the unprivileged isolated fallback contract.
func (l *Loop) safeExecuteFallback(fn func()) {
	if fn == nil {
		return
	}

	l.logCallbackOutcome("task", l.executeCallback(fn, l.ownsLocalQueues()))
}

func (l *Loop) executeCallback(fn func(), owner bool) callbackOutcome {
	if owner {
		return l.executeOwnedCallback(fn)
	}
	return l.executeIsolatedCallback(fn)
}

func (l *Loop) executeOwnedCallback(fn func()) callbackOutcome {
	worker := l.callbackWorker
	// The normal owner path reaches this method while the worker is idle. Only
	// pay the goroutine-id lookup cost when a callback has synchronously entered
	// another callback boundary on the worker itself.
	if worker != nil && worker.active.Load() && worker.ownerID.Load() == goroutineid.Get() {
		value, panicked := invokeCallback(fn)
		return callbackOutcome{panicValue: value, panicked: panicked, returned: true}
	}
	if worker == nil {
		worker = &callbackWorker{
			requests: make(chan func()),
			outcomes: make(chan callbackOutcome),
			done:     make(chan struct{}),
		}
		l.callbackWorker = worker
		go l.runCallbackWorker(worker)
	}
	worker.requests <- fn
	outcome := <-worker.outcomes
	if !outcome.returned {
		<-worker.done
		l.callbackWorker = nil
	}
	return outcome
}

func (l *Loop) runCallbackWorker(worker *callbackWorker) {
	var outcome callbackOutcome
	var previousLoopOwner int64
	var previousTerminalOwner int64
	active := false
	loopOwner := false
	terminalOwner := false
	defer close(worker.done)
	defer func() {
		if active {
			worker.active.Store(false)
			if loopOwner {
				l.loopGoroutineID.Store(previousLoopOwner)
			}
			if terminalOwner {
				l.terminalDrainOwner.Store(previousTerminalOwner)
			}
			worker.outcomes <- outcome
		}
	}()
	workerID := goroutineid.Get()
	worker.ownerID.Store(workerID)
	for fn := range worker.requests {
		outcome = callbackOutcome{}
		previousLoopOwner = l.loopGoroutineID.Load()
		loopOwner = previousLoopOwner != 0
		terminalOwner = !loopOwner && l.terminalDraining.Load()
		if loopOwner {
			l.loopGoroutineID.Store(workerID)
		}
		if terminalOwner {
			previousTerminalOwner = l.terminalDrainOwner.Swap(workerID)
		}
		active = true
		worker.active.Store(true)
		outcome.panicValue, outcome.panicked = invokeCallback(fn)
		outcome.returned = true
		worker.active.Store(false)
		if loopOwner {
			l.loopGoroutineID.Store(previousLoopOwner)
		}
		if terminalOwner {
			l.terminalDrainOwner.Store(previousTerminalOwner)
		}
		active = false
		loopOwner = false
		terminalOwner = false
		worker.outcomes <- outcome
		fn = nil
		outcome = callbackOutcome{}
	}
}

func (l *Loop) stopCallbackWorker() {
	worker := l.callbackWorker
	if worker == nil {
		return
	}
	close(worker.requests)
	<-worker.done
	l.callbackWorker = nil
}

func (l *Loop) executeIsolatedCallback(fn func()) callbackOutcome {
	role := l.captureIsolatedCallbackRole(goroutineid.Get())
	result := make(chan callbackOutcome, 1)
	go func() {
		outcome := callbackOutcome{}
		defer func() { result <- outcome }()
		restore, ok := l.delegateIsolatedCallbackRole(role)
		if !ok {
			outcome.panicValue = errCallbackRoleTransfer
			outcome.panicked = true
			outcome.returned = true
			return
		}
		defer restore()
		outcome.panicValue, outcome.panicked = invokeCallback(fn)
		outcome.returned = true
	}()
	return <-result
}

// executeLoggerCallback contains every configured logger invocation on a
// sacrificial goroutine while preserving the exact synchronous caller role.
// Logging is still synchronous backpressure, but panic and runtime.Goexit can
// no longer terminate a Run owner, terminal finisher, or Promisify worker.
// The returned outcome is deliberately discarded by callers: reporting a
// logger failure through the same logger would recurse.
func (l *Loop) executeLoggerCallback(fn func()) {
	if fn == nil {
		return
	}
	callerID := goroutineid.Get()
	if _, recursive := l.loggerCallbackIDs.Load(callerID); recursive {
		return
	}
	_ = l.executeIsolatedCallback(func() {
		ownerID := goroutineid.Get()
		l.loggerCallbackIDs.Store(ownerID, struct{}{})
		defer l.loggerCallbackIDs.Delete(ownerID)
		fn()
	})
}

func (l *Loop) captureIsolatedCallbackRole(ownerID int64) isolatedCallbackRole {
	role := isolatedCallbackRole{
		ownerID:                 ownerID,
		loopOwner:               l.loopGoroutineID.Load() == ownerID,
		terminalDrainOwner:      l.terminalDraining.Load() && l.terminalDrainOwner.Load() == ownerID,
		terminalCompletionOwner: l.terminalCompletionOwner.Load() == ownerID,
	}
	_, role.promisifyWorker = l.promisifyWorkerIDs.Load(ownerID)
	_, role.loggerCallback = l.loggerCallbackIDs.Load(ownerID)
	// callbackWorker is owner-confined. Only its current logical loop/drain
	// owner may inspect it; arbitrary concurrent logger callers must not.
	if role.loopOwner || role.terminalDrainOwner {
		worker := l.callbackWorker
		if worker != nil && worker.active.Load() && worker.ownerID.Load() == ownerID {
			role.worker = worker
		}
	}
	return role
}

func (l *Loop) delegateIsolatedCallbackRole(role isolatedCallbackRole) (func(), bool) {
	ownerID := goroutineid.Get()
	loopOwner := false
	terminalDrainOwner := false
	terminalCompletionOwner := false
	workerOwner := false
	restore := func() {
		if role.loggerCallback {
			l.loggerCallbackIDs.Delete(ownerID)
		}
		if role.promisifyWorker {
			l.promisifyWorkerIDs.Delete(ownerID)
		}
		if workerOwner {
			role.worker.ownerID.CompareAndSwap(ownerID, role.ownerID)
		}
		if terminalCompletionOwner {
			l.terminalCompletionOwner.CompareAndSwap(ownerID, role.ownerID)
		}
		if terminalDrainOwner {
			l.terminalDrainOwner.CompareAndSwap(ownerID, role.ownerID)
		}
		if loopOwner {
			l.loopGoroutineID.CompareAndSwap(ownerID, role.ownerID)
		}
	}
	fail := func() (func(), bool) {
		restore()
		return func() {}, false
	}
	if role.loopOwner {
		loopOwner = l.loopGoroutineID.CompareAndSwap(role.ownerID, ownerID)
		if !loopOwner {
			return fail()
		}
	}
	if role.terminalDrainOwner {
		terminalDrainOwner = l.terminalDrainOwner.CompareAndSwap(role.ownerID, ownerID)
		if !terminalDrainOwner {
			return fail()
		}
	}
	if role.terminalCompletionOwner {
		terminalCompletionOwner = l.terminalCompletionOwner.CompareAndSwap(role.ownerID, ownerID)
		if !terminalCompletionOwner {
			return fail()
		}
	}
	if role.worker != nil {
		workerOwner = role.worker.ownerID.CompareAndSwap(role.ownerID, ownerID)
		if !workerOwner {
			return fail()
		}
	}
	if role.promisifyWorker {
		l.promisifyWorkerIDs.Store(ownerID, struct{}{})
	}
	if role.loggerCallback {
		l.loggerCallbackIDs.Store(ownerID, struct{}{})
	}
	return restore, true
}

func (l *Loop) logCallbackOutcome(subject string, outcome callbackOutcome) {
	if outcome.panicked {
		l.logCallbackError("eventloop: "+subject+" panicked", outcome.panicValue)
		return
	}
	if !outcome.returned {
		l.logCallbackError("eventloop: "+subject+" exited via runtime.Goexit", errCallbackGoexit)
	}
}

func invokeCallback(fn func()) (value any, panicked bool) {
	completed := false
	defer func() {
		if completed {
			return
		}
		value = recover()
		if value == nil {
			value = new(runtime.PanicNilError)
		}
		panicked = true
	}()
	fn()
	completed = true
	return nil, false
}

func (l *Loop) beginCallbackExecution() bool {
	if l.testHooks != nil && l.testHooks.BeforeCallbackAdmission != nil {
		l.testHooks.BeforeCallbackAdmission()
	}
	l.callbackGateMu.Lock()
	if l.callbackGateMode == callbackGateClosed {
		l.callbackGateMu.Unlock()
		return false
	}
	l.callbackGateMu.Unlock()
	return true
}

// Structured Error Logging Helpers

// logError emits an instance-scoped error event. A disabled or panicking
// logger drops the diagnostic; logging must never alter loop control flow or
// escape through a process-global fallback.
func (l *Loop) logError(msg string, err error) {
	l.logErrorValue(msg, "", err)
}

func (l *Loop) logCallbackError(msg string, panicValue any) {
	l.logErrorValue(msg, "panic", panicValue)
}

func (l *Loop) logErrorValue(msg, key string, value any) {
	l.Log(logiface.LevelError, logiface.ModifierFunc[logiface.Event](func(event logiface.Event) error {
		addLogString(event, "component", "eventloop")
		if key != "" {
			event.AddField(key, value)
		} else if err, ok := value.(error); ok && err != nil {
			addLogError(event, err)
		}
		addLogMessage(event, msg)
		return nil
	}))
}

// logCritical emits an instance-scoped critical event. Logging failures are
// contained and never replaced with process-global output.
func (l *Loop) logCritical(msg string, err error) {
	l.Log(logiface.LevelCritical, logiface.ModifierFunc[logiface.Event](func(event logiface.Event) error {
		addLogString(event, "component", "eventloop")
		if err != nil {
			addLogError(event, err)
		}
		addLogMessage(event, msg)
		return nil
	}))
}

func addLogString(event logiface.Event, key, value string) {
	if !event.AddString(key, value) {
		event.AddField(key, value)
	}
}

func addLogError(event logiface.Event, err error) {
	if !event.AddError(err) {
		event.AddField("err", err)
	}
}

func addLogMessage(event logiface.Event, msg string) {
	if msg != "" && !event.AddMessage(msg) {
		event.AddField("msg", msg)
	}
}

// Metrics returns a detached snapshot of the event loop's latest metrics.
//
// This method samples callback execution-duration percentiles (P50, P90, P95,
// P99) using exact sorting through five observations and the constant-space
// P-Square estimator afterward. Queue residence before callback admission is
// not measured.
//
// Thread Safety:
//
// This method is safe to call concurrently from any goroutine. It returns a
// coherent snapshot from one fully committed sampler epoch. Queue Current
// fields are the latest owner-turn sample rather than a synchronous queue
// inspection. The caller owns the returned value and may retain or modify it
// without affecting the Loop.
func (l *Loop) Metrics() *Metrics {
	return l.metrics.snapshot()
}
