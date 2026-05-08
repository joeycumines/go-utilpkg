package eventloop

import (
	"testing"
	"time"
)

func TestHasTimersPendingTracksDeadlineList(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)
	if loop.hasTimersPending() {
		t.Fatal("new loop reports pending timers")
	}

	pushTestTimer(loop, &timer{when: time.Now().Add(time.Hour)})
	if !loop.hasTimersPending() {
		t.Fatal("loop with a deadline-list timer does not report pending timers")
	}
	resetTestTimerLists(loop)
	if loop.hasTimersPending() {
		t.Fatal("loop reports pending timers after deadline-list reset")
	}
}
