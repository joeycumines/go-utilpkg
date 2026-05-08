package inprocgrpc

import (
	"context"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
)

// statsHandlerHelper wraps a stats.Handler to provide convenience methods
// for reporting RPC events.
type statsHandlerHelper struct {
	handler  stats.Handler
	isClient bool
}

// rpcStatsContext exposes server values while its cancellation follows only
// the caller context. Internal handler cleanup must not cancel the context
// observed by final stats callbacks before End.
type rpcStatsContext struct {
	values  context.Context
	control context.Context
}

func (c rpcStatsContext) Deadline() (time.Time, bool) {
	return c.control.Deadline()
}

func (c rpcStatsContext) Done() <-chan struct{} {
	return c.control.Done()
}

func (c rpcStatsContext) Err() error {
	return c.control.Err()
}

func (c rpcStatsContext) Value(key any) any {
	return c.values.Value(key)
}

// rpcStats serializes one side of one RPC's stats stream and makes terminal
// publication idempotent. Payload events may originate from independent send
// and receive goroutines, while Header, Trailer, and End must each be reported
// at most once.
type rpcStats struct {
	helper    *statsHandlerHelper
	runner    *rpcStatsRunner
	ctx       context.Context
	beginTime time.Time
	method    string

	mu         sync.Mutex
	headerIn   bool
	headerOut  bool
	trailerIn  bool
	trailerOut bool
	ended      bool
	trailer    metadata.MD
	inTail     <-chan struct{}
	outTail    <-chan struct{}
}

func containStatsPanic(subject string, callback func()) (err error) {
	result := make(chan error, 1)
	go func() {
		returned := false
		defer func() {
			if returned {
				result <- nil
				return
			}
			result <- internalRPCError(subject, recover())
		}()
		callback()
		returned = true
	}()
	return <-result
}

func (sh *statsHandlerHelper) tagRPCSafe(
	ctx context.Context,
	method string,
) (tagged context.Context, err error) {
	tagged = ctx
	err = containStatsPanic("stats TagRPC", func() {
		tagged = sh.tagRPC(ctx, method)
	})
	if err == nil && isNil(tagged) {
		tagged = ctx
		err = status.Error(codes.Internal, "stats TagRPC returned nil context")
	}
	return tagged, err
}

func (sh *statsHandlerHelper) tagRPC(ctx context.Context, method string) context.Context {
	if sh == nil {
		return ctx
	}
	return sh.handler.TagRPC(ctx, &stats.RPCTagInfo{
		FullMethodName: method,
	})
}

func (sh *statsHandlerHelper) startRPC(
	ctx context.Context,
	method string,
	isClientStream bool,
	isServerStream bool,
) (*rpcStats, error) {
	if sh == nil {
		return nil, nil
	}
	result := sh.newRPCStats(ctx, method)
	err := result.begin(isClientStream, isServerStream)
	if err != nil {
		result.runner.abort()
	}
	return result, err
}

func (sh *statsHandlerHelper) newRPCStats(
	ctx context.Context,
	method string,
) *rpcStats {
	return &rpcStats{
		helper: sh,
		runner: newRPCStatsRunner(),
		ctx:    ctx,
		method: method,
	}
}

func (r *rpcStats) begin(
	isClientStream bool,
	isServerStream bool,
) error {
	if r == nil {
		return nil
	}
	r.beginTime = time.Now()
	return r.runner.executeMandatory("stats Begin", func() {
		r.helper.handler.HandleRPC(r.ctx, &stats.Begin{
			Client:                    r.helper.isClient,
			BeginTime:                 r.beginTime,
			IsClientStream:            isClientStream,
			IsServerStream:            isServerStream,
			IsTransparentRetryAttempt: false,
		})
	})
}

func (r *rpcStats) inHeader(md metadata.MD, method string) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	if r.headerIn || r.ended {
		r.mu.Unlock()
		return nil
	}
	r.headerIn = true
	event := &stats.InHeader{
		Client: r.helper.isClient,
		Header: cloneMetadata(md),
	}
	if !r.helper.isClient {
		event.FullMethod = method
	}
	call, ok := r.prepareInbound("stats InHeader", func() {
		r.helper.handler.HandleRPC(r.ctx, event)
	})
	r.mu.Unlock()
	if !ok {
		return nil
	}
	return call.execute()
}

func (r *rpcStats) outHeader(md metadata.MD) error {
	call, ok := r.prepareOutHeader(md)
	if !ok {
		return nil
	}
	return call.execute()
}

func (r *rpcStats) prepareOutHeader(
	md metadata.MD,
) (rpcStatsCall, bool) {
	if r == nil {
		return rpcStatsCall{}, false
	}
	r.mu.Lock()
	if r.headerOut || r.ended {
		r.mu.Unlock()
		return rpcStatsCall{}, false
	}
	r.headerOut = true
	fullMethod := ""
	if r.helper.isClient {
		fullMethod = r.method
	}
	call, ok := r.prepareOutbound("stats OutHeader", func() {
		r.helper.handler.HandleRPC(r.ctx, &stats.OutHeader{
			Client:     r.helper.isClient,
			FullMethod: fullMethod,
			Header:     cloneMetadata(md),
		})
	})
	r.mu.Unlock()
	return call, ok
}

