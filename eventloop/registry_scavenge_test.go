package eventloop

import (
	"errors"
	"runtime"
	"testing"
)

func TestRegistryScavengeBatchesSettlement(t *testing.T) {
	r := newRegistry()
	ids := make([]uint64, 6)
	promises := make([]*promise, len(ids))
	for i := range ids {
		ids[i], promises[i] = r.NewPromise()
	}
	promises[1].resolve("fulfilled first batch")
	promises[2].reject(errors.New("rejected first batch"))
	promises[4].resolve("fulfilled second batch")

	r.Scavenge(3)

	r.mu.RLock()
	firstData := make(map[uint64]bool, len(r.data))
	for id := range r.data {
		firstData[id] = true
	}
	firstRing := append([]uint64(nil), r.ring...)
	firstHead := r.head
	r.mu.RUnlock()

	if firstHead != 3 {
		t.Fatalf("head after first batch = %d, want 3", firstHead)
	}
	if len(firstData) != 4 || !firstData[ids[0]] || !firstData[ids[3]] || !firstData[ids[4]] || !firstData[ids[5]] {
		t.Fatalf("data IDs after first batch = %v, want pending or unvisited IDs %v", firstData, []uint64{ids[0], ids[3], ids[4], ids[5]})
	}
	if len(firstRing) != 6 || firstRing[0] != ids[0] || firstRing[1] != 0 || firstRing[2] != 0 {
		t.Fatalf("ring after first batch = %v, want first ID followed by two null markers", firstRing)
	}

	r.Scavenge(3)

	r.mu.RLock()
	secondData := make(map[uint64]bool, len(r.data))
	for id := range r.data {
		secondData[id] = true
	}
	secondRing := append([]uint64(nil), r.ring...)
	secondHead := r.head
	r.mu.RUnlock()

	if secondHead != 0 {
		t.Fatalf("head after complete cycle = %d, want 0", secondHead)
	}
	if len(secondData) != 3 || !secondData[ids[0]] || !secondData[ids[3]] || !secondData[ids[5]] {
		t.Fatalf("data IDs after complete cycle = %v, want pending IDs %v", secondData, []uint64{ids[0], ids[3], ids[5]})
	}
	if len(secondRing) != 6 || secondRing[0] != ids[0] || secondRing[1] != 0 || secondRing[2] != 0 || secondRing[3] != ids[3] || secondRing[4] != 0 || secondRing[5] != ids[5] {
		t.Fatalf("ring after complete cycle = %v, want exact settled markers", secondRing)
	}
	runtime.KeepAlive(promises)
}

func TestRegistryScavengeNonPositiveBatchDoesNotAdvance(t *testing.T) {
	r := newRegistry()
	id, promise := r.NewPromise()

	r.Scavenge(0)
	r.Scavenge(-1)

	r.mu.RLock()
	registered := r.data[id].Value()
	ring := append([]uint64(nil), r.ring...)
	head := r.head
	r.mu.RUnlock()
	if registered != promise || len(ring) != 1 || ring[0] != id || head != 0 {
		t.Fatalf("registry after non-positive batches = (promise %p, ring %v, head %d), want (%p, [%d], 0)", registered, ring, head, promise, id)
	}
}

func TestRegistryScavengeEmptyDoesNotMutate(t *testing.T) {
	r := newRegistry()
	r.Scavenge(100)

	r.mu.RLock()
	dataLen := len(r.data)
	ringLen := len(r.ring)
	head := r.head
	nextID := r.nextID
	r.mu.RUnlock()
	if dataLen != 0 || ringLen != 0 || head != 0 || nextID != 1 {
		t.Fatalf("empty registry after scavenge = (data %d, ring %d, head %d, next ID %d), want (0, 0, 0, 1)", dataLen, ringLen, head, nextID)
	}
}

func TestRegistryScavengeConcurrentExactState(t *testing.T) {
	const (
		promiseCount = 1000
		workerCount  = 8
		passes       = 25
		batchSize    = 5
	)

	r := newRegistry()
	settledIDs := make([]uint64, 0, promiseCount/2)
	pending := make(map[uint64]*promise, promiseCount/2)
	for i := range promiseCount {
		id, promise := r.NewPromise()
		if i%2 == 0 {
			promise.resolve(nil)
			settledIDs = append(settledIDs, id)
		} else {
			pending[id] = promise
		}
	}

	done := make(chan struct{}, workerCount)
	for range workerCount {
		go func() {
			for range passes {
				r.Scavenge(batchSize)
			}
			done <- struct{}{}
		}()
	}
	for range workerCount {
		waitContractSignal(t, done, "concurrent registry scavenge")
	}

	r.mu.RLock()
	if len(r.data) != len(pending) || len(r.ring) != promiseCount || r.head != 0 {
		t.Fatalf("registry after concurrent full cycle = (data %d, ring %d, head %d), want (%d, %d, 0)", len(r.data), len(r.ring), r.head, len(pending), promiseCount)
	}
	for id, promise := range pending {
		if got := r.data[id].Value(); got != promise {
			t.Errorf("pending registry ID %d points to %p, want %p", id, got, promise)
		}
	}
	for _, id := range settledIDs {
		if _, exists := r.data[id]; exists {
			t.Errorf("settled registry ID %d survived concurrent scavenge", id)
		}
	}
	r.mu.RUnlock()
	runtime.KeepAlive(pending)
}
