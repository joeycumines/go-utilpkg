package ingresschunkfree

import "testing"

func FuzzQueueSequentialModel(f *testing.F) {
	f.Add(64, []byte{0, 0, 1, 0, 1, 1})
	f.Add(1, []byte{1, 0, 1, 0, 1})
	f.Fuzz(func(t *testing.T, chunkSize int, operations []byte) {
		if chunkSize < -128 || chunkSize > 128 {
			return
		}
		queue := NewNative(chunkSize)
		model := make([]uint64, 0)
		var next uint64
		for _, operation := range operations {
			if operation&1 == 0 {
				next++
				value := next
				queue.Push(func() { next = value })
				model = append(model, value)
			} else {
				fn, ok := queue.Pop()
				if len(model) == 0 {
					if ok || fn != nil {
						t.Fatalf("empty pop = (nil %v, ok %v)", fn == nil, ok)
					}
				} else {
					if !ok || fn == nil {
						t.Fatalf("nonempty pop = (nil %v, ok %v)", fn == nil, ok)
					}
					fn()
					if next != model[0] {
						t.Fatalf("FIFO value = %d, want %d", next, model[0])
					}
					model = model[1:]
				}
			}
			if queue.Length() != len(model) {
				t.Fatalf("length = %d, want %d", queue.Length(), len(model))
			}
		}
	})
}