func (r *rpcStats) inPayload(payload any) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	if r.ended {
		r.mu.Unlock()
		return nil
	}
	event := &stats.InPayload{
		Client:   r.helper.isClient,
		Payload:  payload,
		RecvTime: time.Now(),
	}
	call, ok := r.prepareInbound("stats InPayload", func() {
		r.helper.handler.HandleRPC(r.ctx, event)
	})
	r.mu.Unlock()
	if !ok {
		return nil
	}
	return call.execute()
}

func (r *rpcStats) prepareOutPayload(
	payload any,
) (rpcStatsCall, bool) {
	if r == nil {
		return rpcStatsCall{}, false
	}
	r.mu.Lock()
	if r.ended {
		r.mu.Unlock()
		return rpcStatsCall{}, false
	}
	event := &stats.OutPayload{
		Client:   r.helper.isClient,
		Payload:  payload,
		SentTime: time.Now(),
	}
	call, ok := r.prepareOutbound("stats OutPayload", func() {
		r.helper.handler.HandleRPC(r.ctx, event)
	})
	r.mu.Unlock()
	return call, ok
}

func (r *rpcStats) quarantine() {
	if r != nil {
		r.runner.quarantine()
	}
}

// prepareInbound and prepareOutbound are called with r.mu held. They preserve
// causal order within each stream direction while allowing one inbound and one
// outbound callback to overlap or synchronously enter one another.
func (r *rpcStats) prepareInbound(
	subject string,
	callback func(),
) (rpcStatsCall, bool) {
	completion := make(chan struct{})
	call, ok := r.runner.prepareSequenced(
		subject,
		callback,
		r.inTail,
		completion,
	)
	if ok {
		r.inTail = completion
	}
	return call, ok
}

func (r *rpcStats) prepareOutbound(
	subject string,
	callback func(),
) (rpcStatsCall, bool) {
	completion := make(chan struct{})
	call, ok := r.runner.prepareSequenced(
		subject,
		callback,
		r.outTail,
		completion,
	)
	if ok {
		r.outTail = completion
	}
	return call, ok
}

func (r *rpcStats) inTrailer(md metadata.MD) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	if r.trailerIn || r.ended {
		r.mu.Unlock()
		return nil
	}
	r.trailerIn = true
	r.trailer = cloneMetadata(md)
	event := &stats.InTrailer{
		Client:  r.helper.isClient,
		Trailer: cloneMetadata(md),
	}
	call, ok := r.prepareInbound("stats InTrailer", func() {
		r.helper.handler.HandleRPC(r.ctx, event)
	})
	r.mu.Unlock()
	if !ok {
		return nil
	}
	return call.execute()
}

func (r *rpcStats) outTrailer(md metadata.MD) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	if r.trailerOut || r.ended {
		r.mu.Unlock()
		return nil
	}
	r.trailerOut = true
	event := &stats.OutTrailer{
		Client:  r.helper.isClient,
		Trailer: cloneMetadata(md),
	}
	call, ok := r.prepareOutbound("stats OutTrailer", func() {
		r.helper.handler.HandleRPC(r.ctx, event)
	})
	r.mu.Unlock()
	if !ok {
		return nil
	}
	return call.execute()
}

func (r *rpcStats) end(err error) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	if r.ended {
		r.mu.Unlock()
		return nil
	}
	r.ended = true
	trailer := cloneMetadata(r.trailer)
	call, ok := r.runner.prepareFinal("stats End", func() {
		r.helper.handler.HandleRPC(r.ctx, &stats.End{
			Client:    r.helper.isClient,
			BeginTime: r.beginTime,
			EndTime:   time.Now(),
			Trailer:   trailer,
			Error:     normalizeRPCError(err),
		})
	})
	r.mu.Unlock()
	if !ok {
		return nil
	}
	return call.execute()
}

func (r *rpcStats) finishInbound(
	headers metadata.MD,
	trailers metadata.MD,
	method string,
	headerPublished bool,
	trailerPublished bool,
	err error,
) {
	if r == nil {
		return
	}
	var calls []rpcStatsCall
	r.mu.Lock()
	if r.ended {
		r.mu.Unlock()
		return
	}
	if headerPublished && !r.headerIn {
		r.headerIn = true
		headers = cloneMetadata(headers)
		event := &stats.InHeader{
			Client: r.helper.isClient,
			Header: headers,
		}
		if !r.helper.isClient {
			event.FullMethod = method
		}
		if call, ok := r.prepareInbound("stats InHeader", func() {
			r.helper.handler.HandleRPC(r.ctx, event)
		}); ok {
			calls = append(calls, call)
		}
	}
	if trailerPublished && !r.trailerIn {
		r.trailerIn = true
		r.trailer = cloneMetadata(trailers)
		trailers = cloneMetadata(trailers)
		event := &stats.InTrailer{
			Client:  r.helper.isClient,
			Trailer: trailers,
		}
		if call, ok := r.prepareInbound("stats InTrailer", func() {
			r.helper.handler.HandleRPC(r.ctx, event)
		}); ok {
			calls = append(calls, call)
		}
	}
	r.ended = true
	trailer := cloneMetadata(r.trailer)
	if call, ok := r.runner.prepareFinal("stats End", func() {
		r.helper.handler.HandleRPC(r.ctx, &stats.End{
			Client:    r.helper.isClient,
			BeginTime: r.beginTime,
			EndTime:   time.Now(),
			Trailer:   trailer,
			Error:     normalizeRPCError(err),
		})
	}); ok {
		calls = append(calls, call)
	}
	r.mu.Unlock()
	for _, call := range calls {
		_ = call.execute()
	}
}
