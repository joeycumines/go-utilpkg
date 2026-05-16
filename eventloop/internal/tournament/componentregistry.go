package tournament

import (
	"fmt"

	"github.com/joeycumines/go-eventloop/internal/tournament/component"
	"github.com/joeycumines/go-eventloop/internal/tournament/component/pollerarray"
	"github.com/joeycumines/go-eventloop/internal/tournament/component/pollerbounded"
	"github.com/joeycumines/go-eventloop/internal/tournament/component/pollerdynamic"
	"github.com/joeycumines/go-eventloop/internal/tournament/component/pollerfixed"
	"github.com/joeycumines/go-eventloop/internal/tournament/component/pollerfixedtoken"
	"github.com/joeycumines/go-eventloop/internal/tournament/component/pollermap"
)

type componentSourceIdentity struct {
	ProvenanceKind string
	Path           string
	OriginCommit   string
	BaseRevision   string
	OriginBlob     string
	SHA256         string
}

type componentDisposition string

const (
	componentExecute    componentDisposition = "execute"
	componentPredictive componentDisposition = "predictive"
	componentNA         componentDisposition = "not-applicable"
)

func validComponentDisposition(value componentDisposition) bool {
	return value == componentExecute || value == componentPredictive || value == componentNA
}

type fdDescriptorDomain string

const (
	fdDomainInline65536    fdDescriptorDomain = "0..65535"
	fdDomainBelow100M      fdDescriptorDomain = "0..99999999"
	fdDomainNonnegativeInt fdDescriptorDomain = "nonnegative-int"
)

type componentPlatformAuthority string

const (
	componentNativeSource       componentPlatformAuthority = "native-source"
	componentPortableExtraction componentPlatformAuthority = "portable-extraction"
)

type fdTableCapability string

const (
	fdCapabilityDiagnostics        fdTableCapability = "diagnostics"
	fdCapabilityMutationVersion    fdTableCapability = "mutation-version"
	fdCapabilityGrowthProjection   fdTableCapability = "growth-projection"
	fdCapabilityGeneration         fdTableCapability = "generation"
	fdCapabilityToken              fdTableCapability = "token"
	fdCapabilityIdentityExhaustion fdTableCapability = "identity-exhaustion"
)

type fdTableWorkloadID string

const (
	fdWorkloadInit                 fdTableWorkloadID = "fdtable.init"
	fdWorkloadDenseCycle           fdTableWorkloadID = "fdtable.dense-set-get-delete"
	fdWorkloadNearbyGap            fdTableWorkloadID = "fdtable.nearby-gap"
	fdWorkloadThresholdGap         fdTableWorkloadID = "fdtable.threshold-gap"
	fdWorkloadExtremeGap           fdTableWorkloadID = "fdtable.extreme-gap"
	fdWorkloadMixedOccupancy       fdTableWorkloadID = "fdtable.mixed-occupancy"
	fdWorkloadRegistrationChurn    fdTableWorkloadID = "fdtable.registration-churn"
	fdWorkloadGenerationRevalidate fdTableWorkloadID = "fdtable.generation-revalidate"
	fdWorkloadTokenLookup          fdTableWorkloadID = "fdtable.token-lookup"
	fdWorkloadMutationVersion      fdTableWorkloadID = "fdtable.mutation-version"
	fdWorkloadGenerationWrapZero   fdTableWorkloadID = "fdtable.generation-wrap-zero"
	fdWorkloadTokenWrapSkip        fdTableWorkloadID = "fdtable.token-wrap-skip"
	fdWorkloadIdentityExhaustion   fdTableWorkloadID = "fdtable.identity-exhaustion"
)

var allFDTableWorkloads = []fdTableWorkloadID{
	fdWorkloadInit,
	fdWorkloadDenseCycle,
	fdWorkloadNearbyGap,
	fdWorkloadThresholdGap,
	fdWorkloadExtremeGap,
	fdWorkloadMixedOccupancy,
	fdWorkloadRegistrationChurn,
	fdWorkloadGenerationRevalidate,
	fdWorkloadTokenLookup,
	fdWorkloadMutationVersion,
	fdWorkloadGenerationWrapZero,
	fdWorkloadTokenWrapSkip,
	fdWorkloadIdentityExhaustion,
}

