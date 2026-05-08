package tournament

import (
	"reflect"
	"slices"
	"testing"
)

func TestTimerReferenceRegistry(t *testing.T) {
	descriptors := timerReferenceDescriptors()
	want := map[string]struct {
		storage string
		driver  timerReferenceDriverID
		bits    uint8
		sources int
		archive bool
	}{
		"timer.ref-core.map-swap-int32.v1": {storage: "timer.pointer-ref-v2", driver: timerReferenceInt32, bits: 32, sources: 1},
		"timer.ref-core.map-swap-int64.v1": {storage: "timer.bucket-phase-v2", driver: timerReferenceInt64, bits: 64, sources: 3, archive: true},
	}
	if len(descriptors) != len(want) {
		t.Fatalf("reference descriptors = %d, want %d", len(descriptors), len(want))
	}
	seen := make(map[string]struct{}, len(want))
	for _, descriptor := range descriptors {
		if _, duplicate := seen[descriptor.ID]; duplicate {
			t.Errorf("duplicate reference descriptor %q", descriptor.ID)
		}
		seen[descriptor.ID] = struct{}{}
		expected, ok := want[descriptor.ID]
		if !ok {
			t.Errorf("unexpected reference descriptor %q", descriptor.ID)
			continue
		}
		if descriptor.AlgorithmFamily != "timer.reference-owner-core" ||
			descriptor.SourceStorageID != expected.storage || descriptor.Driver != expected.driver ||
			descriptor.CounterBits != expected.bits || len(descriptor.Sources) != expected.sources {
			t.Errorf("reference descriptor %q identity = %+v", descriptor.ID, descriptor)
		}
		if len(descriptor.MaterializationSources) != 1 || len(descriptor.Adaptations) != 3 {
			t.Errorf("reference descriptor %q materialization/adaptations = (%d, %d), want (1, 3)", descriptor.ID, len(descriptor.MaterializationSources), len(descriptor.Adaptations))
		}
		if descriptor.MaterializationArchive != timerReferenceComponentArchive() {
			t.Errorf("reference descriptor %q materialization archive = %+v", descriptor.ID, descriptor.MaterializationArchive)
		}
		if archived := descriptor.SourceArchive != (timerSourceArchive{}); archived != expected.archive {
			t.Errorf("reference descriptor %q archived = %v, want %v", descriptor.ID, archived, expected.archive)
		}
		if err := validateTimerReferenceDescriptor(descriptor); err != nil {
			t.Errorf("reference descriptor %q: %v", descriptor.ID, err)
		}
		driver, err := newTimerReferenceDriver(descriptor.Driver)
		if err != nil || !driver.valid() {
			t.Errorf("reference driver %q = (%+v, %v)", descriptor.Driver, driver, err)
		}
	}
	if _, err := newTimerReferenceDriver("invalid"); err == nil {
		t.Fatal("reference driver accepted an unknown ID")
	}
	if _, ok := reflect.TypeFor[timerReferenceDriver]().MethodByName("Apply"); ok {
		t.Fatal("closed reference driver exposes timed generic dispatch")
	}
}

