package gojagrpc

import (
	"context"
	"time"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
	grpcmetadata "google.golang.org/grpc/metadata"
)

// callOpts holds parsed options for a client RPC call.
type callOpts struct {
	ctx       context.Context
	cancel    context.CancelFunc
	onHeader  goja.Callable // optional callback for response headers
	onTrailer goja.Callable // optional callback for response trailers
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
// Must be called on the event loop goroutine.
func (m *Module) parseCallOpts(call goja.FunctionCall, argIndex int) *callOpts {
	ctx, cancel := context.WithCancel(context.Background())
	co := &callOpts{ctx: ctx, cancel: cancel}

	arg := call.Argument(argIndex)
	if arg == nil || goja.IsUndefined(arg) || goja.IsNull(arg) {
		return co
	}

	optsObj, ok := arg.(*goja.Object)
	if !ok {
		return co
	}

	m.applyTimeoutMs(optsObj, co)
	m.applySignal(optsObj, co.cancel)
	m.applyMetadata(optsObj, co)
	m.applyOnHeader(optsObj, co)
	m.applyOnTrailer(optsObj, co)

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
	if ms <= 0 {
		return
	}
	timeoutCtx, timeoutCancel := context.WithTimeout(co.ctx, time.Duration(ms)*time.Millisecond)
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
func (m *Module) applySignal(opts *goja.Object, cancel context.CancelFunc) {
	signalVal := opts.Get("signal")
	if signalVal == nil || goja.IsUndefined(signalVal) || goja.IsNull(signalVal) {
		return
	}

	signalObj, ok := signalVal.(*goja.Object)
	if !ok {
		return
	}

	// Access the native Go AbortSignal stored by the goja-eventloop
	// adapter's wrapAbortSignal method.
	nativeVal := signalObj.Get("_signal")
	if nativeVal == nil || goja.IsUndefined(nativeVal) {
		return
	}

	signal, ok := nativeVal.Export().(*goeventloop.AbortSignal)
	if !ok {
		return
	}

	if signal.Aborted() {
		cancel()
		return
	}

	signal.OnAbort(func(reason any) {
		cancel()
	})
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
	co.onHeader = fn
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
	co.onTrailer = fn
}
