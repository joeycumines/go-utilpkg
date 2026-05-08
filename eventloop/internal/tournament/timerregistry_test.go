package tournament

import (
	"errors"
	"reflect"
	"testing"
)

type timerDescriptorExpectation struct {
	family       string
	revision     string
	native       timerNativeDriverID
	capabilities []timerCapability
	sourceCount  int
	archive      bool
}

func TestTimerComponentRegistry(t *testing.T) {
	want := map[string]timerDescriptorExpectation{
		"timer.value-safe-task-v1": {
			family: "timer.value-heap", revision: "27b93ec3.alternate-one-safe-task",
			native: timerNativeValueOne, sourceCount: 2,
			capabilities: []timerCapability{timerCapabilityDiagnostics, timerCapabilityMutexOwnership, timerCapabilityCleanupRelease},
		},
		"timer.value-task-v1": {
			family: "timer.value-heap", revision: "b77a13cf.alternate-three-task",
			native: timerNativeValueThree, sourceCount: 2,
			capabilities: []timerCapability{timerCapabilityDiagnostics, timerCapabilityCleanupRelease},
		},
		"timer.pointer-deadline-v1": {
			family: "timer.indexed-pointer-heap", revision: "506d6643.deadline-stale-index",
			native: timerNativeHeapDeadline, sourceCount: 1,
			capabilities: []timerCapability{timerCapabilityDiagnostics, timerCapabilityIndexedCancel, timerCapabilityMaxSafeID, timerCapabilityDrainRelease},
		},
		"timer.pointer-ref-v2": {
			family: "timer.indexed-pointer-heap", revision: "cc005d72.ref-index-invalidation",
			native: timerNativeHeapRef, sourceCount: 1,
			capabilities: []timerCapability{timerCapabilityDiagnostics, timerCapabilityIndexedCancel, timerCapabilityMaxSafeID, timerCapabilityReferenceBit, timerCapabilityReentrantCancel, timerCapabilityDrainRelease},
		},
		"timer.pointer-tick-stall-v3a": {
			family: "timer.indexed-pointer-heap", revision: "0def02e2.earliest-tick-head-stall",
			native: timerNativeHeapStall, sourceCount: 1,
			capabilities: []timerCapability{timerCapabilityDiagnostics, timerCapabilityIndexedCancel, timerCapabilityMaxSafeID, timerCapabilityReferenceBit, timerCapabilityEarliestTick, timerCapabilityReentrantCancel, timerCapabilityDrainRelease},
		},
		"timer.pointer-tick-defer-v3b": {
			family: "timer.indexed-pointer-heap", revision: "802436f7.pop-defer",
			native: timerNativeHeapDefer, sourceCount: 2,
			capabilities: []timerCapability{timerCapabilityDiagnostics, timerCapabilityIndexedCancel, timerCapabilityMaxSafeID, timerCapabilityReferenceBit, timerCapabilityEarliestTick, timerCapabilityEligibilityBypass, timerCapabilityReentrantCancel, timerCapabilityDrainRelease},
		},
		"timer.bucket-tick-v1": {
			family: "timer.deadline-bucket", revision: "27b93ec3.bucket-earliest-tick",
			native: timerNativeBucket27, sourceCount: 1,
			capabilities: []timerCapability{timerCapabilityDiagnostics, timerCapabilityIndexedCancel, timerCapabilityMaxSafeID, timerCapabilityReferenceBit, timerCapabilityEarliestTick, timerCapabilityEligibilityBypass, timerCapabilityStaticPriorityFIFO, timerCapabilityStaticDeadlineOrder, timerCapabilityRepeat, timerCapabilityReentrantCancel, timerCapabilityDrainRelease},
		},
		"timer.bucket-retire-v1-1": {
			family: "timer.deadline-bucket", revision: "c8e744e4.bucket-retirement",
			native: timerNativeBucketRetire, sourceCount: 1,
			capabilities: []timerCapability{timerCapabilityDiagnostics, timerCapabilityIndexedCancel, timerCapabilityMaxSafeID, timerCapabilityReferenceBit, timerCapabilityEarliestTick, timerCapabilityEligibilityBypass, timerCapabilityStaticPriorityFIFO, timerCapabilityStaticDeadlineOrder, timerCapabilityRepeat, timerCapabilityRetirement, timerCapabilityReentrantCancel, timerCapabilityDrainRelease},
		},
		"timer.bucket-phase-v2": {
			family: "timer.deadline-bucket", revision: "archive-2d6ae645.publication-phase",
			native: timerNativeBucketCurrent, sourceCount: 5, archive: true,
			capabilities: []timerCapability{timerCapabilityDiagnostics, timerCapabilityIndexedCancel, timerCapabilityStickyID, timerCapabilityReferenceBit, timerCapabilityEligibilityBypass, timerCapabilityPhaseEligibility, timerCapabilityStaticPriorityFIFO, timerCapabilityStaticDeadlineOrder, timerCapabilityRepeat, timerCapabilityRetirement, timerCapabilityPublication, timerCapabilityReentrantCancel, timerCapabilityDrainRelease, timerCapabilityCleanupRelease, timerCapabilityCompleteCleanup},
		},
	}
	if len(timerComponentRegistry) != len(want) {
		t.Fatalf("timer descriptors = %d, want %d", len(timerComponentRegistry), len(want))
	}
	seen := make(map[string]struct{}, len(timerComponentRegistry))
	sourceCount := 0
	for _, descriptor := range timerComponentRegistry {
		if _, duplicate := seen[descriptor.ID]; duplicate {
			t.Errorf("duplicate descriptor ID %q", descriptor.ID)
		}
		seen[descriptor.ID] = struct{}{}
		expected, ok := want[descriptor.ID]
		if !ok {
			t.Errorf("unexpected descriptor ID %q", descriptor.ID)
			continue
		}
		if descriptor.AlgorithmFamily != expected.family || descriptor.ImplementationRevision != expected.revision || descriptor.NativeDriver != expected.native {
			t.Errorf("descriptor %q identity = (%q, %q, %q), want (%q, %q, %q)", descriptor.ID, descriptor.AlgorithmFamily, descriptor.ImplementationRevision, descriptor.NativeDriver, expected.family, expected.revision, expected.native)
		}
		if driver, ok := canonicalTimerDescriptorDriver(descriptor.ID); !ok || driver != descriptor.NativeDriver {
			t.Errorf("descriptor %q canonical binding = (%q, %v)", descriptor.ID, driver, ok)
		}
		if descriptor.ExecutionOwnership != timerOwnerSerialized {
			t.Errorf("descriptor %q ownership = %q, want %q", descriptor.ID, descriptor.ExecutionOwnership, timerOwnerSerialized)
		}
		if !reflect.DeepEqual(descriptor.Capabilities, expected.capabilities) {
			t.Errorf("descriptor %q capabilities = %v, want %v", descriptor.ID, descriptor.Capabilities, expected.capabilities)
		}
		if len(descriptor.Sources) != expected.sourceCount {
			t.Errorf("descriptor %q sources = %d, want %d", descriptor.ID, len(descriptor.Sources), expected.sourceCount)
		}
		if archived := descriptor.SourceArchive != (timerSourceArchive{}); archived != expected.archive {
			t.Errorf("descriptor %q archived source = %v, want %v", descriptor.ID, archived, expected.archive)
		}
		sourceCount += len(descriptor.Sources)
		if len(descriptor.Adaptations) != 3 {
			t.Errorf("descriptor %q adaptations = %d, want 3 exact operation-boundary statements", descriptor.ID, len(descriptor.Adaptations))
		}
		if err := validateTimerDescriptorPolicies(descriptor); err != nil {
			t.Errorf("descriptor %q policy: %v", descriptor.ID, err)
		}
		assertTimerDriverShapes(t, descriptor.NativeDriver)
	}
	if sourceCount != 16 {
		t.Fatalf("timer source records = %d, want 16", sourceCount)
	}
	if _, ok := reflect.TypeFor[timerNativeFactory]().MethodByName("New"); ok {
		t.Fatal("timer native factory exposes a generic constructor")
	}
	if _, ok := reflect.TypeFor[timerStoragePlan]().FieldByName("factory"); ok {
		t.Fatal("storage plan retains a mutable native factory")
	}
}

