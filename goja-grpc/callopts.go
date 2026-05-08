package gojagrpc

import (
	"context"
	"errors"
	"math"
	"sync"
	"time"

	"github.com/joeycumines/goja"
	grpcmetadata "google.golang.org/grpc/metadata"
)

// callOpts holds parsed options for a client RPC call.
type callOpts struct {
	module            *Module
	ctx               context.Context
	cancel            context.CancelFunc
	signalCleanup     func() // optional AbortSignal listener cleanup, safe after loop Submit failure
	callbacks         *callCallbacks
	signalCleanupOnce sync.Once // makes cleanupSignal safe for concurrent stream completion/failure paths
	control           *operationControl
	rootID            supervisorChildID
}

// operationControl is the Go-only join state for one client or reflection
// root. It contains no Module, Goja value, callback, or owner projection.
//
// Construction owns the initial obligation. It must publish either a transport
// release signal or the absence of one before worker completion can release the
// control. stop only records local module shutdown and cancels the context; it
// never selects RPC terminal truth or completes construction.
type operationControl struct {
	ctx        context.Context
	cancel     context.CancelFunc
	done       chan struct{}
	binding    chan (<-chan struct{})
	workerDone chan struct{}

	stopOnce   sync.Once
	workerOnce sync.Once
	doneOnce   sync.Once

	mu        sync.Mutex
	bound     bool
	abandoned bool
	stopped   bool
	stopErr   error
}

type workerRoot struct {
	owner   *ownerDispatcher
	control *operationControl
	id      supervisorChildID
}

func (r workerRoot) finish(err error) {
	if r.owner != nil && r.id != 0 {
		r.owner.disposeOwnerRootWorker(r.id, canonicalWorkerDisposition(err))
	}
	if r.control != nil {
		r.control.finishWorker()
	}
}

func (r workerRoot) failConstruction(err error) {
	if r.owner != nil && r.id != 0 {
		r.owner.disposeOwnerRootWorker(r.id, canonicalWorkerDisposition(err))
	}
	if r.control != nil {
		r.control.finishNoTransport()
	}
}

func canonicalWorkerDisposition(err error) error {
	if err == nil {
		return errModuleUnavailable
	}
	return canonicalWorkerError(err)
}

func (co *callOpts) workerRoot() workerRoot {
	if co == nil || co.module == nil {
		return workerRoot{}
	}
	return workerRoot{
		owner:   co.module.dispatcher,
		control: co.control,
		id:      co.rootID,
	}
}

type callCallbacks struct {
	onHeader  ownerCallbackID
	onTrailer ownerCallbackID
}

func (co *callOpts) headerCallback() ownerCallbackID {
	if co == nil || co.callbacks == nil {
		return ownerCallbackID{}
	}
	return co.callbacks.onHeader
}

func (co *callOpts) trailerCallback() ownerCallbackID {
	if co == nil || co.callbacks == nil {
		return ownerCallbackID{}
	}
	return co.callbacks.onTrailer
}

func (co *callOpts) cleanupSignal() {
	if co == nil {
		return
	}
	co.signalCleanupOnce.Do(func() {
		if co.signalCleanup == nil {
			return
		}
		cleanup := co.signalCleanup
		co.signalCleanup = nil
		cleanup()
	})
}

func (co *callOpts) register() error {
	if co == nil || co.module == nil {
		return errModuleClosed
	}
	if co.rootID == 0 {
		id, err := co.module.control.reserve(supervisorOperation)
		if err != nil {
			co.cancel()
			return err
		}
		co.rootID = id
		if err := co.module.ensureOwnerRoot(id); err != nil {
			co.module.control.abandon(id)
			co.cancel()
			return err
		}
		if err := co.module.addOwnerRootDisposer(id, func(error) {
			co.cleanupSignal()
		}); err != nil {
			co.module.control.abandon(id)
			co.cancel()
			return err
		}
	}
	control := newOperationControl(co.ctx, co.cancel)
	co.control = control
	if err := co.module.executor.install(co.rootID, control); err != nil {
		co.module.control.abandon(co.rootID)
		co.cancel()
		return err
	}
	if err := co.module.control.activate(co.rootID); err != nil {
		control.stop(errModuleUnavailable)
		co.module.disposeOwnerRootOwner(co.rootID, errModuleUnavailable)
		return err
	}
	co.module.activateOwnerRoot(co.rootID)
	if err := control.localStopError(nil); err != nil {
		return err
	}
	return nil
}

func newOperationControl(
	ctx context.Context,
	cancel context.CancelFunc,
) *operationControl {
	control := &operationControl{
		ctx:        ctx,
		cancel:     cancel,
		done:       make(chan struct{}),
		binding:    make(chan (<-chan struct{}), 1),
		workerDone: make(chan struct{}),
	}
	go control.join()
	return control
}

func (c *operationControl) join() {
	release := <-c.binding
	<-c.workerDone
	if release != nil {
		<-release
	}
	c.doneOnce.Do(func() { close(c.done) })
}

