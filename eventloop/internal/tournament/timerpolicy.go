package tournament

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

type timerDisposition string

const (
	timerExecute    timerDisposition = "execute"
	timerDiagnostic timerDisposition = "diagnostic"
	timerNA         timerDisposition = "not-applicable"
)

type timerStorageDiagnosticReason string

const (
	timerStorageDiagnosticUnstableEqualOrder timerStorageDiagnosticReason = "unstable-equal-deadline-order"
	timerStorageDiagnosticStalePopIndex      timerStorageDiagnosticReason = "stale-pop-index-reentrant-cancel"
	timerStorageDiagnosticIneligibleStall    timerStorageDiagnosticReason = "ineligible-head-stall"
)

type timerStorageNAReason string

const (
	timerStorageNANoBuckets     timerStorageNAReason = "no-deadline-buckets"
	timerStorageNANoCancel      timerStorageNAReason = "no-indexed-cancel"
	timerStorageNANoEligibility timerStorageNAReason = "no-eligibility-state"
	timerStorageNANoRepeat      timerStorageNAReason = "no-native-repeat"
	timerStorageNANoRetire      timerStorageNAReason = "no-retire-hook"
	timerStorageNANoPublication timerStorageNAReason = "no-publication-gate"
	timerStorageNANoClamp       timerStorageNAReason = "no-nested-repeat-clamp"
)

type timerQualificationReason string

const (
	timerQualificationReasonValueTail      timerQualificationReason = "value-pop-tail-retention"
	timerQualificationReasonDrainRelease   timerQualificationReason = "drain-reference-release"
	timerQualificationReasonValueCleanup   timerQualificationReason = "value-cleanup-releases-tail"
	timerQualificationReasonHeapCleanup    timerQualificationReason = "heap-cleanup-retains-pointers"
	timerQualificationReasonBucketAnchors  timerQualificationReason = "bucket-reset-anchor-retention"
	timerQualificationReasonFullCleanup    timerQualificationReason = "complete-cleanup-release"
	timerQualificationReasonNegativeClamp  timerQualificationReason = "negative-nested-repeat-clamp"
	timerQualificationReasonDetachedCancel timerQualificationReason = "detached-sibling-tail-cancel"
	timerQualificationReasonPanicValueTail timerQualificationReason = "panic-value-tail-retention"
	timerQualificationReasonPanicRelease   timerQualificationReason = "panic-reference-release"
	timerQualificationReasonCancelRelease  timerQualificationReason = "cancel-reference-release"
	timerQualificationReasonMaxSafe        timerQualificationReason = "max-safe-id-boundary"
	timerQualificationReasonSticky         timerQualificationReason = "sticky-uint64-id-boundary"
	timerQualificationReasonWrapCollision  timerQualificationReason = "wrapped-id-live-collision"
	timerQualificationReasonDeferredOrder  timerQualificationReason = "deferred-reentrant-due-order"
	timerQualificationReasonBucketFloor    timerQualificationReason = "bucket-floor-reentrant-order"
)

type timerQualificationNAReason string

const (
	timerQualificationNANoClamp          timerQualificationNAReason = "no-negative-nested-repeat-clamp"
	timerQualificationNANoDetachedCancel timerQualificationNAReason = "no-detached-sibling-cancel"
	timerQualificationNANoCancel         timerQualificationNAReason = "no-indexed-cancel"
	timerQualificationNANoMaxSafe        timerQualificationNAReason = "no-max-safe-id-boundary"
	timerQualificationNANoSticky         timerQualificationNAReason = "no-sticky-uint64-id-boundary"
	timerQualificationNANoWrap           timerQualificationNAReason = "no-wrapped-id-collision"
	timerQualificationNANoDeferral       timerQualificationNAReason = "no-eligibility-deferral"
	timerQualificationNANoBucketFloor    timerQualificationNAReason = "no-deadline-bucket-floor"
)

const (
	timerStorageWorkloadCount       = 15
	timerQualificationWorkloadCount = 11
)

type timerStorageRule struct {
	ID               timerStorageWorkloadID
	Disposition      timerDisposition
	DiagnosticReason timerStorageDiagnosticReason
	NAReason         timerStorageNAReason
}

