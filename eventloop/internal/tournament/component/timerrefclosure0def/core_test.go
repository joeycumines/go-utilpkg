package timerrefclosure0def

import (
	"errors"
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

func TestOwnerReferenceSemantics(t *testing.T) {
	loop := newLoop(true)
	if !loop.bindOwner() || !loop.seed(1, true) {
		t.Fatal("owner setup failed")
	}
	if err := loop.refTimer(1); err != nil {
		t.Fatal(err)
	}
	if err := loop.unrefTimer(1); err != nil {
		t.Fatal(err)
	}
	if err := loop.refTimer(1); err != nil {
		t.Fatal(err)
	}
	if err := loop.refTimer(99); err != nil {
		t.Fatal(err)
	}
	if got := loop.snapshot(1); !got.refed || got.refedCount != 1 || got.submissionEpoch != 2 || got.wakeAttempts != 2 || got.wakeSuccesses != 2 {
		t.Fatalf("owner snapshot = %+v", got)
	}
}

func TestExternalReferenceModel(t *testing.T) {
	loop := newLoop(true)
	if !loop.bindOwner() || !loop.seed(1, false) || !loop.seed(2, true) {
		t.Fatal("owner setup failed")
	}
	refed := [2]bool{false, true}
	refedCount := int64(1)
	var epoch uint64
	var wakeAttempts uint64
	operations := []byte{0, 0, 1, 1, 2, 3, 3, 2, 2, 3, 4, 5}
	for step, operation := range operations {
		id := timerID(operation/2 + 1)
		wantRefed := operation%2 == 0
		published := make(chan struct{})
		result := make(chan error, 1)
		go func() {
			observer := referenceObserver{wakePublished: func() { close(published) }}
			if wantRefed {
				result <- loop.refTimerObserved(id, observer)
				return
			}
			result <- loop.unrefTimerObserved(id, observer)
		}()
		waitSignal(t, published, "external reference admission wake")
		assertErrorBlocked(t, result)

		present := id <= 2
		currentRefed := false
		if present {
			currentRefed = refed[id-1]
		}
		wantAdmitted := qualificationSnapshot{
			present:         present,
			refed:           currentRefed,
			refedCount:      refedCount,
			submissionEpoch: epoch + 1,
			queued:          1,
			fastWakePending: 1,
			wakeAttempts:    wakeAttempts,
			wakeSuccesses:   wakeAttempts,
			state:           stateRunning,
		}
		if got := loop.snapshot(id); got != wantAdmitted {
			t.Fatalf("step %d admitted snapshot = %+v, want %+v", step, got, wantAdmitted)
		}
		if drained := loop.drain(); drained != 1 {
			t.Fatalf("step %d drain = %d, want 1", step, drained)
		}
		if err := receiveError(t, result); err != nil {
			t.Fatalf("step %d external reference result = %v", step, err)
		}
		epoch++
		if present && refed[id-1] != wantRefed {
			if wantRefed {
				refedCount++
			} else {
				refedCount--
			}
			refed[id-1] = wantRefed
			epoch++
			wakeAttempts++
		}
		wantDrained := qualificationSnapshot{
			present:         present,
			refed:           present && refed[id-1],
			refedCount:      refedCount,
			submissionEpoch: epoch,
			wakeAttempts:    wakeAttempts,
			wakeSuccesses:   wakeAttempts,
			state:           stateRunning,
		}
		if got := loop.snapshot(id); got != wantDrained {
			t.Fatalf("step %d drained snapshot = %+v, want %+v", step, got, wantDrained)
		}
		if len(loop.timerMap) != 2 || loop.nextTimerID.Load() != 2 {
			t.Fatalf("step %d timer identity changed: len=%d next=%d", step, len(loop.timerMap), loop.nextTimerID.Load())
		}
	}
}

func TestDrainBeforePostAdmissionWake(t *testing.T) {
	loop := newLoop(false)
	if !loop.bindOwner() {
		t.Fatal("owner setup failed")
	}
	executed := 0
	if _, err := loop.enqueue(func() { executed++ }); err != nil {
		t.Fatal(err)
	}
	if loop.drain() != 1 || executed != 1 {
		t.Fatal("pre-wake drain did not execute admitted work")
	}
	loop.publishIngressWake()
	if len(loop.fastWakeupCh) != 1 {
		t.Fatal("source-ordered late wake was not modeled")
	}
	if loop.drain() != 0 || len(loop.fastWakeupCh) != 0 {
		t.Fatal("empty normalization drain did not restore baseline")
	}
}

func TestLivenessGateAsymmetryAndEpochAbort(t *testing.T) {
	loop := newLoop(true)
	if !loop.bindOwner() || !loop.seed(1, false) {
		t.Fatal("owner setup failed")
	}
	if !loop.beginQuiescing(loop.snapshot(1).submissionEpoch) {
		t.Fatal("BeginQuiescing failed")
	}
	if err := loop.refTimer(1); !errors.Is(err, errTerminated) {
		t.Fatalf("quiescing refTimer = %v", err)
	}
	result := make(chan error, 1)
	go func() { result <- loop.unrefTimer(1) }()
	waitSignal(t, loop.fastWakeupCh, "quiescing Unref admission")
	if loop.drain() != 1 {
		t.Fatal("quiescing unrefTimer was not drained")
	}
	if err := receiveError(t, result); err != nil {
		t.Fatalf("quiescing external unrefTimer = %v", err)
	}
	if err := loop.refTimer(1); err != nil {
		t.Fatalf("epoch-invalidated quiescence rejected refTimer: %v", err)
	}
	if got := loop.snapshot(1); got.quiescing || !got.refed || got.submissionEpoch != 2 {
		t.Fatalf("quiescence-abort snapshot = %+v", got)
	}
}

func TestQuiescenceRequiresNoSupportedLiveness(t *testing.T) {
	refed := newLoop(true)
	if !refed.bindOwner() || !refed.seed(1, true) {
		t.Fatal("refed setup failed")
	}
	if refed.beginQuiescing(refed.snapshot(1).submissionEpoch) {
		t.Fatal("quiescing began with a refed timer")
	}

	withFD := newLoop(true)
	if !withFD.bindOwner() || !withFD.seed(1, false) || !withFD.configureUserFDCount(1) {
		t.Fatal("FD setup failed")
	}
	if withFD.beginQuiescing(withFD.snapshot(1).submissionEpoch) {
		t.Fatal("quiescing began with a live user FD")
	}

	quiescing := newLoop(true)
	if !quiescing.bindOwner() || !quiescing.seed(1, false) || !quiescing.beginQuiescing(0) {
		t.Fatal("empty quiescence setup failed")
	}
	if quiescing.configureUserFDCount(1) || quiescing.transition(stateSleeping) {
		t.Fatal("active quiescence admitted new liveness or sleep")
	}

}

func TestQualificationAndBinaryWakeFailure(t *testing.T) {
	loop := newLoop(false)
	if loop.beginQuiescing(0) || loop.seed(0, true) || loop.configureUserFDCount(-1) {
		t.Fatal("invalid qualification succeeded")
	}
	if !loop.bindOwner() || loop.bindOwner() || loop.seed(0, true) || !loop.seed(1, true) || loop.seed(1, false) {
		t.Fatal("one-shot owner or monotonic ID qualification violated")
	}
	if !loop.configureUserFDCount(1) || !loop.transition(stateSleeping) || !loop.configureWakeFailure(true) {
		t.Fatal("wake failure setup failed")
	}
	if _, err := loop.enqueue(func() {}); err != nil {
		t.Fatal(err)
	}
	loop.publishIngressWake()
	if _, err := loop.enqueue(func() {}); err != nil {
		t.Fatal(err)
	}
	loop.publishIngressWake()
	if got := loop.snapshot(1); got.queued != 2 || got.wakeAttempts != 1 || got.wakeSuccesses != 0 || !got.wakePending {
		t.Fatalf("sticky failure snapshot = %+v", got)
	}
	if !loop.transition(stateRunning) || loop.drain() != 2 {
		t.Fatal("failed wake queue did not return to Running baseline")
	}
	if !loop.configureWakeFailure(false) || !loop.transition(stateSleeping) {
		t.Fatal("wake retry setup failed")
	}
	if _, err := loop.enqueue(func() {}); err != nil {
		t.Fatal(err)
	}
	loop.publishIngressWake()
	if got := loop.snapshot(1); got.wakeAttempts != 2 || got.wakeSuccesses != 1 || !got.wakePending {
		t.Fatalf("post-reset retry snapshot = %+v", got)
	}
	if !loop.transition(stateRunning) || loop.drain() != 1 {
		t.Fatal("successful retry did not normalize")
	}
	if got := loop.snapshot(1); got.wakePending || got.fastWakePending != 0 {
		t.Fatalf("wake state was not normalized: %+v", got)
	}
}

func TestDrainDoubleBufferFIFOAndRelease(t *testing.T) {
	loop := newLoop(false)
	if !loop.bindOwner() {
		t.Fatal("BindOwner failed")
	}
	order := make([]int, 0, 3)
	if err := loop.submitToQueue(func() {
		order = append(order, 1)
		if err := loop.submitToQueue(func() { order = append(order, 3) }); err != nil {
			t.Errorf("nested admission: %v", err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := loop.submitToQueue(func() { order = append(order, 2) }); err != nil {
		t.Fatal(err)
	}
	if loop.drain() != 2 {
		t.Fatal("first detached batch mismatch")
	}
	if got := loop.snapshot(0); got.queued != 1 || got.fastWakePending != 1 {
		t.Fatalf("next-batch wake = %+v", got)
	}
	assertNilSlots(t, loop.spare)
	if loop.drain() != 1 {
		t.Fatal("second detached batch mismatch")
	}
	assertNilSlots(t, loop.spare)
	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Fatalf("order = %v, want [1 2 3]", order)
	}
}

func TestMultiProducerFIFOByAdmissionEpoch(t *testing.T) {
	loop := newLoop(false)
	if !loop.bindOwner() {
		t.Fatal("owner setup failed")
	}
	const producers = 32
	records := make(chan admissionResult, producers)
	executed := make([]int, 0, producers)
	var start sync.WaitGroup
	start.Add(1)
	for id := range producers {
		go func() {
			start.Wait()
			epoch, err := loop.enqueue(func() { executed = append(executed, id) })
			records <- admissionResult{id: id, epoch: epoch, err: err}
		}()
	}
	start.Done()
	want := make([]admissionResult, 0, producers)
	for range producers {
		value := receiveAdmission(t, records)
		if value.err != nil {
			t.Fatal(value.err)
		}
		want = append(want, value)
	}
	sort.Slice(want, func(left, right int) bool { return want[left].epoch < want[right].epoch })
	loop.publishIngressWake()
	if loop.drain() != producers || len(executed) != producers {
		t.Fatal("multi-producer batch was not executed exactly once")
	}
	for index, value := range want {
		if value.epoch != uint64(index+1) || executed[index] != value.id {
			t.Fatalf("position %d: admission=%+v execution=%d", index, value, executed[index])
		}
	}
	assertNilSlots(t, loop.spare)
}

func TestRunPublishesStartOnLostClaim(t *testing.T) {
	loop := newLoop(false)
	if err := loop.closeLoop(); err != nil {
		t.Fatal(err)
	}
	if loop.run() || loop.ownerID.Load() != 0 {
		t.Fatal("lost Running CAS published an owner")
	}
	assertChannelClosed(t, loop.runCh, "runCh after lost Running CAS")
	assertSourceCleanup(t, loop, nil)
}

func TestQuiescingDoesNotAbsorbPriorIngress(t *testing.T) {
	loop := newLoop(true)
	if !loop.bindOwner() || !loop.seed(1, false) {
		t.Fatal("owner setup failed")
	}
	observed := loop.snapshot(1).submissionEpoch
	result := make(chan error, 1)
	go func() { result <- loop.unrefTimer(1) }()
	waitSignal(t, loop.fastWakeupCh, "prior ingress admission")
	if loop.beginQuiescing(observed) {
		t.Fatal("quiescing absorbed already-admitted work")
	}
	if loop.drain() != 1 {
		t.Fatal("prior ingress was not drained")
	}
	if err := receiveError(t, result); err != nil {
		t.Fatal(err)
	}
}

func TestRepeatedDrainReturnsStationaryBaseline(t *testing.T) {
	loop := newLoop(false)
	if !loop.bindOwner() || !loop.seed(1, true) {
		t.Fatal("owner setup failed")
	}
	wantCapacity := -1
	for iteration := range 6 {
		result := make(chan error, 1)
		if iteration%2 == 0 {
			go func() { result <- loop.unrefTimer(1) }()
		} else {
			go func() { result <- loop.refTimer(1) }()
		}
		waitSignal(t, loop.fastWakeupCh, "stationary admission")
		if loop.drain() != 1 {
			t.Fatalf("iteration %d drain mismatch", iteration)
		}
		if err := receiveError(t, result); err != nil {
			t.Fatal(err)
		}
		if got := loop.snapshot(1); got.queued != 0 || got.fastWakePending != 0 || got.wakePending {
			t.Fatalf("iteration %d baseline = %+v", iteration, got)
		}
		capacity := cap(loop.queue) + cap(loop.spare)
		if iteration == 1 {
			wantCapacity = capacity
		} else if iteration > 1 && capacity != wantCapacity {
			t.Fatalf("iteration %d queue capacity = %d, want %d", iteration, capacity, wantCapacity)
		}
	}
}

func TestOwnerReferenceModel(t *testing.T) {
	runOwnerReferenceModel(t, []byte{
		0, 0, 1, 1,
		2, 2, 3, 3,
		4, 5, 6,
		0, 2, 7,
		1, 3,
	})
}

func TestOwnerReferenceModelExhaustive(t *testing.T) {
	const traceLength = 5
	const traceCount = 1 << (3 * traceLength)
	operations := make([]byte, traceLength)
	for ordinal := range traceCount {
		encoded := ordinal
		for index := range operations {
			operations[index] = byte(encoded & 7)
			encoded >>= 3
		}
		runOwnerReferenceModel(t, operations)
	}
}

func FuzzOwnerReferenceState(f *testing.F) {
	f.Add([]byte{0, 0, 1, 1, 2, 2, 3, 3, 4, 5, 6, 7})
	f.Add([]byte{0, 2, 0, 2, 6, 1, 3, 7, 4, 5})
	f.Fuzz(func(t *testing.T, operations []byte) {
		if len(operations) > 64 {
			operations = operations[:64]
		}
		runOwnerReferenceModel(t, operations)
	})
}

func runOwnerReferenceModel(t *testing.T, operations []byte) {
	t.Helper()
	loop := newLoop(true)
	if !loop.bindOwner() || !loop.seed(1, false) || !loop.seed(2, true) {
		t.Fatal("owner setup failed")
	}

	refed := [2]bool{false, true}
	refedCount := int64(1)
	var epoch uint64
	var wakeAttempts uint64
	fastToken := false

	assertState := func(step int) {
		t.Helper()
		fastWakePending := 0
		if fastToken {
			fastWakePending = 1
		}
		for index, wantRefed := range refed {
			id := timerID(index + 1)
			want := qualificationSnapshot{
				present:         true,
				refed:           wantRefed,
				refedCount:      refedCount,
				submissionEpoch: epoch,
				fastWakePending: fastWakePending,
				wakeAttempts:    wakeAttempts,
				wakeSuccesses:   wakeAttempts,
				state:           stateRunning,
			}
			if got := loop.snapshot(id); got != want {
				t.Fatalf("trace %v step %d timer %d snapshot = %+v, want %+v", operations, step, id, got, want)
			}
		}
		wantMissing := qualificationSnapshot{
			refedCount:      refedCount,
			submissionEpoch: epoch,
			fastWakePending: fastWakePending,
			wakeAttempts:    wakeAttempts,
			wakeSuccesses:   wakeAttempts,
			state:           stateRunning,
		}
		if got := loop.snapshot(3); got != wantMissing {
			t.Fatalf("trace %v step %d missing timer snapshot = %+v, want %+v", operations, step, got, wantMissing)
		}
		if len(loop.timerMap) != 2 || loop.nextTimerID.Load() != 2 {
			t.Fatalf("trace %v step %d timer identity changed: len=%d next=%d", operations, step, len(loop.timerMap), loop.nextTimerID.Load())
		}
	}

	assertState(-1)
	for step, operation := range operations {
		operation %= 8
		switch operation {
		case 0, 1, 2, 3:
			index := int(operation / 2)
			id := timerID(index + 1)
			wantRefed := operation%2 == 0
			var err error
			if wantRefed {
				err = loop.refTimer(id)
			} else {
				err = loop.unrefTimer(id)
			}
			if err != nil {
				t.Fatalf("trace %v step %d timer %d reference change: %v", operations, step, id, err)
			}
			if refed[index] != wantRefed {
				if wantRefed {
					refedCount++
				} else {
					refedCount--
				}
				refed[index] = wantRefed
				epoch++
				wakeAttempts++
				fastToken = true
			}
		case 4:
			if err := loop.refTimer(3); err != nil {
				t.Fatalf("trace %v step %d missing Ref: %v", operations, step, err)
			}
		case 5:
			if err := loop.unrefTimer(3); err != nil {
				t.Fatalf("trace %v step %d missing Unref: %v", operations, step, err)
			}
		case 6:
			loop.drainWake()
			fastToken = false
		case 7:
			if drained := loop.drain(); drained != 0 {
				t.Fatalf("trace %v step %d empty drain = %d, want 0", operations, step, drained)
			}
			fastToken = false
		}
		assertState(step)
	}
}

func TestSourceRefFirstGateTerminalOvertake(t *testing.T) {
	for _, test := range []struct {
		name  string
		owner bool
		close bool
	}{
		{name: "OwnerClose", owner: true, close: true},
		{name: "ExternalShutdown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := newLoop(false)
			runResult, runWaits := startSourceWaitingRun(t, value, lifecycleObserver{})
			timerValue := seedSourceTimer(t, value, false)
			waitSignal(t, runWaits, "post-seed Run wait")

			firstGatePassed := make(chan struct{})
			resumeRef, releaseRef := newSourceRelease(t)
			refResult := make(chan error, 1)
			ref := func() {
				refResult <- value.refTimerObserved(1, referenceObserver{
					firstGatePassed: func() {
						close(firstGatePassed)
						<-resumeRef
					},
				})
			}
			if test.owner {
				if err := value.submitToQueue(ref); err != nil {
					t.Fatal(err)
				}
			} else {
				go ref()
			}
			waitSignal(t, firstGatePassed, test.name+" first liveness gate")
			assertErrorBlocked(t, refResult)
			epoch := value.submissionEpoch.Load()

			terminalBoundary := make(chan struct{})
			terminalResult := make(chan error, 1)
			resumeTerminal, releaseTerminal := newSourceRelease(t)
			if test.close {
				go func() {
					terminalResult <- value.closeLoopObserved(lifecycleObserver{
						closeWait: func(stage closeWaitStage) {
							if stage == closeWaitWinningLoop {
								close(terminalBoundary)
							}
						},
					})
				}()
			} else {
				go func() {
					terminalResult <- value.shutdownObserved(lifecycleObserver{
						shutdownPublished: func(<-chan struct{}) {
							close(terminalBoundary)
							<-resumeTerminal
						},
					})
				}()
			}
			waitSignal(t, terminalBoundary, test.name+" terminal boundary")
			releaseRef()
			if err := receiveError(t, refResult); !errors.Is(err, errTerminated) {
				t.Fatalf("Ref after first gate = %v", err)
			}
			if value.submissionEpoch.Load() != epoch ||
				timerValue.refed.Load() ||
				value.refedTimerCount.Load() != 0 {
				t.Fatal("terminal overtake mutated the rejected reference")
			}
			releaseTerminal()
			if err := receiveError(t, terminalResult); err != nil {
				t.Fatal(err)
			}
			if !receiveBool(t, runResult, test.name+" Run") {
				t.Fatal("Run did not complete terminal overtake")
			}
			wantFastWake := 0
			if test.close {
				wantFastWake = 1
			}
			assertSourceCleanupWake(t, value, timerValue, wantFastWake)
		})
	}
}

func receiveAdmission(t *testing.T, result <-chan admissionResult) admissionResult {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(time.Second):
		t.Fatal("admission result did not return")
		return admissionResult{}
	}
}

func waitSignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatalf("%s did not fire", name)
	}
}

func assertErrorBlocked(t *testing.T, result <-chan error) {
	t.Helper()
	select {
	case err := <-result:
		t.Fatalf("call returned before drain: %v", err)
	default:
	}
}

func assertBoolBlocked(t *testing.T, result <-chan bool) {
	t.Helper()
	select {
	case value := <-result:
		t.Fatalf("call returned before release: %v", value)
	default:
	}
}

func receiveError(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(time.Second):
		t.Fatal("call did not return")
		return nil
	}
}

func receiveBool(t *testing.T, result <-chan bool, name string) bool {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(time.Second):
		t.Fatalf("%s did not return", name)
		return false
	}
}

func assertChannelOpen(t *testing.T, channel <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-channel:
		t.Fatalf("%s closed early", name)
	default:
	}
}

func assertChannelClosed(t *testing.T, channel <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-channel:
	case <-time.After(time.Second):
		t.Fatalf("%s did not close", name)
	}
}

func assertNilSlots(t *testing.T, values []func()) {
	t.Helper()
	for index, value := range values[:cap(values)] {
		if value != nil {
			t.Fatalf("retained closure at slot %d", index)
		}
	}
}
