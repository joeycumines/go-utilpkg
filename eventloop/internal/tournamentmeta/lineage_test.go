package main

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

func TestLineageInventory(t *testing.T) {
	inventory, err := loadLineage("../tournament/lineage.json")
	if err != nil {
		t.Fatalf("loadLineage: %v", err)
	}
	if len(inventory.Introductions) != 25 || len(inventory.Snapshots) != 37 ||
		len(inventory.Implementations) != 25 || len(inventory.Concepts) != 12 ||
		len(inventory.RawRoots) != 4 || len(inventory.Harnesses) != 0 || len(inventory.Workloads) != 0 || len(inventory.Bindings) != 0 ||
		len(inventory.Aliases) != 9 || len(inventory.Reconstructions) != 0 || len(inventory.Dispositions) != 42 {
		t.Fatalf("lineage record counts changed: %+v", countLineageFloorRecords(mustLineageRecords(t, inventory)))
	}
}

func TestLineageCanonicalRoundTrip(t *testing.T) {
	inventory := testLineage(strings.Repeat("a", 64))
	data, err := encodeLineage(inventory)
	if err != nil {
		t.Fatalf("encodeLineage: %v", err)
	}
	got, err := decodeLineage(data)
	if err != nil {
		t.Fatalf("decodeLineage: %v", err)
	}
	canonical, err := encodeLineage(got)
	if err != nil {
		t.Fatalf("encode decoded lineage: %v", err)
	}
	if !bytes.Equal(data, canonical) {
		t.Fatal("lineage canonical round-trip changed bytes")
	}
}

func TestLineageRejectsStructuralAndSemanticMutation(t *testing.T) {
	inventory := testLineage(strings.Repeat("a", 64))
	data, err := encodeLineage(inventory)
	if err != nil {
		t.Fatalf("encodeLineage: %v", err)
	}
	for _, test := range []struct {
		name string
		data []byte
	}{
		{name: "duplicate", data: []byte(strings.Replace(string(data), `"schema_version": 2,`, `"schema_version": 2,\n  "schema_version": 2,`, 1))},
		{name: "case alias", data: []byte(strings.Replace(string(data), `"schema_version": 2`, `"SchemaVersion": 2`, 1))},
		{name: "unknown", data: []byte(strings.Replace(string(data), `"schema_version": 2,`, `"schema_version": 2,\n  "unknown": true,`, 1))},
		{name: "null array", data: []byte(strings.Replace(string(data), `"harnesses": []`, `"harnesses": null`, 1))},
		{name: "unsupported schema", data: []byte(strings.Replace(string(data), `"schema_version": 2`, `"schema_version": 4`, 1))},
		{name: "uppercase digest", data: []byte(strings.Replace(string(data), strings.Repeat("a", 64), strings.Repeat("A", 64), 1))},
		{name: "trailing value", data: append(slices.Clone(data), []byte("{}\n")...)},
		{name: "noncanonical", data: bytes.TrimSpace(data)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := decodeLineage(test.data); err == nil {
				t.Fatal("mutated lineage unexpectedly passed")
			}
		})
	}

	mutated := inventory
	mutated.Introductions = append(slices.Clone(inventory.Introductions), inventory.Introductions[0])
	if err := validateLineage(mutated); err == nil {
		t.Fatal("repeated introduction unexpectedly passed")
	}
	mutated = inventory
	mutated.Implementations = slices.Clone(inventory.Implementations)
	mutated.Implementations[0].IntroductionID = "introduction.missing"
	if err := validateLineage(mutated); err == nil {
		t.Fatal("unknown implementation introduction unexpectedly passed")
	}
	mutated = inventory
	mutated.Dispositions = slices.Clone(inventory.Dispositions)
	mutated.Dispositions[0].CorrectnessStatus = "correctness-perfect"
	if err := validateLineage(mutated); err == nil {
		t.Fatal("unknown disposition status unexpectedly passed")
	}
}

