package inprocgrpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/joeycumines/go-inprocgrpc/internal/callopts"
	"github.com/joeycumines/go-inprocgrpc/internal/stream"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type publishedResultLoop struct {
	done chan struct{}
	once sync.Once
}

func requireRPCStats(
	t *testing.T,
	helper *statsHandlerHelper,
	ctx context.Context,
	method string,
	clientStreams bool,
	serverStreams bool,
) *rpcStats {
	t.Helper()
	result, err := helper.startRPC(
		ctx,
		method,
		clientStreams,
		serverStreams,
	)
	if err != nil {
		t.Fatalf("start RPC stats: %v", err)
	}
	return result
}

type blockingRejectLoop struct {
	done    chan struct{}
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

type capturedInternalLoop struct {
	done chan struct{}
	task chan func()
}

type reentrantMetadataHandler struct {
	client *clientStreamAdapter
	called chan string
}

func (*reentrantMetadataHandler) TagRPC(
	ctx context.Context,
	_ *stats.RPCTagInfo,
) context.Context {
	return ctx
}

func (h *reentrantMetadataHandler) HandleRPC(
	_ context.Context,
	event stats.RPCStats,
) {
	switch event.(type) {
	case *stats.InHeader:
		_, _ = h.client.Header()
		h.called <- "header"
	case *stats.InTrailer:
		_ = h.client.Trailer()
		h.called <- "trailer"
	}
}

func (*reentrantMetadataHandler) TagConn(
	ctx context.Context,
	_ *stats.ConnTagInfo,
) context.Context {
	return ctx
}

func (*reentrantMetadataHandler) HandleConn(
	context.Context,
	stats.ConnStats,
) {
}

func (l *capturedInternalLoop) Submit(func()) error {
	return errors.New("submit unsupported")
}

func (l *capturedInternalLoop) SubmitInternal(task func()) error {
	l.task <- task
	return nil
}

func (l *capturedInternalLoop) Done() <-chan struct{} { return l.done }

type capturedTerminalLoop struct {
	done      chan struct{}
	internal  chan func()
	submitted chan struct{}
}

func (l *capturedTerminalLoop) Submit(task func()) error {
	task()
	l.submitted <- struct{}{}
	return nil
}

func (l *capturedTerminalLoop) SubmitInternal(task func()) error {
	l.internal <- task
	return nil
}

func (l *capturedTerminalLoop) Done() <-chan struct{} { return l.done }

type initializationRaceLoop struct {
	done  chan struct{}
	after func()
}

func (l *initializationRaceLoop) Submit(task func()) error {
	task()
	l.after()
	return nil
}

func (*initializationRaceLoop) SubmitInternal(task func()) error {
	task()
	return nil
}

func (l *initializationRaceLoop) Done() <-chan struct{} { return l.done }

func TestStartRPCPrefersPublishedInitialization(t *testing.T) {
	for _, trigger := range []string{"context", "loop"} {
		t.Run(trigger, func(t *testing.T) {
			for iteration := range 64 {
				ctx, cancel := context.WithCancel(context.Background())
				loop := &initializationRaceLoop{done: make(chan struct{})}
				switch trigger {
				case "context":
					loop.after = cancel
				case "loop":
					loop.after = func() { close(loop.done) }
				}
				channel := NewChannel(WithLoop(loop))
				client, err := channel.startRPC(
					ctx,
					"/test.Service/Call",
					new(callopts.CallOptions),
					rpcTarget{
						callback: func(context.Context, *RPCStream) {},
					},
				)
				if err != nil {
					t.Fatalf("iteration %d: startRPC = %v", iteration, err)
				}
				if client == nil {
					t.Fatalf("iteration %d: startRPC returned nil stream", iteration)
				}
				<-client.life.release
				cancel()
				select {
				case <-loop.done:
				default:
					close(loop.done)
				}
			}
		})
	}
}

func newBlockingRejectLoop() *blockingRejectLoop {
	return &blockingRejectLoop{
		done:    make(chan struct{}),
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (l *blockingRejectLoop) Submit(func()) error {
	return errors.New("submit rejected")
}

func (l *blockingRejectLoop) SubmitInternal(func()) error {
	l.once.Do(func() { close(l.entered) })
	<-l.release
	return errors.New("internal submit rejected")
}

func (l *blockingRejectLoop) Done() <-chan struct{} { return l.done }

func TestTerminalResultStableWhileOwnerSubmissionBlocked(t *testing.T) {
	loop := newBlockingRejectLoop()
	state := stream.NewRPCState("/test.Service/Call", 1)
	life := newRPCLifecycle(loop, state, nil)
	rpcStream := &RPCStream{
		state:         state,
		life:          life,
		clientStreams: true,
		serverStreams: true,
	}
	abortDone := make(chan bool, 1)
	go func() {
		abortDone <- rpcStream.Abort(
			status.Error(codes.Aborted, "requested"),
		)
	}()
	<-loop.entered
	if won := <-abortDone; !won {
		t.Fatal("server abort did not synchronously select the terminal result")
	}

	type observation struct {
		err      error
		terminal bool
	}
	observed := make(chan observation, 1)
	go func() {
		err, terminal := rpcStream.TerminalResult()
		observed <- observation{err: err, terminal: terminal}
	}()
	result := <-observed
	if !result.terminal || status.Code(result.err) != codes.Aborted {
		t.Fatalf("TerminalResult = %+v, want stable Aborted", result)
	}
	close(loop.done)
	schedulerObserved := make(chan schedulerResult, 1)
	go func() { schedulerObserved <- life.resolveScheduler() }()
	close(loop.release)
	<-life.release
	if result := <-schedulerObserved; status.Code(result.err) !=
		codes.Aborted {
		t.Fatalf("scheduler result = %v, want Aborted", result.err)
	}
	err, terminal := rpcStream.TerminalResult()
	if !terminal || status.Code(err) != codes.Aborted {
		t.Fatalf(
			"later TerminalResult = %v, %v, want stable Aborted",
			err,
			terminal,
		)
	}
}

func TestOwnerSendCheckDoesNotJoinPreparedTerminal(t *testing.T) {
	loop := &capturedInternalLoop{
		done: make(chan struct{}),
		task: make(chan func(), 1),
	}
	state := stream.NewRPCState("/test.Service/Call", 1)
	life := newRPCLifecycle(loop, state, nil)

	ownerTurn, admitted := life.control.reserveCallback()
	if !admitted {
		t.Fatal("pre-boundary callback was not reserved")
	}
	life.control.ownerFence(ownerTurn, true)
	capability, started := life.control.startOwner(ownerTurn)
	if !started {
		t.Fatal("pre-boundary callback was not started")
	}
	if !life.serverFinishPrepared(nil, &terminalPreparation{}) {
		t.Fatal("prepared terminal did not win")
	}

	result := make(chan error, 1)
	go func() { result <- life.serverSendError() }()
	select {
	case err := <-result:
		if !errors.Is(err, io.EOF) {
			t.Fatalf("owner send check = %v, want EOF", err)
		}
	case <-time.After(time.Second):
		t.Fatal("owner send check joined prepared terminal")
	}

	life.control.completeCallback(capability, false, state.Responses.Drained())
	task := <-loop.task
	task()
	close(loop.done)
}

func TestCallbackTurnRunOwnsSettlement(t *testing.T) {
	loop := &capturedInternalLoop{
		done: make(chan struct{}),
		task: make(chan func(), 1),
	}
	state := stream.NewRPCState("/test.Service/Call", 1)
	life := newRPCLifecycle(loop, state, nil)
	rpc := &RPCStream{state: state, life: life}
	turn, admitted := rpc.AdmitCallback()
	if !admitted {
		t.Fatal("direct callback was not admitted")
	}
	called := false
	turn.Run(func() {
		nested, nestedAdmitted := rpc.AdmitCallback()
		if !nestedAdmitted {
			t.Fatal("reentrant callback was not admitted")
		}
		nested.Run(func() { called = true })
	})
	if !called {
		t.Fatal("direct callback did not run")
	}
	if _, admitted := rpc.AdmitCallback(); !admitted {
		t.Fatal("direct callback settlement was not recorded")
	}
}

func TestCallbackTurnNilCallbackStillSettles(t *testing.T) {
	loop := &capturedInternalLoop{
		done: make(chan struct{}),
		task: make(chan func(), 1),
	}
	state := stream.NewRPCState("/test.Service/Call", 1)
	rpc := &RPCStream{
		state: state,
		life:  newRPCLifecycle(loop, state, nil),
	}
	turn, admitted := rpc.AdmitCallback()
	if !admitted {
		t.Fatal("direct callback was not admitted")
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("nil callback did not panic")
			}
		}()
		turn.Run(nil)
	}()
	select {
	case task := <-loop.task:
		task()
	case <-time.After(time.Second):
		t.Fatal("terminal task remained blocked by nil callback turn")
	}
	select {
	case <-rpc.Done():
	case <-time.After(time.Second):
		t.Fatal("Done remained blocked by nil callback turn")
	}
	if err, terminal := rpc.TerminalResult(); !terminal ||
		status.Code(err) != codes.Internal {
		t.Fatalf("terminal result = %v, %t, want Internal", err, terminal)
	}
}

func TestClientMetadataStatsCallbacksMayReenter(t *testing.T) {
	loop := newDroppedOwnerLoop()
	t.Cleanup(loop.close)
	state := stream.NewRPCState("/test.Service/Call", 1)
	client := &clientStreamAdapter{
		ctx:       context.Background(),
		callerCtx: context.Background(),
		loop:      loop,
		life:      newRPCLifecycle(loop, state, nil),
		state:     state,
		copts:     new(callopts.CallOptions),
	}
	handler := &reentrantMetadataHandler{
		client: client,
		called: make(chan string, 2),
	}
	client.stats = requireRPCStats(t, &statsHandlerHelper{
		handler:  handler,
		isClient: true,
	}, context.Background(), state.Method, false, false)
	if err := client.storeHeaders(metadata.Pairs("header", "value")); err != nil {
		t.Fatalf("store headers = %v", err)
	}
	if err := client.storeTrailers(metadata.Pairs("trailer", "value")); err != nil {
		t.Fatalf("store trailers = %v", err)
	}
	for _, want := range []string{"header", "trailer"} {
		select {
		case got := <-handler.called:
			if got != want {
				t.Fatalf("reentrant callback = %q, want %q", got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("reentrant %s callback deadlocked", want)
		}
	}
	if err := client.stats.end(nil); err != nil {
		t.Fatalf("stats End = %v", err)
	}
}

func TestCallbackTurnRejectsEscapedInheritedScope(t *testing.T) {
	newRPC := func() (*capturedInternalLoop, *RPCStream) {
		loop := &capturedInternalLoop{
			done: make(chan struct{}),
			task: make(chan func(), 1),
		}
		state := stream.NewRPCState("/test.Service/Call", 1)
		return loop, &RPCStream{
			state: state,
			life:  newRPCLifecycle(loop, state, nil),
		}
	}
	assertRejected := func(t *testing.T, turn *CallbackTurn) {
		t.Helper()
		called := false
		defer func() {
			if recover() == nil {
				t.Fatal("escaped callback turn did not panic")
			}
			if called {
				t.Fatal("escaped callback executed")
			}
		}()
		turn.Run(func() { called = true })
	}

	t.Run("after terminal release", func(t *testing.T) {
		loop, rpc := newRPC()
		parent, admitted := rpc.AdmitCallback()
		if !admitted {
			t.Fatal("parent callback was not admitted")
		}
		var escaped *CallbackTurn
		parent.Run(func() {
			var nested bool
			escaped, nested = rpc.AdmitCallback()
			if !nested {
				t.Fatal("nested callback was not admitted")
			}
		})
		rpc.Abort(status.Error(codes.Aborted, "done"))
		(<-loop.task)()
		<-rpc.Done()
		assertRejected(t, escaped)
	})

	t.Run("during unrelated owner scope", func(t *testing.T) {
		_, rpc := newRPC()
		parent, admitted := rpc.AdmitCallback()
		if !admitted {
			t.Fatal("parent callback was not admitted")
		}
		var escaped *CallbackTurn
		parent.Run(func() {
			var nested bool
			escaped, nested = rpc.AdmitCallback()
			if !nested {
				t.Fatal("nested callback was not admitted")
			}
		})
		later, admitted := rpc.AdmitCallback()
		if !admitted {
			t.Fatal("later callback was not admitted")
		}
		later.Run(func() { assertRejected(t, escaped) })
	})
}

func TestTerminalResultWaitsForPreparedCompletion(t *testing.T) {
	loop := &capturedInternalLoop{
		done: make(chan struct{}),
		task: make(chan func(), 1),
	}
	state := stream.NewRPCState("/test.Service/Call", 1)
	life := newRPCLifecycle(loop, state, nil)
	prepareErr := status.Error(codes.DataLoss, "prepare failed")
	if !life.serverFinishPrepared(nil, &terminalPreparation{err: prepareErr}) {
		t.Fatal("prepared terminal did not win")
	}
	observed := make(chan error, 1)
	go func() {
		err, _ := life.terminalSelection()
		observed <- err
	}()
	select {
	case err := <-observed:
		t.Fatalf("TerminalResult returned before prepare: %v", err)
	default:
	}
	task := <-loop.task
	task()
	before := <-observed
	if status.Code(before) != codes.DataLoss {
		t.Fatalf("TerminalResult = %v, want DataLoss", before)
	}
	<-life.release
	after, terminal := life.terminalSelection()
	if !terminal || after.Error() != before.Error() {
		t.Fatalf(
			"stable TerminalResult = %v, %v, want %v",
			after,
			terminal,
			before,
		)
	}
}

func TestSchedulerExhaustionUsesRecoveryPublication(t *testing.T) {
	for _, finalizationRequired := range []bool{false, true} {
		t.Run(fmt.Sprintf("finalization=%t", finalizationRequired), func(t *testing.T) {
			loop := &capturedInternalLoop{
				done: make(chan struct{}),
				task: make(chan func(), 1),
			}
			state := stream.NewRPCState("/test.Service/Call", 1)
			life := newRPCLifecycle(
				loop,
				state,
				nil,
				finalizationRequired,
			)
			life.control.mu.Lock()
			life.control.state.nextTerminalID = ^uint64(0)
			life.control.state.nextOwnerTurn = ^uint64(0)
			life.control.state.ownerFenced = ^uint64(0)
			life.control.state.ownerSettled = ^uint64(0)
			life.control.state.ownerCompacted = ^uint64(0)
			life.control.mu.Unlock()
			close(loop.done)
			go life.releaseAfterScheduler()
			result := life.resolveScheduler()
			if status.Code(result.err) != codes.Unavailable || result.clean {
				t.Fatalf("scheduler result = %+v", result)
			}
			select {
			case <-life.release:
			case <-time.After(time.Second):
				t.Fatal("scheduler exhaustion did not release")
			}
			if !life.control.usesRecovery() {
				t.Fatal("scheduler exhaustion bypassed recovery")
			}
		})
	}
}

func TestSelectedPreparedResponseBeatsLaterCallerCancellation(t *testing.T) {
	loop := &capturedTerminalLoop{
		done:      make(chan struct{}),
		internal:  make(chan func(), 1),
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
		response:     &wrapperspb.StringValue{Value: "selected"},
		sendResponse: true,
	}) {
		t.Fatal("prepared response did not win")
	}
	cancel()
	select {
	case err := <-result:
		t.Fatalf("RecvMsg returned before selected apply: %v", err)
	default:
	}
	task := <-loop.internal
	task()
	if err := <-result; err != nil {
		t.Fatalf("RecvMsg = %v", err)
	}
	if response.GetValue() != "selected" {
		t.Fatalf("response = %q, want selected", response.GetValue())
	}
	const continuationLimit = 1
	released := false
	for range continuationLimit {
		select {
		case <-life.release:
			released = true
		case continuation := <-loop.internal:
			continuation()
		}
		if released {
			break
		}
	}
	if !released {
		select {
		case <-life.release:
			released = true
		default:
			t.Fatalf(
				"RPC retained accepted work after %d continuations",
				continuationLimit,
			)
		}
	}
	close(loop.done)
}
