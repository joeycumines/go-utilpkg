package eventloop

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRunContextCancellationExitsIdleModes(t *testing.T) {
	for _, tt := range []struct {
		name string
		mode FastPathMode
	}{
		{name: "auto", mode: FastPathAuto},
		{name: "forced", mode: FastPathForced},
		{name: "disabled", mode: FastPathDisabled},
	} {
		t.Run(tt.name, func(t *testing.T) {
			loop, err := New(WithFastPathMode(tt.mode))
			if err != nil {
				t.Fatal(err)
			}
			registerLoopCleanupT(t, loop)
			fastPathEntered := make(chan struct{}, 1)
			hooks, disabledWaitReached := newIdleWaitBoundaryHooks()
			watcherWake := make(chan struct{}, 1)
			hooks.OnFastPathEntry = func() {
				select {
				case fastPathEntered <- struct{}{}:
				default:
				}
			}
			hooks.OnSubmitWakeup = func() {
				select {
				case watcherWake <- struct{}{}:
				default:
				}
			}
			loop.testHooks = hooks
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			runDone := make(chan error, 1)
			go func() { runDone <- loop.Run(ctx) }()

			if tt.mode == FastPathDisabled {
				waitContractSignal(t, disabledWaitReached, "initial disabled-mode wait")
			} else {
				select {
				case <-fastPathEntered:
				case <-time.After(5 * time.Second):
					t.Fatal("Run did not enter the fast-path wait loop")
				}
			}
			cancel()
			if tt.mode == FastPathDisabled {
				select {
				case <-watcherWake:
				case <-time.After(5 * time.Second):
					t.Fatal("Run context watcher did not submit a wake after sleep commitment")
				}
			}
			select {
			case err := <-runDone:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("Run = %v, want context.Canceled", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Run remained blocked after context cancellation")
			}
		})
	}
}

func TestRunContextCancellationBeforePollBlockIsNotLost(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathDisabled))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	pollReached := make(chan struct{})
	releasePoll := make(chan struct{})
	watcherWake := make(chan struct{}, 1)
	release := releaseSignalT(t, releasePoll)
	loop.testHooks = &loopTestHooks{
		PrePollSleep: func() {
			select {
			case <-pollReached:
			default:
				close(pollReached)
			}
			<-releasePoll
		},
		OnSubmitWakeup: func() {
			select {
			case watcherWake <- struct{}{}:
			default:
			}
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()

	select {
	case <-pollReached:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not reach the pre-poll barrier")
	}
	cancel()
	select {
	case <-watcherWake:
	case <-time.After(5 * time.Second):
		t.Fatal("Run context watcher did not commit a wake before poll release")
	}
	release()
	select {
	case err := <-runDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancellation wake was lost before poll block")
	}
}