type timerStoragePolicy struct {
	Rules [timerStorageWorkloadCount]timerStorageRule
}

type timerQualificationRule struct {
	ID          timerQualificationWorkloadID
	Disposition timerDisposition
	Reason      timerQualificationReason
	NAReason    timerQualificationNAReason
}

type timerQualificationPolicy struct {
	Rules [timerQualificationWorkloadCount]timerQualificationRule
}

func newTimerStoragePolicy(driver timerNativeDriverID) timerStoragePolicy {
	matrix, ok := timerStorageMatrix(driver)
	if !ok {
		return timerStoragePolicy{}
	}
	var policy timerStoragePolicy
	for index, id := range allTimerStorageWorkloads {
		policy.Rules[index] = canonicalTimerStorageRule(id, matrix[index])
	}
	return policy
}

func newTimerQualificationPolicy(driver timerNativeDriverID) timerQualificationPolicy {
	matrix, ok := timerQualificationMatrix(driver)
	if !ok {
		return timerQualificationPolicy{}
	}
	var policy timerQualificationPolicy
	for index, id := range allTimerQualificationWorkloads {
		policy.Rules[index] = canonicalTimerQualificationRule(driver, id, matrix[index])
	}
	return policy
}

func timerStorageMatrix(driver timerNativeDriverID) (string, bool) {
	switch driver {
	case timerNativeValueOne, timerNativeValueThree:
		return "EEDNNENNNNNENNN", true
	case timerNativeHeapDeadline:
		return "EEDNEENNNNDEEEN", true
	case timerNativeHeapRef:
		return "EEDNEENNNNEEEEN", true
	case timerNativeHeapStall:
		return "EEDNEEDNNNEEEEN", true
	case timerNativeHeapDefer:
		return "EEDNEEENNNEEEEN", true
	case timerNativeBucket27:
		return "EEEEEEEENNEEEEE", true
	case timerNativeBucketRetire:
		return "EEEEEEEEENEEEEE", true
	case timerNativeBucketCurrent:
		return "EEEEEEEEEEEEEEN", true
	default:
		return "", false
	}
}

func timerQualificationMatrix(driver timerNativeDriverID) (string, bool) {
	switch driver {
	case timerNativeValueOne, timerNativeValueThree:
		return "DDNNDNNNNNN", true
	case timerNativeHeapDeadline, timerNativeHeapRef, timerNativeHeapStall:
		return "DDNNDDDNDNN", true
	case timerNativeHeapDefer:
		return "DDNNDDDNDDN", true
	case timerNativeBucket27, timerNativeBucketRetire:
		return "DDDDDDDNDDD", true
	case timerNativeBucketCurrent:
		return "DDNDDDNDNDD", true
	default:
		return "", false
	}
}

func canonicalTimerStorageRule(id timerStorageWorkloadID, code byte) timerStorageRule {
	rule := timerStorageRule{ID: id}
	switch code {
	case 'E':
		rule.Disposition = timerExecute
	case 'D':
		rule.Disposition = timerDiagnostic
		switch id {
		case timerStorageEqualDrain:
			rule.DiagnosticReason = timerStorageDiagnosticUnstableEqualOrder
		case timerStorageEligibilityBypass:
			rule.DiagnosticReason = timerStorageDiagnosticIneligibleStall
		case timerStorageReentrantReplacement:
			rule.DiagnosticReason = timerStorageDiagnosticStalePopIndex
		}
	case 'N':
		rule.Disposition = timerNA
		switch id {
		case timerStorageSameMillisecond:
			rule.NAReason = timerStorageNANoBuckets
		case timerStorageCancelOne, timerStorageReentrantReplacement,
			timerStorageCancelSequenceEqual, timerStorageCancelSequenceDistinct:
			rule.NAReason = timerStorageNANoCancel
		case timerStorageEligibilityBypass:
			rule.NAReason = timerStorageNANoEligibility
		case timerStorageRepeatOnce:
			rule.NAReason = timerStorageNANoRepeat
		case timerStorageRetireDrainOnce:
			rule.NAReason = timerStorageNANoRetire
		case timerStoragePublicationReadyDrain:
			rule.NAReason = timerStorageNANoPublication
		case timerStorageNestedRepeatClamp:
			rule.NAReason = timerStorageNANoClamp
		}
	}
	return rule
}

