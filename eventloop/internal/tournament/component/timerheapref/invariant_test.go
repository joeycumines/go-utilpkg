package timerheapref

import (
	"testing"
	"time"
)

func assertQueueInvariant(t testing.TB, queue *Queue) {
	t.Helper()
	if len(queue.timers) != len(queue.entries) {
		t.Fatalf("heap/map lengths = (%d, %d)", len(queue.timers), len(queue.entries))
	}
	seen := make(map[*timer]struct{}, len(queue.timers))
	for index, entry := range queue.timers {
		if entry == nil || entry.heapIndex != index || queue.entries[entry.id] != entry {
			t.Fatalf("heap entry %d = %+v", index, entry)
		}
		if _, duplicate := seen[entry]; duplicate {
			t.Fatalf("duplicate heap entry %p", entry)
		}
		seen[entry] = struct{}{}
		if index > 0 {
			parent := (index - 1) / 2
			if queue.timers.Less(index, parent) {
				t.Fatalf("heap child %d sorts before parent %d", index, parent)
			}
		}
	}
	for id, entry := range queue.entries {
		if entry == nil || entry.id != id {
			t.Fatalf("map entry %d = %+v", id, entry)
		}
		if _, ok := seen[entry]; !ok {
			t.Fatalf("map entry %d is absent from heap", id)
		}
	}
	stats := queue.Stats()
	if stats.HeapActive != len(queue.timers) || stats.MapEntries != len(queue.entries) {
		t.Fatalf("Stats disagree with structure: %+v", stats)
	}
}

func TestQueueCancellationRepairsHeapIndices(t *testing.T) {
	epoch := time.Unix(1_700_000_000, 0)
	queue := NewNative()
	var handles [9]Handle
	for index, micros := range []int{900, 100, 700, 300, 800, 200, 600, 400, 500} {
		handle, err := queue.Insert(InsertInput{When: epoch.Add(time.Duration(micros) * time.Microsecond)})
		if err != nil {
			t.Fatal(err)
		}
		handles[index] = handle
		assertQueueInvariant(t, queue)
	}
	for _, index := range []int{1, 4, 8, 0, 5, 2, 7, 3, 6} {
		if err := queue.Cancel(handles[index]); err != nil {
			t.Fatal(err)
		}
		assertQueueInvariant(t, queue)
	}
}

func FuzzQueueStructuralModel(f *testing.F) {
	f.Add([]byte{0, 1, 2, 0, 3, 1, 2, 2, 3, 3})
	f.Add([]byte{0, 0, 0, 1, 1, 2, 3, 2, 1, 3})
	f.Fuzz(func(t *testing.T, operations []byte) {
		if len(operations) > 128 {
			operations = operations[:128]
		}
		epoch := time.Unix(1_700_000_000, 0)
		queue := NewNative()
		var handles []Handle
		for step, operation := range operations {
			switch operation % 4 {
			case 0:
				handle, err := queue.Insert(InsertInput{When: epoch.Add(time.Duration(operation%11) * 100 * time.Microsecond)})
				if err != nil {
					t.Fatal(err)
				}
				handles = append(handles, handle)
			case 1:
				if len(handles) != 0 {
					_ = queue.Cancel(handles[int(operation)%len(handles)])
				}
			case 2:
				queue.BatchDrain(DrainInput{Now: epoch.Add(time.Duration(step%12) * 100 * time.Microsecond)})
			case 3:
				if len(handles) != 0 {
					_ = queue.Cancel(handles[len(handles)-1])
				}
			}
			assertQueueInvariant(t, queue)
		}
	})
}
