package transport

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestUnaryServerTransportStream_Method(t *testing.T) {
	sts := &UnaryServerTransportStream{Name: "/test/Method"}
	if sts.Method() != "/test/Method" {
		t.Errorf("got %q", sts.Method())
	}
}

func TestUnaryServerTransportStream_Headers(t *testing.T) {
	sts := &UnaryServerTransportStream{Name: "/test/Method"}

	if err := sts.SetHeader(metadata.Pairs("key1", "val1")); err != nil {
		t.Fatal(err)
	}
	if err := sts.SetHeader(metadata.Pairs("key2", "val2")); err != nil {
		t.Fatal(err)
	}

	hdrs := sts.GetHeaders()
	if v := hdrs.Get("key1"); len(v) == 0 || v[0] != "val1" {
		t.Errorf("key1: %v", hdrs)
	}
	if v := hdrs.Get("key2"); len(v) == 0 || v[0] != "val2" {
		t.Errorf("key2: %v", hdrs)
	}

	if err := sts.SendHeader(metadata.Pairs("key3", "val3")); err != nil {
		t.Fatal(err)
	}

	if err := sts.SetHeader(metadata.Pairs("late", "val")); err != ErrHeadersAlreadySent {
		t.Errorf("expected ErrHeadersAlreadySent, got %v", err)
	}

	if err := sts.SendHeader(nil); err != ErrHeadersAlreadySent {
		t.Errorf("expected ErrHeadersAlreadySent, got %v", err)
	}
}

func TestUnaryServerTransportStream_Trailers(t *testing.T) {
	sts := &UnaryServerTransportStream{Name: "/test/Method"}

	if err := sts.SetTrailer(metadata.Pairs("tkey", "tval")); err != nil {
		t.Fatal(err)
	}

	tlrs := sts.GetTrailers()
	if v := tlrs.Get("tkey"); len(v) == 0 || v[0] != "tval" {
		t.Errorf("tkey: %v", tlrs)
	}

	sts.Finish()

	if err := sts.SetTrailer(metadata.Pairs("late", "val")); err != ErrTrailersAlreadySent {
		t.Errorf("expected ErrTrailersAlreadySent, got %v", err)
	}
}

func TestUnaryServerTransportStream_Finish(t *testing.T) {
	sts := &UnaryServerTransportStream{Name: "/test/Method"}
	sts.Finish()

	if err := sts.SetHeader(nil); err != ErrHeadersAlreadySent {
		t.Errorf("expected ErrHeadersAlreadySent, got %v", err)
	}
	if err := sts.SetTrailer(nil); err != ErrTrailersAlreadySent {
		t.Errorf("expected ErrTrailersAlreadySent, got %v", err)
	}
}

func TestServerTransportStream_Method(t *testing.T) {
	sts := &ServerTransportStream{Name: "/test/Stream"}
	if sts.Method() != "/test/Stream" {
		t.Errorf("got %q", sts.Method())
	}
}

func TestServerTransportStream_SetHeader(t *testing.T) {
	mock := &mockServerStream{}
	sts := &ServerTransportStream{Name: "/test/Stream", Stream: mock}
	if err := sts.SetHeader(metadata.Pairs("h", "v")); err != nil {
		t.Fatal(err)
	}
	if v := mock.headers.Get("h"); len(v) == 0 || v[0] != "v" {
		t.Errorf("headers: %v", mock.headers)
	}
}

func TestServerTransportStream_SendHeader(t *testing.T) {
	mock := &mockServerStream{}
	sts := &ServerTransportStream{Name: "/test/Stream", Stream: mock}
	if err := sts.SendHeader(metadata.Pairs("h2", "v2")); err != nil {
		t.Fatal(err)
	}
	if !mock.headerSent {
		t.Error("SendHeader not called")
	}
}

func TestServerTransportStream_SetTrailer(t *testing.T) {
	mock := &mockServerStream{}
	sts := &ServerTransportStream{Name: "/test/Stream", Stream: mock}
	if err := sts.SetTrailer(metadata.Pairs("t", "v")); err != nil {
		t.Fatal(err)
	}
	if v := mock.trailers.Get("t"); len(v) == 0 || v[0] != "v" {
		t.Errorf("trailers: %v", mock.trailers)
	}
}

func TestServerTransportStream_SetTrailer_TryTrailer(t *testing.T) {
	mock := &mockServerStreamWithTryTrailer{}
	sts := &ServerTransportStream{Name: "/test/Stream", Stream: mock}
	if err := sts.SetTrailer(metadata.Pairs("t", "v")); err != nil {
		t.Fatal(err)
	}
	if !mock.tryCalled {
		t.Error("TrySetTrailer not called")
	}
}

func TestErrHeadersAlreadySent_Error(t *testing.T) {
	if ErrHeadersAlreadySent.Error() != "headers already sent" {
		t.Errorf("got %q", ErrHeadersAlreadySent.Error())
	}
}

func TestErrTrailersAlreadySent_Error(t *testing.T) {
	if ErrTrailersAlreadySent.Error() != "trailers already sent" {
		t.Errorf("got %q", ErrTrailersAlreadySent.Error())
	}
}

// mockServerStream for testing
type mockServerStream struct {
	headers    metadata.MD
	trailers   metadata.MD
	headerSent bool
}

func (m *mockServerStream) SetHeader(md metadata.MD) error {
	m.headers = metadata.Join(m.headers, md)
	return nil
}

func (m *mockServerStream) SendHeader(md metadata.MD) error {
	m.headers = metadata.Join(m.headers, md)
	m.headerSent = true
	return nil
}

func (m *mockServerStream) SetTrailer(md metadata.MD) {
	m.trailers = metadata.Join(m.trailers, md)
}

func (m *mockServerStream) Context() context.Context {
	return context.Background()
}

func (m *mockServerStream) SendMsg(any) error { return nil }
func (m *mockServerStream) RecvMsg(any) error { return nil }

var _ grpc.ServerStream = (*mockServerStream)(nil)

type mockServerStreamWithTryTrailer struct {
	mockServerStream
	tryCalled bool
}

func (m *mockServerStreamWithTryTrailer) TrySetTrailer(md metadata.MD) error {
	m.tryCalled = true
	return nil
}
