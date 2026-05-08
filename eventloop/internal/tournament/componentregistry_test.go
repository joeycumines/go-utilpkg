package tournament

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/joeycumines/go-eventloop/internal/tournament/component"
	"github.com/joeycumines/go-eventloop/internal/tournament/component/pollerarray"
)

func TestFDTableComponentRegistry(t *testing.T) {
	if len(fdTableComponentRegistry) != 6 {
		t.Fatalf("descriptor count = %d, want 6", len(fdTableComponentRegistry))
	}
	wantSources := map[string][]componentSourceIdentity{
		"poller.map-pointer-v1": {
			{ProvenanceKind: "commit", Path: "internal/alternateone/poller_linux.go", OriginCommit: "506d6643cc1d45b1da156096870991ecb30b8847", OriginBlob: "3add85638c0e31ef22cf68c8db63a00ef0ef2157", SHA256: "3de6a8d51c062c1b8d4c8d9d70d873841ae5c4db667ca686cb111ce95e16b335"},
			{ProvenanceKind: "commit", Path: "internal/alternatethree/poller_linux.go", OriginCommit: "506d6643cc1d45b1da156096870991ecb30b8847", OriginBlob: "2b97caf938a5b0a0909a290f19276873fb7876b3", SHA256: "ffb578a4c090c69a82d5fbaa3e6bbdad56579ecdd72ab175a8f9d5bbe088988e"},
		},
		"poller.inline-fixed-dense-v1": {
			{ProvenanceKind: "commit", Path: "internal/alternatetwo/poller_linux.go", OriginCommit: "506d6643cc1d45b1da156096870991ecb30b8847", OriginBlob: "0db8ed254d9c0db8166b01345904cba4fd114720", SHA256: "1bfe3e2dea40511356c9187242af9c1236857fa3f0cd8aab7c747f2d05492e0f"},
		},
		"poller.dynamic-dense-v1": {
			{ProvenanceKind: "commit", Path: "poller_darwin.go", OriginCommit: "802436f7fa69ff99842a58f5583d24b75c4b753e", OriginBlob: "6a74259abf41c73c27e72f22727cfece80d5cf87", SHA256: "477f588047c60eecc86d0cd9f76aec172be6dd7964955798b4873c57fd6538a6"},
			{ProvenanceKind: "commit", Path: "poller_linux.go", OriginCommit: "802436f7fa69ff99842a58f5583d24b75c4b753e", OriginBlob: "96872f0d09476367bca3f9803c4c84d85c746a43", SHA256: "212ced202dc2b51c07cb4a1bc0d4ef37dd55ac35ad588796e5dad8142841579f"},
			{ProvenanceKind: "commit", Path: "poller_windows.go", OriginCommit: "802436f7fa69ff99842a58f5583d24b75c4b753e", OriginBlob: "84190f63201f6f3e76bfa5df0c4a1cc71520ed65", SHA256: "b58aa5a50dcd396ad5021343c93eca89bd7baac34b61027bbeaefd616db59d95"},
		},
		"poller.fixed-dense-sparse-generation-v1": {
			{ProvenanceKind: "commit", Path: "poller_fdtable.go", OriginCommit: "27b93ec32938ca838e1519bc8e17b6852d7df449", OriginBlob: "064b0218f717f1623580462e2ffce550ab12a11b", SHA256: "f25c6d2b930ddb9e486e4e4de02ffd8d1048eea512fed1ea917d7a23bff1b4c7"},
			{ProvenanceKind: "commit", Path: "poller_darwin.go", OriginCommit: "27b93ec32938ca838e1519bc8e17b6852d7df449", OriginBlob: "686b6f1d4c3b70c9056b70675c1c5afc5aca696e", SHA256: "9f87278a20bf44cc63af624d1dc6b07e28df33f488ed68ce29c3a48d6d5efedb"},
			{ProvenanceKind: "commit", Path: "poller_linux.go", OriginCommit: "27b93ec32938ca838e1519bc8e17b6852d7df449", OriginBlob: "9f01b8bb0c79d6a22e80e8db95b11afc56e03fc7", SHA256: "ab56e6c0e3d06cdc35a4892dcc277649a65b131cf4fd2c5d552e055c2b23b344"},
			{ProvenanceKind: "commit", Path: "poller_windows.go", OriginCommit: "27b93ec32938ca838e1519bc8e17b6852d7df449", OriginBlob: "5518d5602742c1b6c34816d043780c5824621498", SHA256: "1945f27413f9658e36370ea06e3e2725f95b99c4703d6aed692323f32f52cacd"},
		},
		"poller.fixed-dense-sparse-token-v2": {
			{ProvenanceKind: "commit", Path: "poller_fdtable.go", OriginCommit: "986e2378c1484aa917a1bb0fd13aef914bdce50f", OriginBlob: "730f554783f4b0c89553f39437a6c6ed1ea4964f", SHA256: "f0258294f06683960cdfe8260fde767d9bf1e33fc61edbb153fb41c826421241"},
			{ProvenanceKind: "commit", Path: "poller_darwin.go", OriginCommit: "986e2378c1484aa917a1bb0fd13aef914bdce50f", OriginBlob: "a48b56efd43133566dcef53834ac00438783d355", SHA256: "ad013d5ddbaf1e047a85bcb16b4851097b223726afe904eaa1a0714d8d41eadb"},
			{ProvenanceKind: "commit", Path: "poller_linux.go", OriginCommit: "986e2378c1484aa917a1bb0fd13aef914bdce50f", OriginBlob: "d491ef03496571a7dd69cb1a6259f5263165018c", SHA256: "e2ae80547a1d458ac32b3e0c6f46009f0e80963019df65a3367a706bb1a6eb65"},
			{ProvenanceKind: "commit", Path: "poller_windows.go", OriginCommit: "986e2378c1484aa917a1bb0fd13aef914bdce50f", OriginBlob: "ca13453cf505652a2d16884fa948d9339d5531be", SHA256: "86849e47deee908c230be42655e5b59ccbdbe2b3844f360672a1c6f0b6ea8a1b"},
		},
		"poller.bounded-dense-sparse-token-v3": {
			{ProvenanceKind: "index-candidate", Path: "poller_fdtable.go", BaseRevision: "469fd952ed251edc7ea1d2bb0faf4e04fc94dd88", OriginBlob: "4459db136700471c2e3f028f146ee9ce2bc86b78", SHA256: "7db7d7f5d8f77d1e0bb093279d146ee24b05f76795f1e3a9a5c159000791ceb7"},
			{ProvenanceKind: "index-candidate", Path: "readiness.go", BaseRevision: "469fd952ed251edc7ea1d2bb0faf4e04fc94dd88", OriginBlob: "3c7abf6589523e1549a4c9f479bd14d9a05e7b49", SHA256: "6753f5e2f961a4236264a8705aab2481ea2a86d5472e313fa65b1731732fcad9"},
			{ProvenanceKind: "index-candidate", Path: "fd_dispatch.go", BaseRevision: "469fd952ed251edc7ea1d2bb0faf4e04fc94dd88", OriginBlob: "1a0319cbcd2087a2c73132cf346afb9ba1aa4c5d", SHA256: "581e51bec5396f36840095a0e1a8ac23b1b4bb832c9d0b4a4f238146bbfa39db"},
			{ProvenanceKind: "index-candidate", Path: "poller_darwin.go", BaseRevision: "469fd952ed251edc7ea1d2bb0faf4e04fc94dd88", OriginBlob: "49d01c1bfd23813b455b9ddec60d5d3348ddcc35", SHA256: "dbe6ffed23c6ec17d8bcb546ce2c010c8ce18c019643c78d72242a8836763937"},
			{ProvenanceKind: "index-candidate", Path: "poller_linux.go", BaseRevision: "469fd952ed251edc7ea1d2bb0faf4e04fc94dd88", OriginBlob: "d1aa5a3e5e7e68f828eb9351ec8160d30d8d58a1", SHA256: "1346b859643656b3d209aa070047b81b1ad720a1a512eff0d07d74a41d5957b1"},
		},
	}
	wantMetadata := map[string]struct {
		family              string
		revision            string
		capabilities        []fdTableCapability
		nativeDriver        fdTableNativeDriverID
		policy              fdTableWorkloadPolicy
		requiredAdaptations []string
	}{
		"poller.map-pointer-v1": {
			family:       "poller.pointer-map",
			revision:     "506d664.alternate-one-three",
			capabilities: []fdTableCapability{fdCapabilityDiagnostics},
			nativeDriver: fdNativePollerMap,
			policy: newFDTablePolicy(
				fdDomainNonnegativeInt, 0,
				componentExecute, componentExecute, componentExecute,
				componentNA, componentNA, componentNA, componentNA, componentNA, componentNA,
				componentPortableExtraction, componentNativeSource, componentPortableExtraction,
			),
			requiredAdaptations: []string{
				"extract callback map without native poller lifecycle or syscalls",
				"exclude source-specific lock ownership from the storage operation",
			},
		},
		"poller.inline-fixed-dense-v1": {
			family:       "poller.inline-fixed-dense",
			revision:     "506d664.alternate-two",
			capabilities: []fdTableCapability{fdCapabilityDiagnostics, fdCapabilityMutationVersion},
			nativeDriver: fdNativePollerArray,
			policy: newFDTablePolicy(
				fdDomainInline65536, 0,
				componentNA, componentNA, componentNA,
				componentNA, componentNA, componentExecute, componentNA, componentNA, componentNA,
				componentPortableExtraction, componentNativeSource, componentPortableExtraction,
			),
			requiredAdaptations: []string{
				"extract the inline descriptor array without epoll lifecycle or event-buffer behavior",
				"preserve the source cache separation, event-buffer footprint, 65536-entry inline allocation, and active marker",
			},
		},
		"poller.dynamic-dense-v1": {
			family:       "poller.dynamic-dense",
			revision:     "802436f7.dynamic-dense",
			capabilities: []fdTableCapability{fdCapabilityDiagnostics, fdCapabilityGrowthProjection},
			nativeDriver: fdNativePollerDynamic,
			policy: newFDTablePolicy(
				fdDomainBelow100M, 1<<20,
				componentExecute, componentPredictive, componentExecute,
				componentNA, componentNA, componentNA, componentNA, componentNA, componentNA,
				componentNativeSource, componentNativeSource, componentNativeSource,
			),
			requiredAdaptations: []string{
				"extract dynamic descriptor storage without native poller lifecycle, locks, syscalls, or event-buffer behavior",
				"preserve the 65536-entry initial allocation and replacement-copy fd*2+1 growth",
				"retain the source-exact native Register operation while routing projections above 1048576 dense slots away from in-process execution through typed workload policy",
			},
		},
		"poller.fixed-dense-sparse-generation-v1": {
			family:       "poller.fixed-dense-sparse",
			revision:     "27b93ec3.generation",
			capabilities: []fdTableCapability{fdCapabilityDiagnostics, fdCapabilityGeneration},
			nativeDriver: fdNativePollerFixed,
			policy: newFDTablePolicy(
				fdDomainBelow100M, 0,
				componentExecute, componentExecute, componentExecute,
				componentExecute, componentNA, componentNA, componentExecute, componentNA, componentNA,
				componentNativeSource, componentNativeSource, componentNativeSource,
			),
			requiredAdaptations: []string{
				"extract fixed dense and sparse registration storage without native polling behavior",
				"preserve source record width, per-registration dispatch allocation, and lazy sparse map",
				"preserve Add(1) generation wrap to zero and possible live-generation collision",
			},
		},
		"poller.fixed-dense-sparse-token-v2": {
			family:       "poller.fixed-dense-sparse",
			revision:     "986e2378.token",
			capabilities: []fdTableCapability{fdCapabilityDiagnostics, fdCapabilityGeneration, fdCapabilityToken},
			nativeDriver: fdNativePollerFixedToken,
			policy: newFDTablePolicy(
				fdDomainBelow100M, 0,
				componentExecute, componentExecute, componentExecute,
				componentExecute, componentExecute, componentNA, componentNA, componentExecute, componentNA,
				componentNativeSource, componentNativeSource, componentNativeSource,
			),
			requiredAdaptations: []string{
				"extract fixed dense, sparse, and reverse-token registration storage without native polling behavior",
				"remove lifecycleMu and fdMu locking plus initialized, closed, callback, and event validation to isolate owner-serialized storage operations",
				"preserve platform record topology while omitting Unix descriptor duplication and ownership, Darwin kernel-tag allocation, native registration, and rollback",
			},
		},
		"poller.bounded-dense-sparse-token-v3": {
			family:       "poller.bounded-dense-sparse",
			revision:     "index-4459db1.nonwrapping-token",
			capabilities: []fdTableCapability{fdCapabilityDiagnostics, fdCapabilityGeneration, fdCapabilityToken, fdCapabilityIdentityExhaustion},
			nativeDriver: fdNativePollerBounded,
			policy: newFDTablePolicy(
				fdDomainNonnegativeInt, 0,
				componentExecute, componentExecute, componentExecute,
				componentExecute, componentExecute, componentNA, componentNA, componentNA, componentExecute,
				componentNativeSource, componentNativeSource, componentPortableExtraction,
			),
			requiredAdaptations: []string{
				"extract bounded dense, sparse, and reverse-token registration storage without native polling behavior",
				"preserve current Darwin and Linux record topology, dispatch allocation, 64-slot bounded growth, and sparse-to-dense migration",
				"use an explicitly portable Linux-shaped record extraction on platforms without current native source",
			},
		},
	}
	seen := make(map[string]struct{}, len(fdTableComponentRegistry))
	for _, descriptor := range fdTableComponentRegistry {
		if _, exists := seen[descriptor.ID]; exists {
			t.Errorf("duplicate descriptor ID %q", descriptor.ID)
		}
		seen[descriptor.ID] = struct{}{}
		metadata, expected := wantMetadata[descriptor.ID]
		if !expected {
			t.Errorf("unexpected descriptor ID %q", descriptor.ID)
			continue
		}
		if descriptor.AlgorithmFamily != metadata.family || descriptor.ImplementationRevision != metadata.revision {
			t.Errorf("descriptor %q family/revision = (%q, %q), want (%q, %q)", descriptor.ID, descriptor.AlgorithmFamily, descriptor.ImplementationRevision, metadata.family, metadata.revision)
		}
		if !reflect.DeepEqual(descriptor.Sources, wantSources[descriptor.ID]) {
			t.Errorf("descriptor %q sources = %+v, want %+v", descriptor.ID, descriptor.Sources, wantSources[descriptor.ID])
		}
		if !reflect.DeepEqual(descriptor.Capabilities, metadata.capabilities) {
			t.Errorf("descriptor %q capabilities = %+v, want %+v", descriptor.ID, descriptor.Capabilities, metadata.capabilities)
		}
		for _, adaptation := range metadata.requiredAdaptations {
			if !slices.Contains(descriptor.Adaptations, adaptation) {
				t.Errorf("descriptor %q missing required adaptation %q", descriptor.ID, adaptation)
			}
		}
		if descriptor.NativeDriver != metadata.nativeDriver {
			t.Errorf("descriptor %q native driver = %q, want %q", descriptor.ID, descriptor.NativeDriver, metadata.nativeDriver)
		}
		if !reflect.DeepEqual(descriptor.Policy, metadata.policy) {
			t.Errorf("descriptor %q policy = %+v, want %+v", descriptor.ID, descriptor.Policy, metadata.policy)
		}
		assertFDTablePolicy(t, descriptor)
		assertFDTableNativeDriver(t, descriptor.NativeDriver)
		assertFDTableConformance(t, descriptor)
		table := descriptor.QualificationFactory()
		if _, ok := any(table).(component.FDTableVersion); ok != slices.Contains(descriptor.Capabilities, fdCapabilityMutationVersion) {
			t.Errorf("descriptor %q version capability does not match registry", descriptor.ID)
		}
		if _, ok := any(table).(component.FDTableGeneration); ok != slices.Contains(descriptor.Capabilities, fdCapabilityGeneration) {
			t.Errorf("descriptor %q generation capability does not match registry", descriptor.ID)
		}
		if _, ok := any(table).(component.FDTableToken); ok != slices.Contains(descriptor.Capabilities, fdCapabilityToken) {
			t.Errorf("descriptor %q token capability does not match registry", descriptor.ID)
		}
		if _, ok := any(table).(component.FDTableProjection); ok != slices.Contains(descriptor.Capabilities, fdCapabilityGrowthProjection) {
			t.Errorf("descriptor %q projection capability does not match registry", descriptor.ID)
		}
	}
}