type fdTableWorkloadInput struct {
	Registers bool
	MaximumFD int
}

type fdTableWorkloadDefinition struct {
	ID    fdTableWorkloadID
	Input fdTableWorkloadInput
}

var fdTableWorkloadDefinitions = []fdTableWorkloadDefinition{
	{ID: fdWorkloadInit},
	{ID: fdWorkloadDenseCycle, Input: fdTableWorkloadInput{Registers: true, MaximumFD: 4095}},
	{ID: fdWorkloadNearbyGap, Input: fdTableWorkloadInput{Registers: true, MaximumFD: 63}},
	{ID: fdWorkloadThresholdGap, Input: fdTableWorkloadInput{Registers: true, MaximumFD: 524_287}},
	{ID: fdWorkloadExtremeGap, Input: fdTableWorkloadInput{Registers: true, MaximumFD: 99_999_999}},
	{ID: fdWorkloadMixedOccupancy, Input: fdTableWorkloadInput{Registers: true, MaximumFD: 200_000}},
	{ID: fdWorkloadRegistrationChurn, Input: fdTableWorkloadInput{Registers: true, MaximumFD: 4095}},
	{ID: fdWorkloadGenerationRevalidate, Input: fdTableWorkloadInput{Registers: true, MaximumFD: 7}},
	{ID: fdWorkloadTokenLookup, Input: fdTableWorkloadInput{Registers: true, MaximumFD: 7}},
	{ID: fdWorkloadMutationVersion, Input: fdTableWorkloadInput{Registers: true, MaximumFD: 7}},
	{ID: fdWorkloadGenerationWrapZero, Input: fdTableWorkloadInput{Registers: true, MaximumFD: 7}},
	{ID: fdWorkloadTokenWrapSkip, Input: fdTableWorkloadInput{Registers: true, MaximumFD: 7}},
	{ID: fdWorkloadIdentityExhaustion, Input: fdTableWorkloadInput{Registers: true, MaximumFD: 7}},
}

type fdTableWorkloadRule struct {
	ID          fdTableWorkloadID
	Disposition componentDisposition
}

type componentPlatform string

const (
	componentDarwin  componentPlatform = "darwin"
	componentLinux   componentPlatform = "linux"
	componentWindows componentPlatform = "windows"
)

var allComponentPlatforms = []componentPlatform{componentDarwin, componentLinux, componentWindows}

type fdTablePlatformRule struct {
	Platform    componentPlatform
	Disposition componentDisposition
	Authority   componentPlatformAuthority
}

type fdTableWorkloadPolicy struct {
	RegistrationDomain      fdDescriptorDomain
	DenseExecutionSlotLimit int
	Workloads               []fdTableWorkloadRule
	Platforms               []fdTablePlatformRule
}

type fdTableNativeDriverID string

const (
	fdNativePollerMap        fdTableNativeDriverID = "component/pollermap.Table"
	fdNativePollerArray      fdTableNativeDriverID = "component/pollerarray.Table"
	fdNativePollerDynamic    fdTableNativeDriverID = "component/pollerdynamic.Table"
	fdNativePollerFixed      fdTableNativeDriverID = "component/pollerfixed.Table"
	fdNativePollerFixedToken fdTableNativeDriverID = "component/pollerfixedtoken.Table"
	fdNativePollerBounded    fdTableNativeDriverID = "component/pollerbounded.Table"
)

// fdTableNativeDriver is a closed union of concrete native storage drivers.
// Benchmark dispatch switches on ID outside timed regions and then calls the
// matching concrete table directly; qualification adapters are not members.
type fdTableNativeDriver struct {
	ID         fdTableNativeDriverID
	Map        *pollermap.Table
	Array      *pollerarray.Table
	Dynamic    *pollerdynamic.Table
	Fixed      *pollerfixed.Table
	FixedToken *pollerfixedtoken.Table
	Bounded    *pollerbounded.Table
}

