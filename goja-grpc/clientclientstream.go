package gojagrpc

import (
	"github.com/joeycumines/goja"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// makeClientStreamMethod creates the client method for a client-streaming RPC.
func (m *Module) makeClientStreamMethod(
	cc grpc.ClientConnInterface,
	fullMethod string,
	inputDesc protoreflect.MessageDescriptor,
	outputDesc protoreflect.MessageDescriptor,
) goja.Value {
	return m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		options := m.parseCallOpts(call, 0)
		// The projection is created eagerly (not inside the resolve
		// projection): its root disposer removes the stream worker entry from
		// the executor map at disposal, and a construction failure that
		// prevents the stream promise from resolving must still trigger that
		// removal — otherwise the worker entry would leak until the module
		// dies. The stream id equals the reserved root id by construction.
		projection := m.newClientStreamProjection(options.rootID, inputDesc, outputDesc)
		promise := m.newOwnerPromise(options.rootID, func(ownerResult) any {
			return m.newClientStreamCall(projection)
		}, nil)
		if !promise.admitted() {
			options.finishOwner()
			return promise.value
		}
		if err := options.register(); err != nil {
			_ = m.rejectOwnerPromiseInline(promise.id, err)
			options.finishOwner()
			return promise.value
		}
		if err := options.control.ctx.Err(); err != nil {
			_ = m.rejectOwnerPromiseInline(promise.id, err)
			options.finishOwner()
			return promise.value
		}

		promiseID := promise.id
		root := options.workerRoot()
		onHeader := options.headerCallback()
		onTrailer := options.trailerCallback()
		streams := m.streams
		defaultInproc := m.defaultInprocChannel(cc)
		go func() {
			boundary := rootWorkerBoundary{root: root, promise: promiseID}
			boundary.run(func() {
				stream, err := cc.NewStream(
					root.control.ctx,
					&grpc.StreamDesc{ClientStreams: true},
					fullMethod,
				)
				if err != nil {
					snapshot := snapshotWorkerError(err)
					_ = root.owner.rejectOwnerPromiseSnapshot(promiseID, snapshot)
					root.failConstruction(snapshot.err())
					return
				}
				lifecycle, bindErr := bindClientTransport(
					root.control,
					stream,
					defaultInproc,
				)
				boundary.transportBound = true
				worker, workerErr := newClientStreamWorker(
					stream,
					lifecycle,
					inputDesc,
					outputDesc,
					root,
					onHeader,
					onTrailer,
				)
				if workerErr != nil {
					_ = root.owner.rejectOwnerPromise(promiseID, workerErr)
					root.control.stop(workerErr)
					root.finish(workerErr)
					return
				}
				if installErr := streams.install(root.id, worker); installErr != nil {
					_ = root.owner.rejectOwnerPromise(promiseID, installErr)
					root.control.stop(installErr)
					root.finish(installErr)
					return
				}
				boundary.transferred = true
				if bindErr != nil {
					_ = root.owner.rejectOwnerPromise(promiseID, bindErr)
					worker.failLocal(bindErr)
					return
				}
				if settleErr := root.owner.resolveOwnerPromise(
					promiseID,
					ownerStreamResult{id: root.id},
				); settleErr != nil {
					worker.failLocal(settleErr)
				}
			})
		}()
		return promise.value
	})
}

// newClientStreamCall must run under the adapter owner.
func (m *Module) newClientStreamCall(
	projection *clientStreamProjection,
) *goja.Object {
	callObject := m.runtime.NewObject()
	_ = callObject.Set("send", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		return projection.send(call.Argument(0))
	}))
	_ = callObject.Set("closeSend", m.runtime.ToValue(func(goja.FunctionCall) goja.Value {
		return projection.closeSend()
	}))
	// The response promise is created EAGERLY at construction: it is the only
	// path that submits the terminal RecvMsg command, so the transport drains,
	// the recv loop terminates, and the worker/supervisor root retire even if
	// the caller never reads .response. An internal no-op rejection handler is
	// armed on it so a rejected response (e.g. the server returned an error)
	// is never reported as an unhandled rejection while .response is ignored;
	// the user retains the original promise and later access returns it
	// unchanged.
	responsePromise := projection.response()
	m.armResponseRejectionHandler(responsePromise)
	// response is exposed through an accessor that publishes the eagerly
	// created promise as a plain data property on first access. The property
	// shape is identical to the lazy design: enumerable and configurable as
	// defined below, and writable=false so that assignment keeps failing
	// silently (sloppy) or throwing (strict) exactly like a getter-only
	// accessor — the object's observable shape (Object.keys, spread,
	// JSON.stringify, assignment) must not depend on whether .response was
	// read. DefineDataProperty (not Set) is required: Set invokes [[Set]],
	// which fails on an accessor property with no setter. The define cannot
	// fail: the accessor is configurable. If it ever did, the accessor
	// remains functional as a fallback (it returns the cached promise), so
	// the error is intentionally tolerated.
	_ = callObject.DefineAccessorProperty("response",
		m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			_ = callObject.DefineDataProperty("response", responsePromise, goja.FLAG_FALSE, goja.FLAG_TRUE, goja.FLAG_TRUE)
			return responsePromise
		}),
		goja.Undefined(), // no setter
		goja.FLAG_TRUE,   // configurable (allows replacement)
		goja.FLAG_TRUE,   // enumerable
	)
	return callObject
}

// armResponseRejectionHandler attaches an internal no-op rejection handler to
// an eagerly created response promise via the captured %Promise.prototype.then%
// intrinsic. Attaching any reaction marks the promise handled in goja's
// unhandled-rejection tracking, so a later rejection is never reported as
// unhandled while the caller never reads .response. The original promise is
// returned to the user unchanged.
func (m *Module) armResponseRejectionHandler(promise goja.Value) {
	if promise == nil {
		return
	}
	if m.promiseThen == nil {
		// Capture the intrinsic from this promise's own prototype chain (the
		// runtime's internal Promise.prototype, immune to user shadowing of
		// the global Promise). Owner-serialized, so the plain write is safe.
		if obj, ok := promise.(*goja.Object); ok {
			if then, ok := goja.AssertFunction(obj.Get("then")); ok {
				m.promiseThen = then
			}
		}
	}
	if m.promiseThen == nil {
		return
	}
	noop := m.runtime.ToValue(func(goja.FunctionCall) goja.Value {
		return goja.Undefined()
	})
	if _, err := m.promiseThen(promise, goja.Null(), noop); err != nil {
		// The promise is our own NewPromise-derived value and the callback is
		// a native function, so this cannot fail; a failure would merely leave
		// the promise unhandled (the pre-eager behavior).
		return
	}
}
