package eventloop

import "testing"

func TestWakeNonSleepingStatePolicy(t *testing.T) {
	t.Run("running signals fast waiter once", func(t *testing.T) {
		loop := &Loop{state: newFastState(), fastWakeupCh: make(chan struct{}, 1)}
		loop.state.Store(StateRunning)
		if err := loop.Wake(); err != nil {
			t.Fatalf("first Wake: %v", err)
		}
		if err := loop.Wake(); err != nil {
			t.Fatalf("second Wake: %v", err)
		}
		select {
		case <-loop.fastWakeupCh:
		default:
			t.Fatal("Wake in StateRunning did not signal the fast waiter")
		}
		select {
		case <-loop.fastWakeupCh:
			t.Fatal("Wake in StateRunning published more than one pending signal")
		default:
		}
	})

	for _, state := range []LoopState{StateAwake, StateTerminating, StateTerminated} {
		t.Run(state.String(), func(t *testing.T) {
			loop := &Loop{state: newFastState(), fastWakeupCh: make(chan struct{}, 1)}
			loop.state.Store(state)
			if err := loop.Wake(); err != nil {
				t.Fatalf("Wake: %v", err)
			}
			select {
			case <-loop.fastWakeupCh:
				t.Fatalf("Wake in %s signaled the fast waiter", state)
			default:
			}
		})
	}
}