func TestFDTableWorkloadResolver(t *testing.T) {
	if len(fdTableWorkloadDefinitions) != len(allFDTableWorkloads) {
		t.Fatalf("workload definitions = %d, want %d", len(fdTableWorkloadDefinitions), len(allFDTableWorkloads))
	}
	definitionIDs := make(map[fdTableWorkloadID]struct{}, len(fdTableWorkloadDefinitions))
	for _, definition := range fdTableWorkloadDefinitions {
		if _, duplicate := definitionIDs[definition.ID]; duplicate {
			t.Fatalf("duplicate workload definition %q", definition.ID)
		}
		definitionIDs[definition.ID] = struct{}{}
		if definition.Input.Registers && definition.Input.MaximumFD < 0 {
			t.Fatalf("workload definition %q has negative maximum FD", definition.ID)
		}
	}
	for _, id := range allFDTableWorkloads {
		if _, ok := definitionIDs[id]; !ok {
			t.Fatalf("missing workload definition %q", id)
		}
	}

	for _, descriptor := range fdTableComponentRegistry {
		for _, id := range allFDTableWorkloads {
			plan, err := resolveFDTableWorkload(descriptor, id)
			if err != nil {
				t.Errorf("resolve %s/%s: %v", descriptor.ID, id, err)
				continue
			}
			executions := 0
			predictions := 0
			err = runFDTableWorkload(
				plan,
				func(factory fdTableNativeFactory, input fdTableWorkloadInput) error {
					executions++
					driver := factory.New()
					if !driver.valid() {
						t.Error("resolver passed invalid native driver")
					}
					if input != plan.Workload.Input {
						t.Errorf("execute input = %+v, want %+v", input, plan.Workload.Input)
					}
					return nil
				},
				func(projection component.FDProjection, input fdTableWorkloadInput) error {
					predictions++
					if projection.DenseSlots <= descriptor.Policy.DenseExecutionSlotLimit {
						t.Errorf("predictive projection = %+v, limit %d", projection, descriptor.Policy.DenseExecutionSlotLimit)
					}
					if input != plan.Workload.Input {
						t.Errorf("predict input = %+v, want %+v", input, plan.Workload.Input)
					}
					return nil
				},
			)
			if err != nil {
				t.Errorf("run %s/%s: %v", descriptor.ID, id, err)
			}
			switch plan.Disposition {
			case componentExecute:
				if executions != 1 || predictions != 0 {
					t.Errorf("execute route calls = (%d, %d)", executions, predictions)
				}
			case componentPredictive:
				if executions != 0 || predictions != 1 {
					t.Errorf("predictive route calls = (%d, %d)", executions, predictions)
				}
			case componentNA:
				if executions != 0 || predictions != 0 {
					t.Errorf("not-applicable route calls = (%d, %d)", executions, predictions)
				}
			}
		}
	}

	var dynamic fdTableComponentDescriptor
	for _, descriptor := range fdTableComponentRegistry {
		if descriptor.NativeDriver == fdNativePollerDynamic {
			dynamic = descriptor
			break
		}
	}
	if dynamic.ID == "" {
		t.Fatal("dynamic descriptor missing")
	}
	table := newFDTableNativeDriver(dynamic.NativeDriver).Dynamic
	for _, test := range []struct {
		fd        int
		wantSlots int
		overLimit bool
	}{
		{fd: 524_287, wantSlots: 1_048_575},
		{fd: 524_288, wantSlots: 1_048_577, overLimit: true},
	} {
		projection, err := table.Project(test.fd)
		if err != nil {
			t.Fatalf("Project(%d): %v", test.fd, err)
		}
		if projection.DenseSlots != test.wantSlots || (projection.DenseSlots > dynamic.Policy.DenseExecutionSlotLimit) != test.overLimit {
			t.Errorf("Project(%d) = %+v with limit %d", test.fd, projection, dynamic.Policy.DenseExecutionSlotLimit)
		}
	}

	var inline fdTableComponentDescriptor
	for _, descriptor := range fdTableComponentRegistry {
		if descriptor.NativeDriver == fdNativePollerArray {
			inline = descriptor
			break
		}
	}
	if inline.ID == "" {
		t.Fatal("inline descriptor missing")
	}
	mixedPlan, err := resolveFDTableWorkload(inline, fdWorkloadMixedOccupancy)
	if err != nil || mixedPlan.Disposition != componentNA || mixedPlan.factory.valid() {
		t.Fatalf("inline mixed plan = (%+v, %v), want N/A without driver", mixedPlan, err)
	}
	invalid := inline
	invalid.Policy.Workloads = slices.Clone(inline.Policy.Workloads)
	for index := range invalid.Policy.Workloads {
		if invalid.Policy.Workloads[index].ID == fdWorkloadMixedOccupancy {
			invalid.Policy.Workloads[index].Disposition = componentExecute
		}
	}
	if _, err := resolveFDTableWorkload(invalid, fdWorkloadMixedOccupancy); err == nil {
		t.Fatal("resolver admitted out-of-domain executable mixed workload")
	}
	if err := pollerarray.NewNative().Register(mixedPlan.Workload.Input.MaximumFD, pollerarray.NativeRegistration{}); !errors.Is(err, component.ErrFDRange) {
		t.Fatalf("native inline mixed Register error = %v, want range", err)
	}
}

