package tournament

import (
	"fmt"

	"github.com/joeycumines/go-eventloop/internal/tournament/component/timerbucket27"
	"github.com/joeycumines/go-eventloop/internal/tournament/component/timerbucketcurrent"
	"github.com/joeycumines/go-eventloop/internal/tournament/component/timerbucketretire"
	"github.com/joeycumines/go-eventloop/internal/tournament/component/timerheapdeadline"
	"github.com/joeycumines/go-eventloop/internal/tournament/component/timerheapdefer"
	"github.com/joeycumines/go-eventloop/internal/tournament/component/timerheapref"
	"github.com/joeycumines/go-eventloop/internal/tournament/component/timerheapstall"
	"github.com/joeycumines/go-eventloop/internal/tournament/component/timervalueone"
	"github.com/joeycumines/go-eventloop/internal/tournament/component/timervaluethree"
)

type timerCapability string

type timerExecutionOwnership string

const timerOwnerSerialized timerExecutionOwnership = "owner-serialized"

const (
	timerCapabilityDiagnostics         timerCapability = "diagnostics"
	timerCapabilityMutexOwnership      timerCapability = "mutex-ownership"
	timerCapabilityIndexedCancel       timerCapability = "indexed-cancellation"
	timerCapabilityMaxSafeID           timerCapability = "max-safe-id-rejection"
	timerCapabilityStickyID            timerCapability = "sticky-uint64-id-exhaustion"
	timerCapabilityReferenceBit        timerCapability = "reference-bit-storage"
	timerCapabilityEarliestTick        timerCapability = "earliest-tick-state"
	timerCapabilityEligibilityBypass   timerCapability = "eligibility-bypass"
	timerCapabilityPhaseEligibility    timerCapability = "phase-eligibility"
	timerCapabilityStaticPriorityFIFO  timerCapability = "static-equal-priority-fifo"
	timerCapabilityStaticDeadlineOrder timerCapability = "static-exact-deadline-order"
	timerCapabilityRepeat              timerCapability = "native-repeat"
	timerCapabilityRetirement          timerCapability = "retirement"
	timerCapabilityPublication         timerCapability = "publication"
	timerCapabilityReentrantCancel     timerCapability = "reentrant-cancellation"
	timerCapabilityDrainRelease        timerCapability = "drain-reference-release"
	timerCapabilityCleanupRelease      timerCapability = "cleanup-reference-release"
	timerCapabilityCompleteCleanup     timerCapability = "complete-cleanup"
)

type timerNativeDriverID string

const (
	timerNativeValueOne      timerNativeDriverID = "component/timervalueone.Queue"
	timerNativeValueThree    timerNativeDriverID = "component/timervaluethree.Queue"
	timerNativeHeapDeadline  timerNativeDriverID = "component/timerheapdeadline.Queue"
	timerNativeHeapRef       timerNativeDriverID = "component/timerheapref.Queue"
	timerNativeHeapStall     timerNativeDriverID = "component/timerheapstall.Queue"
	timerNativeHeapDefer     timerNativeDriverID = "component/timerheapdefer.Queue"
	timerNativeBucket27      timerNativeDriverID = "component/timerbucket27.Queue"
	timerNativeBucketRetire  timerNativeDriverID = "component/timerbucketretire.Queue"
	timerNativeBucketCurrent timerNativeDriverID = "component/timerbucketcurrent.Queue"
)

// timerNativeFactory is only an ID token. It intentionally has no constructor
// method: an initialization benchmark must select a concrete NewNative call
// before timing rather than measure a generic factory switch.
type timerNativeFactory struct {
	ID timerNativeDriverID
}

func newTimerNativeFactory(id timerNativeDriverID) timerNativeFactory {
	factory := timerNativeFactory{ID: id}
	if !factory.valid() {
		return timerNativeFactory{}
	}
	return factory
}

func (f timerNativeFactory) valid() bool {
	switch f.ID {
	case timerNativeValueOne, timerNativeValueThree, timerNativeHeapDeadline,
		timerNativeHeapRef, timerNativeHeapStall, timerNativeHeapDefer,
		timerNativeBucket27, timerNativeBucketRetire, timerNativeBucketCurrent:
		return true
	default:
		return false
	}
}

