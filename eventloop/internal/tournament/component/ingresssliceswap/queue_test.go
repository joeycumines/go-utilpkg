package ingresssliceswap

import "testing"

func TestQueueSnapshotRecycle(t *testing.T) {
	var queue Queue
	if queue.Length() != 0 || queue.Take() != nil {
		t.Fatal("zero queue is not empty")
	}

	order := make([]int, 0, 3)
	queue.Push(func() { order = append(order, 1) })
	queue.Push(nil)
	queue.Push(func() { order = append(order, 3) })
	if queue.Length() != 3 {
		t.Fatalf("length = %d, want 3", queue.Length())
	}
	batch := queue.Take()
	if queue.Length() != 0 || len(batch) != 3 || batch[1] != nil {
		t.Fatalf("snapshot = (remaining %d, batch %d, nil %v)", queue.Length(), len(batch), batch[1] == nil)
	}
	batch[0]()
	batch[2]()
	if len(order) != 2 || order[0] != 1 || order[1] != 3 {
		t.Fatalf("execution order = %v, want [1 3]", order)
	}

	queue.Push(func() { order = append(order, 4) })
	queue.Recycle(batch)
	for index, fn := range batch {
		if fn != nil {
			t.Errorf("recycled callback %d retained", index)
		}
	}
	next := queue.Take()
	if len(next) != 1 {
		t.Fatalf("next snapshot = %d, want 1", len(next))
	}
	next[0]()
	queue.Recycle(next)
	if queue.Length() != 0 || len(queue.spare) != 0 || cap(queue.spare) == 0 {
		t.Fatalf("recycled queue = (length %d, spare %d/%d)", queue.Length(), len(queue.spare), cap(queue.spare))
	}
	if len(order) != 3 || order[2] != 4 {
		t.Fatalf("final execution order = %v, want [1 3 4]", order)
	}
}

func TestQueuePhaseSnapshot(t *testing.T) {
	var queue Queue
	queue.Push(func() {})
	first := queue.Take()
	queue.Push(func() {})
	queue.Push(func() {})
	if len(first) != 1 || queue.Length() != 2 {
		t.Fatalf("phase snapshot = (first %d, next %d), want (1, 2)", len(first), queue.Length())
	}
	queue.Recycle(first)
	second := queue.Take()
	if len(second) != 2 {
		t.Fatalf("second phase snapshot = %d, want 2", len(second))
	}
	queue.Recycle(second)
}
