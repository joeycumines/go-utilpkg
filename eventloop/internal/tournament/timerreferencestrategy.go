package tournament

import (
	"fmt"
	"reflect"
	"strconv"
)

type timerReferenceStrategyTopology string

const (
	timerReferenceOwnerSubmit  timerReferenceStrategyTopology = "owner-direct-external-submit"
	timerReferenceAlwaysSubmit timerReferenceStrategyTopology = "always-submit"
	timerReferenceSyncMap      timerReferenceStrategyTopology = "sync-map"
	timerReferenceRWMutex      timerReferenceStrategyTopology = "rwmutex-map"
)

type timerReferenceStrategyDescriptor struct {
	ID                          string
	AlgorithmFamily             string
	ImplementationRevision      string
	SourceFunction              string
	Sources                     []componentSourceIdentity
	MaterializationSources      []componentSourceIdentity
	MaterializationArchive      timerReferenceMaterializationArchive
	Adaptations                 []string
	CounterBits                 uint8
	Topology                    timerReferenceStrategyTopology
	RequiresRegistrationBarrier bool
	NativeExecutionRequired     bool
}

func timerReferenceStrategyDescriptors() []timerReferenceStrategyDescriptor {
	return []timerReferenceStrategyDescriptor{
		{
			ID:                     "timer.ref-strategy.owner-direct-submit-int32.v1",
			AlgorithmFamily:        "timer.reference-considered-strategy",
			ImplementationRevision: "802436f7.owner-direct-external-submit",
			SourceFunction:         "refViaIsLoopThread",
			Sources:                timerReferenceConsideredTransitiveSources(),
			MaterializationSources: []componentSourceIdentity{
				{ProvenanceKind: "index-candidate-materialization", Path: "internal/tournament/component/timerrefownersubmit/core.go", BaseRevision: "469fd952ed251edc7ea1d2bb0faf4e04fc94dd88", OriginBlob: "bad855f6c4281b1eecf993068dcf11301b89b984", SHA256: "986a0aa238a5e562f1f0fbf107c64863ae47d5471e0cbc3b52b16c88a98c7c04"},
			},
			MaterializationArchive: timerReferenceConsideredArchive(),
			Adaptations: []string{
				"replace the full historical Loop with an instance-local owner core and closure queue while preserving owner test, owner-direct apply, and external closure submission",
				"retain the exact goroutineid owner predicate used by the source revision",
				"model submitted closure consumption through an untimed explicit drain; exclude terminal admission, submission epoch, wake, and the source-specific queue implementation, so native execution remains mandatory",
			},
			CounterBits:             32,
			Topology:                timerReferenceOwnerSubmit,
			NativeExecutionRequired: true,
		},
		{
			ID:                     "timer.ref-strategy.always-submit-int32.v1",
			AlgorithmFamily:        "timer.reference-considered-strategy",
			ImplementationRevision: "802436f7.always-submit",
			SourceFunction:         "refViaSubmitInternal",
			Sources:                timerReferenceConsideredTransitiveSources(),
			MaterializationSources: []componentSourceIdentity{
				{ProvenanceKind: "index-candidate-materialization", Path: "internal/tournament/component/timerrefalwayssubmit/core.go", BaseRevision: "469fd952ed251edc7ea1d2bb0faf4e04fc94dd88", OriginBlob: "0bbfd8572c2334f8f5cb7640ecc43194981574a4", SHA256: "cff807587d2a7b7a647f667b30598529b1a6fe6e0a4f4b74696036e2a4bdef8e"},
			},
			MaterializationArchive: timerReferenceConsideredArchive(),
			Adaptations: []string{
				"replace SubmitInternal with an instance-local closure queue while preserving unconditional closure submission",
				"model owner consumption as an explicit untimed drain; this normalization does not claim that historical external execution always began after the submitter returned",
				"exclude terminal admission, submission epoch, wake, and the source-specific queue implementation, so native execution remains mandatory",
			},
			CounterBits:             32,
			Topology:                timerReferenceAlwaysSubmit,
			NativeExecutionRequired: true,
		},
		{
			ID:                     "timer.ref-strategy.sync-map-int32.v1",
			AlgorithmFamily:        "timer.reference-considered-strategy",
			ImplementationRevision: "802436f7.sync-map",
			SourceFunction:         "refViaSyncMap",
			Sources:                timerReferenceConsideredSources(),
			MaterializationSources: []componentSourceIdentity{
				{ProvenanceKind: "index-candidate-materialization", Path: "internal/tournament/component/timerrefsyncmap/core.go", BaseRevision: "469fd952ed251edc7ea1d2bb0faf4e04fc94dd88", OriginBlob: "55c6fe6cf871c458852afe0b715bf42ee3d80562", SHA256: "352130445736d2356beadd3d044f72880b73333a0954ba64c75519d91ec6f874"},
			},
			MaterializationArchive: timerReferenceConsideredArchive(),
			Adaptations: []string{
				"replace the source package-global sync.Map with an instance-local sync.Map and couple qualification entry state to the aggregate; the historical benchmark instead seeded an auxiliary false entry beside an already-refed production timer",
				"preserve uint64-key Load, concrete entry assertion, atomic reference-bit swap, and conditional Int32 aggregate add",
				"exclude submission epoch, wake, concurrent lifecycle cleanup, and native timer layout; native execution remains mandatory",
			},
			CounterBits:             32,
			Topology:                timerReferenceSyncMap,
			NativeExecutionRequired: true,
		},
		{
			ID:                     "timer.ref-strategy.rwmutex-map-int32.v1",
			AlgorithmFamily:        "timer.reference-considered-strategy",
			ImplementationRevision: "802436f7.rwmutex-map",
			SourceFunction:         "refViaRWMutex",
			Sources:                timerReferenceConsideredSources(),
			MaterializationSources: []componentSourceIdentity{
				{ProvenanceKind: "index-candidate-materialization", Path: "internal/tournament/component/timerrefrwmutex/core.go", BaseRevision: "469fd952ed251edc7ea1d2bb0faf4e04fc94dd88", OriginBlob: "b035cf708c93e5440fbccbc30d4012528f249300", SHA256: "194449ee0f317d0fbff6b97532b803cf7ce5a0daa520d171b62529358ccd62f6"},
			},
			MaterializationArchive: timerReferenceConsideredArchive(),
			Adaptations: []string{
				"retain the receiver-local timer map while replacing the benchmark-global read lock with a core-local lock",
				"preserve RLock lookup, unlock-before-swap, atomic reference-bit swap, and conditional Int32 aggregate add",
				"model the benchmark's no-writer timed window through a mandatory pre-timing seal; the historical lock did not protect production writers, so native execution remains mandatory",
			},
			CounterBits:                 32,
			Topology:                    timerReferenceRWMutex,
			RequiresRegistrationBarrier: true,
			NativeExecutionRequired:     true,
		},
	}
}