func assertFDTablePolicy(t *testing.T, descriptor fdTableComponentDescriptor) {
	t.Helper()
	if descriptor.Policy.RegistrationDomain == "" {
		t.Errorf("descriptor %q has empty registration domain", descriptor.ID)
	}
	capabilities := make(map[fdTableCapability]struct{}, len(descriptor.Capabilities))
	for _, capability := range descriptor.Capabilities {
		switch capability {
		case fdCapabilityDiagnostics, fdCapabilityMutationVersion, fdCapabilityGrowthProjection, fdCapabilityGeneration, fdCapabilityToken, fdCapabilityIdentityExhaustion:
		default:
			t.Errorf("descriptor %q has invalid capability %q", descriptor.ID, capability)
		}
		if _, duplicate := capabilities[capability]; duplicate {
			t.Errorf("descriptor %q has duplicate capability %q", descriptor.ID, capability)
		}
		capabilities[capability] = struct{}{}
	}
	if _, ok := capabilities[fdCapabilityDiagnostics]; !ok {
		t.Errorf("descriptor %q lacks diagnostics capability", descriptor.ID)
	}
	workloads := make(map[fdTableWorkloadID]componentDisposition, len(descriptor.Policy.Workloads))
	for _, rule := range descriptor.Policy.Workloads {
		if rule.ID == "" || !validComponentDisposition(rule.Disposition) {
			t.Errorf("descriptor %q invalid workload rule %+v", descriptor.ID, rule)
		}
		if _, duplicate := workloads[rule.ID]; duplicate {
			t.Errorf("descriptor %q duplicate workload rule %q", descriptor.ID, rule.ID)
		}
		workloads[rule.ID] = rule.Disposition
	}
	for _, id := range allFDTableWorkloads {
		if _, ok := workloads[id]; !ok {
			t.Errorf("descriptor %q missing workload rule %q", descriptor.ID, id)
		}
	}
	if len(workloads) != len(allFDTableWorkloads) {
		t.Errorf("descriptor %q workload rules = %d, want %d", descriptor.ID, len(workloads), len(allFDTableWorkloads))
	}
	platforms := make(map[componentPlatform]fdTablePlatformRule, len(descriptor.Policy.Platforms))
	for _, rule := range descriptor.Policy.Platforms {
		if !validComponentDisposition(rule.Disposition) || rule.Authority == "" {
			t.Errorf("descriptor %q invalid platform rule %+v", descriptor.ID, rule)
		}
		if _, duplicate := platforms[rule.Platform]; duplicate {
			t.Errorf("descriptor %q duplicate platform rule %q", descriptor.ID, rule.Platform)
		}
		platforms[rule.Platform] = rule
	}
	for _, platform := range allComponentPlatforms {
		if _, ok := platforms[platform]; !ok {
			t.Errorf("descriptor %q missing platform rule %q", descriptor.ID, platform)
		}
	}
	if len(platforms) != len(allComponentPlatforms) {
		t.Errorf("descriptor %q platform rules = %d, want %d", descriptor.ID, len(platforms), len(allComponentPlatforms))
	}
	capabilityWorkloads := map[fdTableCapability]fdTableWorkloadID{
		fdCapabilityMutationVersion:    fdWorkloadMutationVersion,
		fdCapabilityGeneration:         fdWorkloadGenerationRevalidate,
		fdCapabilityToken:              fdWorkloadTokenLookup,
		fdCapabilityIdentityExhaustion: fdWorkloadIdentityExhaustion,
	}
	for capability, workload := range capabilityWorkloads {
		if got, want := workloads[workload] == componentExecute, slices.Contains(descriptor.Capabilities, capability); got != want {
			t.Errorf("descriptor %q capability %q and workload %q disagree", descriptor.ID, capability, workload)
		}
	}
	projection := slices.Contains(descriptor.Capabilities, fdCapabilityGrowthProjection)
	if projection != (descriptor.Policy.DenseExecutionSlotLimit > 0) {
		t.Errorf("descriptor %q projection capability and dense execution limit disagree", descriptor.ID)
	}
	for _, disposition := range workloads {
		if disposition == componentPredictive && !projection {
			t.Errorf("descriptor %q has predictive workload without projection capability", descriptor.ID)
		}
	}
}