// timerNativeDriver is a closed union used only during untimed selection.
// Measured loops retain and call the selected concrete pointer directly.
type timerNativeDriver struct {
	ID            timerNativeDriverID
	ValueOne      *timervalueone.Queue
	ValueThree    *timervaluethree.Queue
	HeapDeadline  *timerheapdeadline.Queue
	HeapRef       *timerheapref.Queue
	HeapStall     *timerheapstall.Queue
	HeapDefer     *timerheapdefer.Queue
	Bucket27      *timerbucket27.Queue
	BucketRetire  *timerbucketretire.Queue
	BucketCurrent *timerbucketcurrent.Queue
}

func newTimerNativeDriver(id timerNativeDriverID, epoch timerPreparedEpoch) (timerNativeDriver, error) {
	if !epoch.valid() {
		return timerNativeDriver{}, fmt.Errorf("timer native driver %q has an unprepared epoch", id)
	}
	driver := timerNativeDriver{ID: id}
	switch id {
	case timerNativeValueOne:
		driver.ValueOne = timervalueone.NewNative()
	case timerNativeValueThree:
		driver.ValueThree = timervaluethree.NewNative()
	case timerNativeHeapDeadline:
		driver.HeapDeadline = timerheapdeadline.NewNative()
	case timerNativeHeapRef:
		driver.HeapRef = timerheapref.NewNative()
	case timerNativeHeapStall:
		driver.HeapStall = timerheapstall.NewNative()
	case timerNativeHeapDefer:
		driver.HeapDefer = timerheapdefer.NewNative()
	case timerNativeBucket27:
		driver.Bucket27 = timerbucket27.NewNative(epoch.value())
	case timerNativeBucketRetire:
		driver.BucketRetire = timerbucketretire.NewNative(epoch.value())
	case timerNativeBucketCurrent:
		driver.BucketCurrent = timerbucketcurrent.NewNative(epoch.value())
	default:
		return timerNativeDriver{}, fmt.Errorf("unknown timer native driver %q", id)
	}
	return driver, nil
}

func (d timerNativeDriver) valid() bool {
	nonNil := 0
	for _, present := range []bool{
		d.ValueOne != nil, d.ValueThree != nil, d.HeapDeadline != nil,
		d.HeapRef != nil, d.HeapStall != nil, d.HeapDefer != nil,
		d.Bucket27 != nil, d.BucketRetire != nil, d.BucketCurrent != nil,
	} {
		if present {
			nonNil++
		}
	}
	if nonNil != 1 {
		return false
	}
	switch d.ID {
	case timerNativeValueOne:
		return d.ValueOne != nil
	case timerNativeValueThree:
		return d.ValueThree != nil
	case timerNativeHeapDeadline:
		return d.HeapDeadline != nil
	case timerNativeHeapRef:
		return d.HeapRef != nil
	case timerNativeHeapStall:
		return d.HeapStall != nil
	case timerNativeHeapDefer:
		return d.HeapDefer != nil
	case timerNativeBucket27:
		return d.Bucket27 != nil
	case timerNativeBucketRetire:
		return d.BucketRetire != nil
	case timerNativeBucketCurrent:
		return d.BucketCurrent != nil
	default:
		return false
	}
}

// timerQualificationDriver is a separate closed union for guarded semantic
// checks. It cannot be converted to or stored in timerNativeDriver.
type timerQualificationDriver struct {
	ID            timerNativeDriverID
	ValueOne      *timervalueone.Qualification
	ValueThree    *timervaluethree.Qualification
	HeapDeadline  *timerheapdeadline.Qualification
	HeapRef       *timerheapref.Qualification
	HeapStall     *timerheapstall.Qualification
	HeapDefer     *timerheapdefer.Qualification
	Bucket27      *timerbucket27.Qualification
	BucketRetire  *timerbucketretire.Qualification
	BucketCurrent *timerbucketcurrent.Qualification
}