func TestTimerWorkloadMatricesAndRouting(t *testing.T) {
	storageMatrix := map[timerNativeDriverID]string{
		timerNativeValueOne:      "EEDNNENNNNNENNN",
		timerNativeValueThree:    "EEDNNENNNNNENNN",
		timerNativeHeapDeadline:  "EEDNEENNNNDEEEN",
		timerNativeHeapRef:       "EEDNEENNNNEEEEN",
		timerNativeHeapStall:     "EEDNEEDNNNEEEEN",
		timerNativeHeapDefer:     "EEDNEEENNNEEEEN",
		timerNativeBucket27:      "EEEEEEEENNEEEEE",
		timerNativeBucketRetire:  "EEEEEEEEENEEEEE",
		timerNativeBucketCurrent: "EEEEEEEEEEEEEEN",
	}
	qualificationMatrix := map[timerNativeDriverID]string{
		timerNativeValueOne:      "DDNNDNNNNNN",
		timerNativeValueThree:    "DDNNDNNNNNN",
		timerNativeHeapDeadline:  "DDNNDDDNDNN",
		timerNativeHeapRef:       "DDNNDDDNDNN",
		timerNativeHeapStall:     "DDNNDDDNDNN",
		timerNativeHeapDefer:     "DDNNDDDNDDN",
		timerNativeBucket27:      "DDDDDDDNDDD",
		timerNativeBucketRetire:  "DDDDDDDNDDD",
		timerNativeBucketCurrent: "DDNDDDNDNDD",
	}
	storageDefinitions := timerStorageDefinitions()
	qualificationDefinitions := timerQualificationDefinitions()
	if len(storageDefinitions) != timerStorageWorkloadCount || len(allTimerStorageWorkloads) != timerStorageWorkloadCount {
		t.Fatalf("storage definitions/workloads = (%d, %d), want %d", len(storageDefinitions), len(allTimerStorageWorkloads), timerStorageWorkloadCount)
	}
	if len(qualificationDefinitions) != timerQualificationWorkloadCount || len(allTimerQualificationWorkloads) != timerQualificationWorkloadCount {
		t.Fatalf("qualification definitions/workloads = (%d, %d), want %d", len(qualificationDefinitions), len(allTimerQualificationWorkloads), timerQualificationWorkloadCount)
	}
	for index, definition := range storageDefinitions {
		if definition.ID != allTimerStorageWorkloads[index] || !validTimerStorageDefinition(definition) {
			t.Errorf("storage definition %d = %+v, want ordered valid %q", index, definition, allTimerStorageWorkloads[index])
		}
	}
	for index, definition := range qualificationDefinitions {
		if definition.ID != allTimerQualificationWorkloads[index] || !validTimerQualificationDefinition(definition) {
			t.Errorf("qualification definition %d = %+v, want ordered valid %q", index, definition, allTimerQualificationWorkloads[index])
		}
	}

	for _, descriptor := range timerComponentRegistry {
		for index, id := range allTimerStorageWorkloads {
			plan, err := resolveTimerStorageWorkload(descriptor, id)
			if err != nil {
				t.Errorf("resolve storage %s/%s: %v", descriptor.ID, id, err)
				continue
			}
			assertTimerStorageRule(t, descriptor.NativeDriver, plan.rule, storageMatrix[descriptor.NativeDriver][index])
			executions, diagnostics := 0, 0
			sentinel := errors.New("storage route sentinel")
			err = runTimerStorageWorkload(
				plan,
				func(factory timerNativeFactory, definition timerStorageDefinition) error {
					executions++
					canonical, _ := canonicalTimerStorageDefinition(id)
					if factory.ID != descriptor.NativeDriver || !factory.valid() || !equalTimerStorageDefinition(definition, canonical) {
						t.Errorf("invalid storage execute route for %s/%s", descriptor.ID, id)
					}
					return sentinel
				},
				func(reason timerStorageDiagnosticReason, definition timerStorageDefinition) error {
					diagnostics++
					canonical, _ := canonicalTimerStorageDefinition(id)
					if reason != plan.rule.DiagnosticReason || !equalTimerStorageDefinition(definition, canonical) {
						t.Errorf("invalid storage diagnostic route for %s/%s", descriptor.ID, id)
					}
					return sentinel
				},
			)
			assertTimerRouteCounts(t, plan.rule.Disposition, executions, diagnostics, err, sentinel)
		}

		for index, id := range allTimerQualificationWorkloads {
			plan, err := resolveTimerQualificationWorkload(descriptor, id)
			if err != nil {
				t.Errorf("resolve qualification %s/%s: %v", descriptor.ID, id, err)
				continue
			}
			assertTimerQualificationRule(t, descriptor.NativeDriver, plan.rule, qualificationMatrix[descriptor.NativeDriver][index])
			diagnostics := 0
			sentinel := errors.New("qualification route sentinel")
			err = runTimerQualificationWorkload(plan, func(reason timerQualificationReason, definition timerQualificationDefinition) error {
				diagnostics++
				canonical, _ := canonicalTimerQualificationDefinition(id)
				if reason != plan.rule.Reason || !equalTimerQualificationDefinition(definition, canonical) {
					t.Errorf("invalid qualification diagnostic route for %s/%s", descriptor.ID, id)
				}
				return sentinel
			})
			if plan.rule.Disposition == timerDiagnostic {
				if diagnostics != 1 || !errors.Is(err, sentinel) {
					t.Errorf("qualification route %s/%s = (%d, %v)", descriptor.ID, id, diagnostics, err)
				}
			} else if diagnostics != 0 || err != nil {
				t.Errorf("qualification N/A route %s/%s = (%d, %v)", descriptor.ID, id, diagnostics, err)
			}
		}
	}
}