func assertFDTableNativeDriver(t *testing.T, id fdTableNativeDriverID) {
	t.Helper()
	factory := newFDTableNativeFactory(id)
	if !factory.valid() {
		t.Errorf("native factory %q is invalid", id)
		return
	}
	driver := factory.New()
	nonNil := 0
	for _, present := range []bool{driver.Map != nil, driver.Array != nil, driver.Dynamic != nil, driver.Fixed != nil, driver.FixedToken != nil, driver.Bounded != nil} {
		if present {
			nonNil++
		}
	}
	if driver.ID != id || nonNil != 1 {
		t.Errorf("native driver %q = %+v, want exactly one concrete table", id, driver)
	}
	switch id {
	case fdNativePollerMap:
		if driver.Map == nil {
			t.Error("map driver missing concrete table")
		}
	case fdNativePollerArray:
		if driver.Array == nil {
			t.Error("array driver missing concrete table")
		}
	case fdNativePollerDynamic:
		if driver.Dynamic == nil {
			t.Error("dynamic driver missing concrete table")
		}
	case fdNativePollerFixed:
		if driver.Fixed == nil {
			t.Error("fixed driver missing concrete table")
		}
	case fdNativePollerFixedToken:
		if driver.FixedToken == nil {
			t.Error("fixed-token driver missing concrete table")
		}
	case fdNativePollerBounded:
		if driver.Bounded == nil {
			t.Error("bounded driver missing concrete table")
		}
	default:
		t.Errorf("unknown native driver %q", id)
	}
}

