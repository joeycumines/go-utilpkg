package inprocgrpc

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/joeycumines/go-inprocgrpc/internal/callopts"
	"github.com/joeycumines/go-inprocgrpc/internal/stream"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestDroppedPreparedResponseBeatsWatcherCancellation(t *testing.T) {
	loop := &droppedApplyLoop{
		done:      make(chan struct{}),
		submitted: make(chan struct{}, 1),
	}
	state := stream.NewRPCState("/test.Service/Call", 1)
	ctx, cancel := context.WithCancel(context.Background())
	life := newRPCLifecycle(loop, state, cancel)
	life.watch(ctx)
	client := &clientStreamAdapter{
		ctx:           ctx,
		callerCtx:     ctx,
		loop:          loop,
		cloner:        ProtoCloner{},
		life:          life,
		state:         state,
		copts:         new(callopts.CallOptions),
		serverStreams: true,
	}
	response := new(wrapperspb.StringValue)
	result := make(chan error, 1)
	go func() { result <- client.RecvMsg(response) }()
	<-loop.submitted
	if !life.serverFinishPrepared(nil, &terminalPreparation{
		response:     &wrapperspb.StringValue{Value: "recovered"},
		sendResponse: true,
	}) {
		t.Fatal("prepared response did not win")
	}
	// The terminal fence is the exact acknowledgement that SubmitInternal
	// returned successfully. Do not close Done merely because the fixture
	// observed entry into SubmitInternal: scheduler loss at that earlier point
	// is allowed to reject the prepared response.
	life.control.mu.Lock()
	for !life.control.state.terminalFenced {
		life.control.cond.Wait()
	}
	accepted := life.control.state.terminalAccepted
	life.control.mu.Unlock()
	if !accepted {
		t.Fatal("prepared terminal submission was not accepted")
	}
	cancel()
	close(loop.done)
	if err := <-result; err != nil {
		t.Fatalf("RecvMsg = %v", err)
	}
	if response.GetValue() != "recovered" {
		t.Fatalf("response = %q, want recovered", response.GetValue())
	}
	<-life.release
}

func TestWatcherCancellationDiscardsUnobservedGracefulData(t *testing.T) {
	loop := &signaledLiveLoop{
		done:      make(chan struct{}),
		submitted: make(chan struct{}, 1),
	}
	state := stream.NewRPCState("/test.Service/Call", 2)
	if err := state.Responses.TrySend(
		&wrapperspb.StringValue{Value: "unobserved"},
	); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	life := newRPCLifecycle(loop, state, nil)
	life.watch(ctx)
	if !life.serverFinish(nil) {
		t.Fatal("graceful terminal did not win")
	}
	cancel()
	select {
	case <-life.release:
	case <-time.After(time.Second):
		t.Fatal("unobserved graceful data was not released")
	}
	if !state.Responses.Drained() {
		t.Fatal("unobserved response remained retained")
	}
	if status.Code(life.abandonmentError()) != codes.Canceled {
		t.Fatalf("abandonment error = %v, want Canceled",
			life.abandonmentError(),
		)
	}
}

func TestSelectedHeadersBeatLaterCallerCancellation(t *testing.T) {
	loop := &capturedTerminalLoop{
		done:      make(chan struct{}),
		internal:  make(chan func(), 1),
		submitted: make(chan struct{}, 1),
	}
	state := stream.NewRPCState("/test.Service/Call", 1)
	state.ResponseHeaders = metadata.Pairs("selected", "header")
	ctx, cancel := context.WithCancel(context.Background())
	life := newRPCLifecycle(loop, state, cancel)
	client := &clientStreamAdapter{
		ctx:       ctx,
		callerCtx: ctx,
		loop:      loop,
		life:      life,
		state:     state,
		copts:     new(callopts.CallOptions),
	}
	type headerResult struct {
		headers metadata.MD
		err     error
	}
	result := make(chan headerResult, 1)
	go func() {
		headers, err := client.Header()
		result <- headerResult{headers: headers, err: err}
	}()
	<-loop.submitted
	if !life.serverFinish(nil) {
		t.Fatal("server terminal did not win")
	}
	cancel()
	select {
	case result := <-result:
		t.Fatalf("Header returned before selected apply: %+v", result)
	default:
	}
	task := <-loop.internal
	task()
	got := <-result
	if got.err != nil || len(got.headers.Get("selected")) != 1 ||
		got.headers.Get("selected")[0] != "header" {
		t.Fatalf("Header = %v, %v, want selected header", got.headers, got.err)
	}
	<-life.release
	close(loop.done)
}

