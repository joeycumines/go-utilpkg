//go:build netbsd

package eventloop

import (
	"errors"
	"testing"

	"golang.org/x/sys/unix"
)

func TestNetBSDKeventTagSignedRoundTrip(t *testing.T) {
	for _, raw := range []uint32{1, 1 << 31, ^uint32(0)} {
		var event unix.Kevent_t
		setKeventTag(&event, keventTag(raw))
		if got := keventEventTag(&event); got != keventTag(raw) {
			t.Fatalf("tag round trip for %#x = %#x", raw, uint32(got))
		}
	}
}

func TestNetBSDKeventTagExhaustionAndReuse(t *testing.T) {
	store := keventTagStore{next: uint64(^uint32(0)) - 1}
	last, err := store.allocate()
	if err != nil || last != keventTag(^uint32(0)) {
		t.Fatalf("last tag = (%#x, %v), want (%#x, nil)", uint32(last), err, ^uint32(0))
	}
	if _, err := store.allocate(); !errors.Is(err, ErrFDRegistrationExhausted) {
		t.Fatalf("exhausted allocation error = %v, want %v", err, ErrFDRegistrationExhausted)
	}
	store.recycle(last)
	if got, err := store.allocate(); err != nil || got != last {
		t.Fatalf("reused tag = (%#x, %v), want (%#x, nil)", uint32(got), err, uint32(last))
	}
}
