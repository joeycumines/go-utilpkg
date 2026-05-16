package eventloop

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestTaskQueueConcurrentAdmission(t *testing.T) {
	tests := []struct {
		name     string
		schedule func(*Loop, func()) error
	}{
		{name: "microtask", schedule: (*Loop).ScheduleMicrotask},
		{name: "nextTick", schedule: (*Loop).ScheduleNextTick},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop, err := New()
			if err != nil {
				t.Fatal(err)
			}
			registerLoopCleanupT(t, loop)
			runContext, cancelRun := context.WithCancel(context.Background())
			t.Cleanup(cancelRun)
			runDone := make(chan error, 1)
			go func() { runDone <- loop.Run(runContext) }()
			ownerStarted := make(chan struct{})
			if err := loop.Submit(func() { close(ownerStarted) }); err != nil {
				t.Fatalf("Submit startup barrier: %v", err)
			}
			waitContractSignal(t, ownerStarted, "loop owner startup barrier")

			const (
				producers        = 16
				tasksPerProducer = 32
				total            = producers * tasksPerProducer
			)
			start := make(chan struct{})
			errorsByTask := make(chan error, total)
			callbacksDone := make(chan struct{})
			var callbackCount atomic.Int64
			var producerGroup sync.WaitGroup
			for range producers {
				producerGroup.Go(func() {
					<-start
					for range tasksPerProducer {
						errorsByTask <- test.schedule(loop, func() {
							if callbackCount.Add(1) == total {
								close(callbacksDone)
							}
						})
					}
				})
			}
			close(start)
			producersDone := make(chan struct{})
			go func() {
				producerGroup.Wait()
				close(producersDone)
			}()
			waitContractSignal(t, producersDone, "concurrent task producers")
			close(errorsByTask)
			for err := range errorsByTask {
				if err != nil {
					t.Fatalf("schedule: %v", err)
				}
			}
			waitContractSignal(t, callbacksDone, "concurrently admitted tasks")
			if got := callbackCount.Load(); got != total {
				t.Fatalf("executed callbacks = %d, want %d", got, total)
			}

			cancelRun()
			if err := waitContractValue(t, runDone, "context-canceled Run completion"); !errors.Is(err, context.Canceled) {
				t.Fatalf("Run = %v, want context.Canceled", err)
			}
		})
	}
}