// fdTableNativeFactory is a closed, zero-allocation constructor token. The
// runner decides whether New belongs inside or outside a timed region, which
// keeps initialization workloads measurable without exposing adapters.
type fdTableNativeFactory struct {
	ID fdTableNativeDriverID
}

func newFDTableNativeFactory(id fdTableNativeDriverID) fdTableNativeFactory {
	factory := fdTableNativeFactory{ID: id}
	if !factory.valid() {
		return fdTableNativeFactory{}
	}
	return factory
}

func (f fdTableNativeFactory) valid() bool {
	switch f.ID {
	case fdNativePollerMap, fdNativePollerArray, fdNativePollerDynamic, fdNativePollerFixed, fdNativePollerFixedToken, fdNativePollerBounded:
		return true
	default:
		return false
	}
}

func (f fdTableNativeFactory) New() fdTableNativeDriver {
	if !f.valid() {
		return fdTableNativeDriver{}
	}
	return newFDTableNativeDriver(f.ID)
}

func (d fdTableNativeDriver) valid() bool {
	nonNil := 0
	for _, present := range []bool{d.Map != nil, d.Array != nil, d.Dynamic != nil, d.Fixed != nil, d.FixedToken != nil, d.Bounded != nil} {
		if present {
			nonNil++
		}
	}
	if nonNil != 1 {
		return false
	}
	switch d.ID {
	case fdNativePollerMap:
		return d.Map != nil
	case fdNativePollerArray:
		return d.Array != nil
	case fdNativePollerDynamic:
		return d.Dynamic != nil
	case fdNativePollerFixed:
		return d.Fixed != nil
	case fdNativePollerFixedToken:
		return d.FixedToken != nil
	case fdNativePollerBounded:
		return d.Bounded != nil
	default:
		return false
	}
}

func newFDTableNativeDriver(id fdTableNativeDriverID) fdTableNativeDriver {
	driver := fdTableNativeDriver{ID: id}
	switch id {
	case fdNativePollerMap:
		driver.Map = pollermap.NewNative()
	case fdNativePollerArray:
		driver.Array = pollerarray.NewNative()
	case fdNativePollerDynamic:
		driver.Dynamic = pollerdynamic.NewNative()
	case fdNativePollerFixed:
		driver.Fixed = pollerfixed.NewNative()
	case fdNativePollerFixedToken:
		driver.FixedToken = pollerfixedtoken.NewNative()
	case fdNativePollerBounded:
		driver.Bounded = pollerbounded.NewNative()
	}
	return driver
}

func newFDTablePolicy(
	domain fdDescriptorDomain,
	denseExecutionSlotLimit int,
	thresholdGap componentDisposition,
	extremeGap componentDisposition,
	mixedOccupancy componentDisposition,
	generation componentDisposition,
	token componentDisposition,
	mutationVersion componentDisposition,
	wrapZero componentDisposition,
	wrapSkip componentDisposition,
	identityExhaustion componentDisposition,
	darwin componentPlatformAuthority,
	linux componentPlatformAuthority,
	windows componentPlatformAuthority,
) fdTableWorkloadPolicy {
	return fdTableWorkloadPolicy{
		RegistrationDomain:      domain,
		DenseExecutionSlotLimit: denseExecutionSlotLimit,
		Workloads: []fdTableWorkloadRule{
			{ID: fdWorkloadInit, Disposition: componentExecute},
			{ID: fdWorkloadDenseCycle, Disposition: componentExecute},
			{ID: fdWorkloadNearbyGap, Disposition: componentExecute},
			{ID: fdWorkloadThresholdGap, Disposition: thresholdGap},
			{ID: fdWorkloadExtremeGap, Disposition: extremeGap},
			{ID: fdWorkloadMixedOccupancy, Disposition: mixedOccupancy},
			{ID: fdWorkloadRegistrationChurn, Disposition: componentExecute},
			{ID: fdWorkloadGenerationRevalidate, Disposition: generation},
			{ID: fdWorkloadTokenLookup, Disposition: token},
			{ID: fdWorkloadMutationVersion, Disposition: mutationVersion},
			{ID: fdWorkloadGenerationWrapZero, Disposition: wrapZero},
			{ID: fdWorkloadTokenWrapSkip, Disposition: wrapSkip},
			{ID: fdWorkloadIdentityExhaustion, Disposition: identityExhaustion},
		},
		Platforms: []fdTablePlatformRule{
			{Platform: componentDarwin, Disposition: componentExecute, Authority: darwin},
			{Platform: componentLinux, Disposition: componentExecute, Authority: linux},
			{Platform: componentWindows, Disposition: componentExecute, Authority: windows},
		},
	}
}

