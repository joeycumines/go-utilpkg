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
func (x *Loop) Log(level logiface.Level, modifier logiface.Modifier[logiface.Event]) {
	if x == nil {
		panic("eventloop: nil Loop")
	}
	logger := x.logger
	x.executeLoggerCallback(func() {
		_ = logger.Log(level, modifier)
	})
}

// safeExecute executes a task with panic recovery.
func (x *Loop) safeExecute(t func()) {
	x.safeExecuteTask(t, true)
}

// safeExecuteControl preserves callback ownership and panic containment for
// internal host plumbing without reporting it as a user callback.
func (x *Loop) safeExecuteControl(t func()) {
	x.safeExecuteTask(t, false)
}

func (x *Loop) safeExecuteTask(t func(), recordMetrics bool) bool {
	if t == nil {
		return false
	}
	if !x.beginCallbackExecution() {
		return false
	}
	if recordMetrics {
		x.releaseMicrotaskYield()
	}

	// Record callback execution-path duration if metrics are enabled.
	var start time.Time
	var end time.Time
	var outcome callbackOutcome
	if recordMetrics && x.metrics != nil {
		start = x.refreshTickTime()
	}

	defer func() {
		if r := recover(); r != nil {
			x.logCallbackError("eventloop: task panicked", r)
		}
		// Record the duration even when the callback panics or exits abnormally.
		if recordMetrics && x.metrics != nil {
			if end.IsZero() {
				end = x.refreshTickTime()
			}
			duration := end.Sub(start)
			x.metrics.recordCallback(duration, end, outcome.returned && !outcome.panicked)
		}
	}()

	outcome = x.executeCallback(t, true)
	x.logCallbackOutcome("task", outcome)
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
func (x *Loop) RunCallback(fn func()) error {
	if x == nil {
		panic("eventloop: nil Loop")
	}
	if fn == nil {
		panic("eventloop: nil RunCallback callback")
	}
	if err := x.runHostCallback(fn); err != nil {
		return err
	}
	x.drainMicrotasks()
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
func (x *Loop) RunCallbackDeferredCheckpoint(fn func()) error {
	if x == nil {
		panic("eventloop: nil Loop")
	}
	if fn == nil {
		panic("eventloop: nil RunCallbackDeferredCheckpoint callback")
	}
	return x.runHostCallback(fn)
}

func (x *Loop) runHostCallback(fn func()) error {
	if !x.ownsLocalQueues() {
		return ErrCallbackOwner
	}
	if !x.safeExecuteTask(fn, true) {
		return ErrLoopTerminated
	}
	return nil
}

// safeExecuteFn preserves a function-shaped callback executor for rejection
// reporting while sharing the scheduled-callback admission and metrics path.
func (x *Loop) safeExecuteFn(fn func()) {
	x.safeExecute(fn)
}

// safeExecuteFnDirect executes a callback that is already inside the isolated
// owner worker. Keeping this boundary separate lets queue drains amortize the
// worker handoff without paying a goroutine-id lookup for every callback.
func (x *Loop) safeExecuteFnDirect(fn func()) bool {
	if fn == nil {
		return false
	}
	if !x.beginCallbackExecution() {
		return false
	}
	var start time.Time
	successful := false
	if x.metrics != nil {
		start = x.refreshTickTime()
		defer func() {
			end := x.refreshTickTime()
			x.metrics.recordCallback(end.Sub(start), end, successful)
		}()
	}
	value, panicked := invokeCallback(fn)
	successful = !panicked
	x.logCallbackOutcome("task", callbackOutcome{
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
func (x *Loop) safeExecuteFallback(fn func()) {
	if fn == nil {
		return
	}

	x.logCallbackOutcome("task", x.executeCallback(fn, x.ownsLocalQueues()))
}

func (x *Loop) executeCallback(fn func(), owner bool) callbackOutcome {
	if owner {
		return x.executeOwnedCallback(fn)
	}
	return x.executeIsolatedCallback(fn)
}

func (x *Loop) executeOwnedCallback(fn func()) callbackOutcome {
	worker := x.callbackWorker
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
		x.callbackWorker = worker
		go x.runCallbackWorker(worker)
	}
	worker.requests <- fn
	outcome := <-worker.outcomes
	if !outcome.returned {
		<-worker.done
		x.callbackWorker = nil
	}
	return outcome
}

func (x *Loop) runCallbackWorker(worker *callbackWorker) {
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
				x.loopGoroutineID.Store(previousLoopOwner)
			}
			if terminalOwner {
				x.terminalDrainOwner.Store(previousTerminalOwner)
			}
			worker.outcomes <- outcome
		}
	}()
	workerID := goroutineid.Get()
	worker.ownerID.Store(workerID)
	for fn := range worker.requests {
		outcome = callbackOutcome{}
		previousLoopOwner = x.loopGoroutineID.Load()
		loopOwner = previousLoopOwner != 0
		terminalOwner = !loopOwner && x.terminalDraining.Load()
		if loopOwner {
			x.loopGoroutineID.Store(workerID)
		}
		if terminalOwner {
			previousTerminalOwner = x.terminalDrainOwner.Swap(workerID)
		}
		active = true
		worker.active.Store(true)
		outcome.panicValue, outcome.panicked = invokeCallback(fn)
		outcome.returned = true
		worker.active.Store(false)
		if loopOwner {
			x.loopGoroutineID.Store(previousLoopOwner)
		}
		if terminalOwner {
			x.terminalDrainOwner.Store(previousTerminalOwner)
		}
		active = false
		loopOwner = false
		terminalOwner = false
		worker.outcomes <- outcome
		fn = nil
		outcome = callbackOutcome{}
	}
}

func (x *Loop) stopCallbackWorker() {
	worker := x.callbackWorker
	if worker == nil {
		return
	}
	close(worker.requests)
	<-worker.done
	x.callbackWorker = nil
}

func (x *Loop) executeIsolatedCallback(fn func()) callbackOutcome {
	role := x.captureIsolatedCallbackRole(goroutineid.Get())
	result := make(chan callbackOutcome, 1)
	go func() {
		outcome := callbackOutcome{}
		defer func() { result <- outcome }()
		restore, ok := x.delegateIsolatedCallbackRole(role)
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
func (x *Loop) executeLoggerCallback(fn func()) {
	if fn == nil {
		return
	}
	callerID := goroutineid.Get()
	if _, recursive := x.loggerCallbackIDs.Load(callerID); recursive {
		return
	}
	_ = x.executeIsolatedCallback(func() {
		ownerID := goroutineid.Get()
		x.loggerCallbackIDs.Store(ownerID, struct{}{})
		defer x.loggerCallbackIDs.Delete(ownerID)
		fn()
	})
}

func (x *Loop) captureIsolatedCallbackRole(ownerID int64) isolatedCallbackRole {
	role := isolatedCallbackRole{
		ownerID:                 ownerID,
		loopOwner:               x.loopGoroutineID.Load() == ownerID,
		terminalDrainOwner:      x.terminalDraining.Load() && x.terminalDrainOwner.Load() == ownerID,
		terminalCompletionOwner: x.terminalCompletionOwner.Load() == ownerID,
	}
	_, role.promisifyWorker = x.promisifyWorkerIDs.Load(ownerID)
	_, role.loggerCallback = x.loggerCallbackIDs.Load(ownerID)
	// callbackWorker is owner-confined. Only its current logical loop/drain
	// owner may inspect it; arbitrary concurrent logger callers must not.
	if role.loopOwner || role.terminalDrainOwner {
		worker := x.callbackWorker
		if worker != nil && worker.active.Load() && worker.ownerID.Load() == ownerID {
			role.worker = worker
		}
	}
	return role
}

func (x *Loop) delegateIsolatedCallbackRole(role isolatedCallbackRole) (func(), bool) {
	ownerID := goroutineid.Get()
	loopOwner := false
	terminalDrainOwner := false
	terminalCompletionOwner := false
	workerOwner := false
	restore := func() {
		if role.loggerCallback {
			x.loggerCallbackIDs.Delete(ownerID)
		}
		if role.promisifyWorker {
			x.promisifyWorkerIDs.Delete(ownerID)
		}
		if workerOwner {
			role.worker.ownerID.CompareAndSwap(ownerID, role.ownerID)
		}
		if terminalCompletionOwner {
			x.terminalCompletionOwner.CompareAndSwap(ownerID, role.ownerID)
		}
		if terminalDrainOwner {
			x.terminalDrainOwner.CompareAndSwap(ownerID, role.ownerID)
		}
		if loopOwner {
			x.loopGoroutineID.CompareAndSwap(ownerID, role.ownerID)
		}
	}
	fail := func() (func(), bool) {
		restore()
		return func() {}, false
	}
	if role.loopOwner {
		loopOwner = x.loopGoroutineID.CompareAndSwap(role.ownerID, ownerID)
		if !loopOwner {
			return fail()
		}
	}
	if role.terminalDrainOwner {
		terminalDrainOwner = x.terminalDrainOwner.CompareAndSwap(role.ownerID, ownerID)
		if !terminalDrainOwner {
			return fail()
		}
	}
	if role.terminalCompletionOwner {
		terminalCompletionOwner = x.terminalCompletionOwner.CompareAndSwap(role.ownerID, ownerID)
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
		x.promisifyWorkerIDs.Store(ownerID, struct{}{})
	}
	if role.loggerCallback {
		x.loggerCallbackIDs.Store(ownerID, struct{}{})
	}
	return restore, true
}

func (x *Loop) logCallbackOutcome(subject string, outcome callbackOutcome) {
	if outcome.panicked {
		x.logCallbackError("eventloop: "+subject+" panicked", outcome.panicValue)
		return
	}
	if !outcome.returned {
		x.logCallbackError("eventloop: "+subject+" exited via runtime.Goexit", errCallbackGoexit)
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

func (x *Loop) beginCallbackExecution() bool {
	if x.testHooks != nil && x.testHooks.BeforeCallbackAdmission != nil {
		x.testHooks.BeforeCallbackAdmission()
	}
	x.callbackGateMu.Lock()
	if x.callbackGateMode == callbackGateClosed {
		x.callbackGateMu.Unlock()
		return false
	}
	x.callbackGateMu.Unlock()
	return true
}

// Structured Error Logging Helpers

// logError emits an instance-scoped error event. A disabled or panicking
// logger drops the diagnostic; logging must never alter loop control flow or
// escape through a process-global fallback.
func (x *Loop) logError(msg string, err error) {
	x.logErrorValue(msg, "", err)
}

func (x *Loop) logCallbackError(msg string, panicValue any) {
	x.logErrorValue(msg, "panic", panicValue)
}

func (x *Loop) logErrorValue(msg, key string, value any) {
	x.Log(logiface.LevelError, logiface.ModifierFunc[logiface.Event](func(event logiface.Event) error {
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
func (x *Loop) logCritical(msg string, err error) {
	x.Log(logiface.LevelCritical, logiface.ModifierFunc[logiface.Event](func(event logiface.Event) error {
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
func (x *Loop) Metrics() *Metrics {
	return x.metrics.snapshot()
}