func TestLineageSchema3DynamicAuthority(t *testing.T) {
	inventory := testLineageHelperAlias()
	setTestLineageSchemaThree(&inventory)
	inventory.Introductions[0].SourceKind = "dynamic-frozen-filesystem"
	inventory.Introductions[0].SourceID = "source.eventloop.current"
	inventory.Introductions[0].SourceIdentityKind = "component-tree-sha256"
	inventory.Introductions[0].SourceIdentity = strings.Repeat("d", 64)
	inventory.Snapshots[0].SourceKind = "dynamic-frozen-filesystem"
	inventory.Snapshots[0].SourceID = "source.eventloop.current"
	inventory.Snapshots[0].IdentityKind = "component-tree-sha256"
	inventory.Snapshots[0].Identity = strings.Repeat("e", 64)
	inventory.Implementations[0].Kind = "eventtarget"
	for index := range inventory.Harnesses {
		inventory.Harnesses[index].BuildSelection.GoVersion = ""
		inventory.Harnesses[index].BuildSelection.GoDirective = "1.26.2"
	}
	data, err := encodeLineage(inventory)
	if err != nil {
		t.Fatalf("encode schema 3 lineage: %v", err)
	}
	if bytes.Contains(data, []byte(`"go_version"`)) || !bytes.Contains(data, []byte(`"go_directive": "1.26.2"`)) {
		t.Fatalf("schema 3 Go selection is not canonical: %s", data)
	}
	if _, err := decodeLineage(data); err != nil {
		t.Fatalf("decode schema 3 lineage: %v", err)
	}
}

func TestLineageSchema3RequiresSemanticHarnessAuthority(t *testing.T) {
	valid := testLineageDiagnostic()
	if err := validateLineage(valid); err != nil {
		t.Fatalf("valid schema-3 semantic authority: %v", err)
	}
	if data, err := encodeLineage(valid); err != nil {
		t.Fatalf("encode schema-3 semantic authority: %v", err)
	} else if !bytes.Contains(data, []byte(`"semantic_harness"`)) {
		t.Fatal("schema-3 lineage omitted semantic harness authority")
	}

	for _, test := range []struct {
		name   string
		mutate func(*lineageCatalog)
	}{
		{name: "missing", mutate: func(inventory *lineageCatalog) {
			inventory.Workloads[0].SemanticHarness = nil
		}},
		{name: "invalid id", mutate: func(inventory *lineageCatalog) {
			inventory.Workloads[0].SemanticHarness.ID = "invalid"
		}},
		{name: "empty setup", mutate: func(inventory *lineageCatalog) {
			inventory.Workloads[0].SemanticHarness.Setup = ""
		}},
		{name: "multiline timed", mutate: func(inventory *lineageCatalog) {
			inventory.Workloads[0].SemanticHarness.Timed = "first\nsecond"
		}},
		{name: "empty teardown", mutate: func(inventory *lineageCatalog) {
			inventory.Workloads[0].SemanticHarness.Teardown = ""
		}},
		{name: "conflicting shared id", mutate: func(inventory *lineageCatalog) {
			inventory.Workloads[1].SemanticHarness.ID = inventory.Workloads[0].SemanticHarness.ID
		}},
		{name: "schema two feature", mutate: func(inventory *lineageCatalog) {
			inventory.SchemaVersion = 2
			for index := range inventory.Harnesses {
				inventory.Harnesses[index].BuildSelection.GoDirective = ""
				inventory.Harnesses[index].BuildSelection.GoVersion = "go1.26.2"
			}
			inventory.Bindings = inventory.Bindings[:2]
			inventory.Dispositions = inventory.Dispositions[2:]
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			inventory := testLineageDiagnostic()
			test.mutate(&inventory)
			if err := validateLineage(inventory); err == nil {
				t.Fatal("invalid semantic harness authority unexpectedly passed")
			}
		})
	}
}