func assertFDTableConformance(t *testing.T, descriptor fdTableComponentDescriptor) {
	t.Helper()
	table := descriptor.QualificationFactory()
	if table == nil {
		t.Errorf("descriptor %q returned nil qualification table", descriptor.ID)
		return
	}
	callbackCalls := 0
	registration := component.FDRegistration{Events: 3, Callback: func(events component.EventMask) {
		callbackCalls++
		if events != 3 {
			t.Errorf("descriptor %q callback events = %d, want 3", descriptor.ID, events)
		}
	}}
	if err := table.Register(7, registration); err != nil {
		t.Errorf("descriptor %q Register: %v", descriptor.ID, err)
		return
	}
	if err := table.Register(7, component.FDRegistration{}); err != component.ErrFDDuplicate {
		t.Errorf("descriptor %q duplicate Register = %v", descriptor.ID, err)
	}
	got, ok := table.Lookup(7)
	if !ok || got.Events != registration.Events || got.Callback == nil {
		t.Errorf("descriptor %q Lookup = (%+v, %t)", descriptor.ID, got, ok)
	} else {
		got.Callback(got.Events)
	}
	if callbackCalls != 1 {
		t.Errorf("descriptor %q callback calls = %d, want 1", descriptor.ID, callbackCalls)
	}
	if err := table.Unregister(7); err != nil {
		t.Errorf("descriptor %q Unregister: %v", descriptor.ID, err)
	}
	if _, ok := table.Lookup(7); ok {
		t.Errorf("descriptor %q retained unregistered descriptor", descriptor.ID)
	}
	if err := table.Unregister(7); err != component.ErrFDMissing {
		t.Errorf("descriptor %q missing Unregister = %v", descriptor.ID, err)
	}
	if err := table.Register(8, component.FDRegistration{}); err != nil {
		t.Errorf("descriptor %q nil-callback Register: %v", descriptor.ID, err)
	}
	nilRegistration, ok := table.Lookup(8)
	if !ok || nilRegistration.Callback != nil {
		t.Errorf("descriptor %q nil-callback Lookup = (%+v, %t)", descriptor.ID, nilRegistration, ok)
	}
	if stats := table.Stats(); stats.ActiveEntries != 1 || stats.ActiveCallbacks != 0 {
		t.Errorf("descriptor %q nil-callback Stats = %+v", descriptor.ID, stats)
	}
	table.Reset()
	if table.Len() != 0 || table.Stats().ActiveCallbacks != 0 {
		t.Errorf("descriptor %q Reset retained state: %+v", descriptor.ID, table.Stats())
	}
}