func canonicalTimerQualificationRule(driver timerNativeDriverID, id timerQualificationWorkloadID, code byte) timerQualificationRule {
	rule := timerQualificationRule{ID: id}
	switch code {
	case 'D':
		rule.Disposition = timerDiagnostic
		switch id {
		case timerQualificationDrainRetention:
			if driver == timerNativeValueOne || driver == timerNativeValueThree {
				rule.Reason = timerQualificationReasonValueTail
			} else {
				rule.Reason = timerQualificationReasonDrainRelease
			}
		case timerQualificationCleanupRetention:
			switch driver {
			case timerNativeValueOne, timerNativeValueThree:
				rule.Reason = timerQualificationReasonValueCleanup
			case timerNativeHeapDeadline, timerNativeHeapRef, timerNativeHeapStall, timerNativeHeapDefer:
				rule.Reason = timerQualificationReasonHeapCleanup
			case timerNativeBucket27, timerNativeBucketRetire:
				rule.Reason = timerQualificationReasonBucketAnchors
			case timerNativeBucketCurrent:
				rule.Reason = timerQualificationReasonFullCleanup
			}
		case timerQualificationNegativeClamp:
			rule.Reason = timerQualificationReasonNegativeClamp
		case timerQualificationDetachedCancel:
			rule.Reason = timerQualificationReasonDetachedCancel
		case timerQualificationPanicRelease:
			if driver == timerNativeValueOne || driver == timerNativeValueThree {
				rule.Reason = timerQualificationReasonPanicValueTail
			} else {
				rule.Reason = timerQualificationReasonPanicRelease
			}
		case timerQualificationCancelRelease:
			rule.Reason = timerQualificationReasonCancelRelease
		case timerQualificationMaxSafeBoundary:
			rule.Reason = timerQualificationReasonMaxSafe
		case timerQualificationStickyBoundary:
			rule.Reason = timerQualificationReasonSticky
		case timerQualificationWrapCollision:
			rule.Reason = timerQualificationReasonWrapCollision
		case timerQualificationDeferredDue:
			rule.Reason = timerQualificationReasonDeferredOrder
		case timerQualificationBucketFloor:
			rule.Reason = timerQualificationReasonBucketFloor
		}
	case 'N':
		rule.Disposition = timerNA
		switch id {
		case timerQualificationNegativeClamp:
			rule.NAReason = timerQualificationNANoClamp
		case timerQualificationDetachedCancel:
			rule.NAReason = timerQualificationNANoDetachedCancel
		case timerQualificationCancelRelease:
			rule.NAReason = timerQualificationNANoCancel
		case timerQualificationMaxSafeBoundary:
			rule.NAReason = timerQualificationNANoMaxSafe
		case timerQualificationStickyBoundary:
			rule.NAReason = timerQualificationNANoSticky
		case timerQualificationWrapCollision:
			rule.NAReason = timerQualificationNANoWrap
		case timerQualificationDeferredDue:
			rule.NAReason = timerQualificationNANoDeferral
		case timerQualificationBucketFloor:
			rule.NAReason = timerQualificationNANoBucketFloor
		}
	}
	return rule
}

func validateTimerStoragePolicy(driver timerNativeDriverID, policy timerStoragePolicy) error {
	expected := newTimerStoragePolicy(driver)
	if expected == (timerStoragePolicy{}) {
		return fmt.Errorf("invalid native driver %q", driver)
	}
	if policy != expected {
		return fmt.Errorf("policy differs from canonical driver policy")
	}
	return nil
}

func validateTimerQualificationPolicy(driver timerNativeDriverID, policy timerQualificationPolicy) error {
	expected := newTimerQualificationPolicy(driver)
	if expected == (timerQualificationPolicy{}) {
		return fmt.Errorf("invalid native driver %q", driver)
	}
	if policy != expected {
		return fmt.Errorf("policy differs from canonical driver policy")
	}
	return nil
}

type timerStoragePlan struct {
	descriptorID string
	driverID     timerNativeDriverID
	definition   timerStorageDefinition
	rule         timerStorageRule
	seal         [sha256.Size]byte
}

