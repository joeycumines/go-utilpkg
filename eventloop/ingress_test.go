package eventloop

import (
	"sync"
	"sync/atomic"
	"testing"
)

// Test_chunkedIngress_ChunkTransition verifies the chunkedIngress queue correctly handles
// chunk boundary transitions during push/pop operations.
func Test_chunkedIngress_ChunkTransition(t *testing.T) {
	q := newChunkedIngress()

	const chunkSize = 128
	const cycles = 3
	total := chunkSize * cycles

	// Push total tasks
	for range total {
		q.Push(func() {})
	}

	if q.Length() != total {
		t.Fatalf("Queue length mismatch. Expected %d, got %d", total, q.Length())
	}

	// Pop all tasks
	for i := range total {
		task, ok := q.Pop()
		if !ok {
			t.Fatalf("Premature exhaustion at index %d", i)
		}
		if task == nil {
			t.Fatalf("Zero-value task at index %d", i)
		}
	}

	// Verify empty
	_, ok := q.Pop()
	if ok {
		t.Fatal("Queue should be empty")
	}
}

// Test_chunkedIngress_ConcurrentPushPop verifies correct behavior under concurrent access.
func Test_chunkedIngress_ConcurrentPushPop(t *testing.T) {
	q := newChunkedIngress()

	const tasks = 10000
	const producers = 8

	var counter atomic.Int64
	var wg sync.WaitGroup
	var mu sync.Mutex // Required: External synchronization for concurrent Push calls

	// Producer goroutines - concurrent pushes
	for i := range producers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for range tasks / producers {
				mu.Lock()
				q.Push(func() {
					counter.Add(1)
				})
				mu.Unlock()
			}
		}(i)
	}

	// Wait for all producers to finish
	wg.Wait()

	// Verify length
	if q.Length() != tasks {
		t.Fatalf("Queue length mismatch. Expected %d, got %d", tasks, q.Length())
	}

	// Pop all tasks
	var received int64
	for received < tasks {
		task, ok := q.Pop()
		if !ok {
			t.Fatalf("Premature exhaustion at received=%d", received)
		}
		task()
		received++
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

// Test_chunkedIngress_PushPopIntegrity verifies sequential push/pop integrity without race.
func Test_chunkedIngress_PushPopIntegrity(t *testing.T) {
	q := newChunkedIngress()

	const count = 1000

	// Push tasks
	for range count {
		q.Push(func() {})
	}

	if q.Length() != count {
		t.Fatalf("Length mismatch: expected %d, got %d", count, q.Length())
	}

	// Pop all tasks
	var popped int
	for i := range count {
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

// Test_chunkedIngress_StressNoTaskLoss performs aggressive stress testing to verify
// absolutely no task loss under extreme producer contention.
func Test_chunkedIngress_StressNoTaskLoss(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	q := newChunkedIngress()

	const tasks = 100000 // 100k tasks for aggressive stress
	const producers = 32 // 32 producers for maximum contention

	var counter atomic.Int64
	var wg sync.WaitGroup
	var mu sync.Mutex // Required: External synchronization for concurrent Push calls

	// Producer goroutines - extreme contention
	for i := range producers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for range tasks / producers {
				mu.Lock()
				q.Push(func() {
					counter.Add(1)
				})
				mu.Unlock()
			}
		}(i)
	}

	// Wait for producers
	wg.Wait()

	// Verify length
	if q.Length() != tasks {
		t.Fatalf("Queue length mismatch after push. Expected %d, got %d", tasks, q.Length())
	}

	// Pop all tasks
	var received int64
	for received < tasks {
		task, ok := q.Pop()
		if !ok {
			t.Fatalf("Premature exhaustion at received=%d", received)
		}
		task()
		received++
	}

	// CRITICAL: Verify ZERO task loss
	if counter.Load() != int64(tasks) {
		t.Fatalf("TASK LOSS DETECTED! Expected %d tasks, executed %d. Loss: %d tasks",
			tasks, counter.Load(), tasks-int(counter.Load()))
	}

	// Verify queue is truly empty
	for range 100 { // Check multiple times to be certain
		if _, ok := q.Pop(); ok {
			t.Fatal("Queue should be empty after processing all tasks")
		}
	}

	t.Logf("Stress test passed: %d tasks processed with 0 loss across %d producers",
		tasks, producers)
}

// Test_chunkedIngress_IsEmpty verifies Length() == 0 behavior.
func Test_chunkedIngress_IsEmpty(t *testing.T) {
	q := newChunkedIngress()

	if q.Length() != 0 {
		t.Fatal("New queue should be empty")
	}

	q.Push(func() {})

	if q.Length() == 0 {
		t.Fatal("Queue with one item should not be empty")
	}

	q.Pop()

	if q.Length() != 0 {
		t.Fatal("Queue after pop should be empty")
	}
}
