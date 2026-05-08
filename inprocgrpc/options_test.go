package inprocgrpc

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/stats"
)

// mockLoop is a minimal Loop implementation for testing options.
type mockLoop struct{ done chan struct{} }

func (m mockLoop) Submit(func()) error {
	return nil
}

type nilDoneLoop struct{}

func (nilDoneLoop) Submit(func()) error         { return nil }
func (nilDoneLoop) SubmitInternal(func()) error { return nil }
func (nilDoneLoop) Done() <-chan struct{}       { return nil }

func assertPanicContains(t *testing.T, want string, fn func()) {
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

func (m mockLoop) SubmitInternal(func()) error {
	return nil
}

func (m mockLoop) Done() <-chan struct{} {
	if m.done == nil {
		return make(chan struct{})
	}
	return m.done
}

// testStatsHandler is a minimal stats.Handler for internal option tests.
type testStatsHandler struct{}

func (testStatsHandler) TagRPC(ctx context.Context, _ *stats.RPCTagInfo) context.Context { return ctx }
func (testStatsHandler) HandleRPC(context.Context, stats.RPCStats)                       {}
func (testStatsHandler) TagConn(ctx context.Context, _ *stats.ConnTagInfo) context.Context {
	return ctx
}
func (testStatsHandler) HandleConn(context.Context, stats.ConnStats) {}

var _ stats.Handler = testStatsHandler{}

func TestResolveOptions_Nil(t *testing.T) {
	loop := mockLoop{}
	opts, err := resolveOptions([]ChannelOption{WithLoop(loop)})
	if err != nil {
		t.Fatal(err)
	}
	if opts == nil {
		t.Fatal("opts should not be nil")
	}
	if opts.loop == nil {
		t.Error("loop should not be nil")
	}
	if opts.cloner != nil {
		t.Error("cloner should be nil")
	}
	if opts.unaryInterceptor != nil {
		t.Error("unaryInterceptor should be nil")
	}
	if opts.streamInterceptor != nil {
		t.Error("streamInterceptor should be nil")
	}
	if opts.clientStats != nil {
		t.Error("clientStats should be nil")
	}
	if opts.serverStats != nil {
		t.Error("serverStats should be nil")
	}
}

func TestNewChannelDefaults(t *testing.T) {
	channel := NewChannel(WithLoop(mockLoop{}))
	if channel.streamBuffer != 16 {
		t.Fatalf("stream buffer = %d, want 16", channel.streamBuffer)
	}
	if _, ok := channel.cloner.(ProtoCloner); !ok {
		t.Fatalf("cloner = %T, want ProtoCloner", channel.cloner)
	}
}

func TestNewChannelPanicsInvalidConfiguration(t *testing.T) {
	var (
		cloner       *ProtoCloner
		loop         *mockLoop
		statsHandler *testStatsHandler
	)
	tests := []struct {
		name    string
		want    string
		options []ChannelOption
	}{
		{name: "missing loop", want: "inprocgrpc: loop must be provided"},
		{name: "nil option", want: "inprocgrpc: channel option 0 is nil", options: []ChannelOption{nil}},
		{name: "typed nil cloner option", want: "channel option 0: cloner must not be nil", options: []ChannelOption{(*ClonerOption)(nil)}},
		{name: "typed nil unary option", want: "unary server interceptor must not be nil", options: []ChannelOption{(*ServerUnaryInterceptorOption)(nil)}},
		{name: "typed nil stream option", want: "stream server interceptor must not be nil", options: []ChannelOption{(*ServerStreamInterceptorOption)(nil)}},
		{name: "typed nil client stats option", want: "client stats handler must not be nil", options: []ChannelOption{(*ClientStatsHandlerOption)(nil)}},
		{name: "typed nil server stats option", want: "server stats handler must not be nil", options: []ChannelOption{(*ServerStatsHandlerOption)(nil)}},
		{name: "typed nil loop option", want: "loop must not be nil", options: []ChannelOption{(*LoopOption)(nil)}},
		{name: "typed nil clone-disabled option", want: "clone-disabled option must not be nil", options: []ChannelOption{(*CloneDisabledOption)(nil)}},
		{name: "typed nil stream-buffer option", want: "stream buffer option must not be nil", options: []ChannelOption{(*StreamBufferOption)(nil)}},
		{name: "typed nil cloner payload", want: "cloner must not be nil", options: []ChannelOption{WithCloner(cloner)}},
		{name: "typed nil loop payload", want: "loop must not be nil", options: []ChannelOption{WithLoop(loop)}},
		{name: "nil loop done", want: "loop Done signal must not be nil", options: []ChannelOption{WithLoop(nilDoneLoop{})}},
		{name: "nil unary interceptor", want: "unary server interceptor must not be nil", options: []ChannelOption{WithServerUnaryInterceptor(nil)}},
		{name: "nil stream interceptor", want: "stream server interceptor must not be nil", options: []ChannelOption{WithServerStreamInterceptor(nil)}},
		{name: "typed nil client stats payload", want: "client stats handler must not be nil", options: []ChannelOption{WithClientStatsHandler(statsHandler)}},
		{name: "typed nil server stats payload", want: "server stats handler must not be nil", options: []ChannelOption{WithServerStatsHandler(statsHandler)}},
		{name: "zero stream buffer", want: "stream buffer must be positive", options: []ChannelOption{WithLoop(mockLoop{}), WithStreamBuffer(0)}},
		{name: "negative stream buffer", want: "stream buffer must be positive", options: []ChannelOption{WithLoop(mockLoop{}), WithStreamBuffer(-1)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertPanicContains(t, test.want, func() {
				NewChannel(test.options...)
			})
		})
	}
}

func TestResolveOptions_RejectsNilElement(t *testing.T) {
	loop := mockLoop{}
	if _, err := resolveOptions(
		[]ChannelOption{WithLoop(loop), nil},
	); err == nil {
		t.Fatal("nil channel option was accepted")
	}
}

func TestResolveOptions_WithCloner(t *testing.T) {
	loop := mockLoop{}
	c := ProtoCloner{}
	opts, err := resolveOptions([]ChannelOption{WithLoop(loop), WithCloner(c)})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := opts.cloner.(ProtoCloner); !ok {
		t.Errorf("cloner = %T, want ProtoCloner", opts.cloner)
	}
}

func TestResolveOptions_WithServerUnaryInterceptor(t *testing.T) {
	loop := mockLoop{}
	called := false
	interceptor := grpc.UnaryServerInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		called = true
		return nil, nil
	})
	opts, err := resolveOptions([]ChannelOption{WithLoop(loop), WithServerUnaryInterceptor(interceptor)})
	if err != nil {
		t.Fatal(err)
	}
	if opts.unaryInterceptor == nil {
		t.Fatal("unaryInterceptor should not be nil")
	}
	_, _ = opts.unaryInterceptor(context.Background(), nil, nil, nil)
	if !called {
		t.Error("interceptor should have been called")
	}
}

