package tournament

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"
)

const (
	timerEpochUnixNano         int64 = 1_700_000_000_000_000_000
	timerParameterDigestDomain       = "go-utilpkg/eventloop/tournament/timer-parameters/v1"
)

type timerStorageWorkloadID string

const (
	timerStorageInit                   timerStorageWorkloadID = "timer.storage.init.v1"
	timerStorageDistinctDrain          timerStorageWorkloadID = "timer.storage.distinct-deadline-drain-3.v1"
	timerStorageEqualDrain             timerStorageWorkloadID = "timer.storage.equal-deadline-drain-3.v1"
	timerStorageSameMillisecond        timerStorageWorkloadID = "timer.storage.same-millisecond-order-3.v1"
	timerStorageCancelOne              timerStorageWorkloadID = "timer.storage.cancel-one.v1"
	timerStorageMixedSteady            timerStorageWorkloadID = "timer.storage.mixed-steady-4.v1"
	timerStorageEligibilityBypass      timerStorageWorkloadID = "timer.storage.eligibility-bypass-2.v1"
	timerStorageRepeatOnce             timerStorageWorkloadID = "timer.storage.repeat-once.v1"
	timerStorageRetireDrainOnce        timerStorageWorkloadID = "timer.storage.retire-drain-once.v2"
	timerStoragePublicationReadyDrain  timerStorageWorkloadID = "timer.storage.publication-ready-drain.v2"
	timerStorageReentrantReplacement   timerStorageWorkloadID = "timer.storage.reentrant-replacement-self-cancel.v2"
	timerStorageCallbackPanicContinue  timerStorageWorkloadID = "timer.storage.callback-panic-continue.v1"
	timerStorageCancelSequenceEqual    timerStorageWorkloadID = "timer.storage.cancel-sequence-equal-64.v1"
	timerStorageCancelSequenceDistinct timerStorageWorkloadID = "timer.storage.cancel-sequence-distinct-64.v1"
	timerStorageNestedRepeatClamp      timerStorageWorkloadID = "timer.storage.nested-repeat-clamp-positive.v2"
)

var allTimerStorageWorkloads = [...]timerStorageWorkloadID{
	timerStorageInit,
	timerStorageDistinctDrain,
	timerStorageEqualDrain,
	timerStorageSameMillisecond,
	timerStorageCancelOne,
	timerStorageMixedSteady,
	timerStorageEligibilityBypass,
	timerStorageRepeatOnce,
	timerStorageRetireDrainOnce,
	timerStoragePublicationReadyDrain,
	timerStorageReentrantReplacement,
	timerStorageCallbackPanicContinue,
	timerStorageCancelSequenceEqual,
	timerStorageCancelSequenceDistinct,
	timerStorageNestedRepeatClamp,
}

type timerStorageParameters interface {
	timerStorageParameters()
}

type timerStorageInitParameters struct {
	EpochUnixNano int64
}

func (timerStorageInitParameters) timerStorageParameters() {}

type timerStorageDrainParameters struct {
	EpochUnixNano int64
	OffsetsNS     [3]int64
	DrainOffsetNS int64
}

func (timerStorageDrainParameters) timerStorageParameters() {}

type timerStorageEqualParameters struct {
	EpochUnixNano int64
	DeadlineNS    int64
	Count         uint8
	DrainOffsetNS int64
}

func (timerStorageEqualParameters) timerStorageParameters() {}

type timerStorageCancelParameters struct {
	EpochUnixNano int64
	DeadlineNS    int64
}

func (timerStorageCancelParameters) timerStorageParameters() {}

type timerStorageMixedParameters struct {
	EpochUnixNano int64
	OffsetsNS     [4]int64
	DrainOffsets  [2]int64
	Executed      [2]uint8
}

func (timerStorageMixedParameters) timerStorageParameters() {}

type timerStorageEligibilityParameters struct {
	EpochUnixNano   int64
	DeadlineOffsets [2]int64
	EarliestTicks   [2]uint64
	ScheduledTicks  [2]uint64
	DeferTicks      [2]bool
	DrainOffsetNS   int64
	Tick            uint64
}

func (timerStorageEligibilityParameters) timerStorageParameters() {}