func TestLineageSchemaFeaturesAreGated(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*lineageCatalog)
	}{
		{name: "schema 2 go directive", mutate: func(inventory *lineageCatalog) {
			inventory.Harnesses[0].BuildSelection.GoDirective = "1.26.2"
		}},
		{name: "schema 3 go version", mutate: func(inventory *lineageCatalog) {
			inventory.SchemaVersion = 3
		}},
		{name: "schema 2 dynamic source", mutate: func(inventory *lineageCatalog) {
			inventory.Introductions[0].SourceKind = "dynamic-frozen-filesystem"
			inventory.Introductions[0].SourceID = "source.eventloop.current"
			inventory.Introductions[0].SourceIdentityKind = "component-tree-sha256"
			inventory.Introductions[0].SourceIdentity = strings.Repeat("d", 64)
		}},
		{name: "schema 2 component kind", mutate: func(inventory *lineageCatalog) {
			inventory.Implementations[0].Kind = "timer"
		}},
		{name: "schema 3 wrong dynamic source", mutate: func(inventory *lineageCatalog) {
			inventory.SchemaVersion = 3
			for index := range inventory.Harnesses {
				inventory.Harnesses[index].BuildSelection.GoVersion = ""
				inventory.Harnesses[index].BuildSelection.GoDirective = "1.26.2"
			}
			inventory.Introductions[0].SourceKind = "dynamic-frozen-filesystem"
			inventory.Introductions[0].SourceID = "source.eventloop.wrong"
			inventory.Introductions[0].SourceIdentityKind = "component-tree-sha256"
			inventory.Introductions[0].SourceIdentity = strings.Repeat("d", 64)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			inventory := testLineageHelperAlias()
			test.mutate(&inventory)
			if err := validateLineage(inventory); err == nil {
				t.Fatal("schema-gated mutation unexpectedly passed")
			}
		})
	}
}

func TestLineageRejectsUnsafeAliasAndReconstruction(t *testing.T) {
	inventory := testLineage(strings.Repeat("a", 64))
	inventory.Aliases = []lineageAlias{{
		ID: "alias.scheduler.example", Kind: "exact-source",
		AliasSubjectID:     "source.commit.sha1.0000000000000000000000000000000000000000",
		CanonicalSubjectID: "snapshot.scheduler.example", Reason: "exact source identity", Rerun: true,
	}}
	if err := validateLineage(inventory); err == nil {
		t.Fatal("rerunnable exact alias unexpectedly passed")
	}
	inventory.Aliases[0].Rerun = false
	inventory.Reconstructions = []lineageReconstruction{{
		ID: "reconstruction.scheduler.example", ConceptID: "concept.scheduler.example",
		BasisSnapshotIDs: []string{"snapshot.scheduler.example"}, Method: "rebuild exact archived patch",
		OutputSnapshotID: "snapshot.scheduler.example", OriginalClaimEligible: true,
	}}
	if err := validateLineage(inventory); err == nil {
		t.Fatal("original-claim-eligible reconstruction unexpectedly passed")
	}
}

func TestLineageHelperAliasRequiresEquivalentExecutableBinding(t *testing.T) {
	if err := validateLineage(testLineageHelperAlias()); err != nil {
		t.Fatalf("equivalent helper binding: %v", err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*lineageCatalog)
	}{
		{name: "canonical applicability", mutate: func(inventory *lineageCatalog) {
			inventory.Bindings[1].Applicability = "alias-only"
		}},
		{name: "implementation", mutate: func(inventory *lineageCatalog) {
			inventory.Bindings[0].ImplementationID = "implementation.scheduler.extra"
		}},
		{name: "workload", mutate: func(inventory *lineageCatalog) {
			inventory.Bindings[0].WorkloadID = "workload.scheduler.extra"
		}},
		{name: "harness", mutate: func(inventory *lineageCatalog) {
			inventory.Bindings[0].HarnessID = "harness.scheduler.example-b"
		}},
		{name: "snapshot set", mutate: func(inventory *lineageCatalog) {
			inventory.Bindings[0].SnapshotIDs = append(inventory.Bindings[0].SnapshotIDs, "snapshot.scheduler.extra")
		}},
		{name: "configuration", mutate: func(inventory *lineageCatalog) {
			inventory.Bindings[0].Configuration = []lineageSetting{{Key: "mode", Value: "alias"}}
		}},
		{name: "stable results", mutate: func(inventory *lineageCatalog) {
			inventory.Bindings[0].Results[0].StableLeaf = "different"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			inventory := testLineageHelperAlias()
			test.mutate(&inventory)
			if err := validateLineage(inventory); err == nil {
				t.Fatal("non-equivalent helper binding unexpectedly passed")
			}
		})
	}
}

