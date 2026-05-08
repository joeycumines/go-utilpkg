package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestManifestV5ProjectionShape(t *testing.T) {
	manifest := testManifestV5(t)
	data := marshalManifestV5(t, manifest)
	if err := validateSourceManifestJSONShape(data); err != nil {
		t.Fatalf("valid manifest v5 shape: %v", err)
	}

	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "v4 inference field", mutate: func(manifest map[string]any) {
			manifest["variants"] = []any{}
		}},
		{name: "empty bindings", mutate: func(manifest map[string]any) {
			manifest["lanes"].([]manifestV5Lane)[0].BenchmarkBindings = []manifestV5BindingProjection{}
		}},
		{name: "null bindings", mutate: func(manifest map[string]any) {
			manifest["lanes"].([]manifestV5Lane)[0].BenchmarkBindings = nil
		}},
		{name: "unknown build cell", mutate: func(manifest map[string]any) {
			manifest["lanes"].([]manifestV5Lane)[0].BuildCellIDs = []string{"eventloop.unknown-cell"}
		}},
		{name: "repeated build cell", mutate: func(manifest map[string]any) {
			cell := manifest["lanes"].([]manifestV5Lane)[0].BuildCellIDs[0]
			manifest["lanes"].([]manifestV5Lane)[0].BuildCellIDs = []string{cell, cell}
		}},
		{name: "repeated binding", mutate: func(manifest map[string]any) {
			binding := manifest["lanes"].([]manifestV5Lane)[0].BenchmarkBindings[0]
			manifest["lanes"].([]manifestV5Lane)[0].BenchmarkBindings = []manifestV5BindingProjection{binding, binding}
		}},
		{name: "unsorted lanes", mutate: func(manifest map[string]any) {
			lane := manifest["lanes"].([]manifestV5Lane)[0]
			other := lane
			other.ID = "alpha"
			other.BenchmarkBindings = []manifestV5BindingProjection{{
				BindingID: "binding.example.alpha", ImplementationID: "implementation.example.alpha", ModuleID: lane.BenchmarkBindings[0].ModuleID,
			}}
			manifest["lanes"] = []manifestV5Lane{lane, other}
		}},
		{name: "unsorted dispositions", mutate: func(manifest map[string]any) {
			manifest["root_dispositions"] = []manifestV5RootDisposition{
				{RawRootID: "raw-root.example.zulu", DispositionID: "disposition.example.zulu"},
				{RawRootID: "raw-root.example.alpha", DispositionID: "disposition.example.alpha"},
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			mutated := testManifestV5(t)
			test.mutate(mutated)
			if err := validateSourceManifestJSONShape(marshalManifestV5(t, mutated)); err == nil {
				t.Fatal("invalid manifest v5 shape unexpectedly passed")
			}
		})
	}

	unknown := []byte(strings.Replace(string(data), `"required":true,`, `"required":true,"benchmarks":[],`, 1))
	if err := validateSourceManifestJSONShape(unknown); err == nil {
		t.Fatal("manifest v5 lane with a v4 benchmark field unexpectedly passed")
	}
}

func TestManifestV5LineageAuthorityRequiresSchemaThree(t *testing.T) {
	manifest := sourceManifestEnvelope{
		SchemaVersion: manifestSchemaVersionV5,
		SourceHistory: json.RawMessage(`{}`),
		Lineage: manifestLineageReference{
			Path: "lineage.json", SchemaVersion: 2, SHA256: strings.Repeat("a", 64),
			Floor: manifestLineageFloorReference{
				Path: "lineagefloors/000002.json", SchemaVersion: 1, Sequence: 2,
				SHA256: strings.Repeat("b", 64), CumulativeRecordSetSHA256: strings.Repeat("c", 64),
			},
		},
	}
	if err := verifyManifestV5Lineage("manifest.json", manifest); err == nil || !strings.Contains(err.Error(), "want 3") {
		t.Fatalf("schema-2 lineage verification error = %v", err)
	}
}