func TestResolveOptions_WithServerStreamInterceptor(t *testing.T) {
	loop := mockLoop{}
	called := false
	interceptor := grpc.StreamServerInterceptor(func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		called = true
		return nil
	})
	opts, err := resolveOptions([]ChannelOption{WithLoop(loop), WithServerStreamInterceptor(interceptor)})
	if err != nil {
		t.Fatal(err)
	}
	if opts.streamInterceptor == nil {
		t.Fatal("streamInterceptor should not be nil")
	}
	_ = opts.streamInterceptor(nil, nil, nil, nil)
	if !called {
		t.Error("interceptor should have been called")
	}
}

func TestResolveOptions_WithClientStatsHandler(t *testing.T) {
	loop := mockLoop{}
	opts, err := resolveOptions([]ChannelOption{WithLoop(loop), WithClientStatsHandler(testStatsHandler{})})
	if err != nil {
		t.Fatal(err)
	}
	if opts.clientStats == nil {
		t.Fatal("clientStats should not be nil")
	}
	if !opts.clientStats.isClient {
		t.Error("clientStats.isClient should be true")
	}
}

func TestResolveOptions_WithServerStatsHandler(t *testing.T) {
	loop := mockLoop{}
	opts, err := resolveOptions([]ChannelOption{WithLoop(loop), WithServerStatsHandler(testStatsHandler{})})
	if err != nil {
		t.Fatal(err)
	}
	if opts.serverStats == nil {
		t.Fatal("serverStats should not be nil")
	}
	if opts.serverStats.isClient {
		t.Error("serverStats.isClient should be false")
	}
}

