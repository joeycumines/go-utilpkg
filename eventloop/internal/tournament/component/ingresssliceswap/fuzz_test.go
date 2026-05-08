package ingresssliceswap

import "testing"

func FuzzQueueSequentialModel(f *testing.F) {
	f.Add([]byte{0, 0, 1, 2, 0, 1, 2})
	f.Add([]byte{2, 2, 0, 0, 1})
	f.Fuzz(func(t *testing.T, operations []byte) {
		var queue Queue
		model := make([]uint64, 0)
		batches := make([][]func(), 0)
		var next uint64
		for _, operation := range operations {
			switch operation % 3 {
			case 0:
				next++
				value := next
				queue.Push(func() { next = value })
				model = append(model, value)
			case 1:
				batch := queue.Take()
				if len(batch) != len(model) {
					t.Fatalf("batch length = %d, want %d", len(batch), len(model))
				}
				for index, fn := range batch {
					before := next
					fn()
					if next != model[index] {
						t.Fatalf("batch[%d] = %d, want %d (before %d)", index, next, model[index], before)
					}
				}
				model = model[:0]
				batches = append(batches, batch)
			case 2:
				if len(batches) != 0 {
					queue.Recycle(batches[0])
					batches = batches[1:]
				}
			}
			if queue.Length() != len(model) {
				t.Fatalf("length = %d, want %d", queue.Length(), len(model))
			}
		}
	})
}