func TestManifestV5HistoryFloorSequenceBound(t *testing.T) {
	manifest := sourceManifestEnvelope{
		SchemaVersion: manifestSchemaVersionV5,
		SourceHistory: json.RawMessage(`{
      "path":"source_history.json",
      "schema_version":2,
      "sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "floor":{
        "path":"historyfloors/1000000.json",
        "schema_version":1,
        "sequence":1000000,
        "sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
        "cumulative_record_set_sha256":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
      }
    }`),
		Lineage: manifestLineageReference{
			Path: "lineage.json", SchemaVersion: 3, SHA256: strings.Repeat("d", 64),
			Floor: manifestLineageFloorReference{
				Path: "lineagefloors/000003.json", SchemaVersion: 1, Sequence: 3,
				SHA256: strings.Repeat("e", 64), CumulativeRecordSetSHA256: strings.Repeat("f", 64),
			},
		},
	}
	if err := verifyManifestV5Lineage("manifest.json", manifest); err == nil || !strings.Contains(err.Error(), "source-history floor") {
		t.Fatalf("oversized source-history floor sequence error = %v", err)
	}
}

func TestManifestV5ProductionLoadersAcceptCompleteAuthorityBundle(t *testing.T) {
	manifestPath := writeManifestV5AuthorityBundle(t)
	authority, _, err := loadManifestSourceAuthority(manifestPath)
	if err != nil {
		t.Fatalf("load manifest v5 source authority: %v", err)
	}
	if len(authority.BuildCells) == 0 {
		t.Fatal("manifest v5 source authority lost build cells")
	}
	profile, err := loadProfileManifest(manifestPath)
	if err != nil {
		t.Fatalf("load manifest v5 profile: %v", err)
	}
	if profile.SchemaVersion != manifestSchemaVersionV5 || len(profile.Lanes) != 1 ||
		len(profile.Lanes[0].BenchmarkBindings) != 2 {
		t.Fatalf("manifest v5 profile projection = %+v", profile)
	}
}

func TestManifestV5ProductionLoaderRejectsAuthorityTamper(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(string)
	}{
		{name: "lineage inventory", mutate: func(manifestPath string) {
			path := filepath.Join(filepath.Dir(manifestPath), "lineage.json")
			replaceManifestV5TestBytes(t, path, []byte("Example scheduler"), []byte("Changed scheduler"))
		}},
		{name: "history inventory", mutate: func(manifestPath string) {
			path := filepath.Join(filepath.Dir(manifestPath), "source_history.json")
			replaceManifestV5TestBytes(t, path, []byte(`"object_format": "sha1"`), []byte(`"object_format": "sha2"`))
		}},
		{name: "lineage floor head", mutate: func(manifestPath string) {
			mutateManifestV5TestJSON(t, manifestPath, func(root map[string]any) {
				lineage := root["lineage"].(map[string]any)
				floor := lineage["floor"].(map[string]any)
				floor["sha256"] = strings.Repeat("0", 64)
			})
		}},
		{name: "history floor head", mutate: func(manifestPath string) {
			mutateManifestV5TestJSON(t, manifestPath, func(root map[string]any) {
				history := root["source_history"].(map[string]any)
				floor := history["floor"].(map[string]any)
				floor["cumulative_record_set_sha256"] = strings.Repeat("0", 64)
			})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			manifestPath := writeManifestV5AuthorityBundle(t)
			test.mutate(manifestPath)
			if _, _, err := loadManifestSourceAuthority(manifestPath); err == nil {
				t.Fatal("tampered manifest v5 authority bundle unexpectedly passed")
			}
		})
	}
}

func TestManifestV5ExecutionProjectionMatchesLineage(t *testing.T) {
	inventory := testManifestV5ProjectionLineage()
	manifest := testManifestV5Projection(t, inventory)
	if err := validateManifestV5ExecutionProjection(manifest, inventory); err != nil {
		t.Fatalf("valid manifest v5 execution projection: %v", err)
	}
}

func TestManifestV5ExecutionProjectionAllowsHistoricalDispositionOnSelectedRoot(t *testing.T) {
	inventory := testManifestV5ProjectionLineage()
	inventory.Dispositions = append(inventory.Dispositions, lineageDisposition{
		ID: "disposition.raw-root.scheduler.example-history", SubjectKind: "raw-root",
		SubjectID: "raw-root.scheduler.example", SnapshotID: "snapshot.scheduler.example", Platform: "all",
		BuildStatus: "build-valid", CorrectnessStatus: "correctness-invalid",
		ComparabilityStatus: "noncomparable", EvidenceStatus: "evidence-complete",
		Reason: "Historical evidence predates the currently selected repaired execution.",
	})
	slices.SortFunc(inventory.Dispositions, func(left, right lineageDisposition) int {
		return strings.Compare(left.ID, right.ID)
	})
	if err := validateLineage(inventory); err != nil {
		t.Fatalf("validate historical-disposition fixture: %v", err)
	}
	manifest := testManifestV5Projection(t, inventory)
	if err := validateManifestV5ExecutionProjection(manifest, inventory); err != nil {
		t.Fatalf("historical disposition blocked selected repaired root: %v", err)
	}
}

