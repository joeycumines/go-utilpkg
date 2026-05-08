//go:build darwin || dragonfly || freebsd || openbsd

package eventloop

import "testing"

func TestKeventPointerTagStorageReuse(t *testing.T) {
	store := new(keventTagStore)
	t.Cleanup(func() {
		if err := store.close(); err != nil {
			t.Errorf("close kqueue tag store: %v", err)
		}
	})
	first, err := store.allocate()
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.allocate()
	if err != nil {
		t.Fatal(err)
	}
	if first == nil || second == nil || first == second {
		t.Fatalf("allocated tags = (%p, %p), want distinct non-nil pointers", first, second)
	}
	store.recycle(first)
	reused, err := store.allocate()
	if err != nil {
		t.Fatal(err)
	}
	if reused != first {
		t.Fatalf("reused tag = %p, want %p", reused, first)
	}
}
