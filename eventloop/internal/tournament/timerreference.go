package tournament

import (
	"fmt"

	"github.com/joeycumines/go-eventloop/internal/tournament/component/timerrefint32"
	"github.com/joeycumines/go-eventloop/internal/tournament/component/timerrefint64"
)

type timerReferenceDriverID string

const (
	timerReferenceInt32 timerReferenceDriverID = "component/timerrefint32.Core"
	timerReferenceInt64 timerReferenceDriverID = "component/timerrefint64.Core"
)

// timerReferenceDriver is a closed union used only during untimed setup.
// Measured paths must select and retain one concrete pointer before timing.
type timerReferenceDriver struct {
	ID    timerReferenceDriverID
	Int32 *timerrefint32.Core
	Int64 *timerrefint64.Core
}

func newTimerReferenceDriver(id timerReferenceDriverID) (timerReferenceDriver, error) {
	driver := timerReferenceDriver{ID: id}
	switch id {
	case timerReferenceInt32:
		driver.Int32 = timerrefint32.New()
	case timerReferenceInt64:
		driver.Int64 = timerrefint64.New()
	default:
		return timerReferenceDriver{}, fmt.Errorf("unknown timer reference driver %q", id)
	}
	return driver, nil
}

func (d timerReferenceDriver) valid() bool {
	switch d.ID {
	case timerReferenceInt32:
		return d.Int32 != nil && d.Int64 == nil
	case timerReferenceInt64:
		return d.Int32 == nil && d.Int64 != nil
	default:
		return false
	}
}

type timerReferenceDescriptor struct {
	ID                     string
	AlgorithmFamily        string
	ImplementationRevision string
	SourceStorageID        string
	Sources                []componentSourceIdentity
	MaterializationSources []componentSourceIdentity
	MaterializationArchive timerReferenceMaterializationArchive
	SourceArchive          timerSourceArchive
	Adaptations            []string
	CounterBits            uint8
	Driver                 timerReferenceDriverID
	Policy                 timerReferencePolicy
}

func timerReferenceDescriptors() []timerReferenceDescriptor {
	return []timerReferenceDescriptor{{
		ID:                     "timer.ref-core.map-swap-int32.v1",
		AlgorithmFamily:        "timer.reference-owner-core",
		ImplementationRevision: "cc005d72.map-swap-int32",
		SourceStorageID:        "timer.pointer-ref-v2",
		Sources: []componentSourceIdentity{
			{ProvenanceKind: "commit", Path: "loop.go", OriginCommit: "cc005d72b329fd91eee03aac62ba7188df7c91b9", OriginBlob: "b2eab189b1104aea1f58b15ace6497599cd31e08", SHA256: "364518a66b48b4180a23103eb9b8bcaf4433d5a919de8f6f89d76ef472349aa6"},
		},
		MaterializationSources: []componentSourceIdentity{
			{ProvenanceKind: "index-candidate-materialization", Path: "internal/tournament/component/timerrefint32/core.go", BaseRevision: "469fd952ed251edc7ea1d2bb0faf4e04fc94dd88", OriginBlob: "ce9c1647e14504f00c04526d12cbdf7b056d2e15", SHA256: "cfb6fca35731b9d304378959aa0133d222c46df31f794c30635bff9d45641554"},
		},
		MaterializationArchive: timerReferenceComponentArchive(),
		Adaptations: []string{
			"extract only owner-local map lookup, atomic reference-bit swap, and conditional aggregate add",
			"preserve the historical Int32 aggregate width and missing-or-idempotent no-op behavior",
			"exclude ingress, liveness gates, submission epoch, wakeup, diagnostics, and lifecycle cleanup",
		},
		CounterBits: 32,
		Driver:      timerReferenceInt32,
		Policy:      newTimerReferencePolicy(),
	},
		{
			ID:                     "timer.ref-core.map-swap-int64.v1",
			AlgorithmFamily:        "timer.reference-owner-core",
			ImplementationRevision: "archive-2d6ae645.map-swap-int64",
			SourceStorageID:        "timer.bucket-phase-v2",
			Sources: []componentSourceIdentity{
				{ProvenanceKind: "archived-index-candidate", Path: "timercancel.go", BaseRevision: "469fd952ed251edc7ea1d2bb0faf4e04fc94dd88", OriginBlob: "ba2badbeb9c639711e8198d45295313eb9ff3a94", SHA256: "9c27dfa9fe87733cad58065a682083df8b4c2ba7802cea56daaf41e13c6e8a45"},
				{ProvenanceKind: "archived-index-candidate", Path: "timer.go", BaseRevision: "469fd952ed251edc7ea1d2bb0faf4e04fc94dd88", OriginBlob: "8e0d5b33b787a21b93e5b7b886b487336793d60c", SHA256: "7c0d7599ed36367642717fe18cd957cb93edf357844620b4e65c9b4282b9c319"},
				{ProvenanceKind: "archived-index-candidate", Path: "loop.go", BaseRevision: "469fd952ed251edc7ea1d2bb0faf4e04fc94dd88", OriginBlob: "dde521223ab2db1a812404a864033a2b286ba8cc", SHA256: "ef116776fd7036a2390b45fe97b376d04c674ee5ed5cc036747cfaf749bfb1be"},
			},
			MaterializationSources: []componentSourceIdentity{
				{ProvenanceKind: "index-candidate-materialization", Path: "internal/tournament/component/timerrefint64/core.go", BaseRevision: "469fd952ed251edc7ea1d2bb0faf4e04fc94dd88", OriginBlob: "94a0ee8300fb6d672eb23a5b4f0ddab4e9d312d0", SHA256: "77de06b7ce9759c0642dd72cbd234fce4ca2e362b985360a9eb8f5a00eb20dad"},
			},
			MaterializationArchive: timerReferenceComponentArchive(),
			SourceArchive:          timerCandidateArchive(),
			Adaptations: []string{
				"extract only owner-local map lookup, atomic reference-bit swap, and conditional aggregate add",
				"preserve the current Int64 aggregate width and missing-or-idempotent no-op behavior",
				"exclude ingress, liveness gates, submission epoch, wakeup, diagnostics, and lifecycle cleanup",
			},
			CounterBits: 64,
			Driver:      timerReferenceInt64,
			Policy:      newTimerReferencePolicy(),
		}}
}