func TestSelectedGracefulTerminalBeatsLaterSendCancellation(t *testing.T) {
	loop := &capturedTerminalLoop{
		done:      make(chan struct{}),
		internal:  make(chan func(), 1),
		submitted: make(chan struct{}, 1),
	}
	state := stream.NewRPCState("/test.Service/Call", 1)
	if err := state.Requests.TrySend(
		&wrapperspb.StringValue{Value: "buffered"},
	); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	life := newRPCLifecycle(loop, state, cancel)
	client := &clientStreamAdapter{
		ctx:           ctx,
		callerCtx:     ctx,
		loop:          loop,
		cloner:        ProtoCloner{},
		life:          life,
		state:         state,
		copts:         new(callopts.CallOptions),
		clientStreams: true,
	}
	result := make(chan error, 1)
	go func() {
		result <- client.SendMsg(
			&wrapperspb.StringValue{Value: "pending"},
		)
	}()
	<-loop.submitted
	if !life.serverFinish(nil) {
		t.Fatal("server terminal did not win")
	}
	cancel()
	select {
	case err := <-result:
		t.Fatalf("SendMsg returned before selected apply: %v", err)
	default:
	}
	task := <-loop.internal
	task()
	if err := <-result; err != io.EOF {
		t.Fatalf("SendMsg = %v, want EOF", err)
	}
	<-life.release
	close(loop.done)
}

func TestTerminalResultSurvivesSchedulerRecovery(t *testing.T) {
	loop := &droppedApplyLoop{done: make(chan struct{})}
	state := stream.NewRPCState("/test.Service/Call", 1)
	if err := state.Responses.TrySend(
		&wrapperspb.StringValue{Value: "retained"},
	); err != nil {
		t.Fatal(err)
	}
	life := newRPCLifecycle(loop, state, nil)
	rpcStream := &RPCStream{
		state:         state,
		life:          life,
		clientStreams: true,
		serverStreams: true,
	}
	if !rpcStream.Finish(nil) {
		t.Fatal("graceful terminal did not win")
	}
	if err, terminal := rpcStream.TerminalResult(); err != nil || !terminal {
		t.Fatalf("TerminalResult before Done = %v, %v", err, terminal)
	}
	close(loop.done)
	client := &clientStreamAdapter{
		ctx:       context.Background(),
		callerCtx: context.Background(),
		life:      life,
	}
	retained := client.terminalReceiveResult()
	message, ok := retained.msg.(*wrapperspb.StringValue)
	if !ok || message.GetValue() != "retained" {
		t.Fatalf("recovered response = %#v", retained.msg)
	}
	if retained.recoveryDelivery == nil {
		t.Fatal("recovered response has no delivery identity")
	}
	life.endRecoveryDelivery(retained.recoveryDelivery)
	terminal := client.terminalReceiveResult()
	if terminal.err != io.EOF {
		t.Fatalf("recovered terminal = %v, want EOF", terminal.err)
	}
	<-life.release
	if err, terminal := rpcStream.TerminalResult(); err != nil || !terminal {
		t.Fatalf("TerminalResult after Done = %v, %v", err, terminal)
	}
	if result := life.resolveScheduler(); result.err != nil || !result.clean {
		t.Fatalf("scheduler observation = %+v, want clean", result)
	}
}

func TestSchedulerRecoveryPreservesResponsesAtDeliveryIDExhaustion(t *testing.T) {
	loop := &droppedApplyLoop{done: make(chan struct{})}
	state := stream.NewRPCState("/test.Service/Call", 2)
	for _, value := range []string{"first", "second"} {
		if err := state.Responses.TrySend(
			&wrapperspb.StringValue{Value: value},
		); err != nil {
			t.Fatal(err)
		}
	}
	life := newRPCLifecycle(loop, state, nil)
	life.control.mu.Lock()
	life.control.state.nextDeliveryID = ^uint64(0)
	life.control.state.deliveriesDone = ^uint64(0)
	life.control.mu.Unlock()
	if !life.serverFinish(nil) {
		t.Fatal("graceful terminal did not win")
	}
	close(loop.done)
	client := &clientStreamAdapter{
		ctx:       context.Background(),
		callerCtx: context.Background(),
		life:      life,
	}
	for _, want := range []string{"first", "second"} {
		result := client.terminalReceiveResult()
		message, ok := result.msg.(*wrapperspb.StringValue)
		if !ok || message.GetValue() != want {
			t.Fatalf("recovered response = %#v, want %q", result.msg, want)
		}
		if result.recoveryDelivery == nil {
			t.Fatal("recovered response has no delivery identity")
		}
		life.endRecoveryDelivery(result.recoveryDelivery)
	}
	if terminal := client.terminalReceiveResult(); terminal.err != io.EOF {
		t.Fatalf("recovered terminal = %v, want EOF", terminal.err)
	}
	select {
	case <-life.release:
	case <-time.After(time.Second):
		t.Fatal("saturated delivery recovery did not release")
	}
}