func TestTimerPolicyRejectsCorruption(t *testing.T) {
	descriptor := timerDescriptor(timerNativeBucketCurrent)
	for name, corrupt := range map[string]func(*timerComponentDescriptor){
		"descriptor ID":             func(value *timerComponentDescriptor) { value.ID = "timer.pointer-ref-v2" },
		"valid wrong driver":        func(value *timerComponentDescriptor) { value.NativeDriver = timerNativeBucketRetire },
		"storage disposition":       func(value *timerComponentDescriptor) { value.StoragePolicy.Rules[0].Disposition = timerDiagnostic },
		"storage valid reason":      func(value *timerComponentDescriptor) { value.StoragePolicy.Rules[14].NAReason = timerStorageNANoRepeat },
		"qualification disposition": func(value *timerComponentDescriptor) { value.QualificationPolicy.Rules[0].Disposition = timerNA },
		"qualification valid reason": func(value *timerComponentDescriptor) {
			value.QualificationPolicy.Rules[2].NAReason = timerQualificationNANoDeferral
		},
	} {
		invalid := descriptor
		corrupt(&invalid)
		if _, err := resolveTimerStorageWorkload(invalid, timerStorageInit); err == nil {
			t.Errorf("resolver accepted corrupted %s", name)
		}
		if _, err := resolveTimerQualificationWorkload(invalid, timerQualificationPanicRelease); err == nil {
			t.Errorf("qualification resolver accepted corrupted %s", name)
		}
	}

	executePlan, err := resolveTimerStorageWorkload(descriptor, timerStorageInit)
	if err != nil {
		t.Fatal(err)
	}
	storageCorruptions := map[string]func(*timerStoragePlan){
		"descriptor":   func(plan *timerStoragePlan) { plan.descriptorID = "timer.pointer-ref-v2" },
		"valid driver": func(plan *timerStoragePlan) { plan.driverID = timerNativeBucketRetire },
		"workload ID":  func(plan *timerStoragePlan) { plan.definition.ID = timerStorageDistinctDrain },
		"parameter value": func(plan *timerStoragePlan) {
			plan.definition.Parameters = timerStorageInitParameters{EpochUnixNano: 1}
		},
		"parameter type": func(plan *timerStoragePlan) {
			plan.definition.Parameters = &timerStorageInitParameters{EpochUnixNano: timerEpochUnixNano}
		},
		"parameter digest": func(plan *timerStoragePlan) { plan.definition.ParameterSHA256 = "forged" },
		"disposition":      func(plan *timerStoragePlan) { plan.rule.Disposition = timerDiagnostic },
		"reason":           func(plan *timerStoragePlan) { plan.rule.DiagnosticReason = timerStorageDiagnosticUnstableEqualOrder },
		"seal":             func(plan *timerStoragePlan) { plan.seal[0] ^= 1 },
	}
	for name, corrupt := range storageCorruptions {
		invalid := executePlan
		corrupt(&invalid)
		if err := runTimerStorageWorkload(invalid, func(timerNativeFactory, timerStorageDefinition) error { return nil }, nil); err == nil {
			t.Errorf("runner accepted corrupted storage %s", name)
		}
	}
	if err := runTimerStorageWorkload(executePlan, nil, nil); err == nil {
		t.Fatal("runner accepted executable storage plan without route")
	}

	qualificationPlan, err := resolveTimerQualificationWorkload(descriptor, timerQualificationPanicRelease)
	if err != nil {
		t.Fatal(err)
	}
	qualificationCorruptions := map[string]func(*timerQualificationPlan){
		"descriptor":   func(plan *timerQualificationPlan) { plan.descriptorID = "timer.pointer-ref-v2" },
		"valid driver": func(plan *timerQualificationPlan) { plan.driverID = timerNativeBucketRetire },
		"workload ID":  func(plan *timerQualificationPlan) { plan.definition.ID = timerQualificationCancelRelease },
		"parameter value": func(plan *timerQualificationPlan) {
			plan.definition.Parameters = timerQualificationPanicParameters{EpochUnixNano: 1}
		},
		"parameter type": func(plan *timerQualificationPlan) {
			plan.definition.Parameters = &timerQualificationPanicParameters{EpochUnixNano: timerEpochUnixNano}
		},
		"parameter digest": func(plan *timerQualificationPlan) { plan.definition.ParameterSHA256 = "forged" },
		"disposition":      func(plan *timerQualificationPlan) { plan.rule.Disposition = timerNA },
		"reason":           func(plan *timerQualificationPlan) { plan.rule.Reason = timerQualificationReasonCancelRelease },
		"seal":             func(plan *timerQualificationPlan) { plan.seal[0] ^= 1 },
	}
	for name, corrupt := range qualificationCorruptions {
		invalid := qualificationPlan
		corrupt(&invalid)
		if err := runTimerQualificationWorkload(invalid, func(timerQualificationReason, timerQualificationDefinition) error { return nil }); err == nil {
			t.Errorf("runner accepted corrupted qualification %s", name)
		}
	}
	if err := runTimerQualificationWorkload(qualificationPlan, nil); err == nil {
		t.Fatal("runner accepted diagnostic qualification plan without route")
	}
}

