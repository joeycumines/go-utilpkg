package alternatethree

import (
	"slices"
	"sync"
	"testing"
)

type lockedIngressQueue struct {
	mu    sync.Mutex
	queue IngressQueue
}

func (q *lockedIngressQueue) push(task Task) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.queue.Push(task)
}

func (q *lockedIngressQueue) pop() (Task, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.queue.popLocked()
}

func (q *lockedIngressQueue) length() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.queue.Length()
}

func TestIngressPop_MultiChunkFIFO(t *testing.T) {
	var queue lockedIngressQueue
	const taskCount = 256
	got := make([]int, 0, taskCount)
	for value := range taskCount {
		queue.push(Task{Runnable: func() { got = append(got, value) }})
	}
	if length := queue.length(); length != taskCount {
		t.Fatalf("queue length = %d, want %d", length, taskCount)
	}
	for index := range taskCount {
		task, ok := queue.pop()
		if !ok || task.Runnable == nil {
			t.Fatalf("pop %d failed", index)
		}
		task.Runnable()
	}
	want := make([]int, taskCount)
	for index := range want {
		want[index] = index
	}
	if !slices.Equal(got, want) {
		t.Errorf("execution order = %v, want FIFO sequence", got)
	}
	if length := queue.length(); length != 0 {
		t.Errorf("queue length after drain = %d, want 0", length)
	}
}

func TestIngressPop_ResetCursorsOnEmpty(t *testing.T) {
	var queue lockedIngressQueue
	executed := false
	queue.push(Task{Runnable: func() { executed = true }})
	task, ok := queue.pop()
	if !ok || task.Runnable == nil {
		t.Fatal("single-task pop failed")
	}
	task.Runnable()
	if !executed {
		t.Error("task did not execute")
	}
	queue.mu.Lock()
	defer queue.mu.Unlock()
	if queue.queue.length != 0 || queue.queue.head == nil || queue.queue.head != queue.queue.tail {
		t.Errorf("empty topology = length %d, head %p, tail %p; want one reusable empty chunk", queue.queue.length, queue.queue.head, queue.queue.tail)
	}
	if queue.queue.head.readPos != 0 || queue.queue.head.pos != 0 {
		t.Errorf("empty cursors = (%d, %d), want (0, 0)", queue.queue.head.readPos, queue.queue.head.pos)
	}
}

func TestIngressPop_ThreeChunkFIFO(t *testing.T) {
	var queue lockedIngressQueue
	const taskCount = 300
	got := make([]int, 0, taskCount)
	for value := range taskCount {
		queue.push(Task{Runnable: func() { got = append(got, value) }})
	}
	for index := range taskCount {
		task, ok := queue.pop()
		if !ok || task.Runnable == nil {
			t.Fatalf("pop %d failed", index)
		}
		task.Runnable()
	}
	for index, value := range got {
		if value != index {
			t.Fatalf("execution[%d] = %d, want %d", index, value, index)
		}
	}
}

func TestIngressPop_ChunkAdvancementAndTailReuse(t *testing.T) {
	var queue lockedIngressQueue
	const (
		batchCount = 2
		batchSize  = 256
	)
	for batch := range batchCount {
		var executed int
		for range batchSize {
			queue.push(Task{Runnable: func() { executed++ }})
		}
		for index := range batchSize {
			task, ok := queue.pop()
			if !ok || task.Runnable == nil {
				t.Fatalf("batch %d pop %d failed", batch, index)
			}
			task.Runnable()
		}
		if executed != batchSize {
			t.Errorf("batch %d executions = %d, want %d", batch, executed, batchSize)
		}
		queue.mu.Lock()
		if queue.queue.length != 0 || queue.queue.head == nil || queue.queue.head != queue.queue.tail || queue.queue.head.pos != 0 || queue.queue.head.readPos != 0 {
			t.Errorf("batch %d did not leave one reusable empty tail", batch)
		}
		queue.mu.Unlock()
	}
}

func TestIngressPop_ConcurrentSafeWithExternalLock(t *testing.T) {
	var queue lockedIngressQueue
	const (
		producerCount    = 4
		tasksPerProducer = 64
		total            = producerCount * tasksPerProducer
	)
	seen := make([]bool, total)
	producerDone := make(chan struct{}, producerCount)
	for producer := range producerCount {
		go func() {
			for offset := range tasksPerProducer {
				value := producer*tasksPerProducer + offset
				queue.push(Task{Runnable: func() { seen[value] = true }})
			}
			producerDone <- struct{}{}
		}()
	}
	for range producerCount {
		waitAlternateThreeSignal(t, producerDone, "ingress producer")
	}
	for index := range total {
		task, ok := queue.pop()
		if !ok || task.Runnable == nil {
			t.Fatalf("pop %d failed", index)
		}
		task.Runnable()
	}
	for value, observed := range seen {
		if !observed {
			t.Errorf("task value %d did not execute", value)
		}
	}
	if length := queue.length(); length != 0 {
		t.Errorf("queue length after concurrent drain = %d, want 0", length)
	}
}
