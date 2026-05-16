//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestPhysicalWakeFailureCannotStrandPollOperations(t *testing.T) {
	tests := []struct {
		name      string
		deadline  time.Duration
		operation string
	}{
		{name: "ingress-indefinite", operation: "ingress"},
		{name: "ingress-near-deadline", deadline: 900 * time.Millisecond, operation: "ingress"},
		{name: "ingress-far-future", deadline: 30 * time.Second, operation: "ingress"},
		{name: "close-indefinite", operation: "close"},
		{name: "close-far-future", deadline: 30 * time.Second, operation: "close"},
		{name: "context-cancel-indefinite", operation: "context-cancel"},
		{name: "context-cancel-far-future", deadline: 30 * time.Second, operation: "context-cancel"},
		{name: "shutdown-indefinite", operation: "shutdown"},
		{name: "shutdown-far-future", deadline: 30 * time.Second, operation: "shutdown"},
		{name: "unregister-last-fd-indefinite", operation: "unregister-last-fd"},
		{name: "unregister-last-fd-far-future", deadline: 30 * time.Second, operation: "unregister-last-fd"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var pipeFDs [2]int
			if err := unix.Pipe(pipeFDs[:]); err != nil {
				t.Fatal(err)
			}
			registerTestFDCleanupT(t, &pipeFDs[0], &pipeFDs[1])

			loop, err := New()
			if err != nil {
				t.Fatal(err)
			}
			registerLoopCleanupT(t, loop)
			if err := loop.RegisterFD(pipeFDs[0], EventRead, func(IOEvents) {}); err != nil {
				t.Fatal(err)
			}
			// RegisterFD intentionally wakes both wait mechanisms. Remove those
			// setup signals before arming the fault so only the simulated bounded
			// PollIO timeout can release the first wait.
			loop.drainWakeUpPipe()
			select {
			case <-loop.fastWakeupCh:
			default:
			}
			if got := loop.wakeUpSignalPending.Load(); got != wakeSignalIdle {
				t.Fatalf("pending state after setup drain = %d, want idle", got)
			}
			if test.deadline > 0 {
				pushTestTimer(loop, &timer{when: time.Now().Add(test.deadline)})
			}

			pollEntered := make(chan int, 1)
			releasePoll := make(chan struct{})
			var releaseOnce sync.Once
			t.Cleanup(func() { releaseOnce.Do(func() { close(releasePoll) }) })
			wakeFailed := make(chan struct{})
			var wakeFailedOnce sync.Once
			var pollCalls atomic.Int32
			sentinel := errors.New("physical wake unavailable")
			loop.testHooks = &loopTestHooks{
				PollIO: func(timeout int) (int, error) {
					if pollCalls.Add(1) == 1 {
						pollEntered <- timeout
						<-releasePoll
					}
					return 0, nil
				},
				WriteWakeFD: func(_ int, _ []byte) (int, error) {
					wakeFailedOnce.Do(func() { close(wakeFailed) })
					return 0, sentinel
				},
			}

			runCtx := context.Background()
			var cancel context.CancelFunc
			if test.operation == "context-cancel" {
				runCtx, cancel = context.WithCancel(runCtx)
				t.Cleanup(cancel)
			}
			runDone := make(chan error, 1)
			go func() { runDone <- loop.Run(runCtx) }()
			select {
			case timeout := <-pollEntered:
				if test.deadline > 0 && test.deadline < time.Duration(maxPhysicalPollWaitMs)*time.Millisecond {
					if timeout <= 0 || timeout > int(test.deadline.Milliseconds()) {
						t.Fatalf("near-deadline PollIO timeout = %d, want 1..%d", timeout, test.deadline.Milliseconds())
					}
				} else if timeout != maxPhysicalPollWaitMs {
					t.Fatalf("native PollIO timeout = %d, want recovery bound %d", timeout, maxPhysicalPollWaitMs)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("loop did not enter the native PollIO boundary")
			}

			operationDone := make(chan error, 1)
			executed := make(chan struct{})
			switch test.operation {
			case "ingress":
				if err := loop.Submit(func() { close(executed) }); err != nil {
					t.Fatal(err)
				}
			case "close":
				go func() { operationDone <- loop.Close() }()
			case "context-cancel":
				cancel()
			case "shutdown":
				go func() { operationDone <- loop.Shutdown(context.Background()) }()
			case "unregister-last-fd":
				if err := loop.UnregisterFD(pipeFDs[0]); err != nil {
					t.Fatal(err)
				}
				if err := loop.Submit(func() { close(executed) }); err != nil {
					t.Fatal(err)
				}
			default:
				t.Fatalf("unknown operation %q", test.operation)
			}
			select {
			case <-wakeFailed:
			case <-time.After(5 * time.Second):
				t.Fatal("operation did not exercise the injected physical wake failure")
			}
			switch test.operation {
			case "ingress", "unregister-last-fd":
				select {
				case <-executed:
					t.Fatalf("%s follow-up ingress executed before the simulated bounded PollIO timeout", test.operation)
				default:
				}
			case "close", "shutdown":
				select {
				case err := <-operationDone:
					t.Fatalf("%s returned before the simulated bounded PollIO timeout: %v", test.operation, err)
				default:
				}
			case "context-cancel":
				select {
				case err := <-runDone:
					t.Fatalf("Run returned before the simulated bounded PollIO timeout: %v", err)
				default:
				}
			}

			releaseOnce.Do(func() { close(releasePoll) })
			switch test.operation {
			case "ingress", "unregister-last-fd":
				select {
				case <-executed:
				case <-time.After(5 * time.Second):
					t.Fatalf("%s follow-up ingress remained stranded after the bounded PollIO timeout", test.operation)
				}
				go func() { operationDone <- loop.Close() }()
				select {
				case err := <-operationDone:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("Close did not complete after the bounded PollIO timeout")
				}
			case "close", "shutdown":
				select {
				case err := <-operationDone:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(5 * time.Second):
					t.Fatalf("%s did not complete after the bounded PollIO timeout", test.operation)
				}
			case "context-cancel":
				select {
				case err := <-runDone:
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("Run after context cancellation = %v, want context.Canceled", err)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("Run did not return after bounded context-cancellation recovery")
				}
				return
			}
			select {
			case err := <-runDone:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Run did not return after Close")
			}
		})
	}
}
