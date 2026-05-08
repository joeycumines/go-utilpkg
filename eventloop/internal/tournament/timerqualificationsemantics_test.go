package tournament

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/joeycumines/go-eventloop/internal/tournament/component"
)

func TestTimerQualificationSemanticMatrix(t *testing.T) {
	diagnosed, notApplicable := 0, 0
	for _, descriptor := range timerComponentRegistry {
		for _, workload := range allTimerQualificationWorkloads {
			descriptor, workload := descriptor, workload
			t.Run(descriptor.ID+"/"+string(workload), func(t *testing.T) {
				plan, err := resolveTimerQualificationWorkload(descriptor, workload)
				if err != nil {
					t.Fatal(err)
				}
				routed := 0
				err = runTimerQualificationWorkload(plan, func(reason timerQualificationReason, definition timerQualificationDefinition) error {
					routed++
					diagnosed++
					return verifyTimerQualificationSemantics(descriptor.NativeDriver, reason, definition)
				})
				if err != nil {
					t.Fatal(err)
				}
				if plan.rule.Disposition == timerNA {
					notApplicable++
					if routed != 0 {
						t.Fatalf("N/A workload routed %d callbacks", routed)
					}
				} else if routed != 1 {
					t.Fatalf("diagnostic workload routed %d callbacks, want 1", routed)
				}
			})
		}
	}
	if total := diagnosed + notApplicable; total != len(timerComponentRegistry)*timerQualificationWorkloadCount {
		t.Fatalf("qualification semantic cells = %d+%d=%d", diagnosed, notApplicable, total)
	}
	if diagnosed != 59 || notApplicable != 40 {
		t.Fatalf("qualification semantic dispositions = (%d diagnostic, %d N/A), want (59, 40)", diagnosed, notApplicable)
	}
}

func TestTimerQualificationIdentityPreparation(t *testing.T) {
	epoch, err := prepareTimerEpoch(timerEpochUnixNano)
	if err != nil {
		t.Fatal(err)
	}
	for _, driver := range []timerNativeDriverID{
		timerNativeHeapDeadline,
		timerNativeHeapRef,
		timerNativeHeapStall,
		timerNativeHeapDefer,
		timerNativeBucket27,
		timerNativeBucketRetire,
		timerNativeBucketCurrent,
	} {
		t.Run(string(driver), func(t *testing.T) {
			queue, err := newTimerQualificationSemanticQueue(driver, epoch)
			if err != nil {
				t.Fatal(err)
			}
			if err := queue.seedID(41); err != nil {
				t.Fatal(err)
			}
			var seedErr, readErr error
			input := timerQualificationSemanticInsert(driver, epoch.value(), func() {
				seedErr = queue.seedID(99)
				_, readErr = queue.idSeed()
			}, nil)
			if _, err := queue.insert(input); err != nil {
				t.Fatal(err)
			}
			if _, err := queue.drain(timerSemanticDrain{now: epoch.value(), tick: 1}); err != nil {
				t.Fatal(err)
			}
			if !errors.Is(seedErr, component.ErrTimerBusy) || !errors.Is(readErr, component.ErrTimerBusy) {
				t.Fatalf("drain callback preparation errors = (%v, %v)", seedErr, readErr)
			}
			seed, err := queue.idSeed()
			if err != nil || seed != 42 {
				t.Fatalf("seed after rejected mutation = (%d, %v), want 42", seed, err)
			}
			if err := queue.reset(); err != nil {
				t.Fatal(err)
			}
			if seed, err = queue.idSeed(); err != nil || seed != 42 {
				t.Fatalf("seed after reset = (%d, %v), want 42", seed, err)
			}
		})
	}
}

