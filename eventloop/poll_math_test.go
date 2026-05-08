package eventloop

import (
	"testing"
	"time"
)

func TestCalculateTimeoutWithoutDeadlineUsesIndefiniteSentinel(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)
	if got := loop.calculateTimeout(); got != -1 {
		t.Fatalf("calculateTimeout without timer = %d, want -1", got)
	}
}

func TestPollTimeoutMillis(t *testing.T) {
	tests := []struct {
		name  string
		delay time.Duration
		want  int
	}{
		{name: "expired", delay: -time.Second, want: 0},
		{name: "zero", want: 0},
		{name: "nanosecond", delay: time.Nanosecond, want: 1},
		{name: "half millisecond", delay: 500 * time.Microsecond, want: 1},
		{name: "millisecond", delay: time.Millisecond, want: 1},
		{name: "fifty milliseconds", delay: 50 * time.Millisecond, want: 50},
		{name: "finite cap", delay: time.Duration(maxFinitePollTimeoutMs+1) * time.Millisecond, want: maxFinitePollTimeoutMs},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := pollTimeoutMillis(test.delay); got != test.want {
				t.Fatalf("pollTimeoutMillis(%v) = %d, want %d", test.delay, got, test.want)
			}
		})
	}
}

func TestCalculateTimeoutRefreshesStaleLoopClock(t *testing.T) {
	l := New()
	t.Cleanup(l.closeFDs)

	anchor := time.Now().Add(-time.Hour)
	l.setTickAnchor(anchor)
	l.tickNow = anchor
	l.state.Store(StateRunning)
	resetTestTimerLists(l)
	pushTestTimer(l, &timer{when: anchor.Add(500 * time.Microsecond)})

	if got := l.calculateTimeout(); got != 0 {
		t.Fatalf("calculateTimeout with expired stale clock = %dms, want 0ms", got)
	}
	if !l.tickNow.After(anchor) {
		t.Fatalf("calculateTimeout left stale tick time %v at anchor %v", l.tickNow, anchor)
	}
}

func TestRunTimersRefreshesCachedClockAtEntry(t *testing.T) {
	l := New()
	t.Cleanup(l.closeFDs)

	anchor := time.Now().Add(-time.Hour)
	l.setTickAnchor(anchor)
	l.tickNow = anchor
	l.state.Store(StateRunning)
	l.tickCount = 1
	resetTestTimerLists(l)

	fired := false
	tm := &timer{
		id:        1,
		when:      time.Now().Add(-time.Millisecond),
		heapIndex: -1,
		task:      func() { fired = true },
	}
	tm.refed.Store(true)
	l.timerMap[tm.id] = tm
	l.refedTimerCount.Add(1)
	pushTestTimer(l, tm)

	l.runTimers()
	if !fired {
		t.Fatal("runTimers did not refresh the cached loop clock before testing due timers")
	}
}
