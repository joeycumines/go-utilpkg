package proxy

import (
	"context"
	"io"
	"net"
	"testing"
	"time"
)

func TestDialTCP(t *testing.T) {
	t.Run("NilContext", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("expected panic for nil context")
			}
		}()
		// Intentionally using nil to test panic behavior
		//lint:ignore SA1012 testing panic behavior
		_, _ = DialTCP(nil, "localhost:8080")
		t.Error("expected panic for nil context")
	})

	t.Run("ValidContext", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()

		l, err := net.ListenTCP(`tcp`, &net.TCPAddr{})
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan struct{})
		go func() {
			defer close(done)
			for {
				v, err := l.AcceptTCP()
				if err != nil {
					return
				}
				_, _ = io.Copy(io.Discard, v)
			}
		}()
		defer func() {
			_ = l.Close()
			<-done
		}()

		for range 3 {
			conn, err := DialTCP(ctx, l.Addr().String())
			if err != nil {
				t.Fatalf("failed to dial: %v", err)
			}
			if err := conn.Close(); err != nil {
				t.Errorf("failed to close connection: %v", err)
			}
		}
	})
}

func TestDialWithCancel(t *testing.T) {
	t.Run("NilContext", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("expected panic for nil context")
			}
		}()
		//lint:ignore SA1012 testing panic behavior
		DialWithCancel(nil, DialTCP)
	})

	t.Run("NilDialer", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("expected panic for nil dialer")
			}
		}()
		DialWithCancel(context.Background(), nil)
	})

	t.Run("ValidContextAndDialer", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		dialer := DialWithCancel(ctx, DialTCP)
		_, err := dialer(context.Background(), "localhost:8080")
		if err == nil {
			t.Errorf("expected error for canceled context")
		}
	})
}

func TestDialWithCancel_UncoveredCases(t *testing.T) {
	t.Run("Context2Error", func(t *testing.T) {
		ctx2, cancel2 := context.WithCancel(context.Background())
		cancel2() // Immediately cancel ctx2

		dialer := DialWithCancel(context.Background(), DialTCP)
		_, err := dialer(ctx2, "localhost:8080")
		if err == nil {
			t.Errorf("expected error for ctx2.Err(), got nil")
		}
	})

	t.Run("ParentContextError", func(t *testing.T) {
		parentCtx, parentCancel := context.WithCancel(context.Background())
		parentCancel() // Immediately cancel parent context

		dialer := DialWithCancel(parentCtx, DialTCP)
		_, err := dialer(context.Background(), "localhost:8080")
		if err != context.Canceled {
			t.Errorf("expected context.Canceled error, got %v", err)
		}
	})
}

func TestDialWithTimeout(t *testing.T) {
	t.Run("ValidTimeout", func(t *testing.T) {
		dialer := DialWithTimeout(time.Second, DialTCP)
		ctx := context.Background()
		_, err := dialer(ctx, "localhost:8080")
		if err == nil {
			t.Errorf("expected error for timeout")
		}
	})
}