type timerStorageRepeatParameters struct {
	EpochUnixNano    int64
	DeadlineOffsetNS int64
	DrainOffsetNS    int64
	RepeatNowNS      int64
	IntervalNS       int64
	Tick             uint64
}

func (timerStorageRepeatParameters) timerStorageParameters() {}

type timerStorageReentrantParameters struct {
	EpochUnixNano       int64
	DeadlineOffsetNS    int64
	ReplacementOffsetNS int64
	DrainOffsetNS       int64
}

func (timerStorageReentrantParameters) timerStorageParameters() {}

type timerStorageEpochOperationParameters struct {
	EpochUnixNano    int64
	DeadlineOffsetNS int64
	DrainOffsetNS    int64
}

func (timerStorageEpochOperationParameters) timerStorageParameters() {}

type timerStoragePanicParameters struct {
	EpochUnixNano int64
	OffsetsNS     [2]int64
	DrainOffsetNS int64
}

func (timerStoragePanicParameters) timerStorageParameters() {}

type timerStorageCancelSequenceParameters struct {
	EpochUnixNano int64
	OffsetsNS     [64]int64
	Order         [64]uint8
}

func (timerStorageCancelSequenceParameters) timerStorageParameters() {}

type timerStorageClampParameters struct {
	EpochUnixNano     int64
	DeadlineOffsetNS  int64
	DrainOffsetNS     int64
	RepeatNowOffsetNS int64
	IntervalNS        int64
	ScheduledNesting  int32
	DrainNesting      int32
	Tick              uint64
}

func (timerStorageClampParameters) timerStorageParameters() {}

type timerStorageDefinition struct {
	ID              timerStorageWorkloadID
	Parameters      timerStorageParameters
	ParameterSHA256 string
}

func timerStorageDefinitions() []timerStorageDefinition {
	const (
		microsecond = int64(time.Microsecond)
		millisecond = int64(time.Millisecond)
		hour        = int64(time.Hour)
	)
	equalOffsets := [64]int64{}
	distinctOffsets := [64]int64{}
	order := [64]uint8{}
	for index := range equalOffsets {
		equalOffsets[index] = hour
		distinctOffsets[index] = hour + int64(index)*microsecond
		order[index] = reverseSixBits(uint8(index))
	}
	definitions := []struct {
		id         timerStorageWorkloadID
		parameters timerStorageParameters
	}{
		{timerStorageInit, timerStorageInitParameters{EpochUnixNano: timerEpochUnixNano}},
		{timerStorageDistinctDrain, timerStorageDrainParameters{EpochUnixNano: timerEpochUnixNano, OffsetsNS: [3]int64{millisecond, 2 * millisecond, 3 * millisecond}, DrainOffsetNS: 3 * millisecond}},
		{timerStorageEqualDrain, timerStorageEqualParameters{EpochUnixNano: timerEpochUnixNano, DeadlineNS: millisecond, Count: 3, DrainOffsetNS: millisecond}},
		{timerStorageSameMillisecond, timerStorageDrainParameters{EpochUnixNano: timerEpochUnixNano, OffsetsNS: [3]int64{100 * microsecond, 200 * microsecond, 300 * microsecond}, DrainOffsetNS: 300 * microsecond}},
		{timerStorageCancelOne, timerStorageCancelParameters{EpochUnixNano: timerEpochUnixNano, DeadlineNS: hour}},
		{timerStorageMixedSteady, timerStorageMixedParameters{EpochUnixNano: timerEpochUnixNano, OffsetsNS: [4]int64{0, millisecond, 2 * millisecond, 3 * millisecond}, DrainOffsets: [2]int64{0, 3 * millisecond}, Executed: [2]uint8{1, 3}}},
		{timerStorageEligibilityBypass, timerStorageEligibilityParameters{EpochUnixNano: timerEpochUnixNano, DeadlineOffsets: [2]int64{0, microsecond}, EarliestTicks: [2]uint64{2, 1}, ScheduledTicks: [2]uint64{1, 0}, DeferTicks: [2]bool{true, false}, DrainOffsetNS: millisecond, Tick: 1}},
		{timerStorageRepeatOnce, timerStorageRepeatParameters{EpochUnixNano: timerEpochUnixNano, IntervalNS: millisecond, Tick: 1}},
		{timerStorageRetireDrainOnce, timerStorageEpochOperationParameters{EpochUnixNano: timerEpochUnixNano}},
		{timerStoragePublicationReadyDrain, timerStorageEpochOperationParameters{EpochUnixNano: timerEpochUnixNano}},
		{timerStorageReentrantReplacement, timerStorageReentrantParameters{EpochUnixNano: timerEpochUnixNano, ReplacementOffsetNS: hour}},
		{timerStorageCallbackPanicContinue, timerStoragePanicParameters{EpochUnixNano: timerEpochUnixNano, OffsetsNS: [2]int64{0, microsecond}, DrainOffsetNS: microsecond}},
		{timerStorageCancelSequenceEqual, timerStorageCancelSequenceParameters{EpochUnixNano: timerEpochUnixNano, OffsetsNS: equalOffsets, Order: order}},
		{timerStorageCancelSequenceDistinct, timerStorageCancelSequenceParameters{EpochUnixNano: timerEpochUnixNano, OffsetsNS: distinctOffsets, Order: order}},
		{timerStorageNestedRepeatClamp, timerStorageClampParameters{EpochUnixNano: timerEpochUnixNano, IntervalNS: millisecond, ScheduledNesting: 5, DrainNesting: 6, Tick: 1}},
	}
	result := make([]timerStorageDefinition, len(definitions))
	for index, definition := range definitions {
		result[index] = newTimerStorageDefinition(definition.id, definition.parameters)
	}
	return result
}

