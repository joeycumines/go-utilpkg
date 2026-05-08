package promisealtone_test

import (
	"context"
	"testing"
	"time"

	"github.com/joeycumines/go-eventloop/internal/promisealtone"
)

func TestPromise_String(t *testing.T) {
	t.Run("Pending", func(t *testing.T) {
		p, _, _ := promisealtone.New(nil)
		if s := p.String(); s != "Promise<Pending>" {
			t.Errorf("expected Promise<Pending>, got %q", s)
		}
	})

	t.Run("Fulfilled", func(t *testing.T) {
		p, resolve, _ := promisealtone.New(nil)
		resolve(123)
		if s := p.String(); s != "Promise<Fulfilled: 123>" {
			t.Errorf("expected Promise<Fulfilled: 123>, got %q", s)
		}
	})

	t.Run("Rejected", func(t *testing.T) {
		p, _, reject := promisealtone.New(nil)
		reject("error")
		if s := p.String(); s != "Promise<Rejected: error>" {
			t.Errorf("expected Promise<Rejected: error>, got %q", s)
		}
	})
}

func TestPromise_Await(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		p, resolve, _ := promisealtone.New(nil)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		go func() {
			resolve(42)
		}()

		res, err := p.Await(ctx)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if res != 42 {
			t.Errorf("expected 42, got %v", res)
		}
	})

	t.Run("Failure", func(t *testing.T) {
		p, _, reject := promisealtone.New(nil)

		go func() {
			reject("fail")
		}()

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, err := p.Await(ctx)
		if err == nil {
			t.Error("expected error, got nil")
		}
		if err != nil && err.Error() != "fail" {
			t.Errorf("expected error 'fail', got %q", err.Error())
		}
	})

	t.Run("ContextCancel", func(t *testing.T) {
		p, _, _ := promisealtone.New(nil)

		// No resolve/reject

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()

		// Loop not strictly needed for this test as we wait on ctx
		// but Await attaches Observe, which might use JS.
		// Since no resolve happens, no microtask scheduled.
		// So loop not needed.

		_, err := p.Await(ctx)
		if err != context.DeadlineExceeded {
			t.Errorf("expected context deadline exceeded, got %v", err)
		}
	})

	t.Run("AlreadyResolved", func(t *testing.T) {
		p, resolve, _ := promisealtone.New(nil)
		resolve("fast")

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		res, err := p.Await(ctx)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if res != "fast" {
			t.Errorf("expected 'fast', got %v", res)
		}
	})
}