func TestManifestV5ExecutionProjectionGovernsAliasRootTransitively(t *testing.T) {
	inventory := testManifestV5ProjectionLineage()
	inventory.RawRoots[0].Benchmarks = []string{"BenchmarkCanonical"}
	aliasRoot := lineageRawRoot{
		ID: "raw-root.scheduler.alias", ModuleID: "eventloop", Package: "example",
		Benchmarks: []string{"BenchmarkAlias"}, SourcePath: "eventloop/alias_bench_test.go",
		IdentityKind: "sha256", Identity: strings.Repeat("8", 64), SnapshotID: "snapshot.scheduler.example",
	}
	inventory.RawRoots = append(inventory.RawRoots, aliasRoot)
	slices.SortFunc(inventory.RawRoots, func(left, right lineageRawRoot) int {
		return strings.Compare(left.ID, right.ID)
	})
	inventory.Bindings[0].RawRootID = aliasRoot.ID
	for index := range inventory.Harnesses {
		inventory.Harnesses[index].PhysicalRoots = append(inventory.Harnesses[index].PhysicalRoots, lineagePhysicalRoot{
			Kind: "benchmark-root", ID: aliasRoot.ID, Path: aliasRoot.SourcePath, Identity: aliasRoot.Identity,
		})
		slices.SortFunc(inventory.Harnesses[index].PhysicalRoots, func(left, right lineagePhysicalRoot) int {
			return strings.Compare(left.ID, right.ID)
		})
	}
	if err := validateLineage(inventory); err != nil {
		t.Fatalf("validate split alias-root fixture: %v", err)
	}
	manifest := testManifestV5Projection(t, inventory)
	if err := validateManifestV5ExecutionProjection(manifest, inventory); err != nil {
		t.Fatalf("selected canonical binding did not govern alias root: %v", err)
	}
}