func TestTimerDefinitionsAreValueSealed(t *testing.T) {
	first, err := resolveTimerStorageWorkload(timerDescriptor(timerNativeBucketCurrent), timerStorageInit)
	if err != nil {
		t.Fatal(err)
	}
	second, err := resolveTimerStorageWorkload(timerDescriptor(timerNativeBucketCurrent), timerStorageInit)
	if err != nil {
		t.Fatal(err)
	}
	first.definition.Parameters = timerStorageInitParameters{EpochUnixNano: 1}
	if err := second.validate(); err != nil {
		t.Fatalf("one resolution mutated another: %v", err)
	}
	canonical, ok := canonicalTimerStorageDefinition(timerStorageInit)
	if !ok || !validTimerStorageDefinition(canonical) {
		t.Fatal("one resolution mutated canonical storage definition")
	}

	for _, definition := range timerStorageDefinitions() {
		assertTimerParameterShape(t, definition.ID, reflect.TypeOf(definition.Parameters))
	}
	for _, definition := range timerQualificationDefinitions() {
		assertTimerParameterShape(t, definition.ID, reflect.TypeOf(definition.Parameters))
	}
}

func TestTimerPreparedInputs(t *testing.T) {
	if (timerPreparedEpoch{}).valid() {
		t.Fatal("zero epoch token is valid")
	}
	epoch, err := prepareTimerEpoch(timerEpochUnixNano)
	if err != nil || !epoch.valid() || epoch.value().UnixNano() != timerEpochUnixNano {
		t.Fatalf("prepared epoch = (%+v, %v)", epoch, err)
	}
	if _, err := newTimerNativeDriver(timerNativeBucket27, timerPreparedEpoch{}); err == nil {
		t.Fatal("native driver accepted unprepared epoch")
	}
	if _, err := newTimerQualificationDriver(timerNativeBucket27, timerPreparedEpoch{}); err == nil {
		t.Fatal("qualification driver accepted unprepared epoch")
	}
	if _, err := newTimerNativeDriver("invalid", epoch); err == nil {
		t.Fatal("native constructor accepted unknown driver")
	}
	if _, err := newTimerQualificationDriver("invalid", epoch); err == nil {
		t.Fatal("qualification constructor accepted unknown driver")
	}

	prepared := newTimerPreparedPublication()
	if !prepared.valid() || (timerPreparedPublication{}).valid() {
		t.Fatal("publication token validity is wrong")
	}
	first, second := prepared.value(), prepared.value()
	if first == nil || second == nil || first == second || cap(first) != 0 || cap(second) != 0 {
		t.Fatalf("prepared publications = (%v, %v)", first, second)
	}
	for index, publication := range []<-chan struct{}{first, second} {
		select {
		case _, open := <-publication:
			if open {
				t.Errorf("publication %d is open", index)
			}
		default:
			t.Errorf("publication %d is not ready", index)
		}
	}
}

