package gojagrpc

import (
	"errors"
	"io"
	"sync"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	grpcmetadata "google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

const clientSendQueueLimit = 64

var (
	errConcurrentRecv = status.Error(codes.FailedPrecondition, "grpc: recv already pending")
	errSendClosed     = status.Error(codes.FailedPrecondition, "grpc: send direction is closed")
	errSendQueueFull  = status.Error(codes.ResourceExhausted, "grpc: send queue is full")
)

func selectedSendError(err error, eof bool) error {
	if err != nil {
		return err
	}
	if eof {
		return errSendClosed
	}
	return status.Error(codes.Internal, "grpc: send operation terminated")
}

func contextError(ctx interface{ Err() error }) error {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return status.FromContextError(err).Err()
		}
	}
	return status.Error(codes.Canceled, "grpc: operation canceled")
}

type clientSendCommand struct {
	message proto.Message
	promise ownerOperationID
	close   bool
}

type clientRecvCommand struct {
	promise      ownerOperationID
	terminalNext bool
}

// clientStreamWorker is the pure Go lifecycle owner for one grpc.ClientStream.
// JavaScript wrappers communicate through scalar stream IDs and copied command
// payloads; no Module, Runtime, Promise, callable, or owner projection is held
// here.
type clientStreamWorker struct {
	stream        grpc.ClientStream
	lifecycle     transportLifecycle
	input         protoreflect.MessageDescriptor
	output        protoreflect.MessageDescriptor
	root          workerRoot
	onHeader      ownerCallbackID
	onTrailer     ownerCallbackID
	sendQueue     chan clientSendCommand
	recvQueue     chan clientRecvCommand
	done          chan struct{}
	headerDone    chan struct{}
	transportDone chan struct{}

	headerErr    error
	transportErr error
	terminal     error
	trailerErr   error
	trailer      grpcmetadata.MD
	transportEOF bool
	eof          bool
	closed       bool
	sendClosed   bool
	recvPending  bool

	terminalOnce sync.Once
	headerOnce   sync.Once
	localOnce    sync.Once
	workers      sync.WaitGroup
	mu           sync.Mutex
}

type clientStreamExecutor struct {
	workers sync.Map // map[supervisorChildID]*clientStreamWorker
}

func newClientStreamExecutor() *clientStreamExecutor {
	return new(clientStreamExecutor)
}

func (e *clientStreamExecutor) install(
	id supervisorChildID,
	worker *clientStreamWorker,
) error {
	if id == 0 || worker == nil {
		return errModuleClosed
	}
	if _, loaded := e.workers.LoadOrStore(id, worker); loaded {
		return errors.New("gojagrpc: duplicate client stream root")
	}
	worker.start(func() { e.workers.Delete(id) })
	return nil
}

func (e *clientStreamExecutor) send(
	id supervisorChildID,
	command clientSendCommand,
) error {
	value, ok := e.workers.Load(id)
	if !ok {
		return errModuleUnavailable
	}
	return value.(*clientStreamWorker).enqueueSend(command)
}

func (e *clientStreamExecutor) recv(
	id supervisorChildID,
	command clientRecvCommand,
) error {
	value, ok := e.workers.Load(id)
	if !ok {
		return errModuleUnavailable
	}
	return value.(*clientStreamWorker).enqueueRecv(command)
}

func newClientStreamWorker(
	stream grpc.ClientStream,
	lifecycle transportLifecycle,
	input protoreflect.MessageDescriptor,
	output protoreflect.MessageDescriptor,
	root workerRoot,
	onHeader ownerCallbackID,
	onTrailer ownerCallbackID,
) (*clientStreamWorker, error) {
	if stream == nil || root.control == nil || root.owner == nil || root.id == 0 {
		return nil, errModuleClosed
	}
	worker := &clientStreamWorker{
		stream:        stream,
		lifecycle:     lifecycle,
		input:         input,
		output:        output,
		root:          root,
		onHeader:      onHeader,
		onTrailer:     onTrailer,
		sendQueue:     make(chan clientSendCommand, clientSendQueueLimit),
		recvQueue:     make(chan clientRecvCommand, 1),
		done:          make(chan struct{}),
		headerDone:    make(chan struct{}),
		transportDone: make(chan struct{}),
	}
	return worker, nil
}

