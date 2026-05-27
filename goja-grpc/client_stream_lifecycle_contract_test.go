package gojagrpc

import (
	"context"
	"errors"
	"math"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/goja"
	"google.golang.org/grpc/codes"
	grpcmetadata "google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

func newClientStreamHarness(
	module *Module,
	stream *phase3MockStream,
	input protoreflect.MessageDescriptor,
	output protoreflect.MessageDescriptor,
	options *callOpts,
) (*clientStreamWorker, *clientStreamProjection, error) {
	root := options.workerRoot()
	lifecycle, err := bindClientTransport(root.control, stream, false)
	if err != nil {
		root.failConstruction(err)
		return nil, nil, err
	}
	worker, err := newClientStreamWorker(
		stream,
		lifecycle,
		input,
		output,
		root,
		options.headerCallback(),
		options.trailerCallback(),
	)
	if err != nil {
		root.finish(err)
		return nil, nil, err
	}
	if err := module.streams.install(root.id, worker); err != nil {
		root.finish(err)
		return nil, nil, err
	}
	return worker, module.newClientStreamProjection(root.id, input, output), nil
}

func TestClientStreamContextCancellationDoesNotReadTrailerEarly(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	ctx, cancel := context.WithCancel(env.grpcMod.ctx)
	options := &callOpts{
		module: env.grpcMod,
		ctx:    ctx,
		cancel: cancel,
	}
	if err := options.register(); err != nil {
		t.Fatal(err)
	}
	var trailerCalls atomic.Int32
	worker, _, err := newClientStreamHarness(
		env.grpcMod,
		&phase3MockStream{
			ctx: ctx,
			trailerFn: func() grpcmetadata.MD {
				trailerCalls.Add(1)
				return nil
			},
		},
		nil,
		nil,
		options,
	)
	if err != nil {
		t.Fatal(err)
	}
	stopLoop := withLoopRunning(t, env, defaultTimeout)
	defer stopLoop()

	cancel()
	select {
	case <-worker.done:
	case <-time.After(defaultTimeout):
		t.Fatal("context cancellation did not terminate idle stream state")
	}
	if got := trailerCalls.Load(); got != 0 {
		t.Fatalf("Trailer calls before terminal RecvMsg = %d, want 0", got)
	}
	if terminal, _ := worker.terminalResult(); status.Code(terminal) != codes.Canceled {
		t.Fatalf("terminal error = %v, want Canceled", terminal)
	}
	timer := time.NewTimer(defaultTimeout)
	defer timer.Stop()
	for {
		operations := supervisorKindCount(env.grpcMod, supervisorOperation)
		if operations == 0 {
			break
		}
		select {
		case <-timer.C:
			t.Fatalf(
				"registered operations after cancellation = %d, want 0",
				operations,
			)
		default:
			runtime.Gosched()
		}
	}
}

func TestClientStreamConstructorReturnsSelectedCloseTerminal(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	ctx, cancel := context.WithCancel(env.grpcMod.ctx)
	options := &callOpts{
		module: env.grpcMod,
		ctx:    ctx,
		cancel: cancel,
	}
	if err := options.register(); err != nil {
		t.Fatal(err)
	}
	stopLoop := withLoopRunning(t, env, defaultTimeout)
	defer stopLoop()
	closeDone := make(chan error, 1)
	go func() { closeDone <- env.grpcMod.Close() }()
	// The admitted-but-unbound construction is a close obligation: Close must
	// cancel it, and a late bindRelease must observe the close-selected
	// terminal (Unavailable). Close itself may resolve the obligation through
	// the abandon path (which consumes the binding with nil), so the ordering
	// assertions here are only the deterministic ones: cancellation first,
	// then the selected terminal on bindRelease, then Close joining once the
	// construction obligation is released.
	select {
	case <-ctx.Done():
	case <-time.After(defaultTimeout):
		t.Fatal("Close did not cancel admitted construction")
	}
	if err := options.control.bindRelease(nil); status.Code(err) != codes.Unavailable {
		t.Fatalf("construction bind error = %v, want selected Unavailable", err)
	}
	options.control.finishWorker()
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(defaultTimeout):
		t.Fatal("Close did not join admitted constructor")
	}
}