func assertTimerDriverShapes(t *testing.T, id timerNativeDriverID) {
	t.Helper()
	epoch, err := prepareTimerEpoch(timerEpochUnixNano)
	if err != nil {
		t.Fatal(err)
	}
	native, err := newTimerNativeDriver(id, epoch)
	if err != nil || !native.valid() {
		t.Errorf("native driver %q = (%+v, %v)", id, native, err)
	}
	qualification, err := newTimerQualificationDriver(id, epoch)
	if err != nil || !qualification.valid() {
		t.Errorf("qualification driver %q = (%+v, %v)", id, qualification, err)
	}
	nativeType := reflect.TypeFor[timerNativeDriver]()
	qualificationType := reflect.TypeFor[timerQualificationDriver]()
	if nativeType.NumField() != 10 || qualificationType.NumField() != 10 {
		t.Errorf("driver field counts = (%d, %d), want 10", nativeType.NumField(), qualificationType.NumField())
	}
	for index := 1; index < nativeType.NumField(); index++ {
		nativeField := nativeType.Field(index)
		qualificationField := qualificationType.Field(index)
		if nativeField.Name != qualificationField.Name || nativeField.Type.Kind() != reflect.Pointer || qualificationField.Type.Kind() != reflect.Pointer {
			t.Errorf("driver field %d shape = (%v, %v)", index, nativeField, qualificationField)
			continue
		}
		if nativeField.Type.Elem().Name() != "Queue" || qualificationField.Type.Elem().Name() != "Qualification" {
			t.Errorf("driver field %s types = (%v, %v), want Queue/Qualification", nativeField.Name, nativeField.Type, qualificationField.Type)
		}
		if _, ok := nativeField.Type.MethodByName("Reset"); ok {
			t.Errorf("native field %s exposes Reset", nativeField.Name)
		}
	}
}