func (w *clientStreamWorker) start(remove func()) {
	workerCount := 3
	if w.lifecycle != nil {
		workerCount++
	}
	w.workers.Add(workerCount)
	go w.headerLoop()
	go w.sendLoop()
	go w.recvLoop()
	if w.lifecycle != nil {
		go w.terminalLoop()
	}
	go func() {
		<-w.done
		w.workers.Wait()
		err, eof := w.terminalResult()
		if err == nil && eof {
			err = io.EOF
		}
		w.root.finish(err)
		if remove != nil {
			remove()
		}
	}()
	go func() {
		select {
		case <-w.done:
		case <-w.root.control.ctx.Done():
			// publishTerminal cancels the transport context only after it has
			// selected this worker's immutable terminal boundary. Do not
			// reinterpret that internal cancellation as a local Canceled result.
			w.mu.Lock()
			closed := w.closed
			w.mu.Unlock()
			if !closed {
				w.failLocal(contextError(w.root.control.ctx))
			}
		}
	}()
}

func (w *clientStreamWorker) headerLoop() {
	defer w.workers.Done()
	w.runTransportLoop(nil, func() {
		metadata, transportErr := w.stream.Header()
		err := canonicalWorkerError(transportErr)
		if err == nil {
			err = w.root.owner.invokeMetadataCallback(w.onHeader, metadata)
			if errors.Is(err, goeventloop.ErrLoopTerminated) {
				err = errModuleUnavailable
			}
			err = canonicalWorkerError(err)
		}
		w.mu.Lock()
		w.headerErr = err
		w.mu.Unlock()
		w.headerOnce.Do(func() { close(w.headerDone) })
		if err != nil {
			if transportErr != nil && w.lifecycle == nil {
				w.publishTerminal(err, false)
				return
			}
			w.failLocal(err)
		}
	})
}

func (w *clientStreamWorker) waitHeader() error {
	select {
	case <-w.headerDone:
	case <-w.done:
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.headerErr
}

func (w *clientStreamWorker) terminalLoop() {
	defer w.workers.Done()
	w.runTransportLoop(nil, func() {
		<-w.lifecycle.TerminalDone()
		transportErr, terminal := w.lifecycle.TerminalResult()
		snapshot := snapshotWorkerError(transportErr)
		err := snapshot.err()
		eof := err == nil || snapshot.eof
		if snapshot.eof {
			err = nil
		}
		if !terminal {
			err = status.Error(
				codes.Internal,
				"inproc terminal signal closed without a result",
			)
			eof = false
			w.root.control.stop(err)
		}
		w.mu.Lock()
		w.transportErr = err
		w.transportEOF = eof
		w.mu.Unlock()
		close(w.transportDone)
		if localErr := w.root.control.localStopError(nil); localErr != nil {
			w.publishTerminalState(localErr, false, false)
		}
	})
}

func (w *clientStreamWorker) waitTransportTerminal() (error, bool) {
	if w.lifecycle == nil {
		return w.terminalResult()
	}
	<-w.transportDone
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.transportErr, w.transportEOF
}

func (w *clientStreamWorker) publishTerminal(
	err error,
	eof bool,
) {
	w.publishTerminalState(err, eof, true)
}

func (w *clientStreamWorker) publishTerminalState(
	err error,
	eof bool,
	readTrailer bool,
) {
	w.terminalOnce.Do(func() {
		if err == nil && !eof {
			err = io.EOF
			eof = true
		} else if err != nil {
			err = canonicalWorkerError(err)
		}
		// Select the terminal admission boundary before cancellation or the
		// owner callback can block. Commands admitted before this lock are
		// drained; every later command observes the cached terminal.
		w.mu.Lock()
		w.terminal = err
		w.eof = eof
		w.closed = true
		w.mu.Unlock()
		// Cancel first so blocked Header/SendMsg/RecvMsg calls can release.
		w.root.control.cancel()
		var trailer grpcmetadata.MD
		if readTrailer {
			var trailerOK bool
			trailer, trailerOK = snapshotClientTrailer(w.stream)
			if !trailerOK {
				w.mu.Lock()
				w.trailerErr = workerErrorFallback().err()
				w.mu.Unlock()
			}
		}
		if callbackErr := w.root.owner.invokeMetadataCallback(
			w.onTrailer,
			trailer,
		); callbackErr != nil {
			w.mu.Lock()
			w.trailerErr = status.Errorf(
				codes.Internal,
				"onTrailer callback: %v",
				callbackErr,
			)
			w.mu.Unlock()
		}
		w.mu.Lock()
		w.trailer = trailer.Copy()
		w.mu.Unlock()
		close(w.done)
	})
}

// failLocal publishes a worker-selected local terminal and retires the root.
//
// Ordering is load-bearing. publishTerminalState must run FIRST: it sets
// w.closed under w.mu and only then cancels the operation context (to release
// blocked transport calls) and closes w.done. The ctx-done watcher started in
// start() treats a cancellation observed while w.closed is false as an
// unexplained local failure and republishes it as context.Canceled — a cancel
// issued before the terminal is published would let that Canceled terminal
// win terminalOnce over the real error (the "expected 13, got 1" flake: a
// server-stream sync-throw header error became context.Canceled). Conversely
// the cancel must precede every observable settlement: w.done only closes
// after the cancel, so sendLoop/recvLoop drains and the root disposal (both
// of which settle promises) always run after the operation context is
// canceled, keeping the "terminal error cancels the caller context" contract.
func (w *clientStreamWorker) failLocal(err error) {
	if err == nil {
		return
	}
	err = canonicalWorkerError(err)
	w.publishTerminalState(err, false, false)
	w.root.control.stop(err)
	w.localOnce.Do(func() {
		w.root.owner.disposeOwnerRootWorker(w.root.id, err)
	})
}

func (w *clientStreamWorker) runTransportLoop(
	current *ownerOperationID,
	fn func(),
) {
	returned := false
	defer func() {
		_ = recover()
		if returned {
			return
		}
		fallback := workerErrorFallback()
		if current != nil && current.root != 0 && current.child != 0 {
			_ = w.root.owner.rejectOwnerPromiseSnapshot(*current, fallback)
		}
		w.failLocal(fallback.err())
	}()
	fn()
	returned = true
}

type clientTrailerSnapshot struct {
	metadata grpcmetadata.MD
	returned bool
}

func snapshotClientTrailer(stream grpc.ClientStream) (grpcmetadata.MD, bool) {
	result := make(chan clientTrailerSnapshot, 1)
	go func() {
		snapshot := clientTrailerSnapshot{}
		defer func() {
			_ = recover()
			result <- snapshot
		}()
		snapshot.metadata = stream.Trailer().Copy()
		snapshot.returned = true
	}()
	snapshot := <-result
	return snapshot.metadata, snapshot.returned
}

func (w *clientStreamWorker) terminalProjectionError() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.trailerErr
}

