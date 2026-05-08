package eventloop

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
)

func TestJSSetIntervalRecoversCallbackPanicAndRepeats(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)
	js := NewJS(loop)

	published := make(chan struct{})
	done := make(chan struct{})
	var count atomic.Int32
	var intervalID uint64
	intervalID, err := js.SetInterval(func() {
		<-published
		switch count.Add(1) {
		case 1:
			panic("interval callback panic")
		case 3:
			if clearErr := js.ClearInterval(intervalID); clearErr != nil {
				t.Errorf("ClearInterval after recovered panic: %v", clearErr)
			}
			close(done)
		}
	}, 0)
	if err != nil {
		t.Fatalf("SetInterval: %v", err)
	}
	close(published)
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitContractSignal(t, done, "post-panic interval repetition")
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := waitContractValue(t, runDone, "post-panic interval Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := count.Load(); got != 3 {
		t.Fatalf("interval callback count = %d, want 3", got)
	}
}

func TestJSSetIntervalConcurrentClearHasOneWinner(t *testing.T) {
	loop := New(WithAutoExit(true))
	registerLoopCleanupT(t, loop)
	js := NewJS(loop)

	fired := make(chan struct{}, 1)
	id, err := js.SetInterval(func() { fired <- struct{}{} }, 3_600_000)
	if err != nil {
		t.Fatalf("SetInterval: %v", err)
	}

	const contenders = 10
	start := make(chan struct{})
	results := make(chan error, contenders)
	joined := make(chan struct{})
	var workers sync.WaitGroup
	workers.Add(contenders)
	for range contenders {
		go func() {
			defer workers.Done()
			<-start
			results <- js.ClearInterval(id)
		}()
	}
	go func() {
		workers.Wait()
		close(joined)
	}()
	close(start)
	waitContractSignal(t, joined, "concurrent ClearInterval contenders")
	close(results)

	var succeeded, missing int
	for result := range results {
		switch result {
		case nil:
			succeeded++
		case ErrTimerNotFound:
			missing++
		default:
			t.Fatalf("ClearInterval result = %v, want nil or ErrTimerNotFound", result)
		}
	}
	if succeeded != 1 || missing != contenders-1 {
		t.Fatalf("concurrent ClearInterval results = (success=%d, missing=%d), want (1, %d)", succeeded, missing, contenders-1)
	}
	if err := runAutoExitLoop(t, loop); err != nil {
		t.Fatalf("Run: %v", err)
	}
	select {
	case <-fired:
		t.Fatal("concurrently cleared interval fired")
	default:
	}
	if got := loop.refedTimerCount.Load(); got != 0 {
		t.Fatalf("refedTimerCount = %d after concurrent clear, want 0", got)
	}
}
