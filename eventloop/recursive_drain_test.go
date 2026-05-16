package eventloop

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"
)

func TestDrain_BoundedRecursiveNextTickMicrotaskOppositeQueues(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	done := make(chan error, 1)
	go func() { done <- loop.Run(context.Background()) }()
	waitLoopOwnerTurnT(t, loop)

	stressDone, result := runBoundedRecursiveDrainStress(t, loop, 128)
	if err := loop.Submit(func() { result.start() }); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	waitContractSignal(t, stressDone, "recursive drain stress completion")
	result.require(t, []string{"next-final", "micro-from-next", "micro-final", "next-from-micro"})
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := waitContractValue(t, done, "recursive drain Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestTerminalDrain_BoundedRecursiveNextTickMicrotaskOppositeQueues(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	terminating := make(chan struct{})
	loop.testHooks = &loopTestHooks{
		AfterShutdownStateTerminating: func() { close(terminating) },
	}

	taskStarted := make(chan struct{})
	releaseTask := make(chan struct{})
	release := contractRelease(t, releaseTask)
	if err := loop.Submit(func() {
		close(taskStarted)
		<-releaseTask
	}); err != nil {
		t.Fatalf("blocking Submit: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- loop.Run(context.Background()) }()
	waitContractSignal(t, taskStarted, "blocking task entry")

	stressDone, result := runBoundedRecursiveDrainStress(t, loop, 128)
	if err := loop.Submit(func() { result.start() }); err != nil {
		release()
		t.Fatalf("queued terminal Submit before Shutdown: %v", err)
	}

	shutdownErr := make(chan error, 1)
	go func() { shutdownErr <- loop.Shutdown(context.Background()) }()

	waitContractSignal(t, terminating, "Shutdown StateTerminating publication")
	release()

	if err := waitContractValue(t, done, "terminal recursive drain Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if err := waitContractValue(t, shutdownErr, "terminal recursive drain Shutdown completion"); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	waitContractSignal(t, stressDone, "terminal recursive drain stress completion")
	result.require(t, []string{"next-final", "micro-from-next", "micro-final", "next-from-micro"})
}

func TestRun_CooperativeHostControlCheckpoint(t *testing.T) {
	const chainLength = 4096
	const controlStep = chainLength / 2

	tests := []struct {
		name    string
		request func(*Loop, context.CancelFunc) error
		wantErr error
	}{
		{
			name: "Run cancellation",
			request: func(_ *Loop, cancel context.CancelFunc) error {
				cancel()
				return nil
			},
			wantErr: context.Canceled,
		},
		{
			name: "graceful Shutdown",
			request: func(loop *Loop, _ context.CancelFunc) error {
				return loop.Shutdown(context.Background())
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop, err := New()
			if err != nil {
				t.Fatal(err)
			}
			registerLoopCleanupT(t, loop)
			js, err := NewJS(loop)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			var (
				nextTicks   int
				microtasks  int
				reactions   int
				checkpoints int
				controlAt   int
				sentinelAt  = -1
				scheduleErr error
			)
			recordErr := func(err error) bool {
				if err == nil {
					return false
				}
				if scheduleErr == nil {
					scheduleErr = err
				}
				cancel()
				return true
			}

			var next func()
			next = func() {
				nextTicks++
				recordErr(loop.ScheduleMicrotask(func() {
					microtasks++
					js.Resolve(checkpoints).Then(func(any) any {
						reactions++
						recordErr(loop.ScheduleMicrotaskCheckpoint(func() {
							checkpoints++
							if checkpoints == controlStep {
								controlAt = checkpoints
								if recordErr(test.request(loop, cancel)) {
									return
								}
							}
							if checkpoints < chainLength {
								recordErr(loop.ScheduleNextTick(next))
							}
						}))
						return nil
					}, nil)
				}))
			}

			if err := loop.ScheduleNextTick(next); err != nil {
				t.Fatalf("ScheduleNextTick: %v", err)
			}
			if err := loop.ScheduleImmediate(func() { sentinelAt = checkpoints }); err != nil {
				t.Fatalf("ScheduleImmediate: %v", err)
			}

			if err := loop.Run(ctx); err != test.wantErr {
				t.Fatalf("Run = %v, want %v", err, test.wantErr)
			}
			if scheduleErr != nil {
				t.Fatalf("recursive scheduling: %v", scheduleErr)
			}
			if controlAt != controlStep {
				t.Fatalf("host control step = %d, want %d", controlAt, controlStep)
			}
			want := [4]int{chainLength, chainLength, chainLength, chainLength}
			if got := [4]int{nextTicks, microtasks, reactions, checkpoints}; got != want {
				t.Fatalf("drained callbacks = %v, want %v", got, want)
			}
			if sentinelAt != chainLength {
				t.Fatalf("later-phase sentinel observed checkpoint %d, want %d", sentinelAt, chainLength)
			}
		})
	}
}

type boundedRecursiveDrainResult struct {
	loop       *Loop
	done       chan struct{}
	limit      int
	closeOnce  sync.Once
	mu         sync.Mutex
	order      []string
	nextCount  int
	microCount int
	err        error
}

func runBoundedRecursiveDrainStress(t *testing.T, loop *Loop, limit int) (<-chan struct{}, *boundedRecursiveDrainResult) {
	t.Helper()
	result := &boundedRecursiveDrainResult{
		loop:  loop,
		done:  make(chan struct{}),
		limit: limit,
	}
	return result.done, result
}

func (r *boundedRecursiveDrainResult) start() {
	var next func()
	var micro func()

	next = func() {
		r.mu.Lock()
		r.nextCount++
		count := r.nextCount
		r.mu.Unlock()

		if count == 1 {
			r.captureErr(r.loop.ScheduleMicrotask(func() { r.record("micro-from-next") }))
		}
		if count < r.limit {
			r.captureErr(r.loop.ScheduleNextTick(next))
			return
		}
		r.record("next-final")
	}

	micro = func() {
		r.mu.Lock()
		r.microCount++
		count := r.microCount
		r.mu.Unlock()

		if count == 1 {
			r.captureErr(r.loop.ScheduleNextTick(func() {
				r.record("next-from-micro")
				r.closeOnce.Do(func() { close(r.done) })
			}))
		}
		if count < r.limit {
			r.captureErr(r.loop.ScheduleMicrotask(micro))
			return
		}
		r.record("micro-final")
	}

	r.captureErr(r.loop.ScheduleNextTick(next))
	r.captureErr(r.loop.ScheduleMicrotask(micro))
}

func (r *boundedRecursiveDrainResult) captureErr(err error) {
	if err == nil {
		return
	}
	r.mu.Lock()
	if r.err == nil {
		r.err = err
	}
	r.mu.Unlock()
	r.closeOnce.Do(func() { close(r.done) })
}

func (r *boundedRecursiveDrainResult) record(entry string) {
	r.mu.Lock()
	r.order = append(r.order, entry)
	r.mu.Unlock()
}

func (r *boundedRecursiveDrainResult) snapshot() boundedRecursiveDrainResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	return boundedRecursiveDrainResult{
		limit:      r.limit,
		order:      append([]string(nil), r.order...),
		nextCount:  r.nextCount,
		microCount: r.microCount,
		err:        r.err,
	}
}

func (r *boundedRecursiveDrainResult) require(t *testing.T, wantOrder []string) {
	t.Helper()
	snapshot := r.snapshot()
	if snapshot.err != nil {
		t.Fatalf("recursive scheduling error: %v", snapshot.err)
	}
	if snapshot.nextCount != snapshot.limit {
		t.Fatalf("nextTick count = %d, want %d", snapshot.nextCount, snapshot.limit)
	}
	if snapshot.microCount != snapshot.limit {
		t.Fatalf("microtask count = %d, want %d", snapshot.microCount, snapshot.limit)
	}
	if !slices.Equal(snapshot.order, wantOrder) {
		t.Fatalf("recursive drain order = %v, want %v", snapshot.order, wantOrder)
	}
}