func verifyTimerQualificationSemantics(driver timerNativeDriverID, reason timerQualificationReason, definition timerQualificationDefinition) error {
	epoch, err := prepareTimerEpoch(timerEpochUnixNano)
	if err != nil {
		return err
	}
	queue, err := newTimerQualificationSemanticQueue(driver, epoch)
	if err != nil {
		return err
	}
	switch definition.ID {
	case timerQualificationDrainRetention:
		return verifyTimerQualificationDrainRetention(queue, driver, reason, definition.Parameters.(timerQualificationDrainRetentionParameters))
	case timerQualificationCleanupRetention:
		return verifyTimerQualificationCleanupRetention(queue, driver, reason, definition.Parameters.(timerQualificationRetentionParameters))
	case timerQualificationNegativeClamp:
		return verifyTimerQualificationNegativeClamp(queue, driver, definition.Parameters.(timerQualificationClampParameters))
	case timerQualificationDetachedCancel:
		return verifyTimerQualificationDetachedCancel(queue, driver, definition.Parameters.(timerQualificationDetachedParameters))
	case timerQualificationPanicRelease:
		return verifyTimerQualificationPanic(queue, driver, reason, definition.Parameters.(timerQualificationPanicParameters))
	case timerQualificationCancelRelease:
		return verifyTimerQualificationCancel(queue, driver, definition.Parameters.(timerQualificationRetentionParameters))
	case timerQualificationMaxSafeBoundary:
		return verifyTimerQualificationMaxSafe(queue, driver, definition.Parameters.(timerQualificationIdentityParameters))
	case timerQualificationStickyBoundary:
		return verifyTimerQualificationSticky(queue, driver, definition.Parameters.(timerQualificationIdentityParameters))
	case timerQualificationWrapCollision:
		return verifyTimerQualificationWrap(queue, driver, definition.Parameters.(timerQualificationIdentityParameters))
	case timerQualificationDeferredDue, timerQualificationBucketFloor:
		return verifyTimerQualificationDeferred(queue, driver, definition.Parameters.(timerQualificationDeferredParameters))
	default:
		return fmt.Errorf("unsupported qualification workload %q", definition.ID)
	}
}

func verifyTimerQualificationDrainRetention(queue timerSemanticQueue, driver timerNativeDriverID, reason timerQualificationReason, parameters timerQualificationDrainRetentionParameters) error {
	epoch := time.Unix(0, parameters.EpochUnixNano)
	called, retired := 0, 0
	if _, err := queue.insert(timerQualificationSemanticInsert(driver, epoch.Add(time.Duration(parameters.DeadlineNS)), func() { called++ }, func() { retired++ })); err != nil {
		return err
	}
	result, err := queue.drain(timerSemanticDrain{now: epoch.Add(time.Duration(parameters.DrainOffsetNS)), tick: 1})
	if err != nil || result.executed != 1 || called != 1 {
		return fmt.Errorf("drain retention result = (%+v, called=%d, err=%v)", result, called, err)
	}
	stats, err := queue.stats()
	if err != nil {
		return err
	}
	if reason == timerQualificationReasonValueTail {
		if stats.active != 0 || stats.retainedCallbacks != 1 {
			return fmt.Errorf("value tail stats = %+v", stats)
		}
	} else if stats.active != 0 || stats.mapEntries != 0 || stats.refed != 0 || stats.retainedCallbacks != 0 || stats.retainedPointers != 0 || stats.retainedAnchors != 0 {
		return fmt.Errorf("drain release stats = %+v", stats)
	}
	if timerDriverRetires(driver) && retired != 1 {
		return fmt.Errorf("drain retire count = %d", retired)
	}
	return nil
}

func verifyTimerQualificationCleanupRetention(queue timerSemanticQueue, driver timerNativeDriverID, reason timerQualificationReason, parameters timerQualificationRetentionParameters) error {
	epoch := time.Unix(0, parameters.EpochUnixNano)
	retired := 0
	if _, err := queue.insert(timerQualificationSemanticInsert(driver, epoch.Add(time.Duration(parameters.DeadlineNS)), func() {}, func() { retired++ })); err != nil {
		return err
	}
	if err := queue.reset(); err != nil {
		return err
	}
	stats, err := queue.stats()
	if err != nil {
		return err
	}
	switch reason {
	case timerQualificationReasonValueCleanup, timerQualificationReasonFullCleanup:
		if stats != (timerSemanticStats{}) {
			return fmt.Errorf("complete cleanup stats = %+v", stats)
		}
	case timerQualificationReasonHeapCleanup:
		if stats.active != 0 || stats.mapEntries != 0 || stats.retainedCallbacks != 0 || stats.retainedPointers != 1 {
			return fmt.Errorf("heap cleanup stats = %+v", stats)
		}
	case timerQualificationReasonBucketAnchors:
		if stats.active != 0 || stats.mapEntries != 0 || stats.listEntries != 0 || stats.retainedCallbacks != 0 || stats.retainedAnchors != 1 {
			return fmt.Errorf("bucket cleanup stats = %+v", stats)
		}
	default:
		return fmt.Errorf("unexpected cleanup reason %q", reason)
	}
	if timerDriverRetires(driver) && retired != 1 {
		return fmt.Errorf("cleanup retire count = %d", retired)
	}
	return nil
}

