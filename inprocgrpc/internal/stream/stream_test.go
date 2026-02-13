package stream

import (
	"errors"
	"io"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestHalfStream_SendThenWait(t *testing.T) {
	var h HalfStream
	if err := h.Send("msg1"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	var got any
	var gotErr error
	h.Recv(func(msg any, err error) {
		got = msg
		gotErr = err
	})
	if got != "msg1" || gotErr != nil {
		t.Fatalf("got (%v, %v), want (msg1, nil)", got, gotErr)
	}
}

func TestHalfStream_WaitThenSend(t *testing.T) {
	var h HalfStream
	var got any
	var gotErr error
	h.Recv(func(msg any, err error) {
		got = msg
		gotErr = err
	})
	if got != nil {
		t.Fatal("callback should not fire before Send")
	}
	if err := h.Send("msg1"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got != "msg1" || gotErr != nil {
		t.Fatalf("got (%v, %v), want (msg1, nil)", got, gotErr)
	}
}

func TestHalfStream_BufferingFIFO(t *testing.T) {
	var h HalfStream
	for _, m := range []string{"a", "b", "c"} {
		if err := h.Send(m); err != nil {
			t.Fatalf("Send(%s): %v", m, err)
		}
	}
	var msgs []any
	for range 3 {
		h.Recv(func(msg any, err error) {
			if err != nil {
				t.Fatalf("Recv: %v", err)
			}
			msgs = append(msgs, msg)
		})
	}
	if len(msgs) != 3 || msgs[0] != "a" || msgs[1] != "b" || msgs[2] != "c" {
		t.Fatalf("got %v, want [a b c]", msgs)
	}
}

func TestHalfStream_InterleavedSendWait(t *testing.T) {
	var h HalfStream
	for i := range 5 {
		if err := h.Send(i); err != nil {
			t.Fatal(err)
		}
		var got any
		h.Recv(func(msg any, err error) {
			if err != nil {
				t.Fatal(err)
			}
			got = msg
		})
		if got != i {
			t.Fatalf("iteration %d: got %v", i, got)
		}
	}
}

func TestHalfStream_SendAfterClose(t *testing.T) {
	var h HalfStream
	h.Close(nil)
	if err := h.Send("x"); err != io.EOF {
		t.Fatalf("got %v, want io.EOF", err)
	}
}

func TestHalfStream_SendAfterCloseWithError(t *testing.T) {
	var h HalfStream
	h.Close(errors.New("boom"))
	if err := h.Send("x"); err != io.EOF {
		t.Fatalf("got %v, want io.EOF", err)
	}
}

func TestHalfStream_WaitOnClosedEmpty(t *testing.T) {
	var h HalfStream
	h.Close(nil)
	var gotErr error
	h.Recv(func(_ any, err error) { gotErr = err })
	if gotErr != io.EOF {
		t.Fatalf("got %v, want io.EOF", gotErr)
	}
}

func TestHalfStream_WaitOnClosedWithError(t *testing.T) {
	var h HalfStream
	myErr := errors.New("test error")
	h.Close(myErr)
	var gotErr error
	h.Recv(func(_ any, err error) { gotErr = err })
	if gotErr != myErr {
		t.Fatalf("got %v, want %v", gotErr, myErr)
	}
}

func TestHalfStream_CloseNotifiesPendingWaiter_Nil(t *testing.T) {
	var h HalfStream
	var gotErr error
	h.Recv(func(_ any, err error) { gotErr = err })
	h.Close(nil)
	if gotErr != io.EOF {
		t.Fatalf("got %v, want io.EOF", gotErr)
	}
}

func TestHalfStream_CloseNotifiesPendingWaiter_Error(t *testing.T) {
	var h HalfStream
	myErr := errors.New("close err")
	var gotErr error
	h.Recv(func(_ any, err error) { gotErr = err })
	h.Close(myErr)
	if gotErr != myErr {
		t.Fatalf("got %v, want %v", gotErr, myErr)
	}
}

func TestHalfStream_DoubleCloseIsIdempotent(t *testing.T) {
	var h HalfStream
	h.Close(nil)
	h.Close(errors.New("second"))
	if !h.Closed() {
		t.Fatal("should be closed")
	}
	// First close wins.
	if h.Err() != nil {
		t.Fatalf("Err() = %v, want nil (from first close)", h.Err())
	}
}

func TestHalfStream_Closed(t *testing.T) {
	var h HalfStream
	if h.Closed() {
		t.Fatal("should not be closed initially")
	}
	h.Close(nil)
	if !h.Closed() {
		t.Fatal("should be closed after Close(nil)")
	}
}

func TestHalfStream_Err(t *testing.T) {
	t.Run("before close", func(t *testing.T) {
		var h HalfStream
		if h.Err() != nil {
			t.Fatalf("Err() = %v, want nil", h.Err())
		}
	})
	t.Run("clean close", func(t *testing.T) {
		var h HalfStream
		h.Close(nil)
		if h.Err() != nil {
			t.Fatalf("Err() = %v, want nil", h.Err())
		}
	})
	t.Run("error close", func(t *testing.T) {
		var h HalfStream
		myErr := errors.New("e")
		h.Close(myErr)
		if h.Err() != myErr {
			t.Fatalf("Err() = %v, want %v", h.Err(), myErr)
		}
	})
}

func TestHalfStream_SendNilPanics(t *testing.T) {
	var h HalfStream
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil message")
		}
	}()
	h.Send(nil) //nolint:errcheck // we expect a panic
}

func TestHalfStream_WaitDrainsBufferBeforeClosed(t *testing.T) {
	var h HalfStream
	_ = h.Send("m1")
	_ = h.Send("m2")
	h.Close(nil)

	var msgs []any
	for range 2 {
		h.Recv(func(msg any, err error) {
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			msgs = append(msgs, msg)
		})
	}
	// Third call should get EOF after buffer is drained.
	var gotErr error
	h.Recv(func(_ any, err error) { gotErr = err })

	if len(msgs) != 2 || msgs[0] != "m1" || msgs[1] != "m2" {
		t.Fatalf("msgs = %v, want [m1 m2]", msgs)
	}
	if gotErr != io.EOF {
		t.Fatalf("gotErr = %v, want io.EOF", gotErr)
	}
}

func TestHalfStream_WaitDrainsBufferBeforeError(t *testing.T) {
	var h HalfStream
	myErr := errors.New("e")
	_ = h.Send("m1")
	h.Close(myErr)

	// First wait should get the buffered message, not the error.
	var got any
	h.Recv(func(msg any, err error) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got = msg
	})
	if got != "m1" {
		t.Fatalf("got %v, want m1", got)
	}

	// Second wait should get the close error.
	var gotErr error
	h.Recv(func(_ any, err error) { gotErr = err })
	if gotErr != myErr {
		t.Fatalf("gotErr = %v, want %v", gotErr, myErr)
	}
}

