package tournament

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/joeycumines/go-eventloop/internal/tournament/component"
)

func TestTimerStorageSemanticMatrix(t *testing.T) {
	executed, diagnosed, notApplicable := 0, 0, 0
	for _, descriptor := range timerComponentRegistry {
		for _, workload := range allTimerStorageWorkloads {
			descriptor, workload := descriptor, workload
			t.Run(descriptor.ID+"/"+string(workload), func(t *testing.T) {
				plan, err := resolveTimerStorageWorkload(descriptor, workload)
				if err != nil {
					t.Fatal(err)
				}
				routed := 0
				err = runTimerStorageWorkload(
					plan,
					func(factory timerNativeFactory, definition timerStorageDefinition) error {
						routed++
						executed++
						return executeTimerStorageSemantics(factory, definition)
					},
					func(reason timerStorageDiagnosticReason, definition timerStorageDefinition) error {
						routed++
						diagnosed++
						return diagnoseTimerStorageSemantics(descriptor.NativeDriver, reason, definition)
					},
				)
				if err != nil {
					t.Fatal(err)
				}
				if plan.rule.Disposition == timerNA {
					notApplicable++
					if routed != 0 {
						t.Fatalf("N/A workload routed %d callbacks", routed)
					}
				} else if routed != 1 {
					t.Fatalf("admitted workload routed %d callbacks, want 1", routed)
				}
			})
		}
	}
	if total := executed + diagnosed + notApplicable; total != len(timerComponentRegistry)*timerStorageWorkloadCount {
		t.Fatalf("storage semantic cells = %d+%d+%d=%d", executed, diagnosed, notApplicable, total)
	}
	if executed != 81 || diagnosed != 8 || notApplicable != 46 {
		t.Fatalf("storage semantic dispositions = (%d execute, %d diagnostic, %d N/A), want (81, 8, 46)", executed, diagnosed, notApplicable)
	}
}

func executeTimerStorageSemantics(factory timerNativeFactory, definition timerStorageDefinition) error {
	return verifyTimerStorageSemantics(factory.ID, definition, "")
}

func diagnoseTimerStorageSemantics(driver timerNativeDriverID, reason timerStorageDiagnosticReason, definition timerStorageDefinition) error {
	if reason == "" {
		return fmt.Errorf("diagnostic %s/%s has no reason", driver, definition.ID)
	}
	return verifyTimerStorageSemantics(driver, definition, reason)
}

func verifyTimerStorageSemantics(driver timerNativeDriverID, definition timerStorageDefinition, reason timerStorageDiagnosticReason) error {
	epoch, err := prepareTimerEpoch(timerEpochUnixNano)
	if err != nil {
		return err
	}
	queue, err := newTimerNativeSemanticQueue(newTimerNativeFactory(driver), epoch)
	if err != nil {
		return err
	}
	switch definition.ID {
	case timerStorageInit:
		return verifyTimerStorageInit(queue)
	case timerStorageDistinctDrain:
		parameters := definition.Parameters.(timerStorageDrainParameters)
		return verifyTimerStorageDrain(queue, driver, parameters, false)
	case timerStorageEqualDrain:
		parameters := definition.Parameters.(timerStorageEqualParameters)
		return verifyTimerStorageEqual(queue, driver, parameters, reason != "")
	case timerStorageSameMillisecond:
		parameters := definition.Parameters.(timerStorageDrainParameters)
		return verifyTimerStorageDrain(queue, driver, parameters, false)
	case timerStorageCancelOne:
		return verifyTimerStorageCancel(queue, driver, definition.Parameters.(timerStorageCancelParameters))
	case timerStorageMixedSteady:
		return verifyTimerStorageMixed(queue, driver, definition.Parameters.(timerStorageMixedParameters))
	case timerStorageEligibilityBypass:
		return verifyTimerStorageEligibility(queue, driver, definition.Parameters.(timerStorageEligibilityParameters), reason)
	case timerStorageRepeatOnce:
		return verifyTimerStorageRepeat(queue, driver, definition.Parameters.(timerStorageRepeatParameters))
	case timerStorageRetireDrainOnce:
		return verifyTimerStorageEpochOperation(queue, driver, definition.Parameters.(timerStorageEpochOperationParameters), false)
	case timerStoragePublicationReadyDrain:
		return verifyTimerStorageEpochOperation(queue, driver, definition.Parameters.(timerStorageEpochOperationParameters), true)
	case timerStorageReentrantReplacement:
		return verifyTimerStorageReentrant(queue, driver, definition.Parameters.(timerStorageReentrantParameters), reason)
	case timerStorageCallbackPanicContinue:
		return verifyTimerStoragePanic(queue, driver, definition.Parameters.(timerStoragePanicParameters))
	case timerStorageCancelSequenceEqual, timerStorageCancelSequenceDistinct:
		return verifyTimerStorageCancelSequence(queue, driver, definition.Parameters.(timerStorageCancelSequenceParameters))
	case timerStorageNestedRepeatClamp:
		return verifyTimerStorageClamp(queue, driver, definition.Parameters.(timerStorageClampParameters))
	default:
		return fmt.Errorf("unsupported storage workload %q", definition.ID)
	}
}