func verifyTimerQualificationNegativeClamp(queue timerSemanticQueue, driver timerNativeDriverID, parameters timerQualificationClampParameters) error {
	epoch := time.Unix(0, parameters.EpochUnixNano)
	input := timerQualificationSemanticInsert(driver, epoch.Add(time.Duration(parameters.DeadlineOffsetNS)), func() {}, nil)
	input.interval = time.Duration(parameters.IntervalNS)
	input.nesting = parameters.ScheduledNesting
	input.nestedClamp = true
	input.repeat = true
	handle, err := queue.insert(input)
	if err != nil {
		return err
	}
	result, err := queue.drain(timerSemanticDrain{now: epoch.Add(time.Duration(parameters.DrainOffsetNS)), repeatNow: epoch.Add(time.Duration(parameters.RepeatNowOffsetNS)), tick: parameters.Tick, currentNesting: parameters.DrainNesting})
	if err != nil || result.executed != 1 || result.repeated != 1 {
		return fmt.Errorf("negative clamp drain = (%+v, %v)", result, err)
	}
	when, ok, err := queue.peek()
	if err != nil || !ok || !when.Equal(epoch.Add(time.Duration(parameters.RepeatNowOffsetNS)+4*time.Millisecond)) {
		return fmt.Errorf("negative clamp peek = (%v, %v, %v)", when, ok, err)
	}
	return queue.cancel(handle)
}

func verifyTimerQualificationDetachedCancel(queue timerSemanticQueue, driver timerNativeDriverID, parameters timerQualificationDetachedParameters) error {
	epoch := time.Unix(0, parameters.EpochUnixNano)
	order := make([]int, 0, parameters.Count)
	retired := 0
	var cancelErr error
	handles := make([]uint64, parameters.Count)
	for index := 0; index < int(parameters.Count); index++ {
		index := index
		input := timerQualificationSemanticInsert(driver, epoch.Add(time.Duration(parameters.DeadlineOffsetNS)), func() {
			order = append(order, index)
			if index == 0 {
				cancelErr = queue.cancel(handles[parameters.CancelIndex])
			}
		}, func() { retired++ })
		handle, err := queue.insert(input)
		if err != nil {
			return err
		}
		handles[index] = handle
	}
	result, err := queue.drain(timerSemanticDrain{now: epoch.Add(time.Duration(parameters.DrainOffsetNS)), tick: parameters.Tick})
	if err != nil || cancelErr != nil || result.executed != 2 || result.canceled != 1 || !reflect.DeepEqual(order, []int{0, 1}) {
		return fmt.Errorf("detached cancel = (%+v, %v, cancel=%v, drain=%v)", result, order, cancelErr, err)
	}
	if timerDriverRetires(driver) && retired != int(parameters.Count) {
		return fmt.Errorf("detached retire count = %d", retired)
	}
	return verifyTimerQualificationReleased(queue)
}

func verifyTimerQualificationPanic(queue timerSemanticQueue, driver timerNativeDriverID, reason timerQualificationReason, parameters timerQualificationPanicParameters) error {
	epoch := time.Unix(0, parameters.EpochUnixNano)
	retired, follower := 0, 0
	if _, err := queue.insert(timerQualificationSemanticInsert(driver, epoch.Add(time.Duration(parameters.OffsetsNS[0])), func() { panic("sentinel") }, func() { retired++ })); err != nil {
		return err
	}
	if _, err := queue.insert(timerQualificationSemanticInsert(driver, epoch.Add(time.Duration(parameters.OffsetsNS[1])), func() { follower++ }, func() { retired++ })); err != nil {
		return err
	}
	result, err := queue.drain(timerSemanticDrain{now: epoch.Add(time.Duration(parameters.DrainOffsetNS)), tick: 1})
	if err != nil || result.executed != 2 || result.panics != 1 || follower != 1 {
		return fmt.Errorf("panic release = (%+v, follower=%d, err=%v)", result, follower, err)
	}
	stats, err := queue.stats()
	if err != nil {
		return err
	}
	if reason == timerQualificationReasonPanicValueTail {
		if stats.active != 0 || stats.retainedCallbacks != 2 {
			return fmt.Errorf("panic value-tail stats = %+v", stats)
		}
	} else if stats.active != 0 || stats.mapEntries != 0 || stats.refed != 0 || stats.retainedCallbacks != 0 || stats.retainedPointers != 0 || stats.retainedAnchors != 0 {
		return fmt.Errorf("panic release stats = %+v", stats)
	}
	if timerDriverRetires(driver) && retired != 2 {
		return fmt.Errorf("panic retire count = %d", retired)
	}
	return nil
}

