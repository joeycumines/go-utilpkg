package callopts

import (
	"context"
	"fmt"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type testAddr struct{}

func (testAddr) Network() string { return "test" }
func (testAddr) String() string  { return "test:0" }

type testCreds struct {
	meta       map[string]string
	metaErr    error
	requireTLS bool
}

func (c *testCreds) GetRequestMetadata(_ context.Context, _ ...string) (map[string]string, error) {
	return c.meta, c.metaErr
}

func (c *testCreds) RequireTransportSecurity() bool { return c.requireTLS }

func TestGetCallOptions(t *testing.T) {
	var hdrs metadata.MD
	var tlrs metadata.MD
	var p peer.Peer
	opts := []grpc.CallOption{
		grpc.Header(&hdrs),
		grpc.Trailer(&tlrs),
		grpc.Peer(&p),
		grpc.PerRPCCredentials(&testCreds{meta: map[string]string{"k": "v"}}),
		grpc.MaxCallRecvMsgSize(1024),
		grpc.MaxCallSendMsgSize(2048),
	}
	co := GetCallOptions(opts)
	if len(co.Headers) != 1 || len(co.Trailers) != 1 || len(co.Peer) != 1 {
		t.Fatal("wrong option counts")
	}
	if co.Creds == nil {
		t.Fatal("creds should be set")
	}
	if co.MaxRecv != 1024 || co.MaxSend != 2048 {
		t.Fatalf("MaxRecv=%d MaxSend=%d", co.MaxRecv, co.MaxSend)
	}
}

func TestGetCallOptions_Empty(t *testing.T) {
	co := GetCallOptions(nil)
	if co == nil || len(co.Headers) != 0 {
		t.Fatal("should be empty but non-nil")
	}
}

func TestCallOptions_SetHeaders(t *testing.T) {
	var h1, h2 metadata.MD
	co := &CallOptions{Headers: []*metadata.MD{&h1, &h2}}
	co.SetHeaders(metadata.Pairs("k", "v"))
	if v := h1.Get("k"); len(v) == 0 || v[0] != "v" {
		t.Error("h1 not set")
	}
	if v := h2.Get("k"); len(v) == 0 || v[0] != "v" {
		t.Error("h2 not set")
	}
}

func TestCallOptions_SetTrailers(t *testing.T) {
	var t1 metadata.MD
	co := &CallOptions{Trailers: []*metadata.MD{&t1}}
	co.SetTrailers(metadata.Pairs("tk", "tv"))
	if v := t1.Get("tk"); len(v) == 0 || v[0] != "tv" {
		t.Error("not set")
	}
}

func TestCallOptions_SetPeer(t *testing.T) {
	var p peer.Peer
	co := &CallOptions{Peer: []*peer.Peer{&p}}
	co.SetPeer(&peer.Peer{Addr: testAddr{}})
	if p.Addr == nil {
		t.Error("peer not set")
	}
}

func TestApplyPerRPCCreds_NoCreds(t *testing.T) {
	ctx := context.Background()
	out, err := ApplyPerRPCCreds(ctx, &CallOptions{}, "u", true)
	if err != nil || out != ctx {
		t.Fatal("unexpected")
	}
}

func TestApplyPerRPCCreds_WithCreds(t *testing.T) {
	co := &CallOptions{Creds: &testCreds{meta: map[string]string{"auth": "tok"}}}
	out, err := ApplyPerRPCCreds(context.Background(), co, "u", true)
	if err != nil {
		t.Fatal(err)
	}
	md, _ := metadata.FromOutgoingContext(out)
	if v := md.Get("auth"); len(v) == 0 || v[0] != "tok" {
		t.Error("creds not applied")
	}
}

func TestApplyPerRPCCreds_RequiresTLS(t *testing.T) {
	co := &CallOptions{Creds: &testCreds{requireTLS: true}}
	_, err := ApplyPerRPCCreds(context.Background(), co, "u", false)
	if err == nil {
		t.Fatal("expected error")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unauthenticated {
		t.Errorf("got %v", st.Code())
	}
}

func TestApplyPerRPCCreds_GetMetadataError(t *testing.T) {
	co := &CallOptions{Creds: &testCreds{metaErr: fmt.Errorf("metadata error")}}
	_, err := ApplyPerRPCCreds(context.Background(), co, "u", true)
	if err == nil {
		t.Fatal("expected error")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unauthenticated {
		t.Errorf("got %v", st.Code())
	}
}

func TestApplyPerRPCCreds_EmptyMetadata(t *testing.T) {
	co := &CallOptions{Creds: &testCreds{meta: nil}}
	ctx := context.Background()
	out, err := ApplyPerRPCCreds(ctx, co, "u", true)
	if err != nil {
		t.Fatal(err)
	}
	// With nil metadata, context should not have outgoing metadata added
	if out != ctx {
		md, ok := metadata.FromOutgoingContext(out)
		if ok && len(md) > 0 {
			t.Errorf("expected no outgoing metadata, got %v", md)
		}
	}
}