func newTimerQualificationDriver(id timerNativeDriverID, epoch timerPreparedEpoch) (timerQualificationDriver, error) {
	if !epoch.valid() {
		return timerQualificationDriver{}, fmt.Errorf("timer qualification driver %q has an unprepared epoch", id)
	}
	driver := timerQualificationDriver{ID: id}
	var err error
	switch id {
	case timerNativeValueOne:
		driver.ValueOne = timervalueone.NewQualification()
	case timerNativeValueThree:
		driver.ValueThree = timervaluethree.NewQualification()
	case timerNativeHeapDeadline:
		driver.HeapDeadline = timerheapdeadline.NewQualification()
	case timerNativeHeapRef:
		driver.HeapRef = timerheapref.NewQualification()
	case timerNativeHeapStall:
		driver.HeapStall = timerheapstall.NewQualification()
	case timerNativeHeapDefer:
		driver.HeapDefer = timerheapdefer.NewQualification()
	case timerNativeBucket27:
		driver.Bucket27, err = timerbucket27.NewQualification(epoch.value())
	case timerNativeBucketRetire:
		driver.BucketRetire, err = timerbucketretire.NewQualification(epoch.value())
	case timerNativeBucketCurrent:
		driver.BucketCurrent, err = timerbucketcurrent.NewQualification(epoch.value())
	default:
		return timerQualificationDriver{}, fmt.Errorf("unknown timer qualification driver %q", id)
	}
	if err != nil {
		return timerQualificationDriver{}, err
	}
	return driver, nil
}

func (d timerQualificationDriver) valid() bool {
	nonNil := 0
	for _, present := range []bool{
		d.ValueOne != nil, d.ValueThree != nil, d.HeapDeadline != nil,
		d.HeapRef != nil, d.HeapStall != nil, d.HeapDefer != nil,
		d.Bucket27 != nil, d.BucketRetire != nil, d.BucketCurrent != nil,
	} {
		if present {
			nonNil++
		}
	}
	if nonNil != 1 {
		return false
	}
	switch d.ID {
	case timerNativeValueOne:
		return d.ValueOne != nil
	case timerNativeValueThree:
		return d.ValueThree != nil
	case timerNativeHeapDeadline:
		return d.HeapDeadline != nil
	case timerNativeHeapRef:
		return d.HeapRef != nil
	case timerNativeHeapStall:
		return d.HeapStall != nil
	case timerNativeHeapDefer:
		return d.HeapDefer != nil
	case timerNativeBucket27:
		return d.Bucket27 != nil
	case timerNativeBucketRetire:
		return d.BucketRetire != nil
	case timerNativeBucketCurrent:
		return d.BucketCurrent != nil
	default:
		return false
	}
}

type timerComponentDescriptor struct {
	ID                     string
	AlgorithmFamily        string
	ImplementationRevision string
	Sources                []componentSourceIdentity
	SourceArchive          timerSourceArchive
	Adaptations            []string
	Capabilities           []timerCapability
	ExecutionOwnership     timerExecutionOwnership
	NativeDriver           timerNativeDriverID
	StoragePolicy          timerStoragePolicy
	QualificationPolicy    timerQualificationPolicy
}

type timerSourceArchive struct {
	PatchPath                  string
	PatchSHA256                string
	PatchBytes                 int64
	BaseRevision               string
	BaseTree                   string
	BaseEventloopTree          string
	ReconstructedTree          string
	ReconstructedEventloopTree string
	UnchangedGojaTree          string
}

func canonicalTimerDescriptorDriver(id string) (timerNativeDriverID, bool) {
	switch id {
	case "timer.value-safe-task-v1":
		return timerNativeValueOne, true
	case "timer.value-task-v1":
		return timerNativeValueThree, true
	case "timer.pointer-deadline-v1":
		return timerNativeHeapDeadline, true
	case "timer.pointer-ref-v2":
		return timerNativeHeapRef, true
	case "timer.pointer-tick-stall-v3a":
		return timerNativeHeapStall, true
	case "timer.pointer-tick-defer-v3b":
		return timerNativeHeapDefer, true
	case "timer.bucket-tick-v1":
		return timerNativeBucket27, true
	case "timer.bucket-retire-v1-1":
		return timerNativeBucketRetire, true
	case "timer.bucket-phase-v2":
		return timerNativeBucketCurrent, true
	default:
		return "", false
	}
}

