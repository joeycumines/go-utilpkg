package eventloop

import (
	"context"
	"testing"
	"time"
)

// TestLoop_TimeFreshness verifies that the loop's tick time is fresh
// (within 10ms of real time) after poll returns.
func TestLoop_TimeFreshness(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatal(err)
	}

	driftDetected := make(chan time.Duration, 1)

	task := Task{
		Runnable: func() {
			loopTime := l.CurrentTickTime()
			realTime := time.Now()

			diff := realTime.Sub(loopTime)
			driftDetected <- diff
		},
	}

	go l.Start(context.Background())
	defer l.Stop(context.Background())

	l.Submit(task)

	drift := <-driftDetected
	if drift > 10*time.Millisecond {
		t.Errorf("Time Drift Detected! Loop time is lagging by %v. "+
			"Post-Poll time refresh is missing.", drift)
	}
}