// bindRelease publishes the construction result exactly once. A nil release
// means the generic transport API or worker completion is the release proof.
func (c *operationControl) bindRelease(release <-chan struct{}) error {
	if c == nil {
		return errModuleClosed
	}
	c.mu.Lock()
	if c.bound {
		stopErr := c.stopErr
		abandoned := c.abandoned
		c.mu.Unlock()
		if abandoned {
			if stopErr == nil {
				return errModuleUnavailable
			}
			return stopErr
		}
		return errors.New("gojagrpc: operation transport already bound")
	}
	c.bound = true
	stopErr := c.stopErr
	stopped := c.stopped
	c.mu.Unlock()
	c.binding <- release
	if stopped {
		if stopErr == nil {
			return errModuleUnavailable
		}
		return stopErr
	}
	return nil
}

func (c *operationControl) stop(err error) {
	if c == nil {
		return
	}
	c.stopOnce.Do(func() {
		if err == nil {
			err = errModuleUnavailable
		}
		c.mu.Lock()
		c.stopped = true
		c.stopErr = canonicalWorkerDisposition(err)
		abandon := !c.bound
		if abandon {
			// Close may observe a reserved operation before construction has
			// published any transport. Claim and acknowledge that construction
			// obligation here; a late bind observes stopErr and cannot publish
			// a transport release into this retired control.
			c.bound = true
			c.abandoned = true
		}
		c.mu.Unlock()
		c.cancel()
		if abandon {
			c.binding <- nil
			c.finishWorker()
		}
	})
}

func (co *callOpts) finishOwner() {
	if co == nil {
		return
	}
	if co.module != nil && co.rootID != 0 {
		co.module.disposeOwnerRootOwner(co.rootID, errModuleUnavailable)
	}
	co.finishControl()
}

func (co *callOpts) finishControl() {
	if co == nil {
		return
	}
	if co.control == nil {
		if co.module != nil && co.rootID != 0 {
			co.module.control.abandon(co.rootID)
		}
		co.cancel()
		return
	}
	co.control.finishNoTransport()
}

func (c *operationControl) finishNoTransport() {
	if c == nil {
		return
	}
	_ = c.bindRelease(nil)
	c.finishWorker()
}

func (c *operationControl) finishWorker() {
	if c == nil {
		return
	}
	c.workerOnce.Do(func() {
		c.cancel()
		close(c.workerDone)
	})
}

func (c *operationControl) wait() <-chan struct{} {
	if c == nil || c.done == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return c.done
}

func (*operationControl) result() error { return nil }

func (c *operationControl) localStopError(fallback error) error {
	if c == nil {
		return fallback
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped && c.stopErr != nil {
		return c.stopErr
	}
	return fallback
}

// parseCallOpts extracts options from the given argument index of a
// client RPC method call. Supports:
//   - signal: An AbortSignal for cancelling the RPC
//   - metadata: A metadata wrapper for outgoing headers
//   - onHeader: Callback invoked with response headers
//   - onTrailer: Callback invoked with response trailers
//   - timeoutMs: RPC deadline in milliseconds
//
// The returned callOpts always has a valid ctx and cancel. The caller
// should call cancel() when the RPC completes to release resources.
//
// Must be called by the current logical adapter callback owner.
func (m *Module) parseCallOpts(call goja.FunctionCall, argIndex int) *callOpts {
	return m.parseCallOptsMode(call, argIndex, true, true)
}

func (m *Module) parseCallOptsDeferred(call goja.FunctionCall, argIndex int) *callOpts {
	// Interceptor setup must not allocate timeout or AbortSignal resources until
	// the interceptor chain actually calls next(req). A short-circuiting
	// interceptor has no RPC terminal path that could otherwise release them.
	return m.parseCallOptsMode(call, argIndex, false, false)
}

type deferredCallOptions struct {
	signal       goja.Value
	timeoutStart time.Time
	timeout      time.Duration
	hasSignal    bool
	hasTimeout   bool
}

func callTimeoutDuration(ms int64) (time.Duration, bool) {
	if ms <= 0 {
		return 0, false
	}
	if ms > math.MaxInt64/int64(time.Millisecond) {
		return time.Duration(math.MaxInt64), true
	}
	return time.Duration(ms) * time.Millisecond, true
}

func (m *Module) snapshotCallOptions(opts *goja.Object) deferredCallOptions {
	if opts == nil {
		return deferredCallOptions{}
	}
	var snap deferredCallOptions
	if val := opts.Get("timeoutMs"); val != nil && !goja.IsUndefined(val) && !goja.IsNull(val) {
		if timeout, ok := callTimeoutDuration(val.ToInteger()); ok {
			snap.timeout = timeout
			snap.timeoutStart = time.Now()
			snap.hasTimeout = true
		}
	}
	if val := opts.Get("signal"); val != nil && !goja.IsUndefined(val) && !goja.IsNull(val) {
		if _, ok := val.(*goja.Object); ok {
			snap.signal = val
			snap.hasSignal = true
		}
	}
	return snap
}

func (m *Module) applySnapshot(snap deferredCallOptions, co *callOpts) {
	if co == nil {
		return
	}
	if snap.hasTimeout {
		deadline := snap.timeoutStart.Add(snap.timeout)
		timeoutCtx, timeoutCancel := context.WithDeadline(co.ctx, deadline)
		oldCancel := co.cancel
		co.ctx = timeoutCtx
		co.cancel = func() {
			timeoutCancel()
			oldCancel()
		}
	}
	if snap.hasSignal {
		m.applySignalValue(snap.signal, co)
	}
}

func (m *Module) parseCallOptsMode(call goja.FunctionCall, argIndex int, includeSignal bool, includeTimeout bool) *callOpts {
	ctx, cancel := context.WithCancel(m.ctx)
	co := &callOpts{module: m, ctx: ctx, cancel: cancel}
	if id, err := m.control.reserve(supervisorOperation); err == nil {
		co.rootID = id
		if rootErr := m.ensureOwnerRoot(id); rootErr != nil {
			m.control.abandon(id)
			co.rootID = 0
		} else {
			_ = m.addOwnerRootDisposer(id, func(error) {
				co.cleanupSignal()
			})
		}
	}
	returned := false
	defer func() {
		reason := recover()
		if reason != nil {
			co.finishOwner()
			panic(reason)
		}
		if !returned {
			co.finishOwner()
		}
	}()

	arg := call.Argument(argIndex)
	if arg != nil && !goja.IsUndefined(arg) && !goja.IsNull(arg) {
		if optsObj, ok := arg.(*goja.Object); ok {
			if includeTimeout {
				m.applyTimeoutMs(optsObj, co)
			}
			if includeSignal {
				m.applySignal(optsObj, co)
			}
			m.applyMetadata(optsObj, co)
			m.applyOnHeader(optsObj, co)
			m.applyOnTrailer(optsObj, co)
		}
	}

	returned = true
	return co
}

// applyTimeoutMs extracts a timeoutMs value from the options object
// and wraps the context with a deadline. The cancel function is updated
// to cancel the timeout context. Must be called before applySignal.
func (m *Module) applyTimeoutMs(opts *goja.Object, co *callOpts) {
	val := opts.Get("timeoutMs")
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return
	}
	ms := val.ToInteger()
	timeout, ok := callTimeoutDuration(ms)
	if !ok {
		return
	}
	timeoutCtx, timeoutCancel := context.WithTimeout(co.ctx, timeout)
	// Chain cancels: cancelling timeoutCancel also cancels timeoutCtx.
	// The old co.cancel still cancels the parent context.
	oldCancel := co.cancel
	co.ctx = timeoutCtx
	co.cancel = func() {
		timeoutCancel()
		oldCancel()
	}
}