func TestManifestV5ExecutionProjectionRejectsLineageDrift(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*sourceManifestEnvelope, *lineageCatalog)
	}{
		{name: "unknown binding", mutate: func(manifest *sourceManifestEnvelope, _ *lineageCatalog) {
			lanes := decodeManifestV5TestLanes(t, *manifest)
			lanes[0].BenchmarkBindings[0].BindingID = "binding.scheduler.unknown"
			manifest.Lanes = encodeManifestV5TestRaw(t, lanes)
		}},
		{name: "implementation mismatch", mutate: func(manifest *sourceManifestEnvelope, _ *lineageCatalog) {
			lanes := decodeManifestV5TestLanes(t, *manifest)
			lanes[0].BenchmarkBindings[0].ImplementationID = "implementation.scheduler.extra"
			manifest.Lanes = encodeManifestV5TestRaw(t, lanes)
		}},
		{name: "module mismatch", mutate: func(manifest *sourceManifestEnvelope, _ *lineageCatalog) {
			lanes := decodeManifestV5TestLanes(t, *manifest)
			lanes[0].BenchmarkBindings[0].ModuleID = "tournament"
			manifest.Lanes = encodeManifestV5TestRaw(t, lanes)
		}},
		{name: "canonical order", mutate: func(manifest *sourceManifestEnvelope, _ *lineageCatalog) {
			lanes := decodeManifestV5TestLanes(t, *manifest)
			lanes[0].BenchmarkBindings[0], lanes[0].BenchmarkBindings[1] =
				lanes[0].BenchmarkBindings[1], lanes[0].BenchmarkBindings[0]
			manifest.Lanes = encodeManifestV5TestRaw(t, lanes)
		}},
		{name: "cross-lane duplicate", mutate: func(manifest *sourceManifestEnvelope, _ *lineageCatalog) {
			lanes := decodeManifestV5TestLanes(t, *manifest)
			duplicate := lanes[0]
			duplicate.ID = "scheduler"
			duplicate.BenchmarkBindings = duplicate.BenchmarkBindings[:1]
			manifest.Lanes = encodeManifestV5TestRaw(t, append(lanes, duplicate))
		}},
		{name: "missing harness cell", mutate: func(manifest *sourceManifestEnvelope, _ *lineageCatalog) {
			lanes := decodeManifestV5TestLanes(t, *manifest)
			lanes[0].BuildCellIDs = []string{"build-cell.eventloop.linux-amd64"}
			manifest.Lanes = encodeManifestV5TestRaw(t, lanes)
		}},
		{name: "omitted executable binding", mutate: func(manifest *sourceManifestEnvelope, _ *lineageCatalog) {
			lanes := decodeManifestV5TestLanes(t, *manifest)
			lanes[0].BenchmarkBindings = lanes[0].BenchmarkBindings[:1]
			manifest.Lanes = encodeManifestV5TestRaw(t, lanes)
		}},
		{name: "alias-only selection", mutate: func(manifest *sourceManifestEnvelope, _ *lineageCatalog) {
			lanes := decodeManifestV5TestLanes(t, *manifest)
			lanes[0].BenchmarkBindings[1] = manifestV5BindingProjection{
				BindingID: "binding.scheduler.alias", ImplementationID: "implementation.scheduler.example", ModuleID: "eventloop",
			}
			manifest.Lanes = encodeManifestV5TestRaw(t, lanes)
		}},
		{name: "source cell mismatch", mutate: func(manifest *sourceManifestEnvelope, _ *lineageCatalog) {
			manifest.SourceAuthority.BuildCells[0].GOARCH = "amd64"
		}},
		{name: "disposition subject mismatch", mutate: func(manifest *sourceManifestEnvelope, _ *lineageCatalog) {
			dispositions := decodeManifestV5TestDispositions(t, *manifest)
			dispositions[0].RawRootID = "raw-root.scheduler.example"
			manifest.RootDispositions = encodeManifestV5TestRaw(t, dispositions)
		}},
		{name: "omitted current disposition", mutate: func(manifest *sourceManifestEnvelope, _ *lineageCatalog) {
			manifest.RootDispositions = encodeManifestV5TestRaw(t, []manifestV5RootDisposition{})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			inventory := testManifestV5ProjectionLineage()
			manifest := testManifestV5Projection(t, inventory)
			test.mutate(&manifest, &inventory)
			if err := validateManifestV5ExecutionProjection(manifest, inventory); err == nil {
				t.Fatal("lineage-inconsistent manifest v5 projection unexpectedly passed")
			}
		})
	}
}

func testManifestV5(t *testing.T) map[string]any {
	t.Helper()
	authority, _, err := loadManifestSourceAuthority("../tournament/manifest.json")
	if err != nil {
		t.Fatalf("load source authority: %v", err)
	}
	cell := authority.BuildCells[0]
	lane := manifestV5Lane{
		ID: "product", Required: true, BuildCellIDs: []string{cell.ID},
		BenchmarkBindings: []manifestV5BindingProjection{{
			BindingID: "binding.example.product", ImplementationID: "implementation.example.product", ModuleID: cell.ModuleID,
		}},
		GoDiagnosticTimeoutNS: 3_300_000_000_000, RunnerWatchdogTimeoutNS: 3_600_000_000_000,
		OrchestrationWatchdogNS: 4_200_000_000_000,
	}
	return map[string]any{
		"schema_version": manifestSchemaVersionV5,
		"source_history": map[string]any{
			"path": "source_history.json", "schema_version": 2, "sha256": strings.Repeat("a", 64),
			"floor": map[string]any{
				"path": "historyfloors/000001.json", "schema_version": 1, "sequence": 1,
				"sha256": strings.Repeat("b", 64), "cumulative_record_set_sha256": strings.Repeat("c", 64),
			},
		},
		"lineage": manifestLineageReference{
			Path: "lineage.json", SchemaVersion: 3, SHA256: strings.Repeat("d", 64),
			Floor: manifestLineageFloorReference{
				Path: "lineagefloors/000003.json", SchemaVersion: 1, Sequence: 3,
				SHA256: strings.Repeat("e", 64), CumulativeRecordSetSHA256: strings.Repeat("f", 64),
			},
		},
		"source_authority":     authority,
		"measurement":          map[string]any{},
		"lanes":                []manifestV5Lane{lane},
		"root_dispositions":    []manifestV5RootDisposition{},
		"concepts":             []any{},
		"revision_variants":    []any{},
		"revision_checkpoints": []any{},
	}
}