func newTimerStorageDefinition(id timerStorageWorkloadID, parameters timerStorageParameters) timerStorageDefinition {
	return timerStorageDefinition{ID: id, Parameters: parameters, ParameterSHA256: timerParametersSHA256("storage", id, parameters)}
}

func canonicalTimerStorageDefinition(id timerStorageWorkloadID) (timerStorageDefinition, bool) {
	for _, definition := range timerStorageDefinitions() {
		if definition.ID == id {
			return definition, true
		}
	}
	return timerStorageDefinition{}, false
}

func validTimerStorageDefinition(definition timerStorageDefinition) bool {
	canonical, ok := canonicalTimerStorageDefinition(definition.ID)
	return ok && equalTimerStorageDefinition(definition, canonical)
}

func equalTimerStorageDefinition(left, right timerStorageDefinition) bool {
	if left.ID != right.ID || left.ParameterSHA256 != right.ParameterSHA256 {
		return false
	}
	switch left.ID {
	case timerStorageInit:
		return equalTimerStorageParameters[timerStorageInitParameters](left.Parameters, right.Parameters)
	case timerStorageDistinctDrain, timerStorageSameMillisecond:
		return equalTimerStorageParameters[timerStorageDrainParameters](left.Parameters, right.Parameters)
	case timerStorageEqualDrain:
		return equalTimerStorageParameters[timerStorageEqualParameters](left.Parameters, right.Parameters)
	case timerStorageCancelOne:
		return equalTimerStorageParameters[timerStorageCancelParameters](left.Parameters, right.Parameters)
	case timerStorageMixedSteady:
		return equalTimerStorageParameters[timerStorageMixedParameters](left.Parameters, right.Parameters)
	case timerStorageEligibilityBypass:
		return equalTimerStorageParameters[timerStorageEligibilityParameters](left.Parameters, right.Parameters)
	case timerStorageRepeatOnce:
		return equalTimerStorageParameters[timerStorageRepeatParameters](left.Parameters, right.Parameters)
	case timerStorageRetireDrainOnce, timerStoragePublicationReadyDrain:
		return equalTimerStorageParameters[timerStorageEpochOperationParameters](left.Parameters, right.Parameters)
	case timerStorageReentrantReplacement:
		return equalTimerStorageParameters[timerStorageReentrantParameters](left.Parameters, right.Parameters)
	case timerStorageCallbackPanicContinue:
		return equalTimerStorageParameters[timerStoragePanicParameters](left.Parameters, right.Parameters)
	case timerStorageCancelSequenceEqual, timerStorageCancelSequenceDistinct:
		return equalTimerStorageParameters[timerStorageCancelSequenceParameters](left.Parameters, right.Parameters)
	case timerStorageNestedRepeatClamp:
		return equalTimerStorageParameters[timerStorageClampParameters](left.Parameters, right.Parameters)
	default:
		return false
	}
}

