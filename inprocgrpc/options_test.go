package inprocgrpc

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/stats"
)

// mockLoop is a minimal Loop implementation for testing options.
type mockLoop struct{}

func (m mockLoop) Submit(func()) error {
	return nil
}

func (m mockLoop) SubmitInternal(func()) error {
	return nil
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
	opts, err := resolveOptions([]Option{WithLoop(loop)})
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

func TestResolveOptions_NilElementSkipped(t *testing.T) {
	loop := mockLoop{}
	opts, err := resolveOptions([]Option{WithLoop(loop), nil, nil})
	if err != nil {
		t.Fatal(err)
	}
	if opts == nil {
		t.Fatal("opts should not be nil")
	}
}

func TestResolveOptions_WithCloner(t *testing.T) {
	loop := mockLoop{}
	c := ProtoCloner{}
	opts, err := resolveOptions([]Option{WithLoop(loop), WithCloner(c)})
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
	opts, err := resolveOptions([]Option{WithLoop(loop), WithServerUnaryInterceptor(interceptor)})
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
	opts, err := resolveOptions([]Option{WithLoop(loop), WithServerStreamInterceptor(interceptor)})
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
	opts, err := resolveOptions([]Option{WithLoop(loop), WithClientStatsHandler(testStatsHandler{})})
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
	opts, err := resolveOptions([]Option{WithLoop(loop), WithServerStatsHandler(testStatsHandler{})})
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
	opts, err := resolveOptions([]Option{
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
	opts, err := resolveOptions([]Option{WithLoop(loop), WithCloner(c1), WithCloner(c2)})
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
	_, err := resolveOptions([]Option{WithClientStatsHandler(nil)})
	if err == nil {
		t.Fatal("expected error for nil client stats handler")
	}
}

func TestResolveOptions_NilServerStatsHandler(t *testing.T) {
	_, err := resolveOptions([]Option{WithServerStatsHandler(nil)})
	if err == nil {
		t.Fatal("expected error for nil server stats handler")
	}
}