func (w *clientStreamWorker) terminalResult() (error, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.terminal, w.eof
}

func (w *clientStreamWorker) enqueueSend(command clientSendCommand) error {
	if command.promise.root == 0 || command.promise.child == 0 {
		return errModuleUnavailable
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return selectedSendError(w.terminal, w.eof)
	}
	if w.sendClosed {
		return errSendClosed
	}
	select {
	case w.sendQueue <- command:
		if command.close {
			w.sendClosed = true
		}
		return nil
	default:
		return errSendQueueFull
	}
}

func (w *clientStreamWorker) enqueueRecv(command clientRecvCommand) error {
	if command.promise.root == 0 || command.promise.child == 0 {
		return errModuleUnavailable
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		if w.eof {
			return io.EOF
		}
		if w.terminal == nil {
			return errModuleUnavailable
		}
		return w.terminal
	}
	if w.recvPending {
		return errConcurrentRecv
	}
	select {
	case w.recvQueue <- command:
		w.recvPending = true
		return nil
	default:
		return errConcurrentRecv
	}
}

func (w *clientStreamWorker) sendLoop() {
	defer w.workers.Done()
	var current ownerOperationID
	w.runTransportLoop(&current, func() {
		for {
			select {
			case <-w.done:
				w.drainSends()
				return
			case command := <-w.sendQueue:
				current = command.promise
				var err error
				if command.close {
					err = w.stream.CloseSend()
				} else {
					err = w.stream.SendMsg(command.message)
				}
				if err != nil {
					snapshot := snapshotWorkerError(err)
					current = ownerOperationID{}
					w.failLocal(snapshot.err())
					<-w.done
					w.drainSends()
					return
				}
				if settleErr := w.root.owner.resolveOwnerPromise(
					command.promise,
					ownerEmptyResult{},
				); settleErr != nil {
					current = ownerOperationID{}
					w.failLocal(settleErr)
					<-w.done
					w.drainSends()
					return
				}
				current = ownerOperationID{}
				if command.close {
					return
				}
			}
		}
	})
}