func equalTimerStorageParameters[T comparable](left, right timerStorageParameters) bool {
	leftValue, leftOK := left.(T)
	rightValue, rightOK := right.(T)
	return leftOK && rightOK && leftValue == rightValue
}

func reverseSixBits(value uint8) uint8 {
	return ((value & 0x01) << 5) |
		((value & 0x02) << 3) |
		((value & 0x04) << 1) |
		((value & 0x08) >> 1) |
		((value & 0x10) >> 3) |
		((value & 0x20) >> 5)
}

type timerQualificationWorkloadID string

const (
	timerQualificationDrainRetention   timerQualificationWorkloadID = "timer.qual.drain-retention.v2"
	timerQualificationCleanupRetention timerQualificationWorkloadID = "timer.qual.cleanup-retention.v1"
	timerQualificationNegativeClamp    timerQualificationWorkloadID = "timer.qual.nested-repeat-clamp-negative.v2"
	timerQualificationDetachedCancel   timerQualificationWorkloadID = "timer.qual.detached-sibling-tail-cancel.v2"
	timerQualificationPanicRelease     timerQualificationWorkloadID = "timer.qual.panic-release.v2"
	timerQualificationCancelRelease    timerQualificationWorkloadID = "timer.qual.cancel-release.v1"
	timerQualificationMaxSafeBoundary  timerQualificationWorkloadID = "timer.qual.id-maxsafe-boundary.v1"
	timerQualificationStickyBoundary   timerQualificationWorkloadID = "timer.qual.id-uint64-sticky-boundary.v1"
	timerQualificationWrapCollision    timerQualificationWorkloadID = "timer.qual.id-wrap-collision.v1"
	timerQualificationDeferredDue      timerQualificationWorkloadID = "timer.qual.deferred-reentrant-due-order.v2"
	timerQualificationBucketFloor      timerQualificationWorkloadID = "timer.qual.bucket-floor-reentrant-order.v1"
)

var allTimerQualificationWorkloads = [...]timerQualificationWorkloadID{
	timerQualificationDrainRetention,
	timerQualificationCleanupRetention,
	timerQualificationNegativeClamp,
	timerQualificationDetachedCancel,
	timerQualificationPanicRelease,
	timerQualificationCancelRelease,
	timerQualificationMaxSafeBoundary,
	timerQualificationStickyBoundary,
	timerQualificationWrapCollision,
	timerQualificationDeferredDue,
	timerQualificationBucketFloor,
}

type timerQualificationParameters interface {
	timerQualificationParameters()
}

type timerQualificationRetentionParameters struct {
	EpochUnixNano int64
	DeadlineNS    int64
}

func (timerQualificationRetentionParameters) timerQualificationParameters() {}

type timerQualificationDrainRetentionParameters struct {
	EpochUnixNano int64
	DeadlineNS    int64
	DrainOffsetNS int64
}

func (timerQualificationDrainRetentionParameters) timerQualificationParameters() {}

type timerQualificationClampParameters struct {
	EpochUnixNano     int64
	DeadlineOffsetNS  int64
	DrainOffsetNS     int64
	RepeatNowOffsetNS int64
	IntervalNS        int64
	ScheduledNesting  int32
	DrainNesting      int32
	Tick              uint64
}

func (timerQualificationClampParameters) timerQualificationParameters() {}

type timerQualificationDetachedParameters struct {
	EpochUnixNano    int64
	DeadlineOffsetNS int64
	DrainOffsetNS    int64
	Tick             uint64
	Count            uint8
	CancelIndex      uint8
}

func (timerQualificationDetachedParameters) timerQualificationParameters() {}

type timerQualificationPanicParameters struct {
	EpochUnixNano int64
	OffsetsNS     [2]int64
	DrainOffsetNS int64
}

func (timerQualificationPanicParameters) timerQualificationParameters() {}

type timerQualificationIdentityParameters struct {
	EpochUnixNano int64
	Seed          uint64
	Attempts      uint8
}

func (timerQualificationIdentityParameters) timerQualificationParameters() {}