func TestLineageSchema3DiagnosticBinding(t *testing.T) {
	if err := validateLineage(testLineageDiagnostic()); err != nil {
		t.Fatalf("valid diagnostic binding: %v", err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*lineageCatalog)
	}{
		{name: "schema 2", mutate: func(inventory *lineageCatalog) {
			inventory.SchemaVersion = 2
			for index := range inventory.Harnesses {
				inventory.Harnesses[index].BuildSelection.GoVersion = "go1.26.2"
				inventory.Harnesses[index].BuildSelection.GoDirective = ""
			}
		}},
		{name: "emitted result", mutate: func(inventory *lineageCatalog) {
			inventory.Bindings[2].Results = []lineageResult{{EmittedLeaf: "Invalid", StableLeaf: "invalid"}}
		}},
		{name: "null results", mutate: func(inventory *lineageCatalog) {
			inventory.Bindings[2].Results = nil
		}},
		{name: "null not-applicable results", mutate: func(inventory *lineageCatalog) {
			inventory.Bindings[3].Results = nil
		}},
		{name: "missing disposition", mutate: func(inventory *lineageCatalog) {
			inventory.Dispositions = inventory.Dispositions[1:]
		}},
		{name: "wrong correctness", mutate: func(inventory *lineageCatalog) {
			inventory.Dispositions[0].CorrectnessStatus = "correctness-unassessed"
		}},
		{name: "wrong comparability", mutate: func(inventory *lineageCatalog) {
			inventory.Dispositions[0].ComparabilityStatus = "unassessed"
		}},
		{name: "wrong evidence", mutate: func(inventory *lineageCatalog) {
			inventory.Dispositions[0].EvidenceStatus = "evidence-incomplete"
		}},
		{name: "wrong platform", mutate: func(inventory *lineageCatalog) {
			inventory.Dispositions[0].Platform = "darwin/amd64"
		}},
		{name: "wrong snapshot", mutate: func(inventory *lineageCatalog) {
			inventory.Dispositions[0].SnapshotID = "snapshot.scheduler.extra"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			inventory := testLineageDiagnostic()
			test.mutate(&inventory)
			if err := validateLineage(inventory); err == nil {
				t.Fatal("invalid diagnostic binding unexpectedly passed")
			}
		})
	}
}

func testLineage(sourceHistorySHA string) lineageCatalog {
	return lineageCatalog{
		SchemaVersion: lineageMinimumSchemaVersion, SourceHistorySHA256: sourceHistorySHA,
		Introductions: []lineageIntroduction{{
			ID: "introduction.scheduler.example", SourceKind: "source-occurrence",
			SourceID:   "source.commit.sha1.506d6643cc1d45b1da156096870991ecb30b8847",
			SourcePath: "eventloop", SourceIdentityKind: "git-tree-sha1",
			SourceIdentity: "78d92966a42c3dfaad780519305361f79d7e693e",
		}},
		Snapshots: []lineageSnapshot{{
			ID: "snapshot.scheduler.example", SourceKind: "source-occurrence",
			SourceID:   "source.commit.sha1.506d6643cc1d45b1da156096870991ecb30b8847",
			SourcePath: "eventloop", IdentityKind: "git-tree-sha1",
			Identity: "78d92966a42c3dfaad780519305361f79d7e693e", Adaptations: []string{},
		}},
		Implementations: []lineageImplementation{{
			ID: "implementation.scheduler.example", Kind: "scheduler", Name: "Example scheduler",
			IntroductionID: "introduction.scheduler.example",
		}},
		Concepts: []lineageConcept{{
			ID: "concept.scheduler.example", Name: "Example concept",
			SourcePath:   "docs/tournament/2026-01-18/ANALYSIS_ALTERNATETWO_HYBRID.md",
			SourceSHA256: strings.Repeat("b", 64), Status: "concept-only", Disposition: "Not implemented.",
		}},
		RawRoots: []lineageRawRoot{}, Harnesses: []lineageHarness{}, Workloads: []lineageWorkload{}, Bindings: []lineageBinding{},
		Aliases: []lineageAlias{}, Reconstructions: []lineageReconstruction{},
		Dispositions: []lineageDisposition{{
			ID: "disposition.snapshot.scheduler.example", SubjectKind: "snapshot",
			SubjectID: "snapshot.scheduler.example", SnapshotID: "snapshot.scheduler.example", Platform: "all",
			BuildStatus: "build-unassessed", CorrectnessStatus: "correctness-unassessed",
			ComparabilityStatus: "unassessed", EvidenceStatus: "evidence-none", Reason: "Awaiting qualification.",
		}},
	}
}