type fdTableWorkloadPlan struct {
	DescriptorID string
	Workload     fdTableWorkloadDefinition
	Disposition  componentDisposition
	Projection   component.FDProjection
	factory      fdTableNativeFactory
}

func resolveFDTableWorkload(descriptor fdTableComponentDescriptor, id fdTableWorkloadID) (fdTableWorkloadPlan, error) {
	plan := fdTableWorkloadPlan{DescriptorID: descriptor.ID}
	foundDefinition := false
	for _, definition := range fdTableWorkloadDefinitions {
		if definition.ID == id {
			plan.Workload = definition
			foundDefinition = true
			break
		}
	}
	if !foundDefinition {
		return plan, fmt.Errorf("unknown FD-table workload %q", id)
	}
	foundRule := false
	for _, rule := range descriptor.Policy.Workloads {
		if rule.ID == id {
			plan.Disposition = rule.Disposition
			foundRule = true
			break
		}
	}
	if !foundRule || !validComponentDisposition(plan.Disposition) {
		return plan, fmt.Errorf("FD-table descriptor %q has no valid rule for %q", descriptor.ID, id)
	}
	if plan.Workload.Input.Registers && !fdTableDomainAdmits(descriptor.Policy.RegistrationDomain, plan.Workload.Input.MaximumFD) {
		if plan.Disposition != componentNA {
			return plan, fmt.Errorf("FD-table descriptor %q workload %q executes descriptor %d outside registration domain %q", descriptor.ID, id, plan.Workload.Input.MaximumFD, descriptor.Policy.RegistrationDomain)
		}
		return plan, nil
	}
	if plan.Disposition == componentNA {
		return plan, nil
	}
	if descriptor.Policy.DenseExecutionSlotLimit > 0 && plan.Workload.Input.Registers {
		if descriptor.NativeDriver != fdNativePollerDynamic {
			return plan, fmt.Errorf("FD-table descriptor %q has a dense execution limit without a dynamic driver", descriptor.ID)
		}
		projection, err := pollerdynamic.NewNative().Project(plan.Workload.Input.MaximumFD)
		if err != nil {
			return plan, fmt.Errorf("FD-table descriptor %q workload %q projection: %w", descriptor.ID, id, err)
		}
		plan.Projection = projection
		overLimit := projection.DenseSlots > descriptor.Policy.DenseExecutionSlotLimit
		if overLimit != (plan.Disposition == componentPredictive) {
			return plan, fmt.Errorf("FD-table descriptor %q workload %q disposition %q disagrees with dense projection %+v and limit %d", descriptor.ID, id, plan.Disposition, projection, descriptor.Policy.DenseExecutionSlotLimit)
		}
		if overLimit {
			return plan, nil
		}
	} else if plan.Disposition == componentPredictive {
		return plan, fmt.Errorf("FD-table descriptor %q workload %q is predictive without a bounded projection", descriptor.ID, id)
	}
	plan.factory = newFDTableNativeFactory(descriptor.NativeDriver)
	if !plan.factory.valid() {
		return plan, fmt.Errorf("FD-table descriptor %q has invalid native driver %q", descriptor.ID, descriptor.NativeDriver)
	}
	return plan, nil
}

func fdTableDomainAdmits(domain fdDescriptorDomain, fd int) bool {
	switch domain {
	case fdDomainInline65536:
		return fd >= 0 && fd < 1<<16
	case fdDomainBelow100M:
		return fd >= 0 && fd < 100_000_000
	case fdDomainNonnegativeInt:
		return fd >= 0
	default:
		return false
	}
}