func verifyTimerQualificationCancel(queue timerSemanticQueue, driver timerNativeDriverID, parameters timerQualificationRetentionParameters) error {
	epoch := time.Unix(0, parameters.EpochUnixNano)
	retired := 0
	handle, err := queue.insert(timerQualificationSemanticInsert(driver, epoch.Add(time.Duration(parameters.DeadlineNS)), func() {}, func() { retired++ }))
	if err != nil {
		return err
	}
	if err := queue.cancel(handle); err != nil {
		return err
	}
	if timerDriverRetires(driver) && retired != 1 {
		return fmt.Errorf("cancel retire count = %d", retired)
	}
	return verifyTimerQualificationReleased(queue)
}

func verifyTimerQualificationMaxSafe(queue timerSemanticQueue, driver timerNativeDriverID, parameters timerQualificationIdentityParameters) error {
	if err := queue.seedID(parameters.Seed); err != nil {
		return err
	}
	retired := 0
	for attempt := uint8(0); attempt < parameters.Attempts; attempt++ {
		handle, err := queue.insert(timerQualificationSemanticInsert(driver, time.Unix(0, parameters.EpochUnixNano), func() {}, func() { retired++ }))
		if attempt == 0 {
			if err != nil || handle != parameters.Seed+1 {
				return fmt.Errorf("max-safe accepted attempt = (%d, %v)", handle, err)
			}
		} else if !errors.Is(err, component.ErrTimerExhausted) {
			return fmt.Errorf("max-safe rejected attempt %d = %v", attempt, err)
		}
	}
	wantSeed := parameters.Seed + uint64(parameters.Attempts)
	if seed, err := queue.idSeed(); err != nil || seed != wantSeed {
		return fmt.Errorf("max-safe seed = (%d, %v), want %d", seed, err, wantSeed)
	}
	if err := queue.reset(); err != nil {
		return err
	}
	if seed, err := queue.idSeed(); err != nil || seed != wantSeed {
		return fmt.Errorf("max-safe reset seed = (%d, %v), want %d", seed, err, wantSeed)
	}
	if timerDriverRetires(driver) && retired != int(parameters.Attempts) {
		return fmt.Errorf("max-safe retire count = %d", retired)
	}
	return nil
}

func verifyTimerQualificationSticky(queue timerSemanticQueue, driver timerNativeDriverID, parameters timerQualificationIdentityParameters) error {
	if err := queue.seedID(parameters.Seed); err != nil {
		return err
	}
	retired := 0
	for attempt := uint8(0); attempt < parameters.Attempts; attempt++ {
		handle, err := queue.insert(timerQualificationSemanticInsert(driver, time.Unix(0, parameters.EpochUnixNano), func() {}, func() { retired++ }))
		if attempt == 0 {
			if err != nil || handle != math.MaxUint64 {
				return fmt.Errorf("sticky accepted attempt = (%d, %v)", handle, err)
			}
		} else if !errors.Is(err, component.ErrTimerExhausted) {
			return fmt.Errorf("sticky rejected attempt %d = %v", attempt, err)
		}
		if seed, seedErr := queue.idSeed(); seedErr != nil || seed != math.MaxUint64 {
			return fmt.Errorf("sticky seed after attempt %d = (%d, %v)", attempt, seed, seedErr)
		}
	}
	if err := queue.reset(); err != nil {
		return err
	}
	if _, err := queue.insert(timerQualificationSemanticInsert(driver, time.Unix(0, parameters.EpochUnixNano), func() {}, func() { retired++ })); !errors.Is(err, component.ErrTimerExhausted) {
		return fmt.Errorf("sticky reset reopened allocation: %v", err)
	}
	if seed, err := queue.idSeed(); err != nil || seed != math.MaxUint64 {
		return fmt.Errorf("sticky reset seed = (%d, %v)", seed, err)
	}
	if retired != int(parameters.Attempts)+1 {
		return fmt.Errorf("sticky retire count = %d", retired)
	}
	return nil
}

