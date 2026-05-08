package ingresschunkfree

import "testing"

func TestQueueBoundaries(t *testing.T) {
	for _, chunkSize := range []int{-1, 0, 1, 64, 65, 128} {
		for _, count := range []int{0, 1, 63, 64, 65, 127, 128, 129, 1023, 1024, 1025} {
			t.Run(testName(chunkSize, count), func(t *testing.T) {
				queue := NewNative(chunkSize)
				for value := range count {
					queue.Push(func() { value = -value - 1 })
				}
				if queue.Length() != count {
					t.Fatalf("length after push = %d, want %d", queue.Length(), count)
				}
				for value := range count {
					fn, ok := queue.Pop()
					if !ok || fn == nil {
						t.Fatalf("pop %d = (nil %v, ok %v)", value, fn == nil, ok)
					}
					fn()
				}
				if fn, ok := queue.Pop(); ok || fn != nil || queue.Length() != 0 {
					t.Fatalf("empty pop = (nil %v, ok %v), length %d", fn == nil, ok, queue.Length())
				}
			})
		}
	}
}

func TestQueueNilAndFIFO(t *testing.T) {
	queue := NewNative(2)
	order := make([]int, 0, 4)
	queue.Push(func() { order = append(order, 1) })
	queue.Push(nil)
	queue.Push(func() { order = append(order, 3) })
	queue.Push(func() { order = append(order, 4) })
	for index := range 4 {
		fn, ok := queue.Pop()
		if !ok {
			t.Fatalf("pop %d reported empty", index)
		}
		if index == 1 {
			if fn != nil {
				t.Fatal("queued nil callback changed identity")
			}
			continue
		}
		fn()
	}
	if len(order) != 3 || order[0] != 1 || order[1] != 3 || order[2] != 4 {
		t.Fatalf("FIFO order = %v, want [1 3 4]", order)
	}
}

func TestQueueBoundedFreeListAndClearing(t *testing.T) {
	queue := NewNative(defaultChunkSize)
	for range 4097 {
		queue.Push(func() {})
	}
	for range 4097 {
		if _, ok := queue.Pop(); !ok {
			t.Fatal("burst pop reported empty")
		}
	}
	if queue.freeCount != defaultFreeLimit {
		t.Fatalf("free count = %d, want %d", queue.freeCount, defaultFreeLimit)
	}
	if queue.head == nil || queue.head != queue.tail || queue.head.pos != 0 || queue.head.readPos != 0 {
		t.Fatalf("drained active chunk = head %p tail %p positions %d/%d", queue.head, queue.tail, queue.head.readPos, queue.head.pos)
	}
	for value := queue.free; value != nil; value = value.next {
		for index, fn := range value.tasks {
			if fn != nil {
				t.Fatalf("free chunk retained callback at %d", index)
			}
		}
	}
	queue.Push(func() {})
	if _, ok := queue.Pop(); !ok || queue.freeCount != defaultFreeLimit {
		t.Fatalf("steady reuse = (ok %v, free %d), want (true, %d)", ok, queue.freeCount, defaultFreeLimit)
	}
}

func TestQueueRepairsUndersizedFreeChunk(t *testing.T) {
	queue := NewNative(64)
	queue.free = &chunk{tasks: make([]func(), 1)}
	queue.freeCount = 1
	queue.Push(func() {})
	if cap(queue.head.tasks) < 64 || queue.freeCount != 0 {
		t.Fatalf("repaired chunk = capacity %d, free %d", cap(queue.head.tasks), queue.freeCount)
	}
}

func testName(chunkSize, count int) string {
	return "chunk-" + integerName(chunkSize) + "/count-" + integerName(count)
}

func integerName(value int) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	var digits [20]byte
	index := len(digits)
	for value != 0 {
		index--
		digits[index] = byte('0' + value%10)
		value /= 10
	}
	if negative {
		index--
		digits[index] = '-'
	}
	return string(digits[index:])
}