func verifyTimerStorageInit(queue timerSemanticQueue) error {
	length, err := queue.length()
	if err != nil || length != 0 {
		return fmt.Errorf("initial length = (%d, %v)", length, err)
	}
	if when, ok, err := queue.peek(); err != nil || ok || !when.IsZero() {
		return fmt.Errorf("initial peek = (%v, %v, %v)", when, ok, err)
	}
	stats, err := queue.stats()
	if err != nil || stats != (timerSemanticStats{}) {
		return fmt.Errorf("initial stats = (%+v, %v)", stats, err)
	}
	return nil
}

func verifyTimerStorageDrain(queue timerSemanticQueue, driver timerNativeDriverID, parameters timerStorageDrainParameters, unstable bool) error {
	epoch := time.Unix(0, parameters.EpochUnixNano)
	order := make([]int, 0, len(parameters.OffsetsNS))
	for index, offset := range parameters.OffsetsNS {
		if _, err := queue.insert(timerStorageSemanticInsert(driver, epoch.Add(time.Duration(offset)), func() { order = append(order, index) }, nil)); err != nil {
			return err
		}
	}
	if when, ok, err := queue.peek(); err != nil || !ok || !when.Equal(epoch.Add(time.Duration(parameters.OffsetsNS[0]))) {
		return fmt.Errorf("pre-drain peek = (%v, %v, %v)", when, ok, err)
	}
	result, err := queue.drain(timerSemanticDrain{now: epoch.Add(time.Duration(parameters.DrainOffsetNS))})
	if err != nil || result != (timerSemanticResult{executed: len(parameters.OffsetsNS)}) {
		return fmt.Errorf("drain result = (%+v, %v)", result, err)
	}
	want := []int{0, 1, 2}
	if unstable {
		want = []int{0, 2, 1}
	}
	if !reflect.DeepEqual(order, want) {
		return fmt.Errorf("drain order = %v, want %v", order, want)
	}
	return verifyTimerSemanticReleased(queue)
}

func verifyTimerStorageEqual(queue timerSemanticQueue, driver timerNativeDriverID, parameters timerStorageEqualParameters, unstable bool) error {
	epoch := time.Unix(0, parameters.EpochUnixNano)
	order := make([]int, 0, parameters.Count)
	for index := 0; index < int(parameters.Count); index++ {
		index := index
		if _, err := queue.insert(timerStorageSemanticInsert(driver, epoch.Add(time.Duration(parameters.DeadlineNS)), func() { order = append(order, index) }, nil)); err != nil {
			return err
		}
	}
	result, err := queue.drain(timerSemanticDrain{now: epoch.Add(time.Duration(parameters.DrainOffsetNS))})
	if err != nil || result != (timerSemanticResult{executed: int(parameters.Count)}) {
		return fmt.Errorf("equal drain = (%+v, %v)", result, err)
	}
	want := []int{0, 1, 2}
	if unstable {
		want = []int{0, 2, 1}
	}
	if !reflect.DeepEqual(order, want) {
		return fmt.Errorf("equal order = %v, want %v", order, want)
	}
	return verifyTimerSemanticReleased(queue)
}