func TestResolveOptions_AllOptions(t *testing.T) {
	loop := mockLoop{}
	opts, err := resolveOptions([]ChannelOption{
		WithLoop(loop),
		WithCloner(ProtoCloner{}),
		WithServerUnaryInterceptor(grpc.UnaryServerInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			return nil, nil
		})),
		WithServerStreamInterceptor(grpc.StreamServerInterceptor(func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			return nil
		})),
		WithClientStatsHandler(testStatsHandler{}),
		WithServerStatsHandler(testStatsHandler{}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.cloner == nil {
		t.Error("cloner missing")
	}
	if opts.unaryInterceptor == nil {
		t.Error("unaryInterceptor missing")
	}
	if opts.streamInterceptor == nil {
		t.Error("streamInterceptor missing")
	}
	if opts.clientStats == nil {
		t.Error("clientStats missing")
	}
	if opts.serverStats == nil {
		t.Error("serverStats missing")
	}
}

func TestResolveOptions_LastWins(t *testing.T) {
	loop := mockLoop{}
	c1 := CloneFunc(func(in any) (any, error) { return "first", nil })
	c2 := CloneFunc(func(in any) (any, error) { return "second", nil })
	opts, err := resolveOptions([]ChannelOption{WithLoop(loop), WithCloner(c1), WithCloner(c2)})
	if err != nil {
		t.Fatal(err)
	}
	if opts.cloner == nil {
		t.Fatal("cloner should not be nil")
	}
	// Verify the last cloner wins by checking the clone result.
	result, err := opts.cloner.Clone(nil)
	if err != nil {
		t.Fatal(err)
	}
	if result != "second" {
		t.Errorf("Clone() = %v, want \"second\" (last wins)", result)
	}
}

func TestResolveOptions_NilClientStatsHandler(t *testing.T) {
	_, err := resolveOptions([]ChannelOption{WithClientStatsHandler(nil)})
	if err == nil {
		t.Fatal("expected error for nil client stats handler")
	}
}

func TestResolveOptions_NilServerStatsHandler(t *testing.T) {
	_, err := resolveOptions([]ChannelOption{WithServerStatsHandler(nil)})
	if err == nil {
		t.Fatal("expected error for nil server stats handler")
	}
}

func TestResolveOptionsRejectsTypedNilOptions(t *testing.T) {
	tests := []struct {
		name   string
		option ChannelOption
	}{
		{name: "cloner", option: (*ClonerOption)(nil)},
		{name: "unary interceptor", option: (*ServerUnaryInterceptorOption)(nil)},
		{name: "stream interceptor", option: (*ServerStreamInterceptorOption)(nil)},
		{name: "client stats", option: (*ClientStatsHandlerOption)(nil)},
		{name: "server stats", option: (*ServerStatsHandlerOption)(nil)},
		{name: "loop", option: (*LoopOption)(nil)},
		{name: "clone disabled", option: (*CloneDisabledOption)(nil)},
		{name: "stream buffer", option: (*StreamBufferOption)(nil)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := resolveOptions(
				[]ChannelOption{test.option},
			); err == nil {
				t.Fatal("typed nil channel option was accepted")
			}
		})
	}
}

func TestResolveOptionsRejectsTypedNilPayloads(t *testing.T) {
	var cloner *ProtoCloner
	var loop *mockLoop
	var handler *testStatsHandler
	tests := []struct {
		name   string
		option ChannelOption
	}{
		{name: "cloner", option: WithCloner(cloner)},
		{name: "loop", option: WithLoop(loop)},
		{name: "client stats", option: WithClientStatsHandler(handler)},
		{name: "server stats", option: WithServerStatsHandler(handler)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := resolveOptions(
				[]ChannelOption{test.option},
			); err == nil {
				t.Fatal("typed nil option payload was accepted")
			}
		})
	}
}
