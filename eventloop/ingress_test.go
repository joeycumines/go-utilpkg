package eventloop

import (
	"sync/atomic"
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

// TestIngress_NilCheckSpinRetry verifies that the spin-retry logic
// prevents task loss when producer is mid-push (Swap done, next not linked).
// Test runs with -race detector to verify correctness.
func TestIngress_NilCheckSpinRetry(t *testing.T) {
	q := NewLockFreeIngress()

	const tasks = 10000
	const producers = 8

	var counter atomic.Int64

	// Producer goroutines - create contention on tail swap
	for i := 0; i < producers; i++ {
		go func(id int) {
			for j := 0; j < tasks/producers; j++ {
				val := id*tasks/producers + j
				q.Push(func() {
					counter.Add(1)
					t.Logf("Task %d executed", val)
				})
			}
		}(i)
	}

	// Consumer - single thread popping continuously
	var received int64
	for received < tasks {
		task, ok := q.Pop()
		if ok {
			task.Runnable()
			received++
		}
	}

	// Verify no task loss - counter should equal tasks
	if counter.Load() != int64(tasks) {
		t.Fatalf("Task loss detected! Expected %d tasks, executed %d",
			tasks, counter.Load())
	}

	// Verify queue is truly empty
	if _, ok := q.Pop(); ok {
		t.Fatal("Queue should be empty after processing all tasks")
	}
}

// TestIngress_PushPopIntegrity verifies sequential push/pop integrity without race.
func TestIngress_PushPopIntegrity(t *testing.T) {
	q := NewLockFreeIngress()

	const count = 1000

	// Push tasks
	for i := 0; i < count; i++ {
		q.Push(func() {})
	}

	if q.Length() != count {
		t.Fatalf("Length mismatch: expected %d, got %d", count, q.Length())
	}

	// Pop all tasks
	var popped int
	for i := 0; i < count; i++ {
		_, ok := q.Pop()
		if !ok {
			t.Fatalf("Premature empty at %d", i)
		}
		popped++
	}

	if popped != count {
		t.Fatalf("Popped count mismatch: expected %d, got %d", count, popped)
	}

	// Verify empty
	if _, ok := q.Pop(); ok {
		t.Fatal("Queue should be empty")
	}
}

// TestIngress_StressNoTaskLoss performs aggressive stress testing to verify
// absolutely no task loss under extreme producer contention.
func TestIngress_StressNoTaskLoss(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	q := NewLockFreeIngress()

	const tasks = 100000 // 100k tasks for aggressive stress
	const producers = 32 // 32 producers for maximum contention

	var counter atomic.Int64
	var done atomic.Bool

	// Producer goroutines - extreme contention
	for i := 0; i < producers; i++ {
		go func(id int) {
			for j := 0; j < tasks/producers; j++ {
				if done.Load() {
					return
				}
				q.Push(func() {
					counter.Add(1)
				})
			}
		}(i)
	}

	// Consumer - single thread popping continuously
	var received int64
	for received < tasks {
		task, ok := q.Pop()
		if ok {
			task.Runnable()
			received++
		}
	}

	done.Store(true)

	// CRITICAL: Verify ZERO task loss
	if counter.Load() != int64(tasks) {
		t.Fatalf("TASK LOSS DETECTED! Expected %d tasks, executed %d. Loss: %d tasks",
			tasks, counter.Load(), int64(tasks)-counter.Load())
	}

	// Verify queue is truly empty
	for i := 0; i < 100; i++ { // Check multiple times to be certain
		if _, ok := q.Pop(); ok {
			t.Fatal("Queue should be empty after processing all tasks")
		}
	}

	t.Logf("Stress test passed: %d tasks processed with 0 loss across %d producers",
		tasks, producers)
}