func runFDTableWorkload(
	plan fdTableWorkloadPlan,
	execute func(fdTableNativeFactory, fdTableWorkloadInput) error,
	predict func(component.FDProjection, fdTableWorkloadInput) error,
) error {
	switch plan.Disposition {
	case componentExecute:
		if !plan.factory.valid() || execute == nil {
			return fmt.Errorf("FD-table workload %q has no executable native driver", plan.Workload.ID)
		}
		return execute(plan.factory, plan.Workload.Input)
	case componentPredictive:
		if plan.factory.valid() || predict == nil || plan.Projection.DenseSlots == 0 {
			return fmt.Errorf("FD-table workload %q has no predictive route", plan.Workload.ID)
		}
		return predict(plan.Projection, plan.Workload.Input)
	case componentNA:
		if plan.factory.valid() || plan.Projection != (component.FDProjection{}) {
			return fmt.Errorf("FD-table workload %q not-applicable plan owns execution state", plan.Workload.ID)
		}
		return nil
	default:
		return fmt.Errorf("FD-table workload %q has invalid disposition %q", plan.Workload.ID, plan.Disposition)
	}
}

type fdTableComponentDescriptor struct {
	ID                     string
	AlgorithmFamily        string
	ImplementationRevision string
	Sources                []componentSourceIdentity
	Adaptations            []string
	Capabilities           []fdTableCapability
	NativeDriver           fdTableNativeDriverID
	Policy                 fdTableWorkloadPolicy
	QualificationFactory   func() component.FDTableImplementation
}

