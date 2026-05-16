package eventloop

import (
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAllocateIDConcurrentExhaustionSaturates(t *testing.T) {
	const (
		limit   = uint64(17)
		workers = 32
	)
	var counter atomic.Uint64
	counter.Store(limit - 1)
	start := make(chan struct{})
	var successes atomic.Int32
	var wrongID atomic.Bool
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			<-start
			id, ok := allocateID(&counter, limit)
			if !ok {
				return
			}
			if id != limit {
				wrongID.Store(true)
			}
			successes.Add(1)
		})
	}
	close(start)
	joined := make(chan struct{})
	go func() {
		wg.Wait()
		close(joined)
	}()
	waitContractSignal(t, joined, "concurrent timer ID exhaustion")

	if wrongID.Load() {
		t.Fatal("allocator published an ID other than the final namespace value")
	}
	if got := successes.Load(); got != 1 {
		t.Fatalf("successful final allocations = %d, want 1", got)
	}
	if got := counter.Load(); got != limit {
		t.Fatalf("counter after concurrent exhaustion = %d, want %d", got, limit)
	}
	if id, ok := allocateID(&counter, limit); ok || id != 0 {
		t.Fatalf("allocation after exhaustion = (%d, %t), want (0, false)", id, ok)
	}
}

func TestScheduleTimerIDIncrement(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	const timerCount = 100
	var previous TimerID
	for index := range timerCount {
		id, err := loop.ScheduleTimer(time.Hour, func() {})
		if err != nil {
			t.Fatalf("ScheduleTimer %d: %v", index, err)
		}
		if id <= previous {
			t.Fatalf("timer ID %d = %d, want greater than %d", index, id, previous)
		}
		previous = id
	}
}

func TestScheduleTimerIDExhaustion(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	loop.nextTimerID.Store(math.MaxUint64 - 1)

	id, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil || id != TimerID(math.MaxUint64) {
		t.Fatalf("last timer allocation = (%d, %v), want (%d, nil)", id, err, uint64(math.MaxUint64))
	}
	for attempt := range 2 {
		id, err := loop.ScheduleTimer(time.Hour, func() {})
		if id != 0 || err != ErrTimerIDExhausted {
			t.Errorf("exhausted allocation %d = (%d, %v), want (0, %v)", attempt, id, err, ErrTimerIDExhausted)
		}
		if got := loop.nextTimerID.Load(); got != math.MaxUint64 {
			t.Fatalf("counter after exhaustion = %d, want %d", got, uint64(math.MaxUint64))
		}
	}
}
