package fangrpcstream

import (
	"context"
	bigbuff "github.com/joeycumines/go-bigbuff"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"io"
	"net"
	"sync"
)

type (
	// Stream wraps a bidirectional gRPC stream client, and provides concurrency-friendly methods for sending and receiving messages.
	// It is intended for use in scenarios where there are multiple concurrent senders and/or receivers.
	Stream[T Client[Request, Response], Request proto.Message, Response proto.Message] struct {
		notifier bigbuff.Notifier
		ctx      context.Context
		stream   T
		err      error
		cancel   context.CancelFunc
		ch       chan Request
		done     chan struct{}
		stop     chan struct{}
		mu       sync.Mutex
	}

	// Factory models a method to create a bidirectional gRPC stream client, and is implemented by generated gRPC clients.
	Factory[T Client[Request, Response], Request proto.Message, Response proto.Message] func(ctx context.Context, opts ...grpc.CallOption) (T, error)

	// Client models a bidirectional gRPC stream client, and is implemented by generated gRPC clients.
	Client[Request proto.Message, Response proto.Message] interface {
		Send(Request) error
		Recv() (Response, error)
		grpc.ClientStream
	}
)

// New opens a new Stream.
func New[T Client[Request, Response], Request proto.Message, Response proto.Message](
	ctx context.Context,
	factory Factory[T, Request, Response],
	opts ...grpc.CallOption,
) (*Stream[T, Request, Response], error) {
	ctx, cancel := context.WithCancel(ctx)

	var success bool
	defer func() {
		if !success {
			cancel()
		}
	}()

	stream, err := factory(ctx, opts...)
	if err != nil {
		return nil, err
	}

	conn := Stream[T, Request, Response]{
		ctx:    ctx,
		cancel: cancel,
		stream: stream,
		ch:     make(chan Request),
		done:   make(chan struct{}),
		stop:   make(chan struct{}, 1),
	}

	go conn.run()

	success = true

	return &conn, nil
}

func (x *Stream[T, Request, Response]) run() {
	defer close(x.done)
	defer x.cancel()

	var wg sync.WaitGroup
	wg.Add(2)

	// receive messages
	go func() {
		defer wg.Done()

		for {
			res, err := x.stream.Recv()
			if err != nil {
				// note: triggered by x.cancel, x.stream.CloseSend, or connection / stream error
				x.fatalErr(err)
				return
			}

			x.publish(res)
		}
	}()

	// send messages
	go func() {
		defer wg.Done()

		for {
			select {
			case <-x.ctx.Done():
				return

			case <-x.stop:
				if err := x.stream.CloseSend(); err != nil {
					x.fatalErr(err)
				}
				// other side should close the stream too
				return

			case req := <-x.ch:
				if err := x.stream.Send(req); err != nil {
					x.fatalErr(err)
					return
				}
			}
		}
	}()

	wg.Wait()
}

func (x *Stream[T, Request, Response]) fatalErr(err error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.err != nil {
		return
	}
	x.cancel()
	if err != nil {
		x.err = err
	} else {
		x.err = x.ctx.Err()
	}
}

func (x *Stream[T, Request, Response]) Done() <-chan struct{} {
	return x.done
}

func (x *Stream[T, Request, Response]) Err() error {
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.err == io.EOF {
		return nil
	}
	return x.err
}

func (x *Stream[T, Request, Response]) Shutdown(ctx context.Context) error {
	select {
	case x.stop <- struct{}{}:
	default:
	}

	select {
	case <-ctx.Done():
		x.cancel()
		<-x.done
	case <-x.done:
	}

	return x.Err()
}

func (x *Stream[T, Request, Response]) Close() error {
	x.cancel()
	<-x.done
	return x.Err()
}

// Send will send a message to the stream.
func (x *Stream[T, Request, Response]) Send(ctx context.Context, req Request) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	select {
	case <-x.ctx.Done():
		return net.ErrClosed
	default:
	}

	select {
	case <-ctx.Done():
		return ctx.Err()

	case <-x.ctx.Done():
		return net.ErrClosed

	case x.ch <- req:
		return nil
	}
}

// Subscribe accepts any `target` that is a channel which can accept Response values.
// The returned cancel func MUST be called, unless `ctx` is cancelled.
// WARNING: Sends to `target` are blocking, and callers must therefore always receive promptly.
func (x *Stream[T, Request, Response]) Subscribe(ctx context.Context, target any) context.CancelFunc {
	return x.notifier.SubscribeCancel(ctx, nil, target)
}

func (x *Stream[T, Request, Response]) publish(value Response) {
	x.notifier.PublishContext(x.ctx, nil, value)
}
