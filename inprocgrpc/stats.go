package inprocgrpc

import (
	"context"
	"time"

	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/stats"
)

// statsHandlerHelper wraps a stats.Handler to provide convenience methods
// for reporting RPC events.
type statsHandlerHelper struct {
	handler  stats.Handler
	isClient bool
}

func (sh *statsHandlerHelper) tagRPC(ctx context.Context, method string) context.Context {
	if sh == nil {
		return ctx
	}
	return sh.handler.TagRPC(ctx, &stats.RPCTagInfo{
		FullMethodName: method,
	})
}

func (sh *statsHandlerHelper) begin(ctx context.Context, isClientStream, isServerStream bool) {
	if sh == nil {
		return
	}
	sh.handler.HandleRPC(ctx, &stats.Begin{
		Client:                    sh.isClient,
		BeginTime:                 time.Now(),
		IsClientStream:            isClientStream,
		IsServerStream:            isServerStream,
		IsTransparentRetryAttempt: false,
	})
}

func (sh *statsHandlerHelper) end(ctx context.Context, err error) {
	if sh == nil {
		return
	}
	sh.handler.HandleRPC(ctx, &stats.End{
		Client:  sh.isClient,
		EndTime: time.Now(),
		Error:   err,
	})
}

func (sh *statsHandlerHelper) inHeader(ctx context.Context, md metadata.MD, method string) {
	if sh == nil {
		return
	}
	sh.handler.HandleRPC(ctx, &stats.InHeader{
		Client:     sh.isClient,
		FullMethod: method,
		Header:     md,
	})
}

func (sh *statsHandlerHelper) inPayload(ctx context.Context, payload any) {
	if sh == nil {
		return
	}
	sh.handler.HandleRPC(ctx, &stats.InPayload{
		Client:   sh.isClient,
		Payload:  payload,
		RecvTime: time.Now(),
		// WireLength and Length are 0 for in-process (no encoding)
	})
}

func (sh *statsHandlerHelper) inTrailer(ctx context.Context, md metadata.MD) {
	if sh == nil {
		return
	}
	sh.handler.HandleRPC(ctx, &stats.InTrailer{
		Client:  sh.isClient,
		Trailer: md,
	})
}

func (sh *statsHandlerHelper) outHeader(ctx context.Context, md metadata.MD) {
	if sh == nil {
		return
	}
	sh.handler.HandleRPC(ctx, &stats.OutHeader{
		Client: sh.isClient,
		Header: md,
	})
}

func (sh *statsHandlerHelper) outPayload(ctx context.Context, payload any) {
	if sh == nil {
		return
	}
	sh.handler.HandleRPC(ctx, &stats.OutPayload{
		Client:   sh.isClient,
		Payload:  payload,
		SentTime: time.Now(),
		// WireLength and Length are 0 for in-process (no encoding)
	})
}

func (sh *statsHandlerHelper) outTrailer(ctx context.Context, md metadata.MD) {
	if sh == nil {
		return
	}
	sh.handler.HandleRPC(ctx, &stats.OutTrailer{
		Client:  sh.isClient,
		Trailer: md,
	})
}
