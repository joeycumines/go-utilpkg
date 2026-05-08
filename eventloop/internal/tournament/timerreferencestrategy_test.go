package tournament

import (
	"reflect"
	"testing"
)

func TestTimerReferenceStrategyDescriptors(t *testing.T) {
	wantArchive := timerReferenceMaterializationArchive{
		PatchPath:         "revisions/candidates/0005-timer-reference-considered-strategies.patch",
		PatchSHA256:       "7c090dafd52ab00ab8dfa35dbda1a7111957791cba4e48ea7b039e10d6a913c4",
		PatchBytes:        23487,
		EmptyTree:         "4b825dc642cb6eb9a060e54bf8d69288fbee4904",
		ReconstructedTree: "a474db0f740db26f05e40f54dc09d405550c3e78",
	}
	want := []struct {
		id           string
		revision     string
		function     string
		path         string
		blob         string
		sha256       string
		topology     timerReferenceStrategyTopology
		sourceCount  int
		needsBarrier bool
	}{
		{
			id: "timer.ref-strategy.owner-direct-submit-int32.v1", revision: "802436f7.owner-direct-external-submit",
			function: "refViaIsLoopThread", path: "internal/tournament/component/timerrefownersubmit/core.go",
			blob: "bad855f6c4281b1eecf993068dcf11301b89b984", sha256: "986a0aa238a5e562f1f0fbf107c64863ae47d5471e0cbc3b52b16c88a98c7c04",
			topology: timerReferenceOwnerSubmit, sourceCount: 4,
		},
		{
			id: "timer.ref-strategy.always-submit-int32.v1", revision: "802436f7.always-submit",
			function: "refViaSubmitInternal", path: "internal/tournament/component/timerrefalwayssubmit/core.go",
			blob: "0bbfd8572c2334f8f5cb7640ecc43194981574a4", sha256: "cff807587d2a7b7a647f667b30598529b1a6fe6e0a4f4b74696036e2a4bdef8e",
			topology: timerReferenceAlwaysSubmit, sourceCount: 4,
		},
		{
			id: "timer.ref-strategy.sync-map-int32.v1", revision: "802436f7.sync-map",
			function: "refViaSyncMap", path: "internal/tournament/component/timerrefsyncmap/core.go",
			blob: "55c6fe6cf871c458852afe0b715bf42ee3d80562", sha256: "352130445736d2356beadd3d044f72880b73333a0954ba64c75519d91ec6f874",
			topology: timerReferenceSyncMap, sourceCount: 2,
		},
		{
			id: "timer.ref-strategy.rwmutex-map-int32.v1", revision: "802436f7.rwmutex-map",
			function: "refViaRWMutex", path: "internal/tournament/component/timerrefrwmutex/core.go",
			blob: "b035cf708c93e5440fbccbc30d4012528f249300", sha256: "194449ee0f317d0fbff6b97532b803cf7ce5a0daa520d171b62529358ccd62f6",
			topology: timerReferenceRWMutex, sourceCount: 2, needsBarrier: true,
		},
	}
	descriptors := timerReferenceStrategyDescriptors()
	if len(descriptors) != len(want) {
		t.Fatalf("strategy descriptor count = %d, want %d", len(descriptors), len(want))
	}
	seals := make(map[[32]byte]string, len(descriptors))
	for index, expected := range want {
		descriptor := descriptors[index]
		if descriptor.ID != expected.id || descriptor.ImplementationRevision != expected.revision || descriptor.SourceFunction != expected.function {
			t.Errorf("strategy descriptor %d identity = (%q, %q, %q)", index, descriptor.ID, descriptor.ImplementationRevision, descriptor.SourceFunction)
		}
		if descriptor.AlgorithmFamily != "timer.reference-considered-strategy" || descriptor.CounterBits != 32 {
			t.Errorf("strategy descriptor %q family/width = (%q, %d)", descriptor.ID, descriptor.AlgorithmFamily, descriptor.CounterBits)
		}
		if descriptor.Topology != expected.topology || descriptor.RequiresRegistrationBarrier != expected.needsBarrier || !descriptor.NativeExecutionRequired {
			t.Errorf("strategy descriptor %q policy = (%q, barrier=%t, native=%t)", descriptor.ID, descriptor.Topology, descriptor.RequiresRegistrationBarrier, descriptor.NativeExecutionRequired)
		}
		if len(descriptor.Sources) != expected.sourceCount || len(descriptor.MaterializationSources) != 1 || len(descriptor.Adaptations) != 3 {
			t.Errorf("strategy descriptor %q relation cardinality = (%d, %d, %d)", descriptor.ID, len(descriptor.Sources), len(descriptor.MaterializationSources), len(descriptor.Adaptations))
			continue
		}
		if descriptor.Sources[0] != (componentSourceIdentity{ProvenanceKind: "commit", Path: "alive_ref_bench_test.go", OriginCommit: "802436f7fa69ff99842a58f5583d24b75c4b753e", OriginBlob: "5b3d6323bc1657a00cd93e5dd516b72ff691ffe4", SHA256: "1d94806888c2c7293b6fffbc324b8257993109c5b28c5e4ae2d7cd6e81e6fd19"}) {
			t.Errorf("strategy descriptor %q first source = %+v", descriptor.ID, descriptor.Sources[0])
		}
		if descriptor.Sources[1] != (componentSourceIdentity{ProvenanceKind: "commit", Path: "alive_ref_bench_test.go", OriginCommit: "986e2378c1484aa917a1bb0fd13aef914bdce50f", OriginBlob: "747bbda8ca22d7db8fbf7395cbf96bae0724f90a", SHA256: "74a70bc06378514f6e009ba83ddf9b456a98610d4467c184112a88036a7f95b6"}) {
			t.Errorf("strategy descriptor %q second source = %+v", descriptor.ID, descriptor.Sources[1])
		}
		if expected.sourceCount == 4 {
			if descriptor.Sources[2] != (componentSourceIdentity{ProvenanceKind: "commit", Path: "loop.go", OriginCommit: "802436f7fa69ff99842a58f5583d24b75c4b753e", OriginBlob: "ceef22c1e5b7af72f905a61e35a0d77062e0fa52", SHA256: "1791227841921341f49ce2f587bbb580f39f981c8dc6828f2718141f6a2ac9af"}) {
				t.Errorf("strategy descriptor %q first transitive source = %+v", descriptor.ID, descriptor.Sources[2])
			}
			if descriptor.Sources[3] != (componentSourceIdentity{ProvenanceKind: "commit", Path: "loop.go", OriginCommit: "986e2378c1484aa917a1bb0fd13aef914bdce50f", OriginBlob: "344042dd21e1fc14f17c55f0b3d5b7272ca2e475", SHA256: "083f8163eadf1757b897128ff5c8d50c5c5b528da83953a438fefb0f69ef09f0"}) {
				t.Errorf("strategy descriptor %q second transitive source = %+v", descriptor.ID, descriptor.Sources[3])
			}
		}
		materialization := descriptor.MaterializationSources[0]
		if materialization != (componentSourceIdentity{ProvenanceKind: "index-candidate-materialization", Path: expected.path, BaseRevision: "469fd952ed251edc7ea1d2bb0faf4e04fc94dd88", OriginBlob: expected.blob, SHA256: expected.sha256}) {
			t.Errorf("strategy descriptor %q materialization = %+v", descriptor.ID, materialization)
		}
		if descriptor.MaterializationArchive != wantArchive {
			t.Errorf("strategy descriptor %q archive = %+v", descriptor.ID, descriptor.MaterializationArchive)
		}
		if err := validateTimerReferenceStrategyDescriptor(descriptor); err != nil {
			t.Errorf("validate strategy descriptor %q: %v", descriptor.ID, err)
		}
		seal := timerReferenceStrategyDescriptorSeal(descriptor)
		if seal == ([32]byte{}) {
			t.Errorf("strategy descriptor %q has zero seal", descriptor.ID)
		}
		if other, exists := seals[seal]; exists {
			t.Errorf("strategy descriptors %q and %q share a seal", other, descriptor.ID)
		}
		seals[seal] = descriptor.ID
	}
	if _, ok := timerReferenceStrategyID("timer.ref-strategy.absent.v1"); ok {
		t.Error("unknown strategy descriptor resolved")
	}
}

func TestTimerReferenceStrategyDescriptorsAreFreshDeepValues(t *testing.T) {
	mutations := []func(*timerReferenceStrategyDescriptor){
		func(value *timerReferenceStrategyDescriptor) { value.Sources[0].SHA256 = "mutated" },
		func(value *timerReferenceStrategyDescriptor) { value.MaterializationSources[0].OriginBlob = "mutated" },
		func(value *timerReferenceStrategyDescriptor) { value.Adaptations[0] = "mutated" },
	}
	for index, mutate := range mutations {
		descriptor := timerReferenceStrategyDescriptors()[0]
		canonical := timerReferenceStrategyDescriptors()[0]
		mutate(&descriptor)
		if reflect.DeepEqual(descriptor, canonical) {
			t.Fatalf("mutation %d did not change descriptor", index)
		}
		if err := validateTimerReferenceStrategyDescriptor(descriptor); err == nil {
			t.Errorf("mutation %d passed validation", index)
		}
		if fresh := timerReferenceStrategyDescriptors()[0]; !reflect.DeepEqual(fresh, canonical) {
			t.Errorf("mutation %d corrupted fresh descriptor", index)
		}
	}
}