func TestTimerReferenceStorageBindings(t *testing.T) {
	if len(timerReferenceStorageBindings) != len(timerComponentRegistry) {
		t.Fatalf("reference bindings = %d, want storage descriptor count %d", len(timerReferenceStorageBindings), len(timerComponentRegistry))
	}
	storage := make(map[string]timerComponentDescriptor, len(timerComponentRegistry))
	for _, descriptor := range timerComponentRegistry {
		storage[descriptor.ID] = descriptor
	}
	descriptors := timerReferenceDescriptors()
	references := make(map[string]timerReferenceDescriptor, len(descriptors))
	for _, descriptor := range descriptors {
		references[descriptor.ID] = descriptor
	}
	seen := make(map[string]struct{}, len(timerReferenceStorageBindings))
	counts := map[timerReferenceBindingDisposition]int{}
	for _, binding := range timerReferenceStorageBindings {
		if _, duplicate := seen[binding.StorageID]; duplicate {
			t.Errorf("duplicate reference binding %q", binding.StorageID)
		}
		seen[binding.StorageID] = struct{}{}
		storageDescriptor, ok := storage[binding.StorageID]
		if !ok {
			t.Errorf("reference binding names unknown storage %q", binding.StorageID)
			continue
		}
		counts[binding.Disposition]++
		switch binding.Disposition {
		case timerReferenceBindingNA:
			if binding.ReferenceID != "" || binding.CanonicalStorageID != "" || binding.Reason != timerReferenceNoBit || binding.NormalizedSource != (componentSourceIdentity{}) || binding.NativeExecutionRequired || slices.Contains(storageDescriptor.Capabilities, timerCapabilityReferenceBit) {
				t.Errorf("invalid N/A reference binding %+v", binding)
			}
		case timerReferenceBindingExecuteCore, timerReferenceBindingNormalizedAlias:
			reference, exists := references[binding.ReferenceID]
			if !exists || binding.CanonicalStorageID != reference.SourceStorageID || !binding.NativeExecutionRequired || !slices.Contains(storageDescriptor.Capabilities, timerCapabilityReferenceBit) {
				t.Errorf("invalid executable reference binding %+v", binding)
			}
			if binding.Disposition == timerReferenceBindingExecuteCore && binding.StorageID != binding.CanonicalStorageID {
				t.Errorf("execute reference binding is not canonical: %+v", binding)
			}
			if binding.Disposition == timerReferenceBindingExecuteCore && binding.NormalizedSource != (componentSourceIdentity{}) {
				t.Errorf("canonical core binding has an alias source: %+v", binding)
			}
			if binding.Disposition == timerReferenceBindingNormalizedAlias && (binding.StorageID == binding.CanonicalStorageID || binding.Reason != timerReferenceNormalizedInt32 || binding.NormalizedSource == (componentSourceIdentity{})) {
				t.Errorf("invalid normalized reference alias %+v", binding)
			}
		default:
			t.Errorf("unknown reference binding disposition %q", binding.Disposition)
		}
	}
	if counts[timerReferenceBindingNA] != 3 || counts[timerReferenceBindingExecuteCore] != 2 || counts[timerReferenceBindingNormalizedAlias] != 4 {
		t.Fatalf("reference binding counts = N/A %d execute-core %d normalized-alias %d, want 3/2/4", counts[timerReferenceBindingNA], counts[timerReferenceBindingExecuteCore], counts[timerReferenceBindingNormalizedAlias])
	}
}

func TestTimerReferenceRegistryReturnsDeepFreshValues(t *testing.T) {
	mutated := timerReferenceDescriptors()[0]
	mutated.Sources[0].Path = "corrupted-source.go"
	mutated.MaterializationSources[0].Path = "corrupted-materialization.go"
	mutated.Adaptations[0] = "corrupted adaptation"
	if err := validateTimerReferenceDescriptor(mutated); err == nil {
		t.Fatal("accepted element-mutated descriptor")
	}
	fresh, ok := timerReferenceDescriptorID(mutated.ID)
	if !ok || fresh.Sources[0].Path != "loop.go" || fresh.MaterializationSources[0].Path != "internal/tournament/component/timerrefint32/core.go" || fresh.Adaptations[0] == "corrupted adaptation" {
		t.Fatalf("canonical registry was mutated through a returned descriptor: %+v", fresh)
	}
}

func TestTimerReferenceArchiveAuthorityShared(t *testing.T) {
	want := timerCandidateArchive()
	storage := timerDescriptor(timerNativeBucketCurrent)
	reference, ok := timerReferenceDescriptorID("timer.ref-core.map-swap-int64.v1")
	if !ok {
		t.Fatal("missing Int64 reference descriptor")
	}
	if !reflect.DeepEqual(storage.SourceArchive, want) || !reflect.DeepEqual(reference.SourceArchive, want) {
		t.Fatalf("candidate archive authority diverged: storage=%+v reference=%+v want=%+v", storage.SourceArchive, reference.SourceArchive, want)
	}
}