type timerReferenceMaterializationArchive struct {
	PatchPath         string
	PatchSHA256       string
	PatchBytes        int64
	EmptyTree         string
	ReconstructedTree string
}

func timerReferenceComponentArchive() timerReferenceMaterializationArchive {
	return timerReferenceMaterializationArchive{
		PatchPath:         "revisions/candidates/0004-timer-reference-materializations.patch",
		PatchSHA256:       "f11f2922f92bb7755e41f5b6c8cff5fd780c2726af08c8eef7ae5829508e904e",
		PatchBytes:        13791,
		EmptyTree:         "4b825dc642cb6eb9a060e54bf8d69288fbee4904",
		ReconstructedTree: "a1ebdf3e3cf109c1ce40870a8ce06b8c6cbe5de1",
	}
}

type timerReferenceBindingDisposition string

const (
	timerReferenceBindingExecuteCore     timerReferenceBindingDisposition = "execute-owner-core"
	timerReferenceBindingNormalizedAlias timerReferenceBindingDisposition = "normalized-core-alias"
	timerReferenceBindingNA              timerReferenceBindingDisposition = "not-applicable"
)

type timerReferenceBindingReason string

const (
	timerReferenceNoBit              timerReferenceBindingReason = "no-reference-bit"
	timerReferenceNormalizedInt32    timerReferenceBindingReason = "exact-source-body-normalized-core"
	timerReferenceIndependentCounter timerReferenceBindingReason = "independent-counter-width"
)

type timerReferenceStorageBinding struct {
	StorageID          string
	Disposition        timerReferenceBindingDisposition
	ReferenceID        string
	CanonicalStorageID string
	Reason             timerReferenceBindingReason
	// NormalizedSource authenticates the native source occurrence whose owner
	// subkernel is normalized by an alias. It is intentionally empty for the
	// canonical core rows and cannot establish native-layout equivalence.
	NormalizedSource componentSourceIdentity
	// NativeExecutionRequired prevents a normalized owner subkernel from being
	// reported as integrated or public Ref/Unref performance evidence.
	NativeExecutionRequired bool
}