func timerDescriptor(id timerNativeDriverID) timerComponentDescriptor {
	for _, descriptor := range timerComponentRegistry {
		if descriptor.NativeDriver == id {
			return descriptor
		}
	}
	return timerComponentDescriptor{}
}

func assertTimerStorageRule(t *testing.T, driver timerNativeDriverID, rule timerStorageRule, code byte) {
	t.Helper()
	if code == 'E' && (rule.Disposition != timerExecute || rule.DiagnosticReason != "" || rule.NAReason != "") {
		t.Errorf("storage execute rule %s/%s = %+v", driver, rule.ID, rule)
	}
	if code == 'D' && (rule.Disposition != timerDiagnostic || rule.DiagnosticReason == "" || rule.NAReason != "") {
		t.Errorf("storage diagnostic rule %s/%s = %+v", driver, rule.ID, rule)
	}
	if code == 'N' && (rule.Disposition != timerNA || rule.DiagnosticReason != "" || rule.NAReason == "") {
		t.Errorf("storage N/A rule %s/%s = %+v", driver, rule.ID, rule)
	}
}

func assertTimerQualificationRule(t *testing.T, driver timerNativeDriverID, rule timerQualificationRule, code byte) {
	t.Helper()
	if code == 'D' && (rule.Disposition != timerDiagnostic || rule.Reason == "" || rule.NAReason != "") {
		t.Errorf("qualification diagnostic rule %s/%s = %+v", driver, rule.ID, rule)
	}
	if code == 'N' && (rule.Disposition != timerNA || rule.Reason != "" || rule.NAReason == "") {
		t.Errorf("qualification N/A rule %s/%s = %+v", driver, rule.ID, rule)
	}
}