func timerReferenceConsideredSources() []componentSourceIdentity {
	return []componentSourceIdentity{
		{ProvenanceKind: "commit", Path: "alive_ref_bench_test.go", OriginCommit: "802436f7fa69ff99842a58f5583d24b75c4b753e", OriginBlob: "5b3d6323bc1657a00cd93e5dd516b72ff691ffe4", SHA256: "1d94806888c2c7293b6fffbc324b8257993109c5b28c5e4ae2d7cd6e81e6fd19"},
		{ProvenanceKind: "commit", Path: "alive_ref_bench_test.go", OriginCommit: "986e2378c1484aa917a1bb0fd13aef914bdce50f", OriginBlob: "747bbda8ca22d7db8fbf7395cbf96bae0724f90a", SHA256: "74a70bc06378514f6e009ba83ddf9b456a98610d4467c184112a88036a7f95b6"},
	}
}

func timerReferenceConsideredTransitiveSources() []componentSourceIdentity {
	return append(timerReferenceConsideredSources(),
		componentSourceIdentity{ProvenanceKind: "commit", Path: "loop.go", OriginCommit: "802436f7fa69ff99842a58f5583d24b75c4b753e", OriginBlob: "ceef22c1e5b7af72f905a61e35a0d77062e0fa52", SHA256: "1791227841921341f49ce2f587bbb580f39f981c8dc6828f2718141f6a2ac9af"},
		componentSourceIdentity{ProvenanceKind: "commit", Path: "loop.go", OriginCommit: "986e2378c1484aa917a1bb0fd13aef914bdce50f", OriginBlob: "344042dd21e1fc14f17c55f0b3d5b7272ca2e475", SHA256: "083f8163eadf1757b897128ff5c8d50c5c5b528da83953a438fefb0f69ef09f0"},
	)
}

