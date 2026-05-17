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
		promise := m.newOwnerPromise(options.rootID, func(result ownerResult) any {
			id := result.(ownerStreamResult).id
			return m.newClientStreamCall(
				m.newClientStreamProjection(id, inputDesc, outputDesc),
			)
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
	// response is lazily created on first access to avoid unhandled
	// promise rejections when the caller never reads the response.
	var responsePromise goja.Value
	_ = callObject.DefineAccessorProperty("response",
		m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			if responsePromise == nil {
				responsePromise = projection.response()
				// Replace the accessor with a plain data property so
				// subsequent reads are direct.
				_ = callObject.Set("response", responsePromise)
			}
			return responsePromise
		}),
		goja.Undefined(), // no setter
		goja.FLAG_TRUE,   // configurable (allows replacement)
		goja.FLAG_TRUE,   // enumerable
	)
	return callObject
}