func TestHalfStream_DuplicateWaiterPanics(t *testing.T) {
	var h HalfStream
	h.Recv(func(any, error) {}) // first waiter - saved
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for duplicate waiter")
		}
	}()
	h.Recv(func(any, error) {}) // second - should panic
}

func TestHalfStream_ReentrantSendDuringWaitCallback(t *testing.T) {
	var h HalfStream
	h.Recv(func(msg any, err error) {
		if err != nil {
			t.Fatal(err)
		}
		// Re-entrant: buffer a message from inside the callback.
		_ = h.Send("inner")
	})
	_ = h.Send("outer") // triggers callback, which buffers "inner"

	var got any
	h.Recv(func(msg any, err error) {
		if err != nil {
			t.Fatal(err)
		}
		got = msg
	})
	if got != "inner" {
		t.Fatalf("got %v, want inner", got)
	}
}

func TestHalfStream_ReentrantCloseDuringWaitCallback(t *testing.T) {
	var h HalfStream
	h.Recv(func(_ any, err error) {
		if err != nil {
			t.Fatal(err)
		}
		h.Close(nil) // close from within callback
	})
	_ = h.Send("trigger")

	if !h.Closed() {
		t.Fatal("should be closed")
	}
	if err := h.Send("after"); err != io.EOF {
		t.Fatalf("Send after close = %v, want io.EOF", err)
	}
}

func TestRPCState_SendHeaders(t *testing.T) {
	var r RPCState
	r.ResponseHeaders = metadata.MD{"key": {"value"}}

	var gotMD metadata.MD
	var gotErr error
	r.HeaderWaiter = func(md metadata.MD, err error) {
		gotMD = md
		gotErr = err
	}

	r.SendHeaders()

	if !r.HeadersSent {
		t.Fatal("HeadersSent should be true")
	}
	if gotErr != nil {
		t.Fatalf("header error: %v", gotErr)
	}
	if len(gotMD["key"]) != 1 || gotMD["key"][0] != "value" {
		t.Fatalf("headers = %v", gotMD)
	}
	if r.HeaderWaiter != nil {
		t.Fatal("HeaderWaiter should be nil after delivery")
	}
}