type timerQualificationDeferredParameters struct {
	EpochUnixNano             int64
	DeferredDeadlineOffsetNS  int64
	TriggerDeadlineOffsetNS   int64
	ReentrantDeadlineOffsetNS int64
	FirstDrainOffsetNS        int64
	SecondDrainOffsetNS       int64
	DeferredTick              uint64
	TriggerTick               uint64
	ReentrantTick             uint64
	FirstTick                 uint64
	SecondTick                uint64
	ExpectedOrder             [3]uint8
	FirstResult               [2]uint8
}

func (timerQualificationDeferredParameters) timerQualificationParameters() {}

type timerQualificationDefinition struct {
	ID              timerQualificationWorkloadID
	Parameters      timerQualificationParameters
	ParameterSHA256 string
}

func timerQualificationDefinitions() []timerQualificationDefinition {
	const (
		microsecond = int64(time.Microsecond)
		millisecond = int64(time.Millisecond)
		hour        = int64(time.Hour)
	)
	definitions := []struct {
		id         timerQualificationWorkloadID
		parameters timerQualificationParameters
	}{
		{timerQualificationDrainRetention, timerQualificationDrainRetentionParameters{EpochUnixNano: timerEpochUnixNano, DeadlineNS: hour, DrainOffsetNS: hour}},
		{timerQualificationCleanupRetention, timerQualificationRetentionParameters{EpochUnixNano: timerEpochUnixNano, DeadlineNS: hour}},
		{timerQualificationNegativeClamp, timerQualificationClampParameters{EpochUnixNano: timerEpochUnixNano, DeadlineOffsetNS: -millisecond, IntervalNS: -millisecond, ScheduledNesting: 5, DrainNesting: 6, Tick: 1}},
		{timerQualificationDetachedCancel, timerQualificationDetachedParameters{EpochUnixNano: timerEpochUnixNano, Tick: 1, Count: 3, CancelIndex: 2}},
		{timerQualificationPanicRelease, timerQualificationPanicParameters{EpochUnixNano: timerEpochUnixNano, OffsetsNS: [2]int64{0, microsecond}, DrainOffsetNS: microsecond}},
		{timerQualificationCancelRelease, timerQualificationRetentionParameters{EpochUnixNano: timerEpochUnixNano, DeadlineNS: hour}},
		{timerQualificationMaxSafeBoundary, timerQualificationIdentityParameters{EpochUnixNano: timerEpochUnixNano, Seed: 9007199254740990, Attempts: 3}},
		{timerQualificationStickyBoundary, timerQualificationIdentityParameters{EpochUnixNano: timerEpochUnixNano, Seed: ^uint64(0) - 1, Attempts: 3}},
		{timerQualificationWrapCollision, timerQualificationIdentityParameters{EpochUnixNano: timerEpochUnixNano, Seed: ^uint64(0), Attempts: 2}},
		{timerQualificationDeferredDue, timerQualificationDeferredParameters{
			EpochUnixNano:             timerEpochUnixNano,
			DeferredDeadlineOffsetNS:  0,
			TriggerDeadlineOffsetNS:   microsecond,
			ReentrantDeadlineOffsetNS: 0,
			FirstDrainOffsetNS:        microsecond,
			SecondDrainOffsetNS:       microsecond,
			DeferredTick:              2,
			TriggerTick:               1,
			ReentrantTick:             2,
			FirstTick:                 1,
			SecondTick:                2,
			ExpectedOrder:             [3]uint8{2, 1, 3},
			FirstResult:               [2]uint8{1, 2},
		}},
		{timerQualificationBucketFloor, timerQualificationDeferredParameters{
			EpochUnixNano:             timerEpochUnixNano,
			DeferredDeadlineOffsetNS:  500 * microsecond,
			TriggerDeadlineOffsetNS:   100 * microsecond,
			ReentrantDeadlineOffsetNS: 500 * microsecond,
			FirstDrainOffsetNS:        100 * microsecond,
			SecondDrainOffsetNS:       500 * microsecond,
			DeferredTick:              2,
			TriggerTick:               1,
			ReentrantTick:             2,
			FirstTick:                 1,
			SecondTick:                2,
			ExpectedOrder:             [3]uint8{2, 3, 1},
			FirstResult:               [2]uint8{1, 1},
		}},
	}
	result := make([]timerQualificationDefinition, len(definitions))
	for index, definition := range definitions {
		result[index] = timerQualificationDefinition{
			ID: definition.id, Parameters: definition.parameters,
			ParameterSHA256: timerParametersSHA256("qualification", definition.id, definition.parameters),
		}
	}
	return result
}

