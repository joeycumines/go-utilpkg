package gojagrpc

import (
	"github.com/joeycumines/goja"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// makeServerStreamMethod creates the client method for a server-streaming RPC.
// Request data is cloned on-owner before the transport worker starts.
func (m *Module) makeServerStreamMethod(
	cc grpc.ClientConnInterface,
	fullMethod string,
	inputDesc protoreflect.MessageDescriptor,
	outputDesc protoreflect.MessageDescriptor,
) goja.Value {
	return m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		request, err := m.snapshotMessage(call.Argument(0), inputDesc)
		if err != nil {
			panic(m.runtime.NewTypeError("server-stream %s: %s", fullMethod, err))
		}
		options := m.parseCallOpts(call, 1)
		// The projection is created eagerly (not inside the resolve
		// projection): its root disposer removes the stream worker entry from
		// the executor map at disposal, and a construction failure that
		// prevents the stream promise from resolving must still trigger that
		// removal — otherwise the worker entry would leak until the module
		// dies. The stream id equals the reserved root id by construction.
		projection := m.newClientStreamProjection(options.rootID, inputDesc, outputDesc)
		promise := m.newOwnerPromise(options.rootID, func(ownerResult) any {
			reader := m.runtime.NewObject()
			_ = reader.Set("recv", m.runtime.ToValue(func(goja.FunctionCall) goja.Value {
				return projection.recv()
			}))
			return reader
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
					&grpc.StreamDesc{ServerStreams: true},
					fullMethod,
				)
				if err != nil {
					snapshot := snapshotWorkerError(err)
					_ = root.owner.rejectOwnerPromiseSnapshot(promiseID, snapshot)
					root.failConstruction(snapshot.err())
					return
				}
				lifecycle, transportErr := bindClientTransport(
					root.control,
					stream,
					defaultInproc,
				)
				boundary.transportBound = true
				if transportErr == nil {
					transportErr = canonicalWorkerError(stream.SendMsg(request))
				}
				if transportErr == nil {
					transportErr = canonicalWorkerError(stream.CloseSend())
				}

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
				if transportErr != nil {
					_ = root.owner.rejectOwnerPromise(promiseID, transportErr)
					worker.failLocal(transportErr)
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
