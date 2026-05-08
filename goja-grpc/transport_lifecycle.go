package gojagrpc

import (
	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// transportLifecycle is the private cross-module contract implemented by the
// concrete inprocgrpc client stream. TerminalDone publishes immutable RPC truth;
// Done is the later full transport release boundary.
type transportLifecycle interface {
	TerminalDone() <-chan struct{}
	TerminalResult() (error, bool)
	Done() <-chan struct{}
}

func (m *Module) defaultInprocChannel(cc grpc.ClientConnInterface) bool {
	channel, ok := cc.(*inprocgrpc.Channel)
	return ok && channel == m.channel
}

func bindClientTransport(
	control *operationControl,
	stream grpc.ClientStream,
	requireLifecycle bool,
) (transportLifecycle, error) {
	if control == nil {
		return nil, errModuleClosed
	}
	if stream == nil {
		if err := control.bindRelease(nil); err != nil {
			return nil, err
		}
		return nil, status.Error(codes.Internal, "client transport returned a nil stream")
	}
	lifecycle, ok := stream.(transportLifecycle)
	if !ok {
		bindErr := control.bindRelease(nil)
		if requireLifecycle {
			return nil, status.Error(
				codes.Internal,
				"inproc client stream does not expose transport lifecycle",
			)
		}
		return nil, bindErr
	}
	if lifecycle.TerminalDone() == nil || lifecycle.Done() == nil {
		_ = control.bindRelease(nil)
		return nil, status.Error(
			codes.Internal,
			"client stream exposes an invalid transport lifecycle",
		)
	}
	if err := control.bindRelease(lifecycle.Done()); err != nil {
		return lifecycle, err
	}
	return lifecycle, nil
}
