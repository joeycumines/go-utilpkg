package gojagrpc

import (
	"github.com/joeycumines/goja"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// makeBidiStreamMethod creates the client method for a bidirectional stream.
// The stream object is delivered without waiting for response headers, so a
// server may legitimately send its headers after receiving the first message.
func (m *Module) makeBidiStreamMethod(
	cc grpc.ClientConnInterface,
	fullMethod string,
	inputDesc protoreflect.MessageDescriptor,
	outputDesc protoreflect.MessageDescriptor,
) goja.Value {
	return m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		options := m.parseCallOpts(call, 0)
		promise := m.newOwnerPromise(options.rootID, func(result ownerResult) any {
			id := result.(ownerStreamResult).id
			return m.newBidiStreamObject(
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
					&grpc.StreamDesc{ClientStreams: true, ServerStreams: true},
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
				settleErr := root.owner.resolveOwnerPromise(
					promiseID,
					ownerStreamResult{id: root.id},
				)
				if settleErr != nil {
					worker.failLocal(settleErr)
				}
			})
		}()
		return promise.value
	})
}

// newBidiStreamObject must run under the adapter owner.
func (m *Module) newBidiStreamObject(
	projection *clientStreamProjection,
) *goja.Object {
	object := m.runtime.NewObject()
	_ = object.Set("send", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		return projection.send(call.Argument(0))
	}))
	_ = object.Set("closeSend", m.runtime.ToValue(func(goja.FunctionCall) goja.Value {
		return projection.closeSend()
	}))
	_ = object.Set("recv", m.runtime.ToValue(func(goja.FunctionCall) goja.Value {
		return projection.recv()
	}))
	return object
}