func TestClientStreamAdmissionLinearizesWithTerminal(t *testing.T) {
	for _, operation := range []string{"send", "recv"} {
		t.Run(operation, func(t *testing.T) {
			for range 1_000 {
				ctx, cancel := context.WithCancel(context.Background())
				control := &operationControl{
					ctx:    ctx,
					cancel: cancel,
					done:   make(chan struct{}),
				}
				worker := &clientStreamWorker{
					root:      workerRoot{control: control},
					sendQueue: make(chan clientSendCommand, 1),
					recvQueue: make(chan clientRecvCommand, 1),
					done:      make(chan struct{}),
				}
				start := make(chan struct{})
				result := make(chan error, 1)
				go func() {
					<-start
					switch operation {
					case "send":
						result <- worker.enqueueSend(clientSendCommand{
							promise: ownerOperationID{root: 1, child: 1},
						})
					case "recv":
						result <- worker.enqueueRecv(clientRecvCommand{
							promise: ownerOperationID{root: 1, child: 1},
						})
					}
				}()
				close(start)
				worker.publishTerminal(
					status.Error(codes.Canceled, "terminal"),
					false,
				)
				err := <-result
				var queued int
				if operation == "send" {
					queued = len(worker.sendQueue)
				} else {
					queued = len(worker.recvQueue)
				}
				switch {
				case err == nil && queued != 1:
					t.Fatalf("accepted %s queue length = %d, want 1", operation, queued)
				case err != nil && queued != 0:
					t.Fatalf("rejected %s queue length = %d, want 0", operation, queued)
				case err != nil && status.Code(err) != codes.Canceled:
					t.Fatalf("rejected %s error = %v, want Canceled", operation, err)
				}
			}
		})
	}
}

func TestClientStreamChildIDExhaustionHasNoTransportEffect(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	ctx, cancel := context.WithCancel(env.grpcMod.ctx)
	options := &callOpts{
		module: env.grpcMod,
		ctx:    ctx,
		cancel: cancel,
	}
	if err := options.register(); err != nil {
		t.Fatal(err)
	}
	var sends, recvs atomic.Int32
	stream := &phase3MockStream{
		ctx: ctx,
		sendMsgFn: func(any) error {
			sends.Add(1)
			return nil
		},
		recvMsgFn: func(any) error {
			recvs.Add(1)
			return nil
		},
	}
	descriptor := phase3FindMsgDesc(t, env, "testgrpc.Item")
	worker, projection, err := newClientStreamHarness(
		env.grpcMod,
		stream,
		descriptor,
		descriptor,
		options,
	)
	if err != nil {
		t.Fatal(err)
	}
	env.grpcMod.owner.roots[options.rootID].nextChild = math.MaxUint64
	message, err := env.pbMod.WrapMessage(dynamicpb.NewMessage(descriptor))
	if err != nil {
		t.Fatal(err)
	}
	_ = projection.send(message)
	_ = projection.recv()
	if queued := len(worker.sendQueue); queued != 0 {
		t.Fatalf("saturated send queue length = %d, want 0", queued)
	}
	if queued := len(worker.recvQueue); queued != 0 {
		t.Fatalf("saturated recv queue length = %d, want 0", queued)
	}
	if got := sends.Load(); got != 0 {
		t.Fatalf("saturated transport sends = %d, want 0", got)
	}
	if got := recvs.Load(); got != 0 {
		t.Fatalf("saturated transport recvs = %d, want 0", got)
	}
	stopLoop := withLoopRunning(t, env, defaultTimeout)
	defer stopLoop()
	worker.publishTerminal(
		status.Error(codes.Canceled, "test complete"),
		false,
	)
	timer := time.NewTimer(defaultTimeout)
	defer timer.Stop()
	for supervisorKindCount(env.grpcMod, supervisorOperation) != 0 {
		select {
		case <-timer.C:
			t.Fatal("saturated stream root did not release")
		default:
			runtime.Gosched()
		}
	}
}

