package eventloop

import (
	"testing"
)

// TestIngress_ChunkTransition verifies the ingress queue correctly handles
// chunk boundary transitions during push/pop operations.
func TestIngress_ChunkTransition(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatal(err)
	}

	const chunkSize = 128
	const cycles = 3
	total := chunkSize * cycles

	l.ingressMu.Lock()
	for i := 0; i < total; i++ {
		l.ingress.Push(Task{Runnable: func() {}})
	}

	if l.ingress.Length() != total {
		l.ingressMu.Unlock()
		t.Fatalf("Queue length mismatch. Expected %d, got %d", total, l.ingress.Length())
	}
	l.ingressMu.Unlock()

	l.ingressMu.Lock()
	for i := 0; i < total; i++ {
		task, ok := l.ingress.popLocked()
		if !ok {
			l.ingressMu.Unlock()
			t.Fatalf("Premature exhaustion at index %d", i)
		}
		if task.Runnable == nil {
			l.ingressMu.Unlock()
			t.Fatalf("Zero-value task at index %d", i)
		}
	}

	_, ok := l.ingress.popLocked()
	if ok {
		l.ingressMu.Unlock()
		t.Fatal("Queue should be empty")
	}
	l.ingressMu.Unlock()
}