func verifyTimerStorageCancel(queue timerSemanticQueue, driver timerNativeDriverID, parameters timerStorageCancelParameters) error {
	epoch := time.Unix(0, parameters.EpochUnixNano)
	called, retired := 0, 0
	handle, err := queue.insert(timerStorageSemanticInsert(driver, epoch.Add(time.Duration(parameters.DeadlineNS)), func() { called++ }, func() { retired++ }))
	if err != nil {
		return err
	}
	if err := queue.cancel(handle); err != nil {
		return err
	}
	if err := queue.cancel(handle); !errors.Is(err, component.ErrTimerMissing) {
		return fmt.Errorf("second cancel = %v", err)
	}
	if called != 0 || (timerDriverRetires(driver) && retired != 1) {
		return fmt.Errorf("cancel callback/retire = (%d, %d)", called, retired)
	}
	return verifyTimerSemanticReleased(queue)
}

func verifyTimerStorageMixed(queue timerSemanticQueue, driver timerNativeDriverID, parameters timerStorageMixedParameters) error {
	epoch := time.Unix(0, parameters.EpochUnixNano)
	order := make([]int, 0, len(parameters.OffsetsNS))
	for index, offset := range parameters.OffsetsNS {
		if _, err := queue.insert(timerStorageSemanticInsert(driver, epoch.Add(time.Duration(offset)), func() { order = append(order, index) }, nil)); err != nil {
			return err
		}
	}
	for index, offset := range parameters.DrainOffsets {
		result, err := queue.drain(timerSemanticDrain{now: epoch.Add(time.Duration(offset))})
		if err != nil || result != (timerSemanticResult{executed: int(parameters.Executed[index])}) {
			return fmt.Errorf("mixed drain %d = (%+v, %v)", index, result, err)
		}
	}
	if !reflect.DeepEqual(order, []int{0, 1, 2, 3}) {
		return fmt.Errorf("mixed order = %v", order)
	}
	return verifyTimerSemanticReleased(queue)
}

func verifyTimerStorageEligibility(queue timerSemanticQueue, driver timerNativeDriverID, parameters timerStorageEligibilityParameters, reason timerStorageDiagnosticReason) error {
	epoch := time.Unix(0, parameters.EpochUnixNano)
	order := make([]int, 0, 2)
	for index := range parameters.DeadlineOffsets {
		input := timerStorageSemanticInsert(driver, epoch.Add(time.Duration(parameters.DeadlineOffsets[index])), func() { order = append(order, index) }, nil)
		input.earliestTick = parameters.EarliestTicks[index]
		input.scheduledTick = parameters.ScheduledTicks[index]
		input.deferTick = parameters.DeferTicks[index]
		if _, err := queue.insert(input); err != nil {
			return err
		}
	}
	first, err := queue.drain(timerSemanticDrain{now: epoch.Add(time.Duration(parameters.DrainOffsetNS)), tick: parameters.Tick})
	if err != nil {
		return err
	}
	if reason == timerStorageDiagnosticIneligibleStall {
		if first != (timerSemanticResult{}) || len(order) != 0 {
			return fmt.Errorf("stall result/order = (%+v, %v)", first, order)
		}
	} else if first != (timerSemanticResult{executed: 1, deferred: 1}) || !reflect.DeepEqual(order, []int{1}) {
		return fmt.Errorf("bypass result/order = (%+v, %v)", first, order)
	}
	second, err := queue.drain(timerSemanticDrain{now: epoch.Add(time.Duration(parameters.DrainOffsetNS)), tick: parameters.Tick + 1})
	if reason == timerStorageDiagnosticIneligibleStall {
		if err != nil || second.executed != 2 || !reflect.DeepEqual(order, []int{0, 1}) {
			return fmt.Errorf("stall recovery drain/order = (%+v, %v, %v)", second, order, err)
		}
	} else if err != nil || second.executed != 1 || !reflect.DeepEqual(order, []int{1, 0}) {
		return fmt.Errorf("second eligibility drain/order = (%+v, %v, %v)", second, order, err)
	}
	return verifyTimerSemanticReleased(queue)
}