var fdTableComponentRegistry = []fdTableComponentDescriptor{
	{
		ID:                     "poller.map-pointer-v1",
		AlgorithmFamily:        "poller.pointer-map",
		ImplementationRevision: "506d664.alternate-one-three",
		Sources: []componentSourceIdentity{
			{
				ProvenanceKind: "commit",
				Path:           "internal/alternateone/poller_linux.go",
				OriginCommit:   "506d6643cc1d45b1da156096870991ecb30b8847",
				OriginBlob:     "3add85638c0e31ef22cf68c8db63a00ef0ef2157",
				SHA256:         "3de6a8d51c062c1b8d4c8d9d70d873841ae5c4db667ca686cb111ce95e16b335",
			},
			{
				ProvenanceKind: "commit",
				Path:           "internal/alternatethree/poller_linux.go",
				OriginCommit:   "506d6643cc1d45b1da156096870991ecb30b8847",
				OriginBlob:     "2b97caf938a5b0a0909a290f19276873fb7876b3",
				SHA256:         "ffb578a4c090c69a82d5fbaa3e6bbdad56579ecdd72ab175a8f9d5bbe088988e",
			},
		},
		Adaptations: []string{
			"extract callback map without native poller lifecycle or syscalls",
			"exclude source-specific lock ownership from the storage operation",
			"reject negative descriptors at the semantic contract boundary",
			"convert typed callback and event values only at the adapter boundary",
			"derive diagnostics outside measured operations",
			"restrict common callback adapters to untimed qualification; measured workloads use native Table operations",
		},
		Capabilities: []fdTableCapability{fdCapabilityDiagnostics},
		NativeDriver: fdNativePollerMap,
		Policy: newFDTablePolicy(
			fdDomainNonnegativeInt, 0,
			componentExecute, componentExecute, componentExecute,
			componentNA, componentNA, componentNA, componentNA, componentNA, componentNA,
			componentPortableExtraction, componentNativeSource, componentPortableExtraction,
		),
		QualificationFactory: func() component.FDTableImplementation { return pollermap.New() },
	},
	{
		ID:                     "poller.inline-fixed-dense-v1",
		AlgorithmFamily:        "poller.inline-fixed-dense",
		ImplementationRevision: "506d664.alternate-two",
		Sources: []componentSourceIdentity{
			{
				ProvenanceKind: "commit",
				Path:           "internal/alternatetwo/poller_linux.go",
				OriginCommit:   "506d6643cc1d45b1da156096870991ecb30b8847",
				OriginBlob:     "0db8ed254d9c0db8166b01345904cba4fd114720",
				SHA256:         "1bfe3e2dea40511356c9187242af9c1236857fa3f0cd8aab7c747f2d05492e0f",
			},
		},
		Adaptations: []string{
			"extract the inline descriptor array without epoll lifecycle or event-buffer behavior",
			"preserve the source cache separation, event-buffer footprint, 65536-entry inline allocation, and active marker",
			"preserve atomic version increments after registration mutations",
			"convert typed callback and event values only at the adapter boundary",
			"derive diagnostics outside measured operations",
			"restrict common callback adapters to untimed qualification; measured workloads use native Table operations",
		},
		Capabilities: []fdTableCapability{fdCapabilityDiagnostics, fdCapabilityMutationVersion},
		NativeDriver: fdNativePollerArray,
		Policy: newFDTablePolicy(
			fdDomainInline65536, 0,
			componentNA, componentNA, componentNA,
			componentNA, componentNA, componentExecute, componentNA, componentNA, componentNA,
			componentPortableExtraction, componentNativeSource, componentPortableExtraction,
		),
		QualificationFactory: func() component.FDTableImplementation { return pollerarray.New() },
	},
	{
		ID:                     "poller.dynamic-dense-v1",
		AlgorithmFamily:        "poller.dynamic-dense",
		ImplementationRevision: "802436f7.dynamic-dense",
		Sources: []componentSourceIdentity{
			{ProvenanceKind: "commit", Path: "poller_darwin.go", OriginCommit: "802436f7fa69ff99842a58f5583d24b75c4b753e", OriginBlob: "6a74259abf41c73c27e72f22727cfece80d5cf87", SHA256: "477f588047c60eecc86d0cd9f76aec172be6dd7964955798b4873c57fd6538a6"},
			{ProvenanceKind: "commit", Path: "poller_linux.go", OriginCommit: "802436f7fa69ff99842a58f5583d24b75c4b753e", OriginBlob: "96872f0d09476367bca3f9803c4c84d85c746a43", SHA256: "212ced202dc2b51c07cb4a1bc0d4ef37dd55ac35ad588796e5dad8142841579f"},
			{ProvenanceKind: "commit", Path: "poller_windows.go", OriginCommit: "802436f7fa69ff99842a58f5583d24b75c4b753e", OriginBlob: "84190f63201f6f3e76bfa5df0c4a1cc71520ed65", SHA256: "b58aa5a50dcd396ad5021343c93eca89bd7baac34b61027bbeaefd616db59d95"},
		},
		Adaptations: []string{
			"extract dynamic descriptor storage without native poller lifecycle, locks, syscalls, or event-buffer behavior",
			"preserve the 65536-entry initial allocation and replacement-copy fd*2+1 growth",
			"retain the source-exact native Register operation while routing projections above 1048576 dense slots away from in-process execution through typed workload policy",
			"convert typed callback and event values only in untimed qualification adapters",
			"derive diagnostics outside measured operations; measured workloads use native Table operations",
		},
		Capabilities: []fdTableCapability{fdCapabilityDiagnostics, fdCapabilityGrowthProjection},
		NativeDriver: fdNativePollerDynamic,
		Policy: newFDTablePolicy(
			fdDomainBelow100M, 1<<20,
			componentExecute, componentPredictive, componentExecute,
			componentNA, componentNA, componentNA, componentNA, componentNA, componentNA,
			componentNativeSource, componentNativeSource, componentNativeSource,
		),
		QualificationFactory: func() component.FDTableImplementation { return pollerdynamic.New() },
	},
	{
		ID:                     "poller.fixed-dense-sparse-generation-v1",
		AlgorithmFamily:        "poller.fixed-dense-sparse",
		ImplementationRevision: "27b93ec3.generation",
		Sources: []componentSourceIdentity{
			{ProvenanceKind: "commit", Path: "poller_fdtable.go", OriginCommit: "27b93ec32938ca838e1519bc8e17b6852d7df449", OriginBlob: "064b0218f717f1623580462e2ffce550ab12a11b", SHA256: "f25c6d2b930ddb9e486e4e4de02ffd8d1048eea512fed1ea917d7a23bff1b4c7"},
			{ProvenanceKind: "commit", Path: "poller_darwin.go", OriginCommit: "27b93ec32938ca838e1519bc8e17b6852d7df449", OriginBlob: "686b6f1d4c3b70c9056b70675c1c5afc5aca696e", SHA256: "9f87278a20bf44cc63af624d1dc6b07e28df33f488ed68ce29c3a48d6d5efedb"},
			{ProvenanceKind: "commit", Path: "poller_linux.go", OriginCommit: "27b93ec32938ca838e1519bc8e17b6852d7df449", OriginBlob: "9f01b8bb0c79d6a22e80e8db95b11afc56e03fc7", SHA256: "ab56e6c0e3d06cdc35a4892dcc277649a65b131cf4fd2c5d552e055c2b23b344"},
			{ProvenanceKind: "commit", Path: "poller_windows.go", OriginCommit: "27b93ec32938ca838e1519bc8e17b6852d7df449", OriginBlob: "5518d5602742c1b6c34816d043780c5824621498", SHA256: "1945f27413f9658e36370ea06e3e2725f95b99c4703d6aed692323f32f52cacd"},
		},
		Adaptations: []string{
			"extract fixed dense and sparse registration storage without native polling behavior",
			"preserve source record width, per-registration dispatch allocation, and lazy sparse map",
			"preserve Add(1) generation wrap to zero and possible live-generation collision",
			"remove fdMu locking to isolate owner-serialized storage operations",
			"convert typed callback and event values only in untimed qualification adapters",
			"derive diagnostics outside measured operations; measured workloads use native Table operations",
		},
		Capabilities: []fdTableCapability{fdCapabilityDiagnostics, fdCapabilityGeneration},
		NativeDriver: fdNativePollerFixed,
		Policy: newFDTablePolicy(
			fdDomainBelow100M, 0,
			componentExecute, componentExecute, componentExecute,
			componentExecute, componentNA, componentNA, componentExecute, componentNA, componentNA,
			componentNativeSource, componentNativeSource, componentNativeSource,
		),
		QualificationFactory: func() component.FDTableImplementation { return pollerfixed.New() },
	},
	{
		ID:                     "poller.fixed-dense-sparse-token-v2",
		AlgorithmFamily:        "poller.fixed-dense-sparse",
		ImplementationRevision: "986e2378.token",
		Sources: []componentSourceIdentity{
			{ProvenanceKind: "commit", Path: "poller_fdtable.go", OriginCommit: "986e2378c1484aa917a1bb0fd13aef914bdce50f", OriginBlob: "730f554783f4b0c89553f39437a6c6ed1ea4964f", SHA256: "f0258294f06683960cdfe8260fde767d9bf1e33fc61edbb153fb41c826421241"},
			{ProvenanceKind: "commit", Path: "poller_darwin.go", OriginCommit: "986e2378c1484aa917a1bb0fd13aef914bdce50f", OriginBlob: "a48b56efd43133566dcef53834ac00438783d355", SHA256: "ad013d5ddbaf1e047a85bcb16b4851097b223726afe904eaa1a0714d8d41eadb"},
			{ProvenanceKind: "commit", Path: "poller_linux.go", OriginCommit: "986e2378c1484aa917a1bb0fd13aef914bdce50f", OriginBlob: "d491ef03496571a7dd69cb1a6259f5263165018c", SHA256: "e2ae80547a1d458ac32b3e0c6f46009f0e80963019df65a3367a706bb1a6eb65"},
			{ProvenanceKind: "commit", Path: "poller_windows.go", OriginCommit: "986e2378c1484aa917a1bb0fd13aef914bdce50f", OriginBlob: "ca13453cf505652a2d16884fa948d9339d5531be", SHA256: "86849e47deee908c230be42655e5b59ccbdbe2b3844f360672a1c6f0b6ea8a1b"},
		},
		Adaptations: []string{
			"extract fixed dense, sparse, and reverse-token registration storage without native polling behavior",
			"preserve source record width, per-registration dispatch allocation, lazy maps, and 65536 dense slots",
			"preserve zero and live-token skipping while allowing freed-token reuse after wrap",
			"remove lifecycleMu and fdMu locking plus initialized, closed, callback, and event validation to isolate owner-serialized storage operations",
			"preserve platform record topology while omitting Unix descriptor duplication and ownership, Darwin kernel-tag allocation, native registration, and rollback",
			"convert typed callback and event values only in untimed qualification adapters",
			"derive diagnostics outside measured operations; measured workloads use native Table operations",
		},
		Capabilities: []fdTableCapability{fdCapabilityDiagnostics, fdCapabilityGeneration, fdCapabilityToken},
		NativeDriver: fdNativePollerFixedToken,
		Policy: newFDTablePolicy(
			fdDomainBelow100M, 0,
			componentExecute, componentExecute, componentExecute,
			componentExecute, componentExecute, componentNA, componentNA, componentExecute, componentNA,
			componentNativeSource, componentNativeSource, componentNativeSource,
		),
		QualificationFactory: func() component.FDTableImplementation { return pollerfixedtoken.New() },
	},
	{
		ID:                     "poller.bounded-dense-sparse-token-v3",
		AlgorithmFamily:        "poller.bounded-dense-sparse",
		ImplementationRevision: "index-3d4fb6e.nonwrapping-token",
		Sources: []componentSourceIdentity{
			{ProvenanceKind: "index-candidate", Path: "poller_fdtable.go", BaseRevision: "469fd952ed251edc7ea1d2bb0faf4e04fc94dd88", OriginBlob: "3d4fb6e0cf8f292638db78d2361dfc09bdf6f5df", SHA256: "dd5f8932625a66f9248dcf779dc98bb76b3e4c698ec32a91cd550d219d9f857a"},
			{ProvenanceKind: "index-candidate", Path: "readiness.go", BaseRevision: "469fd952ed251edc7ea1d2bb0faf4e04fc94dd88", OriginBlob: "98f65bdb30264a8dbe073a221f7c839c19d7bf00", SHA256: "499f531fc372d133c9375b801893186fad512fde1a6b613b924c3f73450eaa6f"},
			{ProvenanceKind: "index-candidate", Path: "fd_dispatch.go", BaseRevision: "469fd952ed251edc7ea1d2bb0faf4e04fc94dd88", OriginBlob: "1a0319cbcd2087a2c73132cf346afb9ba1aa4c5d", SHA256: "581e51bec5396f36840095a0e1a8ac23b1b4bb832c9d0b4a4f238146bbfa39db"},
			{ProvenanceKind: "index-candidate", Path: "poller_linux.go", BaseRevision: "469fd952ed251edc7ea1d2bb0faf4e04fc94dd88", OriginBlob: "9fbd45632a3c5221ab474dea37953bcbf1459f02", SHA256: "60b5b6246161c76fe6973ebedb2e768a9ff6dad97acb2419222d88209ab4ed83"},
		},
		Adaptations: []string{
			"extract bounded dense, sparse, and reverse-token registration storage without native polling behavior",
			"preserve current Darwin and Linux record topology, dispatch allocation, 64-slot bounded growth, and sparse-to-dense migration",
			"preserve arbitrary nonnegative descriptors and sticky nonwrapping identity exhaustion",
			"identify staged index bytes against their base revision without inventing an origin commit",
			"remove lifecycleMu and fdMu locking plus initialized, closed, callback, and event validation to isolate owner-serialized storage operations",
			"preserve platform record topology while omitting Unix descriptor duplication and ownership, Darwin kernel-tag allocation, native registration, and rollback",
			"use an explicitly portable Linux-shaped record extraction on platforms without current native source",
			"convert typed callback and event values only in untimed qualification adapters",
			"derive diagnostics outside measured operations; measured workloads use native Table operations",
		},
		Capabilities: []fdTableCapability{fdCapabilityDiagnostics, fdCapabilityGeneration, fdCapabilityToken, fdCapabilityIdentityExhaustion},
		NativeDriver: fdNativePollerBounded,
		Policy: newFDTablePolicy(
			fdDomainNonnegativeInt, 0,
			componentExecute, componentExecute, componentExecute,
			componentExecute, componentExecute, componentNA, componentNA, componentNA, componentExecute,
			componentNativeSource, componentNativeSource, componentPortableExtraction,
		),
		QualificationFactory: func() component.FDTableImplementation { return pollerbounded.New() },
	},
}
