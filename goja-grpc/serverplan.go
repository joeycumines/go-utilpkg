package gojagrpc

import (
	"context"
	"math"

	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
	"github.com/joeycumines/goja"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type serverMethodID uint64

type serverMethodKind uint8

const (
	serverMethodUnary serverMethodKind = iota + 1
	serverMethodServerStream
	serverMethodClientStream
	serverMethodBidiStream
)

// serverMethodPlan is retained only in the owner bridge. The transport keeps
// an opaque method ID and dispatcher; it never retains this plan or its Goja
// callables.
type serverMethodPlan struct {
	module       *Module
	fullMethod   string
	method       protoreflect.MethodDescriptor
	handler      goja.Callable
	interceptors []goja.Callable
	kind         serverMethodKind
}

func (m *Module) allocateServerMethodPlan(
	plan *serverMethodPlan,
) (serverMethodID, error) {
	if plan == nil {
		return 0, errOwnerIDExhausted
	}
	// postDoneMu serializes this against the root-disposal disposer's
	// removeServerMethodPlans and the post-Done transfer (clearPostDoneOwnerIndexes).
	m.owner.postDoneMu.Lock()
	defer m.owner.postDoneMu.Unlock()
	if m.owner.nextServerPlan == math.MaxUint64 {
		return 0, errOwnerIDExhausted
	}
	m.owner.nextServerPlan++
	id := serverMethodID(m.owner.nextServerPlan)
	m.owner.serverPlans[id] = plan
	return id, nil
}

func (d *ownerDispatcher) serverHandler(
	id serverMethodID,
) inprocgrpc.StreamHandlerFunc {
	return func(ctx context.Context, stream *inprocgrpc.RPCStream) {
		d.bridge.postDoneMu.Lock()
		plan := d.bridge.serverPlans[id]
		d.bridge.postDoneMu.Unlock()
		if plan == nil {
			stream.Finish(errModuleUnavailable)
			return
		}
		plan.start(ctx, stream)
	}
}

func (p *serverMethodPlan) start(
	ctx context.Context,
	stream *inprocgrpc.RPCStream,
) {
	rpc := newServerRPC(ctx, p.module, stream)
	if err := rpc.register(); err != nil {
		rpc.finish(status.Error(codes.Unavailable, err.Error()))
		return
	}
	switch p.kind {
	case serverMethodUnary, serverMethodServerStream:
		p.startRequestRPC(rpc)
	case serverMethodClientStream, serverMethodBidiStream:
		p.startStreamingRequestRPC(rpc)
	default:
		rpc.finish(status.Error(codes.Internal, "invalid server method plan"))
	}
}

func (p *serverMethodPlan) startRequestRPC(rpc *serverRPC) {
	rpc.run(func() {
		rpc.stream.Recv().Recv(func(message any, err error) {
			rpc.run(func() {
				if err != nil {
					rpc.finish(err)
					return
				}
				request, err := p.module.toWrappedMessage(
					message,
					p.method.Input(),
				)
				if err != nil {
					rpc.finish(status.Errorf(
						codes.Internal,
						"request conversion: %v",
						err,
					))
					return
				}
				callObject := p.module.newServerCallObject(rpc)
				_ = callObject.Set("method", p.fullMethod)
				_ = callObject.Set("request", request)
				if p.kind == serverMethodServerStream {
					p.module.addServerSend(
						callObject,
						rpc,
						p.method.Output(),
					)
				}
				result, err := p.buildServerChain(
					callObject,
					func() (goja.Value, error) {
						return p.handler(
							goja.Undefined(),
							callObject.Get("request"),
							callObject,
						)
					},
				)
				if err != nil {
					rpc.finish(p.module.jsErrorToGRPC(err))
					return
				}
				p.module.finishHandler(
					result,
					rpc,
					p.method.Output(),
					p.kind == serverMethodUnary,
				)
			})
		})
	})
}

func (p *serverMethodPlan) startStreamingRequestRPC(rpc *serverRPC) {
	rpc.run(func() {
		callObject := p.module.newServerCallObject(rpc)
		_ = callObject.Set("method", p.fullMethod)
		p.module.addServerRecv(callObject, rpc, p.method.Input())
		if p.kind == serverMethodBidiStream {
			p.module.addServerSend(callObject, rpc, p.method.Output())
		}
		result, err := p.buildServerChain(
			callObject,
			func() (goja.Value, error) {
				return p.handler(goja.Undefined(), callObject)
			},
		)
		if err != nil {
			rpc.finish(p.module.jsErrorToGRPC(err))
			return
		}
		p.module.finishHandler(
			result,
			rpc,
			p.method.Output(),
			p.kind == serverMethodClientStream,
		)
	})
}

func (p *serverMethodPlan) buildServerChain(
	callObject *goja.Object,
	invoke func() (goja.Value, error),
) (goja.Value, error) {
	if len(p.interceptors) == 0 {
		return invoke()
	}
	inner := p.module.runtime.ToValue(func(goja.FunctionCall) goja.Value {
		value, err := invoke()
		if err != nil {
			panic(err)
		}
		return value
	})
	next := inner
	for index := len(p.interceptors) - 1; index >= 0; index-- {
		value, err := p.interceptors[index](goja.Undefined(), next)
		if err != nil {
			return nil, err
		}
		next = value
	}
	outer, ok := goja.AssertFunction(next)
	if !ok {
		return nil, status.Error(
			codes.Internal,
			"server interceptor chain did not produce a callable",
		)
	}
	return outer(goja.Undefined(), callObject)
}