type timerQualificationPlan struct {
	descriptorID string
	driverID     timerNativeDriverID
	definition   timerQualificationDefinition
	rule         timerQualificationRule
	seal         [sha256.Size]byte
}

func resolveTimerStorageWorkload(descriptor timerComponentDescriptor, id timerStorageWorkloadID) (timerStoragePlan, error) {
	if err := validateTimerDescriptorPolicies(descriptor); err != nil {
		return timerStoragePlan{}, err
	}
	definition, ok := canonicalTimerStorageDefinition(id)
	if !ok || !validTimerStorageDefinition(definition) {
		return timerStoragePlan{}, fmt.Errorf("unknown or invalid timer storage workload %q", id)
	}
	index := timerStorageWorkloadIndex(id)
	if index < 0 {
		return timerStoragePlan{}, fmt.Errorf("unknown timer storage workload %q", id)
	}
	plan := timerStoragePlan{
		descriptorID: descriptor.ID,
		driverID:     descriptor.NativeDriver,
		definition:   definition,
		rule:         descriptor.StoragePolicy.Rules[index],
	}
	plan.seal = timerStoragePlanSeal(plan)
	if err := plan.validate(); err != nil {
		return timerStoragePlan{}, err
	}
	return plan, nil
}

func resolveTimerQualificationWorkload(descriptor timerComponentDescriptor, id timerQualificationWorkloadID) (timerQualificationPlan, error) {
	if err := validateTimerDescriptorPolicies(descriptor); err != nil {
		return timerQualificationPlan{}, err
	}
	definition, ok := canonicalTimerQualificationDefinition(id)
	if !ok || !validTimerQualificationDefinition(definition) {
		return timerQualificationPlan{}, fmt.Errorf("unknown or invalid timer qualification workload %q", id)
	}
	index := timerQualificationWorkloadIndex(id)
	if index < 0 {
		return timerQualificationPlan{}, fmt.Errorf("unknown timer qualification workload %q", id)
	}
	plan := timerQualificationPlan{
		descriptorID: descriptor.ID,
		driverID:     descriptor.NativeDriver,
		definition:   definition,
		rule:         descriptor.QualificationPolicy.Rules[index],
	}
	plan.seal = timerQualificationPlanSeal(plan)
	if err := plan.validate(); err != nil {
		return timerQualificationPlan{}, err
	}
	return plan, nil
}

func validateTimerDescriptorPolicies(descriptor timerComponentDescriptor) error {
	driver, ok := canonicalTimerDescriptorDriver(descriptor.ID)
	if !ok || driver != descriptor.NativeDriver {
		return fmt.Errorf("timer descriptor %q has noncanonical native driver %q", descriptor.ID, descriptor.NativeDriver)
	}
	if err := validateTimerStoragePolicy(driver, descriptor.StoragePolicy); err != nil {
		return fmt.Errorf("timer descriptor %q storage policy: %w", descriptor.ID, err)
	}
	if err := validateTimerQualificationPolicy(driver, descriptor.QualificationPolicy); err != nil {
		return fmt.Errorf("timer descriptor %q qualification policy: %w", descriptor.ID, err)
	}
	return nil
}

func (p timerStoragePlan) validate() error {
	driver, ok := canonicalTimerDescriptorDriver(p.descriptorID)
	if !ok || driver != p.driverID {
		return fmt.Errorf("storage plan descriptor and driver binding differs from canonical binding")
	}
	definition, ok := canonicalTimerStorageDefinition(p.definition.ID)
	if !ok || !equalTimerStorageDefinition(p.definition, definition) {
		return fmt.Errorf("storage plan definition differs from canonical definition")
	}
	index := timerStorageWorkloadIndex(p.definition.ID)
	if index < 0 || p.rule != newTimerStoragePolicy(driver).Rules[index] {
		return fmt.Errorf("storage plan rule differs from canonical rule")
	}
	if p.seal != timerStoragePlanSeal(p) {
		return fmt.Errorf("storage plan seal differs from plan content")
	}
	return nil
}