func TestClientStreamTerminalImmutableAcrossBlockedTrailerFailure(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	ctx, cancel := context.WithCancel(env.grpcMod.ctx)
	options := &callOpts{
		module: env.grpcMod,
		ctx:    ctx,
		cancel: cancel,
	}
	if err := options.register(); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	type terminalObservation struct {
		err error
		eof bool
	}
	observed := make(chan terminalObservation, 1)
	var worker *clientStreamWorker
	if err := env.runtime.Set("__observeTrailerTerminal", func() {
		err, eof := worker.terminalResult()
		observed <- terminalObservation{err: err, eof: eof}
		close(entered)
		<-release
	}); err != nil {
		t.Fatal(err)
	}
	value, err := env.runtime.RunString(`
		(function() {
			__observeTrailerTerminal();
			throw new Error("trailer projection failed");
		})
	`)
	if err != nil {
		t.Fatal(err)
	}
	callback, ok := goja.AssertFunction(value)
	if !ok {
		t.Fatal("trailer callback is not callable")
	}
	options.callbacks = &callCallbacks{
		onTrailer: env.grpcMod.rememberOwnerCallback(options.rootID, callback),
	}
	worker, _, err = newClientStreamHarness(
		env.grpcMod,
		&phase3MockStream{ctx: ctx},
		nil,
		nil,
		options,
	)
	if err != nil {
		t.Fatal(err)
	}
	stopLoop := withLoopRunning(t, env, defaultTimeout)
	defer stopLoop()
	finishDone := make(chan error, 1)
	go func() {
		worker.publishTerminal(
			status.Error(codes.Canceled, "selected"),
			false,
		)
		finishDone <- worker.terminalProjectionError()
	}()
	select {
	case <-entered:
	case <-time.After(defaultTimeout):
		t.Fatal("trailer callback did not enter")
	}
	select {
	case observation := <-observed:
		if got := status.Code(observation.err); got != codes.Canceled {
			t.Fatalf("terminal during callback = %v, want Canceled", observation.err)
		}
		if observation.eof {
			t.Fatal("terminal during callback reported EOF")
		}
	case <-time.After(defaultTimeout):
		t.Fatal("trailer callback did not observe terminal")
	}
	if err := worker.enqueueSend(clientSendCommand{
		promise: ownerOperationID{root: options.rootID, child: 1},
	}); status.Code(err) != codes.Canceled {
		t.Fatalf("send during trailer callback = %v, want Canceled", err)
	}
	if err := worker.enqueueRecv(clientRecvCommand{
		promise: ownerOperationID{root: options.rootID, child: 2},
	}); status.Code(err) != codes.Canceled {
		t.Fatalf("recv during trailer callback = %v, want Canceled", err)
	}
	if got := len(worker.sendQueue); got != 0 {
		t.Fatalf("send queue during trailer callback = %d, want 0", got)
	}
	if got := len(worker.recvQueue); got != 0 {
		t.Fatalf("recv queue during trailer callback = %d, want 0", got)
	}
	releaseOnce.Do(func() { close(release) })
	select {
	case projectionErr := <-finishDone:
		if status.Code(projectionErr) != codes.Internal ||
			!strings.Contains(projectionErr.Error(), "trailer projection failed") {
			t.Fatalf(
				"trailer projection error = %v, want Internal callback failure",
				projectionErr,
			)
		}
	case <-time.After(defaultTimeout):
		t.Fatal("finish did not return after trailer callback")
	}
	terminal, eof := worker.terminalResult()
	if got := status.Code(terminal); got != codes.Canceled {
		t.Fatalf("stable terminal after callback = %v, want Canceled", terminal)
	}
	if eof {
		t.Fatal("stable terminal after callback reported EOF")
	}
}