func TestRPCState_SendHeaders_Idempotent(t *testing.T) {
	var r RPCState
	calls := 0
	r.HeaderWaiter = func(metadata.MD, error) { calls++ }

	r.SendHeaders()
	r.SendHeaders()

	if calls != 1 {
		t.Fatalf("waiter called %d times, want 1", calls)
	}
}

func TestRPCState_SendHeaders_NoWaiter(t *testing.T) {
	var r RPCState
	r.ResponseHeaders = metadata.MD{"k": {"v"}}
	r.SendHeaders() // no waiter - should not panic
	if !r.HeadersSent {
		t.Fatal("HeadersSent should be true")
	}
}

func TestRPCState_SetHeaders(t *testing.T) {
	var r RPCState
	if err := r.SetHeaders(metadata.MD{"k1": {"v1"}}); err != nil {
		t.Fatal(err)
	}
	if err := r.SetHeaders(metadata.MD{"k2": {"v2"}}); err != nil {
		t.Fatal(err)
	}
	if r.ResponseHeaders["k1"][0] != "v1" || r.ResponseHeaders["k2"][0] != "v2" {
		t.Fatalf("headers = %v", r.ResponseHeaders)
	}
}

func TestRPCState_SetHeaders_AfterSent(t *testing.T) {
	var r RPCState
	r.SendHeaders()

	err := r.SetHeaders(metadata.MD{"k": {"v"}})
	if err == nil {
		t.Fatal("expected error")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("expected gRPC status error")
	}
	if st.Code() != codes.Internal {
		t.Fatalf("code = %v, want Internal", st.Code())
	}
}

func TestRPCState_SetHeaders_MergesValues(t *testing.T) {
	var r RPCState
	_ = r.SetHeaders(metadata.MD{"k": {"a"}})
	_ = r.SetHeaders(metadata.MD{"k": {"b"}})
	vals := r.ResponseHeaders["k"]
	if len(vals) != 2 || vals[0] != "a" || vals[1] != "b" {
		t.Fatalf("merged values = %v, want [a b]", vals)
	}
}

func TestRPCState_SetTrailers(t *testing.T) {
	var r RPCState
	r.SetTrailers(metadata.MD{"t1": {"v1"}})
	r.SetTrailers(metadata.MD{"t2": {"v2"}})
	if r.ResponseTrailers["t1"][0] != "v1" || r.ResponseTrailers["t2"][0] != "v2" {
		t.Fatalf("trailers = %v", r.ResponseTrailers)
	}
}

func TestRPCState_SetTrailers_MergesValues(t *testing.T) {
	var r RPCState
	r.SetTrailers(metadata.MD{"k": {"a"}})
	r.SetTrailers(metadata.MD{"k": {"b"}})
	vals := r.ResponseTrailers["k"]
	if len(vals) != 2 || vals[0] != "a" || vals[1] != "b" {
		t.Fatalf("merged values = %v, want [a b]", vals)
	}
}

func TestRPCState_FinishWithTrailers_Success(t *testing.T) {
	var r RPCState
	_ = r.SetHeaders(metadata.MD{"h": {"hv"}})

	var gotMD metadata.MD
	var gotErr error
	r.HeaderWaiter = func(md metadata.MD, err error) {
		gotMD = md
		gotErr = err
	}

	r.FinishWithTrailers(nil)

	if !r.HeadersSent {
		t.Fatal("HeadersSent should be true")
	}
	if gotErr != nil {
		t.Fatalf("header error: %v", gotErr)
	}
	if len(gotMD["h"]) != 1 || gotMD["h"][0] != "hv" {
		t.Fatalf("headers = %v", gotMD)
	}
	if !r.Responses.Closed() {
		t.Fatal("Responses should be closed")
	}
	if r.Responses.Err() != nil {
		t.Fatalf("Responses.Err() = %v, want nil", r.Responses.Err())
	}
}

func TestRPCState_FinishWithTrailers_ErrorBeforeHeaders(t *testing.T) {
	var r RPCState
	myErr := status.Error(codes.NotFound, "not found")

	var gotMD metadata.MD
	var gotErr error
	r.HeaderWaiter = func(md metadata.MD, err error) {
		gotMD = md
		gotErr = err
	}

	r.FinishWithTrailers(myErr)

	if !r.HeadersSent {
		t.Fatal("HeadersSent should be true")
	}
	if gotMD != nil {
		t.Fatalf("headers = %v, want nil", gotMD)
	}
	if gotErr != myErr {
		t.Fatalf("header error = %v, want %v", gotErr, myErr)
	}
	if !r.Responses.Closed() {
		t.Fatal("Responses should be closed")
	}
	if r.Responses.Err() != myErr {
		t.Fatalf("Responses.Err() = %v, want %v", r.Responses.Err(), myErr)
	}
}