func timerReferenceConsideredArchive() timerReferenceMaterializationArchive {
	return timerReferenceMaterializationArchive{
		PatchPath:         "revisions/candidates/0005-timer-reference-considered-strategies.patch",
		PatchSHA256:       "7c090dafd52ab00ab8dfa35dbda1a7111957791cba4e48ea7b039e10d6a913c4",
		PatchBytes:        23487,
		EmptyTree:         "4b825dc642cb6eb9a060e54bf8d69288fbee4904",
		ReconstructedTree: "a474db0f740db26f05e40f54dc09d405550c3e78",
	}
}

func timerReferenceStrategyID(id string) (timerReferenceStrategyDescriptor, bool) {
	for _, descriptor := range timerReferenceStrategyDescriptors() {
		if descriptor.ID == id {
			return descriptor, true
		}
	}
	return timerReferenceStrategyDescriptor{}, false
}

func validateTimerReferenceStrategyDescriptor(descriptor timerReferenceStrategyDescriptor) error {
	canonical, ok := timerReferenceStrategyID(descriptor.ID)
	if !ok {
		return fmt.Errorf("unknown timer reference strategy descriptor %q", descriptor.ID)
	}
	if !reflect.DeepEqual(descriptor, canonical) {
		return fmt.Errorf("invalid timer reference strategy descriptor %q", descriptor.ID)
	}
	return nil
}

func timerReferenceStrategyDescriptorSeal(descriptor timerReferenceStrategyDescriptor) [32]byte {
	fields := []string{
		descriptor.ID,
		descriptor.AlgorithmFamily,
		descriptor.ImplementationRevision,
		descriptor.SourceFunction,
		strconv.FormatUint(uint64(descriptor.CounterBits), 10),
		string(descriptor.Topology),
		strconv.FormatBool(descriptor.RequiresRegistrationBarrier),
		strconv.FormatBool(descriptor.NativeExecutionRequired),
		strconv.Itoa(len(descriptor.Sources)),
	}
	for _, source := range descriptor.Sources {
		fields = append(fields, source.ProvenanceKind, source.Path, source.OriginCommit, source.BaseRevision, source.OriginBlob, source.SHA256)
	}
	fields = append(fields, strconv.Itoa(len(descriptor.MaterializationSources)))
	for _, source := range descriptor.MaterializationSources {
		fields = append(fields, source.ProvenanceKind, source.Path, source.OriginCommit, source.BaseRevision, source.OriginBlob, source.SHA256)
	}
	archive := descriptor.MaterializationArchive
	fields = append(fields, archive.PatchPath, archive.PatchSHA256, strconv.FormatInt(archive.PatchBytes, 10), archive.EmptyTree, archive.ReconstructedTree, strconv.Itoa(len(descriptor.Adaptations)))
	fields = append(fields, descriptor.Adaptations...)
	return framedTimerSeal("go-utilpkg/eventloop/tournament/timer-reference-strategy-descriptor/v1", fields...)
}