var timerComponentRegistry = []timerComponentDescriptor{
	{
		ID:                     "timer.value-safe-task-v1",
		AlgorithmFamily:        "timer.value-heap",
		ImplementationRevision: "27b93ec3.alternate-one-safe-task",
		Sources: []componentSourceIdentity{
			{ProvenanceKind: "commit", Path: "internal/alternateone/loop.go", OriginCommit: "27b93ec32938ca838e1519bc8e17b6852d7df449", OriginBlob: "92f6a712ec79f613accc9a08bafe04f48839f4ee", SHA256: "556318602f3ccfa9e43dfe4fb83936202093e8b12e7fa330b770028fc5f0f92f"},
			{ProvenanceKind: "commit", Path: "internal/alternateone/chunk.go", OriginCommit: "27b93ec32938ca838e1519bc8e17b6852d7df449", OriginBlob: "c533e23f14c1eb024901f3ee70a94d338bd53681", SHA256: "1a4b368960d349a42e97c288dee9f9ac361f5aca67140f3ae98f5540f67e86a0"},
		},
		Adaptations: []string{
			"extract the mutex-owned timer heap from the restored mature AlternateOne snapshot",
			"preserve 48-byte copied values, 24-byte SafeTask payloads, callback unlock, and retained Pop tails",
			"exclude loop state, wakeup, wall clock, and panic reporting while retaining callback recovery cost",
		},
		Capabilities:        []timerCapability{timerCapabilityDiagnostics, timerCapabilityMutexOwnership, timerCapabilityCleanupRelease},
		ExecutionOwnership:  timerOwnerSerialized,
		NativeDriver:        timerNativeValueOne,
		StoragePolicy:       newTimerStoragePolicy(timerNativeValueOne),
		QualificationPolicy: newTimerQualificationPolicy(timerNativeValueOne),
	},
	{
		ID:                     "timer.value-task-v1",
		AlgorithmFamily:        "timer.value-heap",
		ImplementationRevision: "b77a13cf.alternate-three-task",
		Sources: []componentSourceIdentity{
			{ProvenanceKind: "commit", Path: "internal/alternatethree/loop.go", OriginCommit: "b77a13cf646877598039f2446673ad981486d58e", OriginBlob: "d5676de5ac43f65bfca44d386e0d4756c44b364d", SHA256: "35a30baac7985fb1756bbaa39f1efcc3daf71e347fb951329fd5be6f3755b1ec"},
			{ProvenanceKind: "commit", Path: "internal/alternatethree/ingress.go", OriginCommit: "b77a13cf646877598039f2446673ad981486d58e", OriginBlob: "516268d672af199e933b44ac794c15e6b518fdfd", SHA256: "f8e5e231a30c2fe9a1ead71eb932ab4a8a3278125f48becb74f283811fa78145"},
		},
		Adaptations: []string{
			"extract the owner-only timer heap from the restored mature AlternateThree snapshot",
			"preserve 32-byte copied values, 8-byte Task payloads, and retained Pop tails",
			"exclude loop state, wakeup, wall clock, microtasks, and panic reporting while retaining callback recovery cost",
		},
		Capabilities:        []timerCapability{timerCapabilityDiagnostics, timerCapabilityCleanupRelease},
		ExecutionOwnership:  timerOwnerSerialized,
		NativeDriver:        timerNativeValueThree,
		StoragePolicy:       newTimerStoragePolicy(timerNativeValueThree),
		QualificationPolicy: newTimerQualificationPolicy(timerNativeValueThree),
	},
	{
		ID:                     "timer.pointer-deadline-v1",
		AlgorithmFamily:        "timer.indexed-pointer-heap",
		ImplementationRevision: "506d6643.deadline-stale-index",
		Sources: []componentSourceIdentity{
			{ProvenanceKind: "commit", Path: "loop.go", OriginCommit: "506d6643cc1d45b1da156096870991ecb30b8847", OriginBlob: "f10d7c782a8785b4fcfdb61f75f3a6a21a378cc2", SHA256: "f213677a818476fadac5f44643f5c4fea6d5a550f75988d0b846ee27e1fa3d28"},
		},
		Adaptations: []string{
			"extract indexed deadline heap, map, pool, drain, cancellation, and cleanup without loop ingress",
			"preserve the stale popped heapIndex and reentrant replacement-removal defect",
			"use fixed absolute deadlines and owner-serialized native operations",
		},
		Capabilities:        []timerCapability{timerCapabilityDiagnostics, timerCapabilityIndexedCancel, timerCapabilityMaxSafeID, timerCapabilityDrainRelease},
		ExecutionOwnership:  timerOwnerSerialized,
		NativeDriver:        timerNativeHeapDeadline,
		StoragePolicy:       newTimerStoragePolicy(timerNativeHeapDeadline),
		QualificationPolicy: newTimerQualificationPolicy(timerNativeHeapDeadline),
	},
	{
		ID:                     "timer.pointer-ref-v2",
		AlgorithmFamily:        "timer.indexed-pointer-heap",
		ImplementationRevision: "cc005d72.ref-index-invalidation",
		Sources: []componentSourceIdentity{
			{ProvenanceKind: "commit", Path: "loop.go", OriginCommit: "cc005d72b329fd91eee03aac62ba7188df7c91b9", OriginBlob: "b2eab189b1104aea1f58b15ace6497599cd31e08", SHA256: "364518a66b48b4180a23103eb9b8bcaf4433d5a919de8f6f89d76ef472349aa6"},
		},
		Adaptations: []string{
			"extract the ref-aware indexed deadline heap and safe detached cancellation ownership",
			"preserve package-global pooling, Pop index invalidation, map ownership, and ref state",
			"use fixed absolute deadlines and owner-serialized native operations",
		},
		Capabilities:        []timerCapability{timerCapabilityDiagnostics, timerCapabilityIndexedCancel, timerCapabilityMaxSafeID, timerCapabilityReferenceBit, timerCapabilityReentrantCancel, timerCapabilityDrainRelease},
		ExecutionOwnership:  timerOwnerSerialized,
		NativeDriver:        timerNativeHeapRef,
		StoragePolicy:       newTimerStoragePolicy(timerNativeHeapRef),
		QualificationPolicy: newTimerQualificationPolicy(timerNativeHeapRef),
	},
	{
		ID:                     "timer.pointer-tick-stall-v3a",
		AlgorithmFamily:        "timer.indexed-pointer-heap",
		ImplementationRevision: "0def02e2.earliest-tick-head-stall",
		Sources: []componentSourceIdentity{
			{ProvenanceKind: "commit", Path: "loop.go", OriginCommit: "0def02e2ff987be01a38d237a5d84dae256a85ac", OriginBlob: "a9f343f0893478ea0591bc50c0fc159e13f12f4e", SHA256: "5913e0841e319c0b746adceecfb0fd8fc0f58a485b5c0be390ee28ec1e6cb1c4"},
		},
		Adaptations: []string{
			"extract the earliestTick comparator and runTimers break-on-ineligible-head behavior",
			"preserve the exact 72-byte node, global pool, cancellation, and ref ownership",
			"replace tick and time reads with typed fixed inputs without changing drain order",
		},
		Capabilities:        []timerCapability{timerCapabilityDiagnostics, timerCapabilityIndexedCancel, timerCapabilityMaxSafeID, timerCapabilityReferenceBit, timerCapabilityEarliestTick, timerCapabilityReentrantCancel, timerCapabilityDrainRelease},
		ExecutionOwnership:  timerOwnerSerialized,
		NativeDriver:        timerNativeHeapStall,
		StoragePolicy:       newTimerStoragePolicy(timerNativeHeapStall),
		QualificationPolicy: newTimerQualificationPolicy(timerNativeHeapStall),
	},
	{
		ID:                     "timer.pointer-tick-defer-v3b",
		AlgorithmFamily:        "timer.indexed-pointer-heap",
		ImplementationRevision: "802436f7.pop-defer",
		Sources: []componentSourceIdentity{
			{ProvenanceKind: "commit", Path: "loop.go", OriginCommit: "0bc4ad0ae702ce2205615c31dcf37992d67ff9c8", OriginBlob: "30af1e53f31a01f035e68b0f58cf66f46d6de637", SHA256: "e0f4a10749f7da5cbcdb89aed6b3df7386b035d611898c73eb410272a6c7d636"},
			{ProvenanceKind: "commit", Path: "loop.go", OriginCommit: "802436f7fa69ff99842a58f5583d24b75c4b753e", OriginBlob: "ceef22c1e5b7af72f905a61e35a0d77062e0fa52", SHA256: "1791227841921341f49ce2f587bbb580f39f981c8dc6828f2718141f6a2ac9af"},
		},
		Adaptations: []string{
			"retain 0bc introduction and selected 802 materialization source identities for one exact operation body",
			"preserve pop-and-defer eligibility scanning, reinsertion, global pool, cancellation, and 72-byte layout",
			"replace tick and time reads with typed fixed inputs without changing drain order",
		},
		Capabilities:        []timerCapability{timerCapabilityDiagnostics, timerCapabilityIndexedCancel, timerCapabilityMaxSafeID, timerCapabilityReferenceBit, timerCapabilityEarliestTick, timerCapabilityEligibilityBypass, timerCapabilityReentrantCancel, timerCapabilityDrainRelease},
		ExecutionOwnership:  timerOwnerSerialized,
		NativeDriver:        timerNativeHeapDefer,
		StoragePolicy:       newTimerStoragePolicy(timerNativeHeapDefer),
		QualificationPolicy: newTimerQualificationPolicy(timerNativeHeapDefer),
	},
	{
		ID:                     "timer.bucket-tick-v1",
		AlgorithmFamily:        "timer.deadline-bucket",
		ImplementationRevision: "27b93ec3.bucket-earliest-tick",
		Sources: []componentSourceIdentity{
			{ProvenanceKind: "commit", Path: "loop.go", OriginCommit: "27b93ec32938ca838e1519bc8e17b6852d7df449", OriginBlob: "657b88c500022b09fef4acff47b7cbbbde0e0d71", SHA256: "534eabbcdd10a69d5ea303e389a04e332debabc83649f7da053b937fdf6b80e8"},
		},
		Adaptations: []string{
			"extract fixed-epoch millisecond buckets, exact deadline and earliestTick lists, cancellation, repeat, and cleanup",
			"preserve 104-byte nodes, 64-byte lists, stable equal keys, global pooling, and retained cleanup anchors",
			"replace cached clock, tick, and nesting reads with typed fixed inputs",
		},
		Capabilities:        []timerCapability{timerCapabilityDiagnostics, timerCapabilityIndexedCancel, timerCapabilityMaxSafeID, timerCapabilityReferenceBit, timerCapabilityEarliestTick, timerCapabilityEligibilityBypass, timerCapabilityStaticPriorityFIFO, timerCapabilityStaticDeadlineOrder, timerCapabilityRepeat, timerCapabilityReentrantCancel, timerCapabilityDrainRelease},
		ExecutionOwnership:  timerOwnerSerialized,
		NativeDriver:        timerNativeBucket27,
		StoragePolicy:       newTimerStoragePolicy(timerNativeBucket27),
		QualificationPolicy: newTimerQualificationPolicy(timerNativeBucket27),
	},
	{
		ID:                     "timer.bucket-retire-v1-1",
		AlgorithmFamily:        "timer.deadline-bucket",
		ImplementationRevision: "c8e744e4.bucket-retirement",
		Sources: []componentSourceIdentity{
			{ProvenanceKind: "commit", Path: "loop.go", OriginCommit: "c8e744e4867c351d5b83e438fd2cb438c9b04898", OriginBlob: "51bd5698a6c7a05e1df6e73505a8463b2c497a9e", SHA256: "7f0ff7e2b5c0f2daf638116237f74101ed6b580db0256430128f0c208df697c0"},
		},
		Adaptations: []string{
			"extract the retirement-aware deadline buckets without sharing a mode branch with the 27 revision",
			"preserve 112-byte nodes, 64-byte lists, exactly-once retire-before-pool behavior, and retained cleanup anchors",
			"replace cached clock, tick, and nesting reads with typed fixed inputs",
		},
		Capabilities:        []timerCapability{timerCapabilityDiagnostics, timerCapabilityIndexedCancel, timerCapabilityMaxSafeID, timerCapabilityReferenceBit, timerCapabilityEarliestTick, timerCapabilityEligibilityBypass, timerCapabilityStaticPriorityFIFO, timerCapabilityStaticDeadlineOrder, timerCapabilityRepeat, timerCapabilityRetirement, timerCapabilityReentrantCancel, timerCapabilityDrainRelease},
		ExecutionOwnership:  timerOwnerSerialized,
		NativeDriver:        timerNativeBucketRetire,
		StoragePolicy:       newTimerStoragePolicy(timerNativeBucketRetire),
		QualificationPolicy: newTimerQualificationPolicy(timerNativeBucketRetire),
	},
	{
		ID:                     "timer.bucket-phase-v2",
		AlgorithmFamily:        "timer.deadline-bucket",
		ImplementationRevision: "archive-2d6ae645.publication-phase",
		Sources: []componentSourceIdentity{
			{ProvenanceKind: "archived-index-candidate", Path: "timer.go", BaseRevision: "469fd952ed251edc7ea1d2bb0faf4e04fc94dd88", OriginBlob: "919a22141f9511158c981abc519063e3c07e05bd", SHA256: "9a7e41d739c47259533738401ccd201320b37fea8cd9e66cad5cddb2bc57ef8f"},
			{ProvenanceKind: "archived-index-candidate", Path: "timercancel.go", BaseRevision: "469fd952ed251edc7ea1d2bb0faf4e04fc94dd88", OriginBlob: "065971859d43a3d775ad9a95b1cbc008cfd075b8", SHA256: "10abb483e6817c0fb48ff602e5213daaddd284d186d4486358399fc5ce6be955"},
			{ProvenanceKind: "archived-index-candidate", Path: "timerid.go", BaseRevision: "469fd952ed251edc7ea1d2bb0faf4e04fc94dd88", OriginBlob: "eb0e42f8bf220c7f62992500583c2bfa6f171a69", SHA256: "50d8df638566c95e7f255710957be536929af238aa5c5380b1f077149331a8d2"},
			{ProvenanceKind: "archived-index-candidate", Path: "loop.go", BaseRevision: "469fd952ed251edc7ea1d2bb0faf4e04fc94dd88", OriginBlob: "5c316e52dc7d0bf39c402b433e8c356cfe6ef58d", SHA256: "18cc725d1755b0d641be17344fe77a6198b5aa02bdf87b4855c6e3b8956751ba"},
			{ProvenanceKind: "archived-index-candidate", Path: "scheduler.go", BaseRevision: "469fd952ed251edc7ea1d2bb0faf4e04fc94dd88", OriginBlob: "4acd4c3910ea93b0abd5130b2d8dc72adfbb2eea", SHA256: "7272412ffc0421026037108c27df349bc803ff63b4ebbf9d9c834702cdff7391"},
		},
		SourceArchive: timerCandidateArchive(),
		Adaptations: []string{
			"reconstruct archived timer, cancellation, identity, queue-field, and phase bytes against the committed HEAD base revision",
			"preserve 120-byte nodes, 64-byte lists, stable exact deadlines, publication wait, nonwrapping IDs, phase eligibility, repeat, retirement, and full cleanup",
			"replace owner clock and phase fields with typed fixed inputs; canonical publication uses a prepared non-nil closed channel",
		},
		Capabilities:        []timerCapability{timerCapabilityDiagnostics, timerCapabilityIndexedCancel, timerCapabilityStickyID, timerCapabilityReferenceBit, timerCapabilityEligibilityBypass, timerCapabilityPhaseEligibility, timerCapabilityStaticPriorityFIFO, timerCapabilityStaticDeadlineOrder, timerCapabilityRepeat, timerCapabilityRetirement, timerCapabilityPublication, timerCapabilityReentrantCancel, timerCapabilityDrainRelease, timerCapabilityCleanupRelease, timerCapabilityCompleteCleanup},
		ExecutionOwnership:  timerOwnerSerialized,
		NativeDriver:        timerNativeBucketCurrent,
		StoragePolicy:       newTimerStoragePolicy(timerNativeBucketCurrent),
		QualificationPolicy: newTimerQualificationPolicy(timerNativeBucketCurrent),
	},
}
