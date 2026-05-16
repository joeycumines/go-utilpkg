package eventloop

import (
	"testing"
)

func TestPollFastModeReusesFiniteSleepTimer(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		loop.stopFastSleepTimer()
		closeFDResourcesT(t, loop)
	})

	loop.state.Store(StateSleeping)
	loop.pollFastMode(1)
	first := loop.fastSleepTimer
	if first == nil {
		t.Fatal("finite fast-mode sleep did not allocate the reusable timer")
	}

	loop.state.Store(StateSleeping)
	loop.pollFastMode(1)
	if loop.fastSleepTimer != first {
		t.Fatal("finite fast-mode sleep replaced the reusable timer")
	}
}

func TestPollFastModeFastWakePreservesPhysicalPending(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerFDResourceCleanupT(t, loop)

	loop.state.Store(StateSleeping)
	loop.wakeUpSignalPending.Store(wakeSignalPending)
	loop.fastWakeupCh <- struct{}{}
	loop.pollFastMode(-1)

	if pending := loop.wakeUpSignalPending.Load(); pending != wakeSignalPending {
		t.Fatalf("physical pending state after fast wake = %d, want %d", pending, wakeSignalPending)
	}
}

func TestPollFastModeBlockingFastWakePreservesPhysicalPending(t *testing.T) {
	tests := []struct {
		name    string
		timeout int
	}{
		{name: "indefinite", timeout: -1},
		{name: "finite", timeout: 5_000},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop, err := New()
			if err != nil {
				t.Fatal(err)
			}
			registerFDResourceCleanupT(t, loop)
			loop.state.Store(StateSleeping)
			loop.wakeUpSignalPending.Store(wakeSignalPending)

			waitEntered := make(chan int, 1)
			loop.testHooks = &loopTestHooks{
				BeforeFastPollWait: func(timeout int) { waitEntered <- timeout },
			}
			done := make(chan struct{})
			go func() {
				loop.pollFastMode(test.timeout)
				close(done)
			}()
			if timeout := waitContractValue(t, waitEntered, "blocking fast wait entry"); timeout != test.timeout {
				t.Fatalf("fast wait timeout = %d, want %d", timeout, test.timeout)
			}
			loop.fastWakeupCh <- struct{}{}
			waitContractSignal(t, done, "blocking fast wait completion")
			if pending := loop.wakeUpSignalPending.Load(); pending != wakeSignalPending {
				t.Fatalf("physical pending state after blocking fast wake = %d, want %d", pending, wakeSignalPending)
			}
		})
	}
}