func testLineageHelperAlias() lineageCatalog {
	inventory := testLineage(strings.Repeat("a", 64))
	inventory.Snapshots = append(inventory.Snapshots, lineageSnapshot{
		ID: "snapshot.scheduler.extra", SourceKind: "source-occurrence",
		SourceID:   "source.commit.sha1.506d6643cc1d45b1da156096870991ecb30b8847",
		SourcePath: "eventloop", IdentityKind: "git-tree-sha1",
		Identity: "89d92966a42c3dfaad780519305361f79d7e693e", Adaptations: []string{},
	})
	inventory.Implementations = append(inventory.Implementations, lineageImplementation{
		ID: "implementation.scheduler.extra", Kind: "scheduler", Name: "Extra scheduler",
		IntroductionID: "introduction.scheduler.example",
	})
	inventory.RawRoots = []lineageRawRoot{{
		ID: "raw-root.scheduler.example", ModuleID: "eventloop", Package: "example",
		Benchmarks: []string{"BenchmarkAlias", "BenchmarkCanonical"}, SourcePath: "eventloop/example_bench_test.go",
		IdentityKind: "sha256", Identity: strings.Repeat("c", 64), SnapshotID: "snapshot.scheduler.example",
	}}
	selection := lineageBuildSelection{
		BuildCellID: "build-cell.eventloop.darwin-arm64", ModuleID: "eventloop", Package: "example",
		GOOS: "darwin", GOARCH: "arm64", CGOEnabled: false, GoVersion: "go1.26.2",
		ArchitectureFeature: lineageArchitectureFeature{Name: "GOARM64", Value: "v8.0"},
		BuildTags:           []string{}, SelectionFlags: []string{},
	}
	physicalRoots := []lineagePhysicalRoot{{
		ID: "raw-root.scheduler.example", Kind: "benchmark-root", Path: "eventloop/example_bench_test.go",
		Identity: strings.Repeat("c", 64),
	}}
	inventory.Harnesses = []lineageHarness{
		{ID: "harness.scheduler.example-a", PhysicalRoots: physicalRoots, ClosurePolicy: "go-test-package-capture-v1", BuildSelection: selection},
		{ID: "harness.scheduler.example-b", PhysicalRoots: physicalRoots, ClosurePolicy: "go-test-package-capture-v1", BuildSelection: selection},
	}
	inventory.Workloads = []lineageWorkload{
		{ID: "workload.scheduler.example", Operation: "Execute the example workload.", Parameters: []lineageSetting{}},
		{ID: "workload.scheduler.extra", Operation: "Execute the extra workload.", Parameters: []lineageSetting{}},
	}
	inventory.Bindings = []lineageBinding{
		{
			ID: "binding.scheduler.alias", RawRootID: "raw-root.scheduler.example", Benchmark: "BenchmarkAlias",
			ImplementationID: "implementation.scheduler.example", SnapshotIDs: []string{"snapshot.scheduler.example"},
			WorkloadID: "workload.scheduler.example", HarnessID: "harness.scheduler.example-a", Applicability: "alias-only",
			Configuration: []lineageSetting{}, Results: []lineageResult{{EmittedLeaf: "Alias", StableLeaf: "stable"}},
		},
		{
			ID: "binding.scheduler.canonical", RawRootID: "raw-root.scheduler.example", Benchmark: "BenchmarkCanonical",
			ImplementationID: "implementation.scheduler.example", SnapshotIDs: []string{"snapshot.scheduler.example"},
			WorkloadID: "workload.scheduler.example", HarnessID: "harness.scheduler.example-a", Applicability: "executable",
			Configuration: []lineageSetting{}, Results: []lineageResult{{EmittedLeaf: "Canonical", StableLeaf: "stable"}},
		},
	}
	inventory.Aliases = []lineageAlias{{
		ID: "alias.scheduler.example", Kind: "helper-identity", AliasSubjectID: "binding.scheduler.alias",
		CanonicalSubjectID: "binding.scheduler.canonical", Reason: "The helper root executes the canonical workload.", Rerun: false,
	}}
	return inventory
}