func TestCallerTerminalBeatsLateServerTerminal(t *testing.T) {
	loop := newPublishedResultLoop()
	state := stream.NewRPCState("/test.Service/Call", 1)
	life := newRPCLifecycle(loop, state, nil)
	rpcStream := &RPCStream{
		state:         state,
		life:          life,
		clientStreams: true,
		serverStreams: true,
	}
	if !life.callerCancel(context.Canceled) {
		t.Fatal("caller cancellation did not win")
	}
	before, terminal := rpcStream.TerminalResult()
	if !terminal || status.Code(before) != codes.Canceled {
		t.Fatalf("TerminalResult = %v, %v, want Canceled", before, terminal)
	}
	if rpcStream.Finish(nil) {
		t.Fatal("late server Finish won")
	}
	if rpcStream.Abort(status.Error(codes.Aborted, "late")) {
		t.Fatal("late server Abort won")
	}
	<-life.release
	after, terminal := rpcStream.TerminalResult()
	if !terminal || after.Error() != before.Error() {
		t.Fatalf(
			"stable TerminalResult = %v, %v, want %v",
			after,
			terminal,
			before,
		)
	}
}

func TestSchedulerTerminalBeatsLateServerTerminal(t *testing.T) {
	loop := newDroppedOwnerLoop()
	loop.close()
	state := stream.NewRPCState("/test.Service/Call", 1)
	life := newRPCLifecycle(loop, state, nil)
	rpcStream := &RPCStream{
		state:         state,
		life:          life,
		clientStreams: true,
		serverStreams: true,
	}
	if !life.schedulerFailure(unavailableError()) {
		t.Fatal("scheduler failure did not win")
	}
	before, terminal := rpcStream.TerminalResult()
	if !terminal || status.Code(before) != codes.Unavailable {
		t.Fatalf(
			"TerminalResult = %v, %v, want Unavailable",
			before,
			terminal,
		)
	}
	if rpcStream.Finish(nil) {
		t.Fatal("late server Finish won")
	}
	if rpcStream.Abort(status.Error(codes.Aborted, "late")) {
		t.Fatal("late server Abort won")
	}
	<-life.release
	after, terminal := rpcStream.TerminalResult()
	if !terminal || after.Error() != before.Error() {
		t.Fatalf(
			"stable TerminalResult = %v, %v, want %v",
			after,
			terminal,
			before,
		)
	}
}

func newPublishedResultLoop() *publishedResultLoop {
	return &publishedResultLoop{done: make(chan struct{})}
}

func (l *publishedResultLoop) Submit(task func()) error {
	task()
	l.once.Do(func() { close(l.done) })
	return nil
}

func (l *publishedResultLoop) SubmitInternal(task func()) error {
	return l.Submit(task)
}

func (l *publishedResultLoop) Done() <-chan struct{} {
	return l.done
}

func TestPublishedOwnerResultsBeatDone(t *testing.T) {
	for range 128 {
		t.Run("header", func(t *testing.T) {
			loop := newPublishedResultLoop()
			state := stream.NewRPCState("/test.Service/Call", 1)
			state.ResponseHeaders = metadata.Pairs("result", "header")
			state.SendHeaders()
			life := newRPCLifecycle(loop, state, nil)
			client := &clientStreamAdapter{
				ctx:       context.Background(),
				callerCtx: context.Background(),
				loop:      loop,
				life:      life,
				state:     state,
				copts:     new(callopts.CallOptions),
			}
			headers, err := client.Header()
			if err != nil || headers.Get("result")[0] != "header" {
				t.Fatalf("Header = %v, %v", headers, err)
			}
		})

		t.Run("trailer", func(t *testing.T) {
			loop := newPublishedResultLoop()
			state := stream.NewRPCState("/test.Service/Call", 1)
			state.ResponseTrailers = metadata.Pairs("result", "trailer")
			state.Complete(status.Error(codes.Aborted, "done"))
			life := newRPCLifecycle(loop, state, nil)
			client := &clientStreamAdapter{
				ctx:       context.Background(),
				callerCtx: context.Background(),
				loop:      loop,
				life:      life,
				state:     state,
				copts:     new(callopts.CallOptions),
			}
			trailers := client.Trailer()
			if trailers.Get("result")[0] != "trailer" {
				t.Fatalf("Trailer = %v", trailers)
			}
		})

		t.Run("server receive", func(t *testing.T) {
			loop := newPublishedResultLoop()
			state := stream.NewRPCState("/test.Service/Call", 1)
			if err := state.Requests.TrySend(
				&wrapperspb.StringValue{Value: "request"},
			); err != nil {
				t.Fatal(err)
			}
			life := newRPCLifecycle(loop, state, nil)
			server := &serverStreamAdapter{
				ctx:           context.Background(),
				loop:          loop,
				life:          life,
				state:         state,
				cloneDisabled: true,
				clientStreams: true,
			}
			message := new(wrapperspb.StringValue)
			if err := server.RecvMsg(message); err != nil {
				t.Fatalf("RecvMsg = %v", err)
			}
			if message.GetValue() != "request" {
				t.Fatalf("message = %q", message.GetValue())
			}
		})

		t.Run("server publication", func(t *testing.T) {
			loop := newPublishedResultLoop()
			state := stream.NewRPCState("/test.Service/Call", 1)
			life := newRPCLifecycle(loop, state, nil)
			server := &serverStreamAdapter{
				ctx:   context.Background(),
				loop:  loop,
				life:  life,
				state: state,
			}
			result := make(chan error, 1)
			result <- nil
			loop.once.Do(func() { close(loop.done) })
			if err := server.waitOwner(result); err != nil {
				t.Fatalf("waitOwner = %v", err)
			}
		})
	}
}

