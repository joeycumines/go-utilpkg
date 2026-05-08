package timerbucketretire

import (
	"testing"
	"time"
)

func assertQueueInvariant(t testing.TB, queue *Queue) {
	t.Helper()
	if len(queue.timers) != len(queue.lists) {
		t.Fatalf("heap/list map lengths = (%d, %d)", len(queue.timers), len(queue.lists))
	}
	seenLists := make(map[*timerList]struct{}, len(queue.timers))
	seenEntries := make(map[*timer]struct{}, len(queue.entries))
	for index, list := range queue.timers {
		if list == nil || list.heapIndex != index {
			t.Fatalf("heap list %d = %+v", index, list)
		}
		if queue.lists[list.key] != list {
			t.Fatalf("heap list %d is not map owner for key %d", index, list.key)
		}
		if _, duplicate := seenLists[list]; duplicate {
			t.Fatalf("duplicate heap list %p", list)
		}
		seenLists[list] = struct{}{}
		if index > 0 {
			parent := (index - 1) / 2
			if queue.timers.Less(index, parent) {
				t.Fatalf("heap child %d sorts before parent %d", index, parent)
			}
		}
		if list.head == nil || list.tail == nil || list.len <= 0 || list.head.prev != nil || list.tail.next != nil || !list.deadline.Equal(list.head.when) {
			t.Fatalf("invalid list anchors for key %d: %+v", list.key, list)
		}
		count := 0
		var previous *timer
		for entry := list.head; entry != nil; entry = entry.next {
			count++
			if entry.list != list || entry.prev != previous || (previous != nil && previous.next != entry) {
				t.Fatalf("invalid node links for key %d at node %p", list.key, entry)
			}
			if previous != nil && timerNodeBefore(entry, previous) {
				t.Fatalf("list key %d is out of order", list.key)
			}
			if queue.entries[entry.id] != entry {
				t.Fatalf("node %d is not its map owner", entry.id)
			}
			if _, duplicate := seenEntries[entry]; duplicate {
				t.Fatalf("duplicate node %p", entry)
			}
			seenEntries[entry] = struct{}{}
			previous = entry
		}
		if count != list.len || previous != list.tail {
			t.Fatalf("list key %d traversal = (%d, %p), want (%d, %p)", list.key, count, previous, list.len, list.tail)
		}
	}
	if len(seenEntries) != len(queue.entries) {
		t.Fatalf("list/map entry counts = (%d, %d)", len(seenEntries), len(queue.entries))
	}
	for id, entry := range queue.entries {
		if entry == nil || entry.id != id {
			t.Fatalf("map entry %d = %+v", id, entry)
		}
		if _, ok := seenEntries[entry]; !ok {
			t.Fatalf("map entry %d is absent from lists", id)
		}
	}
	stats := queue.Stats()
	if stats.Active != len(queue.entries) || stats.MapEntries != len(queue.entries) || stats.HeapLists != len(queue.timers) || stats.ListEntries != len(queue.lists) {
		t.Fatalf("Stats disagree with structure: %+v", stats)
	}
}

func TestQueueCancellationRepairsIntrusiveListsAndHeap(t *testing.T) {
	epoch := time.Unix(1_700_000_000, 0)
	queue := NewNative(epoch)
	var same [4]Handle
	for index, micros := range []int{100, 200, 300, 400} {
		handle, err := queue.Insert(InsertInput{When: epoch.Add(time.Duration(micros) * time.Microsecond)})
		if err != nil {
			t.Fatal(err)
		}
		same[index] = handle
	}
	assertQueueInvariant(t, queue)
	for _, handle := range []Handle{same[0], same[2], same[3], same[1]} {
		if err := queue.Cancel(handle); err != nil {
			t.Fatal(err)
		}
		assertQueueInvariant(t, queue)
	}

	var distinct [5]Handle
	for index, millis := range []int{5, 1, 4, 2, 3} {
		handle, err := queue.Insert(InsertInput{When: epoch.Add(time.Duration(millis) * time.Millisecond)})
		if err != nil {
			t.Fatal(err)
		}
		distinct[index] = handle
	}
	assertQueueInvariant(t, queue)
	for _, handle := range []Handle{distinct[1], distinct[2], distinct[4], distinct[0], distinct[3]} {
		if err := queue.Cancel(handle); err != nil {
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
		queue := NewNative(epoch)
		var handles []Handle
		for step, operation := range operations {
			switch operation % 4 {
			case 0:
				handle, err := queue.Insert(InsertInput{
					When:         epoch.Add(time.Duration(operation%11) * 100 * time.Microsecond),
					EarliestTick: uint64(operation % 3),
				})
				if err != nil {
					t.Fatal(err)
				}
				handles = append(handles, handle)
			case 1:
				if len(handles) != 0 {
					_ = queue.Cancel(handles[int(operation)%len(handles)])
				}
			case 2:
				queue.BatchDrain(DrainInput{
					Now:  epoch.Add(time.Duration(step%12) * 100 * time.Microsecond),
					Tick: uint64(step % 4),
				})
			case 3:
				if len(handles) != 0 {
					_ = queue.Cancel(handles[len(handles)-1])
				}
			}
			assertQueueInvariant(t, queue)
		}
	})
}