func verifyTimerQualificationWrap(queue timerSemanticQueue, driver timerNativeDriverID, parameters timerQualificationIdentityParameters) error {
	epoch := time.Unix(0, parameters.EpochUnixNano)
	executed, retired := 0, 0
	first, err := queue.insert(timerQualificationSemanticInsert(driver, epoch, func() { executed++ }, func() { retired++ }))
	if err != nil || first != 1 {
		return fmt.Errorf("wrap first handle = (%d, %v)", first, err)
	}
	if err := queue.seedID(parameters.Seed); err != nil {
		return err
	}
	zero, err := queue.insert(timerQualificationSemanticInsert(driver, epoch, func() { executed++ }, func() { retired++ }))
	if err != nil || zero != 0 {
		return fmt.Errorf("wrap zero handle = (%d, %v)", zero, err)
	}
	collision, err := queue.insert(timerQualificationSemanticInsert(driver, epoch, func() { executed++ }, func() { retired++ }))
	if err != nil || collision != 1 {
		return fmt.Errorf("wrap collision handle = (%d, %v)", collision, err)
	}
	stats, err := queue.stats()
	if err != nil || stats.mapEntries != 2 {
		return fmt.Errorf("wrap collision stats = (%+v, %v)", stats, err)
	}
	result, err := queue.drain(timerSemanticDrain{now: epoch, tick: 1})
	if err != nil || result.executed != 3 || executed != 3 {
		return fmt.Errorf("wrap drain = (%+v, executed=%d, err=%v)", result, executed, err)
	}
	if seed, err := queue.idSeed(); err != nil || seed != 1 {
		return fmt.Errorf("wrap final seed = (%d, %v)", seed, err)
	}
	if timerDriverRetires(driver) && retired != 3 {
		return fmt.Errorf("wrap retire count = %d", retired)
	}
	return verifyTimerQualificationReleased(queue)
}

func verifyTimerQualificationDeferred(queue timerSemanticQueue, driver timerNativeDriverID, parameters timerQualificationDeferredParameters) error {
	epoch := time.Unix(0, parameters.EpochUnixNano)
	order := make([]uint8, 0, 3)
	var reentrantErr error
	deferred := timerQualificationSemanticInsert(driver, epoch.Add(time.Duration(parameters.DeferredDeadlineOffsetNS)), func() { order = append(order, 1) }, nil)
	deferred.earliestTick = parameters.DeferredTick
	deferred.scheduledTick = parameters.FirstTick
	deferred.deferTick = true
	if _, err := queue.insert(deferred); err != nil {
		return err
	}
	trigger := timerQualificationSemanticInsert(driver, epoch.Add(time.Duration(parameters.TriggerDeadlineOffsetNS)), func() {
		order = append(order, 2)
		reentrant := timerQualificationSemanticInsert(driver, epoch.Add(time.Duration(parameters.ReentrantDeadlineOffsetNS)), func() { order = append(order, 3) }, nil)
		reentrant.earliestTick = parameters.ReentrantTick
		reentrant.scheduledTick = parameters.FirstTick
		reentrant.deferTick = true
		_, reentrantErr = queue.insert(reentrant)
	}, nil)
	trigger.earliestTick = parameters.TriggerTick
	if _, err := queue.insert(trigger); err != nil {
		return err
	}
	first, err := queue.drain(timerSemanticDrain{now: epoch.Add(time.Duration(parameters.FirstDrainOffsetNS)), tick: parameters.FirstTick})
	if err != nil || reentrantErr != nil || first.executed != int(parameters.FirstResult[0]) || first.deferred != int(parameters.FirstResult[1]) {
		return fmt.Errorf("deferred first drain = (%+v, insert=%v, drain=%v)", first, reentrantErr, err)
	}
	second, err := queue.drain(timerSemanticDrain{now: epoch.Add(time.Duration(parameters.SecondDrainOffsetNS)), tick: parameters.SecondTick})
	if err != nil || second.executed != 2 || !reflect.DeepEqual(order, parameters.ExpectedOrder[:]) {
		return fmt.Errorf("deferred second drain/order = (%+v, %v, %v)", second, order, err)
	}
	return verifyTimerQualificationReleased(queue)
}

func timerQualificationSemanticInsert(driver timerNativeDriverID, when time.Time, task, retire func()) timerSemanticInsert {
	input := timerSemanticInsert{when: when, task: task, retire: retire, refed: true}
	if driver == timerNativeBucketCurrent {
		input.publication = newTimerPreparedPublication().value()
	}
	return input
}

func verifyTimerQualificationReleased(queue timerSemanticQueue) error {
	length, err := queue.length()
	if err != nil || length != 0 {
		return fmt.Errorf("qualification released length = (%d, %v)", length, err)
	}
	stats, err := queue.stats()
	if err != nil || stats != (timerSemanticStats{}) {
		return fmt.Errorf("qualification released stats = (%+v, %v)", stats, err)
	}
	return nil
}