func TestFDTableComponentSourcesAuthenticate(t *testing.T) {
	repository, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, descriptor := range fdTableComponentRegistry {
		for _, source := range descriptor.Sources {
			t.Run(descriptor.ID+"/"+source.ProvenanceKind+"/"+strings.ReplaceAll(source.Path, "/", "_"), func(t *testing.T) {
				var payload []byte
				var blob string
				switch source.ProvenanceKind {
				case "commit":
					object := source.OriginCommit + ":eventloop/" + source.Path
					payload = runComponentGit(t, repository, "show", object)
					blob = strings.TrimSpace(string(runComponentGit(t, repository, "rev-parse", object)))
				case "index-candidate":
					if source.BaseRevision == "" {
						t.Fatal("index-candidate source has empty base revision")
					}
					base := strings.TrimSpace(string(runComponentGit(t, repository, "rev-parse", source.BaseRevision+"^{commit}")))
					if base != source.BaseRevision {
						t.Errorf("base revision = %s, want %s", base, source.BaseRevision)
					}
					object := ":eventloop/" + source.Path
					payload = runComponentGit(t, repository, "show", object)
					blob = strings.TrimSpace(string(runComponentGit(t, repository, "rev-parse", object)))
				default:
					t.Fatalf("unsupported provenance kind %q", source.ProvenanceKind)
				}
				if blob != source.OriginBlob {
					t.Errorf("blob = %s, want %s", blob, source.OriginBlob)
				}
				if got := fmt.Sprintf("%x", sha256.Sum256(payload)); got != source.SHA256 {
					t.Errorf("SHA-256 = %s, want %s", got, source.SHA256)
				}
			})
		}
	}
}

func runComponentGit(t *testing.T, repository string, arguments ...string) []byte {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", repository}, arguments...)...)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(arguments, " "), err)
	}
	return output
}
