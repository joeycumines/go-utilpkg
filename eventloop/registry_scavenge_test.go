package eventloop

import (
	"errors"
	"runtime"
	"testing"
	"weak"
)

func TestRegistryScavengeBatchesSettlement(t *testing.T) {
	r := newRegistry()
	wps := make([]weak.Pointer[promise], 6)
	promises := make([]*promise, len(wps))
	for i := range wps {
		p := r.NewPromise()
		wps[i] = weak.Make(p)
		promises[i] = p
	}
	promises[1].resolve("fulfilled first batch")
	promises[2].reject(errors.New("rejected first batch"))
	promises[4].resolve("fulfilled second batch")

	r.Scavenge(3)

	r.mu.RLock()
	firstData := make(map[weak.Pointer[promise]]bool, len(r.data))
	for wp := range r.data {
		firstData[wp] = true
	}
	firstRing := append([]weak.Pointer[promise](nil), r.ring...)
	firstHead := r.head
	r.mu.RUnlock()

	if firstHead != 3 {
		t.Fatalf("head after first batch = %d, want 3", firstHead)
	}
	if len(firstData) != 4 || !firstData[wps[0]] || !firstData[wps[3]] || !firstData[wps[4]] || !firstData[wps[5]] {
		t.Fatalf("data after first batch = %v, want pending or unvisited", firstData)
	}
	if len(firstRing) != 6 || firstRing[0] != wps[0] || firstRing[1] != (weak.Pointer[promise]{}) || firstRing[2] != (weak.Pointer[promise]{}) {
		t.Fatalf("ring after first batch = %v, want first wp followed by two zero markers", firstRing)
	}

	r.Scavenge(3)

	r.mu.RLock()
	secondData := make(map[weak.Pointer[promise]]bool, len(r.data))
	for wp := range r.data {
		secondData[wp] = true
	}
	secondRing := append([]weak.Pointer[promise](nil), r.ring...)
	secondHead := r.head
	r.mu.RUnlock()

	if secondHead != 0 {
		t.Fatalf("head after complete cycle = %d, want 0", secondHead)
	}
	if len(secondData) != 3 || !secondData[wps[0]] || !secondData[wps[3]] || !secondData[wps[5]] {
		t.Fatalf("data after complete cycle = %v, want pending", secondData)
	}
	if len(secondRing) != 6 || secondRing[0] != wps[0] || secondRing[1] != (weak.Pointer[promise]{}) || secondRing[2] != (weak.Pointer[promise]{}) || secondRing[3] != wps[3] || secondRing[4] != (weak.Pointer[promise]{}) || secondRing[5] != wps[5] {
		t.Fatalf("ring after complete cycle = %v, want exact settled markers", secondRing)
	}
	runtime.KeepAlive(promises)
}

func TestRegistryScavengeNonPositiveBatchDoesNotAdvance(t *testing.T) {
	r := newRegistry()
	p := r.NewPromise()
	wp := weak.Make(p)

	r.Scavenge(0)
	r.Scavenge(-1)

	r.mu.RLock()
	_, exists := r.data[wp]
	ring := append([]weak.Pointer[promise](nil), r.ring...)
	head := r.head
	r.mu.RUnlock()
	if !exists || len(ring) != 1 || ring[0] != wp || head != 0 {
		t.Fatalf("registry after non-positive batches = (exists %v, ring %v, head %d), want (true, [%v], 0)", exists, ring, head, wp)
	}
	runtime.KeepAlive(p)
}

func TestRegistryScavengeEmptyDoesNotMutate(t *testing.T) {
	r := newRegistry()
	r.Scavenge(100)

	r.mu.RLock()
	dataLen := len(r.data)
	ringLen := len(r.ring)
	head := r.head
	r.mu.RUnlock()
	if dataLen != 0 || ringLen != 0 || head != 0 {
		t.Fatalf("empty registry after scavenge = (data %d, ring %d, head %d), want (0, 0, 0)", dataLen, ringLen, head)
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
	settledWPs := make([]weak.Pointer[promise], 0, promiseCount/2)
	pending := make(map[weak.Pointer[promise]]*promise, promiseCount/2)
	var allWPs []weak.Pointer[promise]
	for i := range promiseCount {
		promise := r.NewPromise()
		wp := weak.Make(promise)
		allWPs = append(allWPs, wp)
		if i%2 == 0 {
			promise.resolve(nil)
			settledWPs = append(settledWPs, wp)
		} else {
			pending[wp] = promise
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
	for wp, promise := range pending {
		if _, ok := r.data[wp]; !ok {
			t.Errorf("pending registry wp %v missing", wp)
		}
		if wp.Value() != promise {
			t.Errorf("pending wp %v points to %p, want %p", wp, wp.Value(), promise)
		}
	}
	for _, wp := range settledWPs {
		if _, exists := r.data[wp]; exists {
			t.Errorf("settled registry wp %v survived concurrent scavenge", wp)
		}
	}
	r.mu.RUnlock()
	runtime.KeepAlive(pending)
	runtime.KeepAlive(allWPs)
}