type droppedOwnerLoop struct {
	done chan struct{}
	once sync.Once
}

func newDroppedOwnerLoop() *droppedOwnerLoop {
	return &droppedOwnerLoop{done: make(chan struct{})}
}

func (*droppedOwnerLoop) Submit(func()) error         { return nil }
func (*droppedOwnerLoop) SubmitInternal(func()) error { return nil }
func (l *droppedOwnerLoop) Done() <-chan struct{}     { return l.done }
func (l *droppedOwnerLoop) close() {
	l.once.Do(func() { close(l.done) })
}

func TestClientFailureRetainsLocalStatusAcrossInternalCancel(t *testing.T) {
	loop := newDroppedOwnerLoop()
	t.Cleanup(loop.close)
	state := stream.NewRPCState("/test.Service/Call", 1)
	ctx, cancel := context.WithCancel(context.Background())
	life := newRPCLifecycle(loop, state, cancel)
	if !life.serverFinish(nil) {
		t.Fatal("initial terminal claim failed")
	}
	local := cloneError("copy response", context.Canceled)
	life.clientFailure(local)
	loop.close()
	client := &clientStreamAdapter{
		ctx:       ctx,
		callerCtx: context.Background(),
		loop:      loop,
		life:      life,
		state:     state,
	}
	result := client.receive()
	if status.Code(result.err) != codes.Internal {
		t.Fatalf("receive error = %v, want Internal", result.err)
	}
	if !errors.Is(result.err, context.Canceled) {
		t.Fatalf("receive error = %v, want wrapped cause", result.err)
	}
}

func TestClaimedTerminalGuardsLateHalfClose(t *testing.T) {
	loop := newDroppedOwnerLoop()
	t.Cleanup(loop.close)
	state := stream.NewRPCState("/test.Service/Call", 1)
	life := newRPCLifecycle(loop, state, nil)
	if !life.serverFinish(nil) {
		t.Fatal("initial terminal claim failed")
	}
	sender := rpcStreamSender{stream: &RPCStream{state: state, life: life}}
	sender.Close(status.Error(codes.Aborted, "late"))
	if state.Responses.Closed() {
		t.Fatal("late half-close overrode the claimed RPC terminal")
	}
}

func TestRPCStreamAbortAnyGoroutineFirstWinner(t *testing.T) {
	loop := newPublishedResultLoop()
	state := stream.NewRPCState("/test.Service/Call", 1)
	rpc := &RPCStream{state: state, life: newRPCLifecycle(loop, state, nil)}
	result := make(chan bool, 1)
	go func() {
		result <- rpc.Abort(status.Error(codes.Aborted, "first"))
	}()
	if !<-result {
		t.Fatal("first Abort lost")
	}
	if rpc.Abort(status.Error(codes.DataLoss, "late")) {
		t.Fatal("second Abort won")
	}
	if err, terminal := rpc.TerminalResult(); !terminal ||
		status.Code(err) != codes.Aborted {
		t.Fatalf("terminal result = %v, %t", err, terminal)
	}
	<-rpc.Done()
}

type signaledLiveLoop struct {
	done      chan struct{}
	submitted chan struct{}
}

func (l *signaledLiveLoop) Submit(task func()) error {
	task()
	l.submitted <- struct{}{}
	return nil
}

func (*signaledLiveLoop) SubmitInternal(task func()) error {
	task()
	return nil
}

func (l *signaledLiveLoop) Done() <-chan struct{} {
	return l.done
}

type blockingSubmitLoop struct {
	done    chan struct{}
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (l *blockingSubmitLoop) Submit(task func()) error {
	l.once.Do(func() { close(l.entered) })
	<-l.release
	task()
	return nil
}

func (*blockingSubmitLoop) SubmitInternal(task func()) error {
	task()
	return nil
}

func (l *blockingSubmitLoop) Done() <-chan struct{} {
	return l.done
}