func marshalManifestV5(t *testing.T, manifest map[string]any) []byte {
	t.Helper()
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func testManifestV5ProjectionLineage() lineageCatalog {
	inventory := testLineageDiagnostic()
	inventory.RawRoots = append(inventory.RawRoots, lineageRawRoot{
		ID: "raw-root.scheduler.zulu", ModuleID: "eventloop", Package: "example",
		Benchmarks: []string{"BenchmarkDisposed"}, SourcePath: "eventloop/disposed_bench_test.go",
		IdentityKind: "sha256", Identity: strings.Repeat("9", 64), SnapshotID: "snapshot.scheduler.example",
	})
	disposition := lineageDisposition{
		ID: "disposition.raw-root.scheduler.zulu", SubjectKind: "raw-root",
		SubjectID: "raw-root.scheduler.zulu", SnapshotID: "snapshot.scheduler.example", Platform: "all",
		BuildStatus: "build-valid", CorrectnessStatus: "correctness-unassessed",
		ComparabilityStatus: "noncomparable", EvidenceStatus: "evidence-complete",
		Reason: "The raw root is explicitly disposed for this execution catalog.",
	}
	inventory.Dispositions = append(inventory.Dispositions, disposition)
	slices.SortFunc(inventory.Dispositions, func(left, right lineageDisposition) int {
		return strings.Compare(left.ID, right.ID)
	})
	return inventory
}

func testManifestV5Projection(t *testing.T, inventory lineageCatalog) sourceManifestEnvelope {
	t.Helper()
	if err := validateLineage(inventory); err != nil {
		t.Fatalf("validate projection lineage fixture: %v", err)
	}
	selection := inventory.Harnesses[0].BuildSelection
	cell := manifestSourceCell{
		ID: selection.BuildCellID, ModuleID: selection.ModuleID, GOOS: selection.GOOS, GOARCH: selection.GOARCH,
		CGOEnabled: new(selection.CGOEnabled),
		ArchitectureFeature: manifestArchitectureFeature{
			Name: selection.ArchitectureFeature.Name, Value: selection.ArchitectureFeature.Value,
		},
		BuildTags: selection.BuildTags, SelectionFlags: selection.SelectionFlags, PackagePatterns: []string{"./..."},
	}
	lane := manifestV5Lane{
		ID: "product", Required: true, BuildCellIDs: []string{selection.BuildCellID},
		BenchmarkBindings: []manifestV5BindingProjection{
			{BindingID: "binding.scheduler.canonical", ImplementationID: "implementation.scheduler.example", ModuleID: "eventloop"},
			{BindingID: "binding.scheduler.diagnostic", ImplementationID: "implementation.scheduler.extra", ModuleID: "eventloop"},
		},
		GoDiagnosticTimeoutNS: 3_300_000_000_000, RunnerWatchdogTimeoutNS: 3_600_000_000_000,
		OrchestrationWatchdogNS: 4_200_000_000_000,
	}
	return sourceManifestEnvelope{
		SchemaVersion:   manifestSchemaVersionV5,
		SourceAuthority: manifestSourceAuthority{BuildCells: []manifestSourceCell{cell}},
		Lanes:           encodeManifestV5TestRaw(t, []manifestV5Lane{lane}),
		RootDispositions: encodeManifestV5TestRaw(t, []manifestV5RootDisposition{{
			RawRootID: "raw-root.scheduler.zulu", DispositionID: "disposition.raw-root.scheduler.zulu",
		}}),
	}
}

func decodeManifestV5TestLanes(t *testing.T, manifest sourceManifestEnvelope) []manifestV5Lane {
	t.Helper()
	var lanes []manifestV5Lane
	if err := json.Unmarshal(manifest.Lanes, &lanes); err != nil {
		t.Fatal(err)
	}
	return lanes
}

func decodeManifestV5TestDispositions(t *testing.T, manifest sourceManifestEnvelope) []manifestV5RootDisposition {
	t.Helper()
	var dispositions []manifestV5RootDisposition
	if err := json.Unmarshal(manifest.RootDispositions, &dispositions); err != nil {
		t.Fatal(err)
	}
	return dispositions
}

func encodeManifestV5TestRaw(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func writeManifestV5AuthorityBundle(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	if err := os.Mkdir(filepath.Join(directory, "historyfloors"), 0o755); err != nil {
		t.Fatalf("create history floor directory: %v", err)
	}
	if err := os.Mkdir(filepath.Join(directory, "lineagefloors"), 0o755); err != nil {
		t.Fatalf("create lineage floor directory: %v", err)
	}
	copyManifestV5TestFile(t, "../tournament/source_history.json", filepath.Join(directory, "source_history.json"))
	copyManifestV5TestFile(t, "../tournament/historyfloors/000001.json", filepath.Join(directory, "historyfloors/000001.json"))
	copyManifestV5TestFile(t, "../tournament/historyfloors/000002.json", filepath.Join(directory, "historyfloors/000002.json"))

	historyPath := filepath.Join(directory, "source_history.json")
	history, err := loadHistory(historyPath)
	if err != nil {
		t.Fatalf("load copied source history: %v", err)
	}
	historyData, err := readRegularStable(historyPath, 0o644)
	if err != nil {
		t.Fatalf("read copied source history: %v", err)
	}
	historySHA := fmt.Sprintf("%x", sha256.Sum256(historyData))
	inventoryPath := filepath.Join(directory, "lineage.json")
	floorDirectory := filepath.Join(directory, "lineagefloors")

	base := testLineage(historySHA)
	writeLineageFixture(t, inventoryPath, base)
	floorOnePath := filepath.Join(floorDirectory, "000001.json")
	floorOne, err := createLineageFloor(base, inventoryPath, history, historyPath, floorDirectory, floorOnePath)
	if err != nil {
		t.Fatalf("create manifest fixture lineage floor 1: %v", err)
	}
	writeLineageFloorFixture(t, floorOnePath, floorOne)

	secondConcept := lineageConcept{
		ID: "concept.scheduler.second", Name: "Second schema-two concept",
		SourcePath:   "docs/tournament/2026-01-18/ANALYSIS_ALTERNATETWO_HYBRID.md",
		SourceSHA256: strings.Repeat("8", 64), Status: "concept-only", Disposition: "Not implemented.",
	}
	schemaTwo := base
	schemaTwo.Concepts = append(slices.Clone(base.Concepts), secondConcept)
	writeLineageFixture(t, inventoryPath, schemaTwo)
	floorTwoPath := filepath.Join(floorDirectory, "000002.json")
	floorTwo, err := createLineageFloor(schemaTwo, inventoryPath, history, historyPath, floorDirectory, floorTwoPath)
	if err != nil {
		t.Fatalf("create manifest fixture lineage floor 2: %v", err)
	}
	writeLineageFloorFixture(t, floorTwoPath, floorTwo)

	authority, _, err := loadManifestSourceAuthority("../tournament/manifest.json")
	if err != nil {
		t.Fatalf("load live source authority fixture: %v", err)
	}
	cell := authority.BuildCells[0]
	inventory := testManifestV5ProjectionLineage()
	inventory.SourceHistorySHA256 = historySHA
	inventory.Concepts = append(inventory.Concepts, secondConcept)
	slices.SortFunc(inventory.Concepts, func(left, right lineageConcept) int {
		return strings.Compare(left.ID, right.ID)
	})
	retargetManifestV5Lineage(&inventory, cell)
	writeLineageFixture(t, inventoryPath, inventory)
	floorThreePath := filepath.Join(floorDirectory, "000003.json")
	floorThree, err := createLineageFloor(inventory, inventoryPath, history, historyPath, floorDirectory, floorThreePath)
	if err != nil {
		t.Fatalf("create manifest fixture lineage floor 3: %v", err)
	}
	writeLineageFloorFixture(t, floorThreePath, floorThree)

	liveManifestData, err := readRegularStable("../tournament/manifest.json", 0o644)
	if err != nil {
		t.Fatalf("read live manifest fixture: %v", err)
	}
	var liveEnvelope sourceManifestEnvelope
	if err := decodeManifestReference(liveManifestData, &liveEnvelope); err != nil {
		t.Fatalf("decode live manifest fixture: %v", err)
	}
	lineageData, err := readRegularStable(inventoryPath, 0o644)
	if err != nil {
		t.Fatalf("read manifest fixture lineage: %v", err)
	}
	floorThreeData, err := readRegularStable(floorThreePath, 0o644)
	if err != nil {
		t.Fatalf("read manifest fixture lineage floor 3: %v", err)
	}
	lane := manifestV5Lane{
		ID: "product", Required: true, BuildCellIDs: []string{cell.ID},
		BenchmarkBindings: []manifestV5BindingProjection{
			{BindingID: "binding.scheduler.canonical", ImplementationID: "implementation.scheduler.example", ModuleID: cell.ModuleID},
			{BindingID: "binding.scheduler.diagnostic", ImplementationID: "implementation.scheduler.extra", ModuleID: cell.ModuleID},
		},
		GoDiagnosticTimeoutNS: 3_300_000_000_000, RunnerWatchdogTimeoutNS: 3_600_000_000_000,
		OrchestrationWatchdogNS: 4_200_000_000_000,
	}
	manifest := map[string]any{
		"schema_version": manifestSchemaVersionV5, "source_history": liveEnvelope.SourceHistory,
		"lineage": manifestLineageReference{
			Path: "lineage.json", SchemaVersion: 3, SHA256: fmt.Sprintf("%x", sha256.Sum256(lineageData)),
			Floor: manifestLineageFloorReference{
				Path: "lineagefloors/000003.json", SchemaVersion: 1, Sequence: 3,
				SHA256:                    fmt.Sprintf("%x", sha256.Sum256(floorThreeData)),
				CumulativeRecordSetSHA256: floorThree.CumulativeRecordSetSHA256,
			},
		},
		"source_authority": authority, "measurement": liveEnvelope.Measurement,
		"lanes": []manifestV5Lane{lane},
		"root_dispositions": []manifestV5RootDisposition{{
			RawRootID: "raw-root.scheduler.zulu", DispositionID: "disposition.raw-root.scheduler.zulu",
		}},
		"concepts": []any{}, "revision_variants": []any{}, "revision_checkpoints": []any{},
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("encode manifest v5 authority fixture: %v", err)
	}
	manifestPath := filepath.Join(directory, "manifest.json")
	if err := os.WriteFile(manifestPath, append(manifestData, '\n'), 0o644); err != nil {
		t.Fatalf("write manifest v5 authority fixture: %v", err)
	}
	return manifestPath
}

func retargetManifestV5Lineage(inventory *lineageCatalog, cell manifestSourceCell) {
	for index := range inventory.Harnesses {
		inventory.Harnesses[index].BuildSelection.BuildCellID = cell.ID
		inventory.Harnesses[index].BuildSelection.ModuleID = cell.ModuleID
		inventory.Harnesses[index].BuildSelection.GOOS = cell.GOOS
		inventory.Harnesses[index].BuildSelection.GOARCH = cell.GOARCH
		inventory.Harnesses[index].BuildSelection.CGOEnabled = *cell.CGOEnabled
		inventory.Harnesses[index].BuildSelection.ArchitectureFeature = lineageArchitectureFeature{
			Name: cell.ArchitectureFeature.Name, Value: cell.ArchitectureFeature.Value,
		}
		inventory.Harnesses[index].BuildSelection.BuildTags = slices.Clone(cell.BuildTags)
		inventory.Harnesses[index].BuildSelection.SelectionFlags = slices.Clone(cell.SelectionFlags)
	}
	for index := range inventory.Dispositions {
		if inventory.Dispositions[index].SubjectKind == "binding" {
			inventory.Dispositions[index].Platform = cell.GOOS + "/" + cell.GOARCH
		}
	}
}

func copyManifestV5TestFile(t *testing.T, source, destination string) {
	t.Helper()
	data, err := readRegularStable(source, 0o644)
	if err != nil {
		t.Fatalf("read fixture %s: %v", source, err)
	}
	if err := os.WriteFile(destination, data, 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", destination, err)
	}
}

func replaceManifestV5TestBytes(t *testing.T, path string, old, replacement []byte) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	mutated := bytes.Replace(data, old, replacement, 1)
	if bytes.Equal(mutated, data) {
		t.Fatalf("fixture %s omits replacement target %q", path, old)
	}
	if err := os.WriteFile(path, mutated, 0o644); err != nil {
		t.Fatal(err)
	}
}

func mutateManifestV5TestJSON(t *testing.T, path string, mutate func(map[string]any)) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	mutate(root)
	data, err = json.MarshalIndent(root, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
