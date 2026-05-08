package inprocgrpc

import (
	"context"
	"errors"
	"testing"

	"github.com/joeycumines/go-inprocgrpc/internal/callopts"
	"github.com/joeycumines/go-inprocgrpc/internal/stream"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestLastConsumerCommitsAbandonmentBeforeNextAdmission(t *testing.T) {
	loop := &blockingSubmitLoop{
		done:    make(chan struct{}),
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	state := stream.NewRPCState("/test.Service/Call", 1)
	if err := state.Responses.TrySend(
		&wrapperspb.StringValue{Value: "discard"},
	); err != nil {
		t.Fatal(err)
	}
	life := newRPCLifecycle(loop, state, nil)
	if !life.serverFinish(nil) {
		t.Fatal("graceful terminal did not win")
	}
	if !life.beginTerminalConsumer() {
		t.Fatal("initial terminal consumer was not admitted")
	}
	cancelErr := status.Error(codes.Canceled, "discard")
	life.abandonClientData(cancelErr)
	ended := make(chan struct{})
	go func() {
		life.endTerminalConsumer()
		close(ended)
	}()
	<-loop.entered
	if life.beginTerminalConsumer() {
		t.Fatal("consumer entered after abandonment commit")
	}
	if status.Code(life.abandonmentError()) != codes.Canceled {
		t.Fatalf("abandonment error = %v", life.abandonmentError())
	}
	close(loop.release)
	<-ended
	<-life.release
}

func TestObservedConsumerClearsPendingAbandonment(t *testing.T) {
	loop := newDroppedOwnerLoop()
	t.Cleanup(loop.close)
	state := stream.NewRPCState("/test.Service/Call", 1)
	life := newRPCLifecycle(loop, state, nil)
	if !life.serverFinish(nil) {
		t.Fatal("graceful terminal did not win")
	}
	if !life.beginTerminalConsumer() {
		t.Fatal("terminal consumer was not admitted")
	}
	life.abandonClientData(status.Error(codes.Canceled, "pending"))
	life.mu.Lock()
	life.clientObserved = true
	life.mu.Unlock()
	life.endTerminalConsumer()
	life.mu.RLock()
	defer life.mu.RUnlock()
	if life.pendingAbandon != nil {
		t.Fatalf("observed consumer retained pending cause: %v",
			life.pendingAbandon,
		)
	}
	if life.abandonmentCommitted || life.abandonmentErr != nil {
		t.Fatalf("observed consumer committed abandonment: %v",
			life.abandonmentErr,
		)
	}
}

func TestLateUnaryReceiveObservesCommittedCancellation(t *testing.T) {
	loop := &signaledLiveLoop{
		done:      make(chan struct{}),
		submitted: make(chan struct{}, 1),
	}
	state := stream.NewRPCState("/test.Service/Call", 1)
	if err := state.Responses.TrySend(
		&wrapperspb.StringValue{Value: "discard"},
	); err != nil {
		t.Fatal(err)
	}
	life := newRPCLifecycle(loop, state, nil)
	if !life.serverFinish(nil) {
		t.Fatal("graceful terminal did not win")
	}
	life.abandonClientData(context.Canceled)
	<-life.release
	client := &clientStreamAdapter{
		ctx:       context.Background(),
		callerCtx: context.Background(),
		loop:      loop,
		cloner:    ProtoCloner{},
		life:      life,
		state:     state,
		copts:     new(callopts.CallOptions),
	}
	err := client.RecvMsg(new(wrapperspb.StringValue))
	if status.Code(err) != codes.Canceled {
		t.Fatalf("late RecvMsg = %v, want Canceled", err)
	}
}

func TestCallerAbortDoesNotPublishAccumulatedTrailers(t *testing.T) {
	loop := &signaledLiveLoop{
		done:      make(chan struct{}),
		submitted: make(chan struct{}, 1),
	}
	state := stream.NewRPCState("/test.Service/Call", 1)
	state.ResponseTrailers = metadata.Pairs("unpublished", "value")
	handler := new(mockStatsHandler)
	clientStats := requireRPCStats(t, &statsHandlerHelper{
		handler:  handler,
		isClient: true,
	}, context.Background(), state.Method, false, true)
	var trailer metadata.MD
	life := newRPCLifecycle(loop, state, nil, true)
	life.clientStats = clientStats
	client := &clientStreamAdapter{
		ctx:       context.Background(),
		callerCtx: context.Background(),
		loop:      loop,
		life:      life,
		state:     state,
		copts: &callopts.CallOptions{
			Trailers: []*metadata.MD{&trailer},
		},
		stats:         clientStats,
		serverStreams: true,
	}
	result := make(chan error, 1)
	go func() {
		result <- client.RecvMsg(new(wrapperspb.StringValue))
	}()
	<-loop.submitted
	if !life.callerCancel(context.Canceled) {
		t.Fatal("caller cancellation did not win")
	}
	if err := <-result; status.Code(err) != codes.Canceled {
		t.Fatalf("RecvMsg = %v, want Canceled", err)
	}
	methodTrailer := client.Trailer()
	if len(trailer) != 0 || len(methodTrailer) != 0 {
		t.Fatalf("unpublished trailer escaped: option=%v method=%v", trailer, methodTrailer)
	}
	handler.mu.Lock()
	defer handler.mu.Unlock()
	for _, event := range handler.events {
		if _, ok := event.(*stats.InTrailer); ok {
			t.Fatal("unpublished trailer emitted InTrailer")
		}
	}
}

type blockingCopyCloner struct {
	started chan struct{}
	release chan struct{}
	err     error
}

func (*blockingCopyCloner) Clone(value any) (any, error) {
	return value, nil
}

func (c *blockingCopyCloner) Copy(target, source any) error {
	close(c.started)
	<-c.release
	if c.err != nil {
		return c.err
	}
	shallowCopy(target, source)
	return nil
}

func TestClientStatsEndWaitsForHandedOffPayload(t *testing.T) {
	loop := &signaledLiveLoop{
		done:      make(chan struct{}),
		submitted: make(chan struct{}, 1),
	}
	state := stream.NewRPCState("/test.Service/Call", 1)
	if err := state.Responses.TrySend(
		&wrapperspb.StringValue{Value: "response"},
	); err != nil {
		t.Fatal(err)
	}
	state.ResponseTrailers = metadata.Pairs("result", "aborted")
	handler := new(mockStatsHandler)
	clientStats := requireRPCStats(t, &statsHandlerHelper{
		handler:  handler,
		isClient: true,
	}, context.Background(), state.Method, false, true)
	life := newRPCLifecycle(loop, state, nil, true)
	life.clientStats = clientStats
	cloner := &blockingCopyCloner{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	client := &clientStreamAdapter{
		ctx:           context.Background(),
		callerCtx:     context.Background(),
		loop:          loop,
		cloner:        cloner,
		life:          life,
		state:         state,
		copts:         new(callopts.CallOptions),
		stats:         clientStats,
		serverStreams: true,
	}
	result := make(chan error, 1)
	go func() {
		result <- client.RecvMsg(new(wrapperspb.StringValue))
	}()
	<-cloner.started

	if !life.serverAbort(status.Error(codes.Aborted, "abort")) {
		t.Fatal("server abort did not win")
	}
	handler.mu.Lock()
	for _, event := range handler.events {
		if _, ok := event.(*stats.End); ok {
			handler.mu.Unlock()
			t.Fatal("End preceded handed-off InPayload")
		}
	}
	handler.mu.Unlock()

	close(cloner.release)
	if err := <-result; err != nil {
		t.Fatalf("RecvMsg = %v", err)
	}
	waitMockStatsEnd(t, handler)
	handler.mu.Lock()
	defer handler.mu.Unlock()
	var names []string
	var endEvent *stats.End
	for _, event := range handler.events {
		switch event := event.(type) {
		case *stats.Begin:
			names = append(names, "Begin")
		case *stats.InPayload:
			names = append(names, "InPayload")
		case *stats.InTrailer:
			names = append(names, "InTrailer")
		case *stats.End:
			names = append(names, "End")
			endEvent = event
		}
	}
	want := []string{"Begin", "InPayload", "InTrailer", "End"}
	if len(names) != len(want) {
		t.Fatalf("stats = %v, want %v", names, want)
	}
	for index := range want {
		if names[index] != want[index] {
			t.Fatalf("stats = %v, want %v", names, want)
		}
	}
	if endEvent == nil || status.Code(endEvent.Error) != codes.Aborted {
		t.Fatalf("End error = %v, want Aborted", endEvent)
	}
	if values := endEvent.Trailer.Get("result"); len(values) != 1 ||
		values[0] != "aborted" {
		t.Fatalf("End trailer = %v, want aborted trailer", endEvent.Trailer)
	}
}

type droppedApplyLoop struct {
	done      chan struct{}
	submitted chan struct{}
}

func (l *droppedApplyLoop) SubmitInternal(func()) error {
	return nil
}

func (l *droppedApplyLoop) Submit(task func()) error {
	task()
	if l.submitted != nil {
		l.submitted <- struct{}{}
	}
	return nil
}

func (l *droppedApplyLoop) Done() <-chan struct{} {
	return l.done
}

func TestHeaderRecoversPublishedMetadataAfterDroppedTerminalApply(t *testing.T) {
	loop := &droppedApplyLoop{
		done:      make(chan struct{}),
		submitted: make(chan struct{}, 1),
	}
	state := stream.NewRPCState("/test.Service/Call", 1)
	state.ResponseHeaders = metadata.Pairs("recovered", "header")
	ctx, cancel := context.WithCancel(context.Background())
	life := newRPCLifecycle(loop, state, cancel)
	life.setServer(cancel, nil)
	client := &clientStreamAdapter{
		ctx:       ctx,
		callerCtx: context.Background(),
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
	if !life.serverFinish(status.Error(codes.Aborted, "selected")) {
		t.Fatal("server terminal did not win")
	}
	close(loop.done)
	first := <-result
	if first.err != nil || len(first.headers.Get("recovered")) != 1 ||
		first.headers.Get("recovered")[0] != "header" {
		t.Fatalf(
			"Header = %v, %v, want recovered header",
			first.headers,
			first.err,
		)
	}
	<-life.release
	second, err := client.Header()
	if err != nil || len(second.Get("recovered")) != 1 ||
		second.Get("recovered")[0] != "header" {
		t.Fatalf("later Header = %v, %v", second, err)
	}
}

func TestSchedulerRecoveryWaitsForHandedOffPayload(t *testing.T) {
	loop := &droppedApplyLoop{done: make(chan struct{})}
	state := stream.NewRPCState("/test.Service/Call", 1)
	if err := state.Responses.TrySend(
		&wrapperspb.StringValue{Value: "response"},
	); err != nil {
		t.Fatal(err)
	}
	state.ResponseTrailers = metadata.Pairs("result", "recovered")
	handler := new(mockStatsHandler)
	clientStats := requireRPCStats(t, &statsHandlerHelper{
		handler:  handler,
		isClient: true,
	}, context.Background(), state.Method, false, true)
	life := newRPCLifecycle(loop, state, nil, true)
	life.clientStats = clientStats
	cloner := &blockingCopyCloner{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	client := &clientStreamAdapter{
		ctx:           context.Background(),
		callerCtx:     context.Background(),
		loop:          loop,
		cloner:        cloner,
		life:          life,
		state:         state,
		copts:         new(callopts.CallOptions),
		stats:         clientStats,
		serverStreams: true,
	}
	result := make(chan error, 1)
	go func() {
		result <- client.RecvMsg(new(wrapperspb.StringValue))
	}()
	<-cloner.started
	if !life.serverAbort(status.Error(codes.Aborted, "abort")) {
		t.Fatal("server abort did not win")
	}
	close(loop.done)
	select {
	case <-life.release:
		t.Fatal("scheduler recovery released a handed-off payload")
	default:
	}
	handler.mu.Lock()
	for _, event := range handler.events {
		if _, ok := event.(*stats.End); ok {
			handler.mu.Unlock()
			t.Fatal("scheduler recovery End preceded handed-off InPayload")
		}
	}
	handler.mu.Unlock()

	close(cloner.release)
	if err := <-result; err != nil {
		t.Fatalf("RecvMsg = %v", err)
	}
	<-life.release
	handler.mu.Lock()
	defer handler.mu.Unlock()
	var names []string
	var endEvent *stats.End
	for _, event := range handler.events {
		switch event := event.(type) {
		case *stats.Begin:
			names = append(names, "Begin")
		case *stats.InPayload:
			names = append(names, "InPayload")
		case *stats.InTrailer:
			names = append(names, "InTrailer")
		case *stats.End:
			names = append(names, "End")
			endEvent = event
		}
	}
	want := []string{"Begin", "InPayload", "InTrailer", "End"}
	if len(names) != len(want) {
		t.Fatalf("stats = %v, want %v", names, want)
	}
	for index := range want {
		if names[index] != want[index] {
			t.Fatalf("stats = %v, want %v", names, want)
		}
	}
	if endEvent == nil || status.Code(endEvent.Error) != codes.Aborted {
		t.Fatalf("End error = %v, want Aborted", endEvent)
	}
	if values := endEvent.Trailer.Get("result"); len(values) != 1 ||
		values[0] != "recovered" {
		t.Fatalf("End trailer = %v, want recovered trailer", endEvent.Trailer)
	}
}

func TestObservedCopyFailureOverridesPendingServerTerminal(t *testing.T) {
	loop := &signaledLiveLoop{
		done:      make(chan struct{}),
		submitted: make(chan struct{}, 1),
	}
	state := stream.NewRPCState("/test.Service/Call", 1)
	if err := state.Responses.TrySend(
		&wrapperspb.StringValue{Value: "response"},
	); err != nil {
		t.Fatal(err)
	}
	state.ResponseTrailers = metadata.Pairs("result", "aborted")
	handler := new(mockStatsHandler)
	clientStats := requireRPCStats(t, &statsHandlerHelper{
		handler:  handler,
		isClient: true,
	}, context.Background(), state.Method, false, true)
	life := newRPCLifecycle(loop, state, nil, true)
	life.clientStats = clientStats
	copyErr := errors.New("copy failed")
	cloner := &blockingCopyCloner{
		started: make(chan struct{}),
		release: make(chan struct{}),
		err:     copyErr,
	}
	client := &clientStreamAdapter{
		ctx:           context.Background(),
		callerCtx:     context.Background(),
		loop:          loop,
		cloner:        cloner,
		life:          life,
		state:         state,
		copts:         new(callopts.CallOptions),
		stats:         clientStats,
		serverStreams: true,
	}
	result := make(chan error, 1)
	go func() {
		result <- client.RecvMsg(new(wrapperspb.StringValue))
	}()
	<-cloner.started
	if !life.serverAbort(status.Error(codes.Aborted, "abort")) {
		t.Fatal("server abort did not win")
	}
	close(cloner.release)
	err := <-result
	if status.Code(err) != codes.Internal || !errors.Is(err, copyErr) {
		t.Fatalf("RecvMsg = %v, want Internal wrapping copy failure", err)
	}

	waitMockStatsEnd(t, handler)
	handler.mu.Lock()
	defer handler.mu.Unlock()
	var (
		names    []string
		endEvent *stats.End
	)
	for _, event := range handler.events {
		switch value := event.(type) {
		case *stats.Begin:
			names = append(names, "Begin")
		case *stats.InPayload:
			names = append(names, "InPayload")
		case *stats.InTrailer:
			names = append(names, "InTrailer")
		case *stats.End:
			names = append(names, "End")
			endEvent = value
		}
	}
	want := []string{"Begin", "InTrailer", "End"}
	if len(names) != len(want) {
		t.Fatalf("stats = %v, want %v", names, want)
	}
	for index := range want {
		if names[index] != want[index] {
			t.Fatalf("stats = %v, want %v", names, want)
		}
	}
	if status.Code(endEvent.Error) != codes.Internal ||
		!errors.Is(endEvent.Error, copyErr) {
		t.Fatalf("End error = %v, want Internal wrapping copy failure", endEvent.Error)
	}
	if values := endEvent.Trailer.Get("result"); len(values) != 1 ||
		values[0] != "aborted" {
		t.Fatalf("End trailer = %v, want published trailer", endEvent.Trailer)
	}
}

func TestClientFailureOverridesPendingServerTerminal(t *testing.T) {
	loop := &signaledLiveLoop{
		done:      make(chan struct{}),
		submitted: make(chan struct{}, 1),
	}
	state := stream.NewRPCState("/test.Service/Call", 1)
	state.ResponseTrailers = metadata.Pairs("result", "aborted")
	handler := new(mockStatsHandler)
	clientStats := requireRPCStats(t, &statsHandlerHelper{
		handler:  handler,
		isClient: true,
	}, context.Background(), state.Method, true, true)
	life := newRPCLifecycle(loop, state, nil, true)
	life.clientStats = clientStats
	capability, admitted := life.control.admitDirect()
	if !admitted {
		t.Fatal("client delivery owner was not admitted")
	}
	deliveryID := life.control.deliveryBegin(capability)
	if deliveryID == 0 {
		t.Fatal("client delivery was not reserved")
	}
	life.control.completeCallback(capability, false, state.Responses.Drained())
	life.trackClientDelivery(deliveryID)
	if !life.serverAbort(status.Error(codes.Aborted, "abort")) {
		t.Fatal("server abort did not win")
	}

	localErr := cloneError("clone request", errors.New("clone failed"))
	completed := life.clientFailure(localErr)
	life.clientFailure(status.Error(codes.DataLoss, "later failure"))
	life.endClientDelivery(deliveryID)
	<-completed

	handler.mu.Lock()
	defer handler.mu.Unlock()
	var endEvent *stats.End
	for _, event := range handler.events {
		if value, ok := event.(*stats.End); ok {
			if endEvent != nil {
				t.Fatal("multiple End events")
			}
			endEvent = value
		}
	}
	if endEvent == nil {
		t.Fatal("missing End event")
	}
	if status.Code(endEvent.Error) != codes.Internal ||
		!errors.Is(endEvent.Error, localErr) {
		t.Fatalf("End error = %v, want local Internal", endEvent.Error)
	}
	if got := life.clientError(); got != localErr {
		t.Fatalf("client error = %v, want first local failure", got)
	}
	if values := endEvent.Trailer.Get("result"); len(values) != 1 ||
		values[0] != "aborted" {
		t.Fatalf("End trailer = %v, want published trailer", endEvent.Trailer)
	}
}

type capturedTaskLoop struct {
	done chan struct{}
	task chan func()
}

func (l *capturedTaskLoop) Submit(task func()) error {
	l.task <- task
	return nil
}

func (l *capturedTaskLoop) SubmitInternal(task func()) error {
	task()
	return nil
}

func (l *capturedTaskLoop) Done() <-chan struct{} {
	return l.done
}

func TestAbandonedReceiveReleasesLatePayloadHandoff(t *testing.T) {
	loop := &capturedTaskLoop{
		done: make(chan struct{}),
		task: make(chan func(), 1),
	}
	state := stream.NewRPCState("/test.Service/Call", 1)
	if err := state.Responses.TrySend(
		&wrapperspb.StringValue{Value: "late"},
	); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	life := newRPCLifecycle(loop, state, nil)
	client := &clientStreamAdapter{
		ctx:       ctx,
		callerCtx: ctx,
		loop:      loop,
		life:      life,
		state:     state,
	}
	result := make(chan receiveResult, 1)
	go func() {
		result <- client.receive()
	}()
	task := <-loop.task
	cancel()
	if err := (<-result).err; status.Code(err) != codes.Canceled {
		t.Fatalf("receive = %v, want Canceled", err)
	}
	task()
	<-life.release
	life.mu.RLock()
	deliveries := life.clientDeliveries
	life.mu.RUnlock()
	if len(deliveries) != 0 {
		t.Fatalf("client deliveries = %d, want 0", len(deliveries))
	}
	if !state.Responses.Drained() {
		t.Fatal("abandoned response remains retained")
	}
}