// applySignal extracts an AbortSignal from the options object and
// wires it to cancel the context when the signal aborts.
func (m *Module) applySignal(opts *goja.Object, co *callOpts) {
	if co == nil {
		return
	}
	signalVal := opts.Get("signal")
	m.applySignalValue(signalVal, co)
}

func (m *Module) applySignalValue(signalVal goja.Value, co *callOpts) {
	if co == nil {
		return
	}
	if signalVal == nil || goja.IsUndefined(signalVal) || goja.IsNull(signalVal) {
		return
	}

	if _, ok := signalVal.(*goja.Object); !ok {
		return
	}

	cleanup, aborted, ok := m.adapter.TrackAbortSignal(signalVal, co.cancel)
	if ok {
		if aborted {
			co.cancel()
			return
		}
		co.signalCleanup = cleanup
		return
	}
}

// applyMetadata extracts a metadata wrapper from the options object
// and attaches it as outgoing gRPC metadata on the context.
func (m *Module) applyMetadata(opts *goja.Object, co *callOpts) {
	metadataVal := opts.Get("metadata")
	if metadataVal == nil || goja.IsUndefined(metadataVal) || goja.IsNull(metadataVal) {
		return
	}

	md := m.metadataToGo(metadataVal)
	if md != nil {
		co.ctx = grpcmetadata.NewOutgoingContext(co.ctx, md)
	}
}

// applyOnHeader extracts an onHeader callback from the options object.
// The callback receives a metadata wrapper when response headers arrive.
func (m *Module) applyOnHeader(opts *goja.Object, co *callOpts) {
	val := opts.Get("onHeader")
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return
	}
	fn, ok := goja.AssertFunction(val)
	if !ok {
		panic(m.runtime.NewTypeError("onHeader must be a function"))
	}
	if co.callbacks == nil {
		co.callbacks = new(callCallbacks)
	}
	co.callbacks.onHeader = m.rememberOwnerCallback(co.rootID, fn)
}

// applyOnTrailer extracts an onTrailer callback from the options object.
// The callback receives a metadata wrapper when response trailers arrive.
func (m *Module) applyOnTrailer(opts *goja.Object, co *callOpts) {
	val := opts.Get("onTrailer")
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return
	}
	fn, ok := goja.AssertFunction(val)
	if !ok {
		panic(m.runtime.NewTypeError("onTrailer must be a function"))
	}
	if co.callbacks == nil {
		co.callbacks = new(callCallbacks)
	}
	co.callbacks.onTrailer = m.rememberOwnerCallback(co.rootID, fn)
}