func assertTimerRouteCounts(t *testing.T, disposition timerDisposition, executions, diagnostics int, err, sentinel error) {
	t.Helper()
	switch disposition {
	case timerExecute:
		if executions != 1 || diagnostics != 0 || !errors.Is(err, sentinel) {
			t.Errorf("execute route = (%d, %d, %v)", executions, diagnostics, err)
		}
	case timerDiagnostic:
		if executions != 0 || diagnostics != 1 || !errors.Is(err, sentinel) {
			t.Errorf("diagnostic route = (%d, %d, %v)", executions, diagnostics, err)
		}
	case timerNA:
		if executions != 0 || diagnostics != 0 || err != nil {
			t.Errorf("N/A route = (%d, %d, %v)", executions, diagnostics, err)
		}
	}
}

func assertTimerParameterShape(t *testing.T, id any, parameterType reflect.Type) {
	t.Helper()
	if parameterType == nil || parameterType.Kind() != reflect.Struct {
		t.Fatalf("parameter %q type = %v, want concrete struct", id, parameterType)
	}
	var visit func(reflect.Type)
	visit = func(valueType reflect.Type) {
		switch valueType.Kind() {
		case reflect.Array:
			visit(valueType.Elem())
		case reflect.Struct:
			for field := range valueType.Fields() {
				visit(field.Type)
			}
		case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
			reflect.String:
		default:
			t.Errorf("parameter %q contains reference-bearing or unsupported type %v", id, valueType)
		}
	}
	visit(parameterType)
}
