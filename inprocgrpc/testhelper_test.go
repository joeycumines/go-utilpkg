package inprocgrpc_test

import (
	"context"
	"testing"

	eventloop "github.com/joeycumines/go-eventloop"
	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
)

// newTestLoop creates a new event loop, starts it, and registers cleanup.
func newTestLoop(t testing.TB) *eventloop.Loop {
	t.Helper()
	loop, err := eventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = loop.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return loop
}

// newTestChannel creates a new event-loop-driven channel with the test service
// registered. The loop is created and managed automatically.
func newTestChannel(t testing.TB, opts ...inprocgrpc.Option) *inprocgrpc.Channel {
	t.Helper()
	loop := newTestLoop(t)
	opts = append([]inprocgrpc.Option{inprocgrpc.WithLoop(loop)}, opts...)
	ch := inprocgrpc.NewChannel(opts...)
	ch.RegisterService(&testServiceDesc, &echoServer{})
	return ch
}

// newBareChannel creates a new event-loop-driven channel WITHOUT registering
// any services. Call RegisterService manually on the returned channel.
func newBareChannel(t testing.TB, opts ...inprocgrpc.Option) *inprocgrpc.Channel {
	t.Helper()
	loop := newTestLoop(t)
	opts = append([]inprocgrpc.Option{inprocgrpc.WithLoop(loop)}, opts...)
	ch := inprocgrpc.NewChannel(opts...)
	return ch
}