func (w *clientStreamWorker) drainSends() {
	err, eof := w.terminalResult()
	err = selectedSendError(err, eof)
	for {
		select {
		case command := <-w.sendQueue:
			_ = w.root.owner.rejectOwnerPromise(command.promise, err)
		default:
			return
		}
	}
}

func (w *clientStreamWorker) recvLoop() {
	defer w.workers.Done()
	var current ownerOperationID
	w.runTransportLoop(&current, func() {
		for {
			select {
			case <-w.done:
				w.drainRecvs()
				return
			case command := <-w.recvQueue:
				current = command.promise
				message := dynamicpb.NewMessage(w.output)
				snapshot := snapshotWorkerError(w.stream.RecvMsg(message))
				if headerErr := w.waitHeader(); headerErr != nil {
					snapshot = snapshotWorkerError(headerErr)
				}
				err := snapshot.err()
				if err != nil {
					eof := snapshot.eof
					if w.lifecycle != nil {
						err, eof = w.waitTransportTerminal()
						w.publishTerminal(err, eof)
					} else {
						if eof {
							w.publishTerminal(nil, true)
						} else {
							w.publishTerminal(err, false)
						}
					}
					<-w.done
					projectionErr := w.terminalProjectionError()
					if projectionErr != nil {
						_ = w.root.owner.rejectOwnerPromise(
							command.promise,
							projectionErr,
						)
					} else if eof {
						_ = w.root.owner.resolveOwnerPromise(
							command.promise,
							ownerEmptyResult{},
						)
					} else {
						_ = w.root.owner.rejectOwnerPromise(
							command.promise,
							err,
						)
					}
					current = ownerOperationID{}
					w.drainRecvs()
					return
				}
				if command.terminalNext {
					if w.lifecycle != nil {
						terminalErr, eof := w.waitTransportTerminal()
						w.publishTerminal(terminalErr, eof)
					} else {
						w.publishTerminal(nil, true)
					}
					<-w.done
					if projectionErr := w.terminalProjectionError(); projectionErr != nil {
						_ = w.root.owner.rejectOwnerPromise(
							command.promise,
							projectionErr,
						)
						current = ownerOperationID{}
						w.drainRecvs()
						return
					}
				}
				// RecvMsg and all transport-side checks are complete. Release
				// admission before resolving because the owner Promise reaction
				// may synchronously request the next item.
				w.mu.Lock()
				w.recvPending = false
				w.mu.Unlock()
				if settleErr := w.root.owner.resolveOwnerPromise(
					command.promise,
					ownerMessageResult{message: cloneOwnerMessage(message)},
				); settleErr != nil {
					current = ownerOperationID{}
					w.failLocal(settleErr)
					<-w.done
					w.drainRecvs()
					return
				}
				current = ownerOperationID{}
				if command.terminalNext {
					w.drainRecvs()
					return
				}
			}
		}
	})
}

func (w *clientStreamWorker) drainRecvs() {
	err, eof := w.terminalResult()
	for {
		select {
		case command := <-w.recvQueue:
			if eof {
				_ = w.root.owner.resolveOwnerPromise(
					command.promise,
					ownerEmptyResult{},
				)
			} else {
				_ = w.root.owner.rejectOwnerPromise(command.promise, err)
			}
		default:
			return
		}
	}
}

// clientStreamProjection is retained only by owner-created JS closures. It is
// mutated only on-owner, including its root-disposal terminal transition.
//
// The terminal/eof fields are written by the root-disposal disposer and read
// by the JS-facing recv/response entry points. Pre-Done both sides run on the
// owner (serialized by the loop); post-Done the disposer may run on the Close
// goroutine while a late recv/response runs on the runtime goroutine, so both
// sides guard the fields with postDoneMu (see terminalState).
type clientStreamProjection struct {
	module   *Module
	input    protoreflect.MessageDescriptor
	output   protoreflect.MessageDescriptor
	rootID   supervisorChildID
	streamID supervisorChildID
	terminal error
	eof      bool
}

// terminalState returns the terminal error and EOF flag, guarded by
// postDoneMu against the concurrent root-disposal disposer write.
func (p *clientStreamProjection) terminalState() (err error, eof bool) {
	p.module.owner.postDoneMu.Lock()
	err, eof = p.terminal, p.eof
	p.module.owner.postDoneMu.Unlock()
	return
}

