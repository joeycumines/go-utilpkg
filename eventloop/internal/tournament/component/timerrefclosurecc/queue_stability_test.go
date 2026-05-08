package timerrefclosurecc

import (
	"sort"
	"sync"
	"testing"
	"time"
)

type admissionResult struct {
	id    int
	epoch uint64
	err   error
}

func TestMultiProducerFIFOByAdmissionEpoch(t *testing.T) {
	value := newLoop(false)
	waiting := make(chan struct{})
	resumeRun := make(chan struct{})
	drained := make(chan struct{})
	resumeDrained := make(chan struct{})
	t.Cleanup(func() {
		releaseSignal(resumeRun)
		releaseSignal(resumeDrained)
	})
	waits := 0
	runResult := make(chan bool, 1)
	go func() {
		runResult <- value.runObserved(lifecycleObserver{runWait: func() {
			waits++
			switch waits {
			case 1:
				close(waiting)
				<-resumeRun
			case 2:
				close(drained)
				<-resumeDrained
			}
		}})
	}()
	waitSignal(t, waiting, "multi-producer Run wait")
	const producers = 32
	records := make(chan admissionResult, producers)
	executed := make([]int, 0, producers)
	var start sync.WaitGroup
	start.Add(1)
	for id := range producers {
		go func() {
			start.Wait()
			record := admissionResult{id: id}
			record.err = value.submitToQueueObserved(
				func() { executed = append(executed, id) },
				referenceObserver{queueAdmitted: func(epoch uint64) { record.epoch = epoch }},
			)
			records <- record
		}()
	}
	start.Done()
	want := make([]admissionResult, 0, producers)
	for range producers {
		result := receiveAdmission(t, records)
		if result.err != nil {
			t.Fatal(result.err)
		}
		want = append(want, result)
	}
	sort.Slice(want, func(left, right int) bool { return want[left].epoch < want[right].epoch })
	close(resumeRun)
	waitSignal(t, drained, "multi-producer Run drain")
	if len(executed) != producers || len(value.queue) != 0 {
		t.Fatal("multi-producer batch was not executed exactly once")
	}
	for index, result := range want {
		if result.epoch != uint64(index+1) || executed[index] != result.id {
			t.Fatalf("position %d: admission=%+v execution=%d", index, result, executed[index])
		}
	}
	assertNilSlots(t, value.spare)
	closeResult := make(chan error, 1)
	go func() { closeResult <- value.closeLoop() }()
	close(resumeDrained)
	if !receiveBool(t, runResult, "multi-producer Close Run") {
		t.Fatal("Run did not exit for Close")
	}
	if err := receiveError(t, closeResult); err != nil {
		t.Fatal(err)
	}
}

func TestRepeatedDrainReturnsStationaryBaseline(t *testing.T) {
	value := newLoop(false)
	committed := make(chan struct{})
	var id timerID
	var err error
	id, err = value.scheduleTimerObserved(time.Hour, func() {}, registrationObserver{
		registrationCommitted: func() { close(committed) },
	})
	if err != nil || id != 1 {
		t.Fatalf("ScheduleTimer = (%d, %v)", id, err)
	}
	ready := make(chan int)
	resume := make(chan struct{})
	t.Cleanup(func() { releaseSignal(resume) })
	waits := 0
	runResult := make(chan bool, 1)
	go func() {
		runResult <- value.runObserved(lifecycleObserver{runWait: func() {
			waits++
			if waits >= 2 {
				ready <- waits - 2
				<-resume
			}
		}})
	}()
	waitSignal(t, committed, "stationary timer registration")
	receiveReady := func(want int) {
		t.Helper()
		select {
		case got := <-ready:
			if got != want {
				t.Fatalf("stationary turn = %d, want %d", got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("stationary turn %d did not occur", want)
		}
	}
	receiveReady(0)
	wantCapacity := -1
	for iteration := range 6 {
		admitted := make(chan struct{})
		result := make(chan error, 1)
		if iteration%2 == 0 {
			go func() {
				result <- value.unrefTimerObserved(id, referenceObserver{
					queueAdmitted: func(uint64) { close(admitted) },
				})
			}()
		} else {
			go func() {
				result <- value.refTimerObserved(id, referenceObserver{
					queueAdmitted: func(uint64) { close(admitted) },
				})
			}()
		}
		waitSignal(t, admitted, "stationary reference admission")
		assertErrorBlocked(t, result)
		resume <- struct{}{}
		if err := receiveError(t, result); err != nil {
			t.Fatal(err)
		}
		receiveReady(iteration + 1)
		timerValue := value.timerMap[id]
		wantRefed := iteration%2 != 0
		if timerValue == nil || timerValue.refed.Load() != wantRefed || len(value.queue) != 0 ||
			len(value.fastWakeupCh) != 0 || value.wakeUpSignalPending.Load() != 0 {
			t.Fatalf("iteration %d did not return to a stationary source baseline", iteration)
		}
		assertNilSlots(t, value.spare)
		capacity := cap(value.queue) + cap(value.spare)
		if iteration == 1 {
			wantCapacity = capacity
		} else if iteration > 1 && capacity != wantCapacity {
			t.Fatalf("iteration %d queue capacity = %d, want %d", iteration, capacity, wantCapacity)
		}
	}
	closeResult := make(chan error, 1)
	go func() { closeResult <- value.closeLoop() }()
	close(resume)
	if !receiveBool(t, runResult, "stationary-baseline Close Run") {
		t.Fatal("Run did not exit for Close")
	}
	if err := receiveError(t, closeResult); err != nil {
		t.Fatal(err)
	}
}