func canonicalTimerQualificationDefinition(id timerQualificationWorkloadID) (timerQualificationDefinition, bool) {
	for _, definition := range timerQualificationDefinitions() {
		if definition.ID == id {
			return definition, true
		}
	}
	return timerQualificationDefinition{}, false
}

func validTimerQualificationDefinition(definition timerQualificationDefinition) bool {
	canonical, ok := canonicalTimerQualificationDefinition(definition.ID)
	return ok && equalTimerQualificationDefinition(definition, canonical)
}

func equalTimerQualificationDefinition(left, right timerQualificationDefinition) bool {
	if left.ID != right.ID || left.ParameterSHA256 != right.ParameterSHA256 {
		return false
	}
	switch left.ID {
	case timerQualificationDrainRetention:
		return equalTimerQualificationParameters[timerQualificationDrainRetentionParameters](left.Parameters, right.Parameters)
	case timerQualificationCleanupRetention, timerQualificationCancelRelease:
		return equalTimerQualificationParameters[timerQualificationRetentionParameters](left.Parameters, right.Parameters)
	case timerQualificationNegativeClamp:
		return equalTimerQualificationParameters[timerQualificationClampParameters](left.Parameters, right.Parameters)
	case timerQualificationDetachedCancel:
		return equalTimerQualificationParameters[timerQualificationDetachedParameters](left.Parameters, right.Parameters)
	case timerQualificationPanicRelease:
		return equalTimerQualificationParameters[timerQualificationPanicParameters](left.Parameters, right.Parameters)
	case timerQualificationMaxSafeBoundary, timerQualificationStickyBoundary, timerQualificationWrapCollision:
		return equalTimerQualificationParameters[timerQualificationIdentityParameters](left.Parameters, right.Parameters)
	case timerQualificationDeferredDue, timerQualificationBucketFloor:
		return equalTimerQualificationParameters[timerQualificationDeferredParameters](left.Parameters, right.Parameters)
	default:
		return false
	}
}

func equalTimerQualificationParameters[T comparable](left, right timerQualificationParameters) bool {
	leftValue, leftOK := left.(T)
	rightValue, rightOK := right.(T)
	return leftOK && rightOK && leftValue == rightValue
}

func timerParametersSHA256(kind string, id any, parameters any) string {
	payload, err := json.Marshal(parameters)
	if err != nil {
		panic(fmt.Sprintf("marshal canonical timer %s parameters %q: %v", kind, id, err))
	}
	digest := framedTimerSeal(
		timerParameterDigestDomain,
		kind,
		fmt.Sprint(id),
		reflect.TypeOf(parameters).String(),
		string(payload),
	)
	return fmt.Sprintf("%x", digest)
}

type timerPreparedEpoch struct {
	unixNano int64
	prepared bool
}

func prepareTimerEpoch(unixNano int64) (timerPreparedEpoch, error) {
	value := time.Unix(0, unixNano)
	if value.IsZero() {
		return timerPreparedEpoch{}, fmt.Errorf("timer epoch is zero")
	}
	return timerPreparedEpoch{unixNano: unixNano, prepared: true}, nil
}

func (e timerPreparedEpoch) valid() bool { return e.prepared && !time.Unix(0, e.unixNano).IsZero() }

func (e timerPreparedEpoch) value() time.Time {
	if !e.valid() {
		return time.Time{}
	}
	return time.Unix(0, e.unixNano)
}

type timerPreparedPublication struct {
	prepared bool
}

func newTimerPreparedPublication() timerPreparedPublication {
	return timerPreparedPublication{prepared: true}
}

func (p timerPreparedPublication) valid() bool { return p.prepared }

func (p timerPreparedPublication) value() <-chan struct{} {
	if !p.valid() {
		return nil
	}
	publication := make(chan struct{})
	close(publication)
	return publication
}