func testLineageDiagnostic() lineageCatalog {
	inventory := testLineageHelperAlias()
	setTestLineageSchemaThree(&inventory)
	inventory.Bindings = append(inventory.Bindings, lineageBinding{
		ID: "binding.scheduler.diagnostic", RawRootID: "raw-root.scheduler.example", Benchmark: "BenchmarkCanonical",
		ImplementationID: "implementation.scheduler.extra", SnapshotIDs: []string{"snapshot.scheduler.example"},
		WorkloadID: "workload.scheduler.extra", HarnessID: "harness.scheduler.example-b", Applicability: "diagnostic",
		Configuration: []lineageSetting{}, Results: []lineageResult{},
	})
	inventory.Bindings = append(inventory.Bindings, lineageBinding{
		ID: "binding.scheduler.not-applicable", RawRootID: "raw-root.scheduler.example", Benchmark: "BenchmarkCanonical",
		ImplementationID: "implementation.scheduler.extra", SnapshotIDs: []string{"snapshot.scheduler.example"},
		WorkloadID: "workload.scheduler.extra", HarnessID: "harness.scheduler.example-b", Applicability: "not-applicable",
		Configuration: []lineageSetting{}, Results: []lineageResult{},
	})
	inventory.Dispositions = append([]lineageDisposition{
		{
			ID: "disposition.binding.scheduler.diagnostic", SubjectKind: "binding",
			SubjectID: "binding.scheduler.diagnostic", SnapshotID: "snapshot.scheduler.example", Platform: "darwin/arm64",
			BuildStatus: "build-valid", CorrectnessStatus: "correctness-invalid",
			ComparabilityStatus: "noncomparable", EvidenceStatus: "evidence-complete",
			Reason: "The diagnostic proves this implementation is correctness-invalid.",
		},
		{
			ID: "disposition.binding.scheduler.not-applicable", SubjectKind: "binding",
			SubjectID: "binding.scheduler.not-applicable", SnapshotID: "snapshot.scheduler.example", Platform: "darwin/arm64",
			BuildStatus: "not-applicable", CorrectnessStatus: "not-applicable",
			ComparabilityStatus: "noncomparable", EvidenceStatus: "evidence-complete",
			Reason: "The implementation does not support this workload.",
		},
	}, inventory.Dispositions...)
	return inventory
}

func setTestLineageSchemaThree(inventory *lineageCatalog) {
	inventory.SchemaVersion = 3
	for index := range inventory.Harnesses {
		inventory.Harnesses[index].BuildSelection.GoVersion = ""
		inventory.Harnesses[index].BuildSelection.GoDirective = "1.26.2"
	}
	for index := range inventory.Workloads {
		inventory.Workloads[index].SemanticHarness = &lineageSemanticHarness{
			ID:       "semantic-harness." + strings.TrimPrefix(inventory.Workloads[index].ID, "workload."),
			Setup:    "Construct fresh state outside the measured interval.",
			Timed:    inventory.Workloads[index].Operation,
			Teardown: "Verify completion and release resources outside the measured interval.",
		}
	}
}

func mustLineageRecords(t *testing.T, inventory lineageCatalog) map[string]lineageFloorRecord {
	t.Helper()
	records, err := lineageFloorRecords(inventory)
	if err != nil {
		t.Fatalf("lineageFloorRecords: %v", err)
	}
	return records
}