func verifyTimerStorageRepeat(queue timerSemanticQueue, driver timerNativeDriverID, parameters timerStorageRepeatParameters) error {
	epoch := time.Unix(0, parameters.EpochUnixNano)
	called, retired := 0, 0
	input := timerStorageSemanticInsert(driver, epoch.Add(time.Duration(parameters.DeadlineOffsetNS)), func() { called++ }, func() { retired++ })
	input.interval = time.Duration(parameters.IntervalNS)
	input.repeat = true
	handle, err := queue.insert(input)
	if err != nil {
		return err
	}
	result, err := queue.drain(timerSemanticDrain{now: epoch.Add(time.Duration(parameters.DrainOffsetNS)), repeatNow: epoch.Add(time.Duration(parameters.RepeatNowNS)), tick: parameters.Tick})
	if err != nil || result != (timerSemanticResult{executed: 1, repeated: 1}) || called != 1 {
		return fmt.Errorf("repeat drain = (%+v, %d, %v)", result, called, err)
	}
	when, ok, err := queue.peek()
	if err != nil || !ok || !when.Equal(epoch.Add(time.Duration(parameters.RepeatNowNS+parameters.IntervalNS))) {
		return fmt.Errorf("repeat peek = (%v, %v, %v)", when, ok, err)
	}
	if err := queue.cancel(handle); err != nil || timerDriverRetires(driver) && retired != 1 {
		return fmt.Errorf("repeat cleanup = (%d, %v)", retired, err)
	}
	return verifyTimerSemanticReleased(queue)
}

func verifyTimerStorageEpochOperation(queue timerSemanticQueue, driver timerNativeDriverID, parameters timerStorageEpochOperationParameters, publicationOnly bool) error {
	epoch := time.Unix(0, parameters.EpochUnixNano)
	called, retired, before := 0, 0, 0
	input := timerStorageSemanticInsert(driver, epoch.Add(time.Duration(parameters.DeadlineOffsetNS)), func() { called++ }, func() { retired++ })
	if publicationOnly && driver != timerNativeBucketCurrent {
		return fmt.Errorf("publication workload reached %s", driver)
	}
	if _, err := queue.insert(input); err != nil {
		return err
	}
	result, err := queue.drain(timerSemanticDrain{
		now: epoch.Add(time.Duration(parameters.DrainOffsetNS)),
		beforePublish: func(uint64) {
			before++
			if called != 0 {
				panic("timer callback ran before publication boundary")
			}
		},
	})
	wantBefore := 0
	if driver == timerNativeBucketCurrent {
		wantBefore = 1
	}
	if err != nil || result != (timerSemanticResult{executed: 1}) || called != 1 || before != wantBefore || retired != 1 {
		return fmt.Errorf("epoch operation = (%+v, called=%d retired=%d before=%d err=%v)", result, called, retired, before, err)
	}
	return verifyTimerSemanticReleased(queue)
}

func verifyTimerStorageReentrant(queue timerSemanticQueue, driver timerNativeDriverID, parameters timerStorageReentrantParameters, reason timerStorageDiagnosticReason) error {
	epoch := time.Unix(0, parameters.EpochUnixNano)
	var active, replacement uint64
	var callbackErr error
	active, callbackErr = queue.insert(timerStorageSemanticInsert(driver, epoch.Add(time.Duration(parameters.DeadlineOffsetNS)), func() {
		replacement, callbackErr = queue.insert(timerStorageSemanticInsert(driver, epoch.Add(time.Duration(parameters.ReplacementOffsetNS)), func() {}, nil))
		if callbackErr == nil {
			callbackErr = queue.cancel(active)
		}
	}, nil))
	if callbackErr != nil {
		return callbackErr
	}
	result, err := queue.drain(timerSemanticDrain{now: epoch.Add(time.Duration(parameters.DrainOffsetNS))})
	if err != nil || result.executed != 1 || callbackErr != nil {
		return fmt.Errorf("reentrant drain = (%+v, %v, %v)", result, callbackErr, err)
	}
	stats, err := queue.stats()
	if err != nil {
		return err
	}
	if reason == timerStorageDiagnosticStalePopIndex {
		if stats.active != 0 || stats.mapEntries != 1 || stats.retainedCallbacks != 1 {
			return fmt.Errorf("stale-index stats = %+v", stats)
		}
		return nil
	}
	if stats.active != 1 || stats.mapEntries != 1 {
		return fmt.Errorf("replacement stats = %+v", stats)
	}
	if err := queue.cancel(replacement); err != nil {
		return err
	}
	return verifyTimerSemanticReleased(queue)
}