func TestClientStreamTrailerFailureRejectsOwningResponseOnly(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	ctx, cancel := context.WithCancel(env.grpcMod.ctx)
	options := &callOpts{
		module: env.grpcMod,
		ctx:    ctx,
		cancel: cancel,
	}
	if err := options.register(); err != nil {
		t.Fatal(err)
	}
	value, err := env.runtime.RunString(`
		(function() { throw new Error("response trailer failed"); })
	`)
	if err != nil {
		t.Fatal(err)
	}
	callback, ok := goja.AssertFunction(value)
	if !ok {
		t.Fatal("trailer callback is not callable")
	}
	options.callbacks = &callCallbacks{
		onTrailer: env.grpcMod.rememberOwnerCallback(options.rootID, callback),
	}
	descriptor := phase3FindMsgDesc(t, env, "testgrpc.Item")
	worker, projection, err := newClientStreamHarness(
		env.grpcMod,
		&phase3MockStream{
			ctx:       ctx,
			recvMsgFn: func(any) error { return nil },
		},
		descriptor,
		descriptor,
		options,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.runtime.Set("__trailerFailurePromise", projection.response()); err != nil {
		t.Fatal(err)
	}
	rejected := make(chan struct{}, 1)
	var rejectionCode atomic.Int64
	var rejectionMessage atomic.Value
	if err := env.runtime.Set(
		"__trailerFailureRejected",
		func(code int64, message string) {
			rejectionCode.Store(code)
			rejectionMessage.Store(message)
			rejected <- struct{}{}
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := env.runtime.RunString(`
		__trailerFailurePromise.then(
			function() {
				throw new Error("response unexpectedly resolved");
			},
			function(error) {
				__trailerFailureRejected(error.code, error.message);
			}
		);
	`); err != nil {
		t.Fatal(err)
	}
	stopLoop := withLoopRunning(t, env, defaultTimeout)
	defer stopLoop()
	select {
	case <-rejected:
	case <-time.After(defaultTimeout):
		t.Fatal("response was not rejected after trailer callback failure")
	}
	if got := codes.Code(rejectionCode.Load()); got != codes.Internal {
		t.Fatalf("response rejection code = %v, want Internal", got)
	}
	message, _ := rejectionMessage.Load().(string)
	if !strings.Contains(message, "response trailer failed") {
		t.Fatalf(
			"response rejection message = %q, want trailer failure",
			message,
		)
	}
	select {
	case <-worker.done:
	case <-time.After(defaultTimeout):
		t.Fatal("worker did not reach terminal completion")
	}
	terminal, eof := worker.terminalResult()
	if terminal != nil || !eof {
		t.Fatalf(
			"canonical transport terminal = (%v, %v), want (nil, true)",
			terminal,
			eof,
		)
	}
}

func TestClientStreamCloseSendClosesAdmission(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	worker := &clientStreamWorker{
		root: workerRoot{control: &operationControl{
			ctx:    ctx,
			cancel: cancel,
			done:   make(chan struct{}),
		}},
		sendQueue: make(chan clientSendCommand, 2),
		done:      make(chan struct{}),
	}
	closeCommand := clientSendCommand{
		promise: ownerOperationID{root: 1, child: 1},
		close:   true,
	}
	if err := worker.enqueueSend(closeCommand); err != nil {
		t.Fatal(err)
	}
	if err := worker.enqueueSend(clientSendCommand{
		promise: ownerOperationID{root: 1, child: 2},
	}); !errors.Is(err, errSendClosed) {
		t.Fatalf("send after admitted CloseSend = %v, want %v", err, errSendClosed)
	}
	if got := len(worker.sendQueue); got != 1 {
		t.Fatalf("send queue after admitted CloseSend = %d, want 1", got)
	}
}

func TestClientStreamCloseWinsInflightSenderSettlement(t *testing.T) {
	for _, operation := range []string{"send", "closeSend"} {
		t.Run(operation, func(t *testing.T) {
			env := newGrpcTestEnv(t)
			defer env.shutdown()

			ctx, cancel := context.WithCancel(env.grpcMod.ctx)
			options := &callOpts{
				module: env.grpcMod,
				ctx:    ctx,
				cancel: cancel,
			}
			if err := options.register(); err != nil {
				t.Fatal(err)
			}
			entered := make(chan struct{})
			release := make(chan struct{})
			var releaseOnce sync.Once
			defer releaseOnce.Do(func() { close(release) })
			var transportCalls atomic.Int32
			blockTransport := func() {
				if transportCalls.Add(1) == 1 {
					close(entered)
				}
				<-release
			}
			stream := &phase3MockStream{ctx: ctx}
			stream.sendMsgFn = func(any) error {
				blockTransport()
				return nil
			}
			stream.closeSendFn = func() error {
				blockTransport()
				return nil
			}
			input := phase3FindMsgDesc(t, env, "testgrpc.Item")
			_, projection, err := newClientStreamHarness(
				env.grpcMod,
				stream,
				input,
				input,
				options,
			)
			if err != nil {
				t.Fatal(err)
			}
			var promise any
			switch operation {
			case "send":
				message, wrapErr := env.pbMod.WrapMessage(dynamicpb.NewMessage(input))
				if wrapErr != nil {
					t.Fatal(wrapErr)
				}
				promise = projection.send(message)
			case "closeSend":
				promise = projection.closeSend()
			default:
				t.Fatalf("unsupported operation %q", operation)
			}
			if err := env.runtime.Set("__senderPromise", promise); err != nil {
				t.Fatal(err)
			}
			settled := make(chan struct{}, 1)
			var resolves atomic.Int32
			var rejects atomic.Int32
			var rejectionCode atomic.Int64
			if err := env.runtime.Set("__senderResolved", func() {
				resolves.Add(1)
				settled <- struct{}{}
			}); err != nil {
				t.Fatal(err)
			}
			if err := env.runtime.Set("__senderRejected", func(code int64) {
				rejectionCode.Store(code)
				rejects.Add(1)
				settled <- struct{}{}
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := env.runtime.RunString(`
				__senderPromise.then(
					function() { __senderResolved(); },
					function(error) { __senderRejected(error.code); }
				);
			`); err != nil {
				t.Fatal(err)
			}
			select {
			case <-entered:
			case <-time.After(defaultTimeout):
				t.Fatal("sender did not enter transport call")
			}

			stop := withLoopRunning(t, env, defaultTimeout)
			defer stop()
			closeDone := make(chan error, 1)
			go func() { closeDone <- env.grpcMod.Close() }()
			select {
			case <-settled:
			case <-time.After(defaultTimeout):
				t.Fatal("Module.Close did not settle in-flight sender")
			}
			select {
			case err := <-closeDone:
				t.Fatalf("Module.Close returned before transport release: %v", err)
			default:
			}
			releaseOnce.Do(func() { close(release) })
			select {
			case err := <-closeDone:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(defaultTimeout):
				t.Fatal("Module.Close did not join released sender")
			}

			if got := resolves.Load(); got != 0 {
				t.Fatalf("resolve calls = %d, want 0", got)
			}
			if got := rejects.Load(); got != 1 {
				t.Fatalf("reject calls = %d, want 1", got)
			}
			if got := codes.Code(rejectionCode.Load()); got != codes.Unavailable {
				t.Fatalf("rejection code = %v, want Unavailable", got)
			}
			if got := transportCalls.Load(); got != 1 {
				t.Fatalf("transport calls = %d, want 1", got)
			}
		})
	}
}
