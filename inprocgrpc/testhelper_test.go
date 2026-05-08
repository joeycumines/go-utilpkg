package inprocgrpc_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	eventloop "github.com/joeycumines/go-eventloop"
	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
)

func requirePanicContains(t testing.TB, want string, fn func()) {
	t.Helper()
	defer func() {
		reason := recover()
		if reason == nil {
			t.Fatalf("expected panic containing %q", want)
		}
		if message := fmt.Sprint(reason); !strings.Contains(message, want) {
			t.Fatalf("panic = %q, want substring %q", message, want)
		}
	}()
	fn()
}

// newTestLoop creates a new event loop, starts it, and registers cleanup.
func newTestLoop(t testing.TB) *eventloop.Loop {
	t.Helper()
	loop := eventloop.New()
	started := make(chan error, 1)
	if err := loop.Submit(func() {
		_, err := loop.ScheduleTimer(time.Hour, func() {})
		started <- err
	}); err != nil {
		t.Fatalf("queue test loop startup: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = loop.Run(ctx)
	}()
	if err := <-started; err != nil {
		cancel()
		<-done
		t.Fatalf("ScheduleTimer keepalive: %v", err)
	}
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return loop
}

// newTestChannel creates a new event-loop-driven channel with the test service
// registered. The loop is created and managed automatically.
func newTestChannel(
	t testing.TB,
	opts ...inprocgrpc.ChannelOption,
) *inprocgrpc.Channel {
	t.Helper()
	loop := newTestLoop(t)
	opts = append(
		[]inprocgrpc.ChannelOption{inprocgrpc.WithLoop(loop)},
		opts...,
	)
	ch := mustNewChannel(t, opts...)
	ch.RegisterService(&testServiceDesc, &echoServer{})
	return ch
}

// newBareChannel creates a new event-loop-driven channel WITHOUT registering
// any services. Call RegisterService manually on the returned channel.
func newBareChannel(
	t testing.TB,
	opts ...inprocgrpc.ChannelOption,
) *inprocgrpc.Channel {
	t.Helper()
	loop := newTestLoop(t)
	opts = append(
		[]inprocgrpc.ChannelOption{inprocgrpc.WithLoop(loop)},
		opts...,
	)
	return mustNewChannel(t, opts...)
}

func mustNewChannel(
	t testing.TB,
	opts ...inprocgrpc.ChannelOption,
) *inprocgrpc.Channel {
	t.Helper()
	return inprocgrpc.NewChannel(opts...)
}
