package eventloop

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type panicDoneContext struct{ context.Context }

func (panicDoneContext) Done() <-chan struct{} { panic("context Done panic") }

type panicErrContext struct{ context.Context }

func (panicErrContext) Done() <-chan struct{} {
	done := make(chan struct{})
	close(done)
	return done
}

func (panicErrContext) Err() error { panic("context Err panic") }

func TestPromisifyNilContext(t *testing.T) {
	wantErr := errors.New("nil-context error")
	errNilContext := errors.New("Promisify passed a nil context")
	tests := []struct {
		name string
		fn   func(context.Context) (any, error)
		want any
	}{
		{
			name: "success",
			fn: func(ctx context.Context) (any, error) {
				if ctx == nil {
					return nil, errNilContext
				}
				return "ok", nil
			},
			want: "ok",
		},
		{
			name: "error",
			fn: func(ctx context.Context) (any, error) {
				if ctx == nil {
					return nil, errNilContext
				}
				return nil, wantErr
			},
			want: wantErr,
		},
		{
			name: "panic",
			fn: func(context.Context) (any, error) {
				panic("nil-context panic")
			},
			want: PanicError{Value: "nil-context panic"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loop := New()
			registerLoopCleanupT(t, loop)
			runDone := make(chan error, 1)
			go func() { runDone <- loop.Run(context.Background()) }()

			var nilContext context.Context
			promise := loop.Promisify(nilContext, tt.fn)
			select {
			case got := <-promise.ToChannel():
				switch want := tt.want.(type) {
				case PanicError:
					var gotPanic PanicError
					gotErr, ok := got.(error)
					if !ok || !errors.As(gotErr, &gotPanic) || gotPanic.Value != want.Value {
						t.Fatalf("result = %#v, want PanicError value %#v", got, want.Value)
					}
				case error:
					gotErr, ok := got.(error)
					if !ok || !errors.Is(gotErr, want) {
						t.Fatalf("result = %v, want error %v", got, want)
					}
				default:
					if got != want {
						t.Fatalf("result = %#v, want %#v", got, want)
					}
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Promisify did not settle")
			}

			if err := loop.Shutdown(context.Background()); err != nil {
				t.Fatalf("Shutdown: %v", err)
			}
			if err := waitContractValue(t, runDone, "nil-context Run completion"); err != nil {
				t.Fatalf("Run: %v", err)
			}
		})
	}
}

func TestPromisifyRecoversContextDonePanic(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	var called atomic.Bool
	promise := loop.Promisify(panicDoneContext{Context: context.Background()}, func(context.Context) (any, error) {
		called.Store(true)
		return "unexpected", nil
	})
	select {
	case got := <-promise.ToChannel():
		var panicErr PanicError
		gotErr, ok := got.(error)
		if !ok || !errors.As(gotErr, &panicErr) || panicErr.Value != "context Done panic" {
			t.Fatalf("result = %#v, want recovered context panic", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Promisify did not settle")
	}
	if called.Load() {
		t.Fatal("user function ran after context Done panic")
	}
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := waitContractValue(t, runDone, "context Done panic Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestPromisifyRecoversContextErrPanic(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	var called atomic.Bool
	promise := loop.Promisify(panicErrContext{Context: context.Background()}, func(context.Context) (any, error) {
		called.Store(true)
		return "unexpected", nil
	})
	select {
	case got := <-promise.ToChannel():
		var panicErr PanicError
		gotErr, ok := got.(error)
		if !ok || !errors.As(gotErr, &panicErr) || panicErr.Value != "context Err panic" {
			t.Fatalf("result = %#v, want recovered context panic", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Promisify did not settle")
	}
	if called.Load() {
		t.Fatal("user function ran after context Err panic")
	}
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := waitContractValue(t, runDone, "context Err panic Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestNewLoopPerformanceNilPanicsImmediately(t *testing.T) {
	defer func() {
		if got := recover(); got != "eventloop: nil Loop" {
			t.Fatalf("panic = %#v, want %q", got, "eventloop: nil Loop")
		}
	}()
	NewLoopPerformance(nil)
}