func (m *Module) newClientStreamProjection(
	rootID supervisorChildID,
	input protoreflect.MessageDescriptor,
	output protoreflect.MessageDescriptor,
) *clientStreamProjection {
	projection := &clientStreamProjection{
		module:   m,
		input:    input,
		output:   output,
		rootID:   rootID,
		streamID: rootID,
	}
	_ = m.addOwnerRootDisposer(rootID, func(err error) {
		m.owner.postDoneMu.Lock()
		projection.terminal = err
		projection.eof = errors.Is(err, io.EOF)
		m.owner.postDoneMu.Unlock()
	})
	return projection
}

func (p *clientStreamProjection) immediateRecv() goja.Value {
	promise, resolve, reject := p.module.runtime.NewPromise()
	err, eof := p.terminalState()
	if eof {
		object := p.module.runtime.NewObject()
		_ = object.Set("done", true)
		_ = object.Set("value", goja.Undefined())
		_ = resolve(object)
	} else {
		if err == nil {
			err = errModuleUnavailable
		}
		_ = reject(p.module.grpcErrorFromGoError(err))
	}
	return p.module.runtime.ToValue(promise)
}

func (p *clientStreamProjection) newRecvPromise(
	terminalNext bool,
) goja.Value {
	terminal, eof := p.terminalState()
	if terminal != nil || eof {
		return p.immediateRecv()
	}
	promise := p.module.newOwnerPromise(
		p.rootID,
		func(result ownerResult) any {
			object := p.module.runtime.NewObject()
			switch value := result.(type) {
			case ownerEmptyResult:
				_ = object.Set("done", true)
				_ = object.Set("value", goja.Undefined())
			case ownerMessageResult:
				message, err := p.module.wrapMessage(value.message, p.output)
				if err != nil {
					panic(err)
				}
				_ = object.Set("done", false)
				_ = object.Set("value", message)
			default:
				panic("gojagrpc: invalid stream receive result")
			}
			return object
		},
		nil,
	)
	if !promise.admitted() {
		return promise.value
	}
	err := p.module.streams.recv(p.streamID, clientRecvCommand{
		promise:      promise.id,
		terminalNext: terminalNext,
	})
	if errors.Is(err, io.EOF) {
		_ = p.module.resolveOwnerPromiseInline(promise.id, ownerEmptyResult{})
	} else if err != nil {
		_ = p.module.rejectOwnerPromiseInline(promise.id, err)
	}
	return promise.value
}

func (p *clientStreamProjection) recv() goja.Value {
	return p.newRecvPromise(false)
}

func (p *clientStreamProjection) response() goja.Value {
	terminal, eof := p.terminalState()
	if terminal != nil || eof {
		return p.immediateRecv()
	}
	promise := p.module.newOwnerPromise(
		p.rootID,
		func(result ownerResult) any {
			value, ok := result.(ownerMessageResult)
			if !ok {
				panic("gojagrpc: invalid client stream response")
			}
			message, err := p.module.wrapMessage(value.message, p.output)
			if err != nil {
				panic(err)
			}
			return message
		},
		nil,
	)
	if !promise.admitted() {
		return promise.value
	}
	err := p.module.streams.recv(p.streamID, clientRecvCommand{
		promise:      promise.id,
		terminalNext: true,
	})
	if err != nil {
		_ = p.module.rejectOwnerPromiseInline(promise.id, err)
	}
	return promise.value
}

func (p *clientStreamProjection) send(value goja.Value) goja.Value {
	message, err := p.module.snapshotMessage(value, p.input)
	if err != nil {
		panic(p.module.runtime.NewTypeError("send: %s", err))
	}
	promise := p.module.newOwnerPromise(p.rootID, nil, nil)
	if !promise.admitted() {
		return promise.value
	}
	err = p.module.streams.send(p.streamID, clientSendCommand{
		message: message,
		promise: promise.id,
	})
	if err != nil {
		_ = p.module.rejectOwnerPromiseInline(promise.id, err)
	}
	return promise.value
}

func (p *clientStreamProjection) closeSend() goja.Value {
	promise := p.module.newOwnerPromise(p.rootID, nil, nil)
	if !promise.admitted() {
		return promise.value
	}
	err := p.module.streams.send(p.streamID, clientSendCommand{
		promise: promise.id,
		close:   true,
	})
	if err != nil {
		_ = p.module.rejectOwnerPromiseInline(promise.id, err)
	}
	return promise.value
}