func (p timerQualificationPlan) validate() error {
	driver, ok := canonicalTimerDescriptorDriver(p.descriptorID)
	if !ok || driver != p.driverID {
		return fmt.Errorf("qualification plan descriptor and driver binding differs from canonical binding")
	}
	definition, ok := canonicalTimerQualificationDefinition(p.definition.ID)
	if !ok || !equalTimerQualificationDefinition(p.definition, definition) {
		return fmt.Errorf("qualification plan definition differs from canonical definition")
	}
	index := timerQualificationWorkloadIndex(p.definition.ID)
	if index < 0 || p.rule != newTimerQualificationPolicy(driver).Rules[index] {
		return fmt.Errorf("qualification plan rule differs from canonical rule")
	}
	if p.seal != timerQualificationPlanSeal(p) {
		return fmt.Errorf("qualification plan seal differs from plan content")
	}
	return nil
}

func runTimerStorageWorkload(
	plan timerStoragePlan,
	execute func(timerNativeFactory, timerStorageDefinition) error,
	diagnose func(timerStorageDiagnosticReason, timerStorageDefinition) error,
) error {
	if err := plan.validate(); err != nil {
		return fmt.Errorf("timer storage workload %q: %w", plan.definition.ID, err)
	}
	definition, _ := canonicalTimerStorageDefinition(plan.definition.ID)
	switch plan.rule.Disposition {
	case timerExecute:
		factory := newTimerNativeFactory(plan.driverID)
		if !factory.valid() || execute == nil {
			return fmt.Errorf("timer storage workload %q has no executable route", plan.definition.ID)
		}
		return execute(factory, definition)
	case timerDiagnostic:
		if diagnose == nil {
			return fmt.Errorf("timer storage workload %q has no diagnostic route", plan.definition.ID)
		}
		return diagnose(plan.rule.DiagnosticReason, definition)
	case timerNA:
		return nil
	default:
		return fmt.Errorf("timer storage workload %q has invalid disposition %q", plan.definition.ID, plan.rule.Disposition)
	}
}

func runTimerQualificationWorkload(
	plan timerQualificationPlan,
	diagnose func(timerQualificationReason, timerQualificationDefinition) error,
) error {
	if err := plan.validate(); err != nil {
		return fmt.Errorf("timer qualification workload %q: %w", plan.definition.ID, err)
	}
	if plan.rule.Disposition == timerNA {
		return nil
	}
	if plan.rule.Disposition != timerDiagnostic || diagnose == nil {
		return fmt.Errorf("timer qualification workload %q has no diagnostic route", plan.definition.ID)
	}
	definition, _ := canonicalTimerQualificationDefinition(plan.definition.ID)
	return diagnose(plan.rule.Reason, definition)
}

func timerStorageWorkloadIndex(id timerStorageWorkloadID) int {
	for index, candidate := range allTimerStorageWorkloads {
		if candidate == id {
			return index
		}
	}
	return -1
}

func timerQualificationWorkloadIndex(id timerQualificationWorkloadID) int {
	for index, candidate := range allTimerQualificationWorkloads {
		if candidate == id {
			return index
		}
	}
	return -1
}

func timerStoragePlanSeal(plan timerStoragePlan) [sha256.Size]byte {
	return framedTimerSeal(
		"go-utilpkg/eventloop/tournament/timer-storage-plan/v1",
		plan.descriptorID,
		string(plan.driverID),
		string(plan.definition.ID),
		plan.definition.ParameterSHA256,
		string(plan.rule.Disposition),
		string(plan.rule.DiagnosticReason),
		string(plan.rule.NAReason),
	)
}

func timerQualificationPlanSeal(plan timerQualificationPlan) [sha256.Size]byte {
	return framedTimerSeal(
		"go-utilpkg/eventloop/tournament/timer-qualification-plan/v1",
		plan.descriptorID,
		string(plan.driverID),
		string(plan.definition.ID),
		plan.definition.ParameterSHA256,
		string(plan.rule.Disposition),
		string(plan.rule.Reason),
		string(plan.rule.NAReason),
	)
}

func framedTimerSeal(domain string, fields ...string) [sha256.Size]byte {
	hash := sha256.New()
	var size [8]byte
	for _, field := range append([]string{domain}, fields...) {
		binary.BigEndian.PutUint64(size[:], uint64(len(field)))
		hash.Write(size[:])
		hash.Write([]byte(field))
	}
	var result [sha256.Size]byte
	copy(result[:], hash.Sum(nil))
	return result
}
