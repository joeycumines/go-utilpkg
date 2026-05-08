package tournament

import (
	"fmt"
	"sync/atomic"
	"testing"
)

func TestExternalInternalMixedWorkload(t *testing.T) {
	for _, impl := range Implementations() {
		t.Run(impl.Name, func(t *testing.T) {
			loop, cleanup := startTournamentTestLoop(t, impl)
			const producerCount = 10
			const tasksPerProducer = 100
			const taskCount = producerCount * tasksPerProducer
			producerDone := make(chan struct{}, producerCount)
			externalDone := make(chan struct{}, taskCount)
			internalDone := make(chan struct{}, taskCount)
			submitErrors := make(chan error, 1)
			var externalCount, internalCount atomic.Int64

			recordError := func(err error) {
				select {
				case submitErrors <- err:
				default:
				}
			}
			for range producerCount {
				go func() {
					defer func() { producerDone <- struct{}{} }()
					for range tasksPerProducer {
						if err := loop.Submit(func() {
							externalCount.Add(1)
							externalDone <- struct{}{}
							if err := loop.SubmitInternal(func() {
								internalCount.Add(1)
								internalDone <- struct{}{}
							}); err != nil {
								recordError(fmt.Errorf("SubmitInternal: %w", err))
								internalDone <- struct{}{}
							}
						}); err != nil {
							recordError(fmt.Errorf("Submit: %w", err))
							externalDone <- struct{}{}
							internalDone <- struct{}{}
						}
					}
				}()
			}

			waitTournamentCount(t, producerDone, producerCount, "mixed-workload producer exit")
			waitTournamentCount(t, externalDone, taskCount, "external callback drain")
			waitTournamentCount(t, internalDone, taskCount, "internal callback drain")
			select {
			case err := <-submitErrors:
				t.Error(err)
			default:
			}
			cleanup()
			if got := externalCount.Load(); got != taskCount {
				t.Errorf("external tasks = %d, want %d", got, taskCount)
			}
			if got := internalCount.Load(); got != taskCount {
				t.Errorf("internal tasks = %d, want %d", got, taskCount)
			}
		})
	}
}