func TestRPCState_FinishWithTrailers_ErrorAfterHeaders(t *testing.T) {
	var r RPCState
	r.SendHeaders()

	myErr := status.Error(codes.Internal, "fail")
	r.FinishWithTrailers(myErr)

	if !r.Responses.Closed() {
		t.Fatal("Responses should be closed")
	}
	if r.Responses.Err() != myErr {
		t.Fatalf("Responses.Err() = %v, want %v", r.Responses.Err(), myErr)
	}
}

func TestRPCState_FinishWithTrailers_NoHeaderWaiter(t *testing.T) {
	var r RPCState
	r.FinishWithTrailers(nil)
	if !r.HeadersSent {
		t.Fatal("HeadersSent should be true")
	}
	if !r.Responses.Closed() {
		t.Fatal("Responses should be closed")
	}
}

func TestRPCState_FullFlow(t *testing.T) {
	var r RPCState
	r.Method = "/test.Service/Method"

	// Accumulate headers.
	if err := r.SetHeaders(metadata.MD{"auth": {"token"}}); err != nil {
		t.Fatal(err)
	}

	// Client waits for headers.
	var headerMD metadata.MD
	r.HeaderWaiter = func(md metadata.MD, err error) {
		if err != nil {
			t.Fatalf("header err: %v", err)
		}
		headerMD = md
	}

	// Server sends headers explicitly.
	r.SendHeaders()
	if headerMD["auth"][0] != "token" {
		t.Fatalf("headers = %v", headerMD)
	}

	// Client sends a request message (client-to-server).
	_ = r.Requests.Send("req1")
	var gotReq any
	r.Requests.Recv(func(msg any, err error) {
		if err != nil {
			t.Fatal(err)
		}
		gotReq = msg
	})
	if gotReq != "req1" {
		t.Fatalf("request = %v", gotReq)
	}

	// Server sends response messages (server-to-client).
	_ = r.Responses.Send("resp1")
	_ = r.Responses.Send("resp2")

	var resps []any
	for range 2 {
		r.Responses.Recv(func(msg any, err error) {
			if err != nil {
				t.Fatal(err)
			}
			resps = append(resps, msg)
		})
	}
	if len(resps) != 2 || resps[0] != "resp1" || resps[1] != "resp2" {
		t.Fatalf("responses = %v", resps)
	}

	// Set trailers and finish.
	r.SetTrailers(metadata.MD{"status-detail": {"ok"}})
	r.FinishWithTrailers(nil)

	if !r.Responses.Closed() {
		t.Fatal("Responses should be closed")
	}
	if r.ResponseTrailers["status-detail"][0] != "ok" {
		t.Fatalf("trailers = %v", r.ResponseTrailers)
	}

	// Another recv should yield EOF.
	var eofErr error
	r.Responses.Recv(func(_ any, err error) { eofErr = err })
	if eofErr != io.EOF {
		t.Fatalf("post-finish recv = %v, want io.EOF", eofErr)
	}
}

func TestRPCState_SetHeaders_AfterFinish(t *testing.T) {
	var r RPCState
	r.FinishWithTrailers(nil)

	err := r.SetHeaders(metadata.MD{"k": {"v"}})
	if err == nil {
		t.Fatal("expected error setting headers after finish")
	}
}

func TestRPCState_FinishWithTrailers_ErrorNoWaiter(t *testing.T) {
	var r RPCState
	myErr := errors.New("handler failed")
	r.FinishWithTrailers(myErr)

	if !r.HeadersSent {
		t.Fatal("HeadersSent should be true")
	}
	if !r.Responses.Closed() {
		t.Fatal("Responses should be closed")
	}
	if r.Responses.Err() != myErr {
		t.Fatalf("Responses.Err() = %v, want %v", r.Responses.Err(), myErr)
	}
}

func TestRPCState_MethodField(t *testing.T) {
	r := RPCState{Method: "/pkg.Svc/Foo"}
	if r.Method != "/pkg.Svc/Foo" {
		t.Fatalf("Method = %q", r.Method)
	}
}