func verifyTimerStoragePanic(queue timerSemanticQueue, driver timerNativeDriverID, parameters timerStoragePanicParameters) error {
	epoch := time.Unix(0, parameters.EpochUnixNano)
	order := make([]int, 0, 2)
	if _, err := queue.insert(timerStorageSemanticInsert(driver, epoch.Add(time.Duration(parameters.OffsetsNS[0])), func() { order = append(order, 0); panic("sentinel") }, nil)); err != nil {
		return err
	}
	if _, err := queue.insert(timerStorageSemanticInsert(driver, epoch.Add(time.Duration(parameters.OffsetsNS[1])), func() { order = append(order, 1) }, nil)); err != nil {
		return err
	}
	result, err := queue.drain(timerSemanticDrain{now: epoch.Add(time.Duration(parameters.DrainOffsetNS))})
	if err != nil || result != (timerSemanticResult{executed: 2, panics: 1}) || !reflect.DeepEqual(order, []int{0, 1}) {
		return fmt.Errorf("panic drain = (%+v, %v, %v)", result, order, err)
	}
	return verifyTimerSemanticReleased(queue)
}

func verifyTimerStorageCancelSequence(queue timerSemanticQueue, driver timerNativeDriverID, parameters timerStorageCancelSequenceParameters) error {
	epoch := time.Unix(0, parameters.EpochUnixNano)
	handles := [64]uint64{}
	called, retired := 0, 0
	for index, offset := range parameters.OffsetsNS {
		handle, err := queue.insert(timerStorageSemanticInsert(driver, epoch.Add(time.Duration(offset)), func() { called++ }, func() { retired++ }))
		if err != nil {
			return err
		}
		handles[index] = handle
	}
	for _, index := range parameters.Order {
		if err := queue.cancel(handles[index]); err != nil {
			return err
		}
	}
	if called != 0 || timerDriverRetires(driver) && retired != len(handles) {
		return fmt.Errorf("cancel sequence callback/retire = (%d, %d)", called, retired)
	}
	return verifyTimerSemanticReleased(queue)
}

func verifyTimerStorageClamp(queue timerSemanticQueue, driver timerNativeDriverID, parameters timerStorageClampParameters) error {
	epoch := time.Unix(0, parameters.EpochUnixNano)
	input := timerStorageSemanticInsert(driver, epoch.Add(time.Duration(parameters.DeadlineOffsetNS)), func() {}, nil)
	input.interval = time.Duration(parameters.IntervalNS)
	input.nesting = parameters.ScheduledNesting
	input.nestedClamp = true
	input.repeat = true
	handle, err := queue.insert(input)
	if err != nil {
		return err
	}
	result, err := queue.drain(timerSemanticDrain{now: epoch.Add(time.Duration(parameters.DrainOffsetNS)), repeatNow: epoch.Add(time.Duration(parameters.RepeatNowOffsetNS)), tick: parameters.Tick, currentNesting: parameters.DrainNesting})
	if err != nil || result != (timerSemanticResult{executed: 1, repeated: 1}) {
		return fmt.Errorf("clamp drain = (%+v, %v)", result, err)
	}
	when, ok, err := queue.peek()
	if err != nil || !ok || !when.Equal(epoch.Add(time.Duration(parameters.RepeatNowOffsetNS)+4*time.Millisecond)) {
		return fmt.Errorf("clamp peek = (%v, %v, %v)", when, ok, err)
	}
	if err := queue.cancel(handle); err != nil {
		return err
	}
	return verifyTimerSemanticReleased(queue)
}

func timerStorageSemanticInsert(driver timerNativeDriverID, when time.Time, task, retire func()) timerSemanticInsert {
	input := timerSemanticInsert{when: when, task: task, retire: retire, refed: true}
	if driver == timerNativeBucketCurrent {
		input.publication = newTimerPreparedPublication().value()
	}
	return input
}

func timerDriverRetires(driver timerNativeDriverID) bool {
	return driver == timerNativeBucketRetire || driver == timerNativeBucketCurrent
}

func verifyTimerSemanticReleased(queue timerSemanticQueue) error {
	length, err := queue.length()
	if err != nil || length != 0 {
		return fmt.Errorf("released length = (%d, %v)", length, err)
	}
	stats, err := queue.stats()
	if err != nil {
		return err
	}
	stats.retainedCallbacks = 0
	stats.retainedPointers = 0
	stats.retainedAnchors = 0
	if stats != (timerSemanticStats{}) {
		return fmt.Errorf("released stats = %+v", stats)
	}
	return nil
}
