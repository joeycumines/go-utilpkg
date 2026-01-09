package eventloop

import (
	"sync"
	"testing"
)

// FuzzIngressQueue performs fuzz testing on the IngressQueue to verify
// invariants hold under various push/pop sequences.
func FuzzIngressQueue(f *testing.F) {
	f.Add(uint8(10), uint8(10))

	f.Fuzz(func(t *testing.T, pushCount uint8, popCount uint8) {
		pushes := int(pushCount) % 200
		pops := int(popCount) % 200

		q := IngressQueue{}
		mu := sync.Mutex{}

		for i := 0; i < pushes; i++ {
			mu.Lock()
			q.Push(Task{})
			mu.Unlock()
		}

		popped := 0
		for i := 0; i < pops; i++ {
			mu.Lock()
			_, ok := q.popLocked()
			mu.Unlock()
			if ok {
				popped++
			}
		}

		mu.Lock()
		length := q.Length()
		mu.Unlock()

		expectedLen := pushes - popped
		if length != expectedLen {
			t.Fatalf("Invariant violation: Pushed %d, Popped %d. Expected %d, got %d",
				pushes, popped, expectedLen, length)
		}

		for length > 0 {
			mu.Lock()
			_, ok := q.popLocked()
			mu.Unlock()
			if !ok {
				t.Fatalf("Queue reported length %d but pop failed", length)
			}
			length--
		}
	})
}