var timerReferenceStorageBindings = [...]timerReferenceStorageBinding{
	{StorageID: "timer.value-safe-task-v1", Disposition: timerReferenceBindingNA, Reason: timerReferenceNoBit},
	{StorageID: "timer.value-task-v1", Disposition: timerReferenceBindingNA, Reason: timerReferenceNoBit},
	{StorageID: "timer.pointer-deadline-v1", Disposition: timerReferenceBindingNA, Reason: timerReferenceNoBit},
	{StorageID: "timer.pointer-ref-v2", Disposition: timerReferenceBindingExecuteCore, ReferenceID: "timer.ref-core.map-swap-int32.v1", CanonicalStorageID: "timer.pointer-ref-v2", NativeExecutionRequired: true},
	{StorageID: "timer.pointer-tick-stall-v3a", Disposition: timerReferenceBindingNormalizedAlias, ReferenceID: "timer.ref-core.map-swap-int32.v1", CanonicalStorageID: "timer.pointer-ref-v2", Reason: timerReferenceNormalizedInt32, NormalizedSource: componentSourceIdentity{ProvenanceKind: "commit", Path: "loop.go", OriginCommit: "0def02e2ff987be01a38d237a5d84dae256a85ac", OriginBlob: "a9f343f0893478ea0591bc50c0fc159e13f12f4e", SHA256: "5913e0841e319c0b746adceecfb0fd8fc0f58a485b5c0be390ee28ec1e6cb1c4"}, NativeExecutionRequired: true},
	{StorageID: "timer.pointer-tick-defer-v3b", Disposition: timerReferenceBindingNormalizedAlias, ReferenceID: "timer.ref-core.map-swap-int32.v1", CanonicalStorageID: "timer.pointer-ref-v2", Reason: timerReferenceNormalizedInt32, NormalizedSource: componentSourceIdentity{ProvenanceKind: "commit", Path: "loop.go", OriginCommit: "0bc4ad0ae702ce2205615c31dcf37992d67ff9c8", OriginBlob: "30af1e53f31a01f035e68b0f58cf66f46d6de637", SHA256: "e0f4a10749f7da5cbcdb89aed6b3df7386b035d611898c73eb410272a6c7d636"}, NativeExecutionRequired: true},
	{StorageID: "timer.bucket-tick-v1", Disposition: timerReferenceBindingNormalizedAlias, ReferenceID: "timer.ref-core.map-swap-int32.v1", CanonicalStorageID: "timer.pointer-ref-v2", Reason: timerReferenceNormalizedInt32, NormalizedSource: componentSourceIdentity{ProvenanceKind: "commit", Path: "loop.go", OriginCommit: "27b93ec32938ca838e1519bc8e17b6852d7df449", OriginBlob: "657b88c500022b09fef4acff47b7cbbbde0e0d71", SHA256: "534eabbcdd10a69d5ea303e389a04e332debabc83649f7da053b937fdf6b80e8"}, NativeExecutionRequired: true},
	{StorageID: "timer.bucket-retire-v1-1", Disposition: timerReferenceBindingNormalizedAlias, ReferenceID: "timer.ref-core.map-swap-int32.v1", CanonicalStorageID: "timer.pointer-ref-v2", Reason: timerReferenceNormalizedInt32, NormalizedSource: componentSourceIdentity{ProvenanceKind: "commit", Path: "loop.go", OriginCommit: "c8e744e4867c351d5b83e438fd2cb438c9b04898", OriginBlob: "51bd5698a6c7a05e1df6e73505a8463b2c497a9e", SHA256: "7f0ff7e2b5c0f2daf638116237f74101ed6b580db0256430128f0c208df697c0"}, NativeExecutionRequired: true},
	{StorageID: "timer.bucket-phase-v2", Disposition: timerReferenceBindingExecuteCore, ReferenceID: "timer.ref-core.map-swap-int64.v1", CanonicalStorageID: "timer.bucket-phase-v2", Reason: timerReferenceIndependentCounter, NativeExecutionRequired: true},
}

func timerCandidateArchive() timerSourceArchive {
	return timerSourceArchive{
		PatchPath:                  "revisions/candidates/0001-index-on-469fd952-timer-bucket-phase-v2.patch",
		PatchSHA256:                "2d6ae645435d945de260e4cfa4bf0dc74312aee033e781f227514924b1b50c2b",
		PatchBytes:                 233206,
		BaseRevision:               "469fd952ed251edc7ea1d2bb0faf4e04fc94dd88",
		BaseTree:                   "def5cb294735fd57b36e7075084ba88991916421",
		BaseEventloopTree:          "227ee042f259cdb35c3e33a6dbecb2b8ec746a21",
		ReconstructedTree:          "16b35ccc1445cb8784f95cbb8be48eed54a38e04",
		ReconstructedEventloopTree: "2d320d39718705c5c63ac940b567863ba35d2ba3",
		UnchangedGojaTree:          "69d8cf81666942396704d3d4bdb75208a0e523c6",
	}
}
