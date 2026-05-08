package eventloop

import (
	"errors"
	"testing"
)

func TestJS_ConvenienceHelpers(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	js := NewJS(loop)

	t.Run("js.Resolve() returns already fulfilled promise", func(t *testing.T) {
		p := js.Resolve("immediate value")

		if p.State() != Fulfilled {
			t.Fatalf("state = %v, want Fulfilled", p.State())
		}

		if p.Value() != "immediate value" {
			t.Fatalf("value = %#v, want immediate value", p.Value())
		}
	})

	t.Run("js.Resolve() with nil", func(t *testing.T) {
		p := js.Resolve(nil)

		if p.State() != Fulfilled {
			t.Fatalf("state = %v, want Fulfilled", p.State())
		}

		if p.Value() != nil {
			t.Fatalf("value = %#v, want nil", p.Value())
		}
	})

	t.Run("js.Reject() returns already rejected promise", func(t *testing.T) {
		p := js.Reject("immediate error")

		if p.State() != Rejected {
			t.Fatalf("state = %v, want Rejected", p.State())
		}

		if p.Reason() != "immediate error" {
			t.Fatalf("reason = %#v, want immediate error", p.Reason())
		}
	})

	t.Run("js.Reject() with error type", func(t *testing.T) {
		expectedErr := errors.New("actual error")
		p := js.Reject(expectedErr)

		if p.State() != Rejected {
			t.Fatalf("state = %v, want Rejected", p.State())
		}

		if reason := p.Reason(); reason != expectedErr {
			t.Fatalf("reason = %T %#v, want exact error sentinel", reason, reason)
		}
	})
}
