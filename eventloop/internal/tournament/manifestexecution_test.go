package tournament

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func checkManifestV5MatchesImplementations(t *testing.T, lineageData []byte) {
	t.Helper()
	lineage := decodeRetainedLineage(t, lineageData)
	checkManifestV5ImplementationSet(t, lineage)
}

func checkManifestV5ImplementationSet(t *testing.T, lineage retainedLineageCatalog) {
	t.Helper()
	implementations := make(map[string]struct{}, len(lineage.Implementations))
	for _, implementation := range lineage.Implementations {
		implementations[implementation.ID] = struct{}{}
	}
	for _, implementation := range Implementations() {
		id := implementation.VariantID
		if strings.HasPrefix(id, "scheduler.main.") {
			id = "scheduler.main"
		}
		if _, ok := implementations[id]; !ok {
			t.Errorf("scheduler implementation %q projects to absent lineage implementation %q", implementation.VariantID, id)
		}
	}
	for _, implementation := range PromiseImplementations() {
		if _, ok := implementations[implementation.VariantID]; !ok {
			t.Errorf("promise implementation %q is absent from lineage", implementation.VariantID)
		}
	}
	if _, ok := implementations["scheduler.libuv.native"]; !ok {
		t.Error("native libuv implementation is absent from lineage")
	}
}

func checkManifestV5RetainedExecutables(t *testing.T, want map[string]retainedExecutable, lineageData []byte) {
	t.Helper()
	lineage := decodeRetainedLineage(t, lineageData)
	implementations := make(map[string]struct{}, len(lineage.Implementations))
	for _, implementation := range lineage.Implementations {
		implementations[implementation.ID] = struct{}{}
	}
	for id := range want {
		lineageID := id
		if strings.HasPrefix(id, "scheduler.main.") {
			lineageID = "scheduler.main"
		}
		if _, ok := implementations[lineageID]; !ok {
			t.Errorf("immutable executable %q projects to absent lineage implementation %q", id, lineageID)
		}
	}
}

func checkManifestV5HistoricalSourcePackages(t *testing.T, lineageData []byte) {
	t.Helper()
	lineage := decodeRetainedLineage(t, lineageData)
	sourcePaths := make(map[string]struct{}, len(lineage.Snapshots))
	for _, snapshot := range lineage.Snapshots {
		sourcePaths[snapshot.SourcePath] = struct{}{}
	}
	entries, err := os.ReadDir("..")
	if err != nil {
		t.Fatalf("read internal source packages: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || !(strings.HasPrefix(name, "alternate") ||
			strings.HasPrefix(name, "promisealt") || name == "gojabaseline" || name == "libuvbaseline") {
			continue
		}
		path := filepath.ToSlash(filepath.Join("eventloop", "internal", name))
		if _, ok := sourcePaths[path]; !ok {
			t.Errorf("historical implementation package %q has no lineage snapshot", eventloopPackage+"/internal/"+name)
		}
	}
}

func checkManifestV5CoverageContract(t *testing.T, manifest tournamentManifest) {
	t.Helper()
	if manifest.Lineage.Path != "lineage.json" || manifest.Lineage.SchemaVersion != 3 {
		t.Errorf("lineage reference = %+v", manifest.Lineage)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(lineageJSON)); got != manifest.Lineage.SHA256 {
		t.Errorf("lineage SHA-256 = %s, want %s", got, manifest.Lineage.SHA256)
	}
	if manifest.SourceHistory.Path != "source_history.json" || manifest.SourceHistory.SchemaVersion != 2 {
		t.Errorf("source history reference = %+v", manifest.SourceHistory)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(sourceHistoryJSON)); got != manifest.SourceHistory.SHA256 {
		t.Errorf("source history SHA-256 = %s, want %s", got, manifest.SourceHistory.SHA256)
	}
	checkManifestSourceAuthority(t, manifest.SourceAuthority)
	checkManifestMeasurement(t, manifest.Measurement)
	if manifest.Variants != nil || manifest.VariantGroups != nil {
		t.Error("manifest v5 retains removed schema-4 inference fields")
	}
	if _, _, err := projectBenchmarkRoots(manifest, lineageJSON); err != nil {
		t.Errorf("project schema-5 benchmark roots: %v", err)
	}
}

func checkLineageV3SemanticFamilies(t *testing.T, manifest tournamentManifest, lineage retainedLineageCatalog) {
	t.Helper()
	if manifest.SchemaVersion != 5 {
		t.Errorf("lineage schema 3 is projected by manifest schema %d, want 5", manifest.SchemaVersion)
	}
	if len(lineage.RawRoots) == 0 || len(lineage.Harnesses) == 0 || len(lineage.Workloads) == 0 || len(lineage.Bindings) == 0 {
		t.Errorf("lineage schema 3 execution census is incomplete: raw roots=%d harnesses=%d workloads=%d bindings=%d", len(lineage.RawRoots), len(lineage.Harnesses), len(lineage.Workloads), len(lineage.Bindings))
	}
	checkManifestV5ImplementationSet(t, lineage)
	for _, alias := range lineage.Aliases {
		if alias.Rerun {
			t.Errorf("exact lineage alias %q is rerunnable", alias.ID)
		}
	}
}

func TestManifestV5LineageConsumersUseExecutionAuthority(t *testing.T) {
	lineage := retainedLineageCatalog{
		SchemaVersion: 3,
		RawRoots:      []retainedLineageRawRoot{{ID: "raw-root.example"}},
		Harnesses:     []retainedLineageID{{ID: "harness.example"}},
		Workloads:     []retainedLineageID{{ID: "workload.example"}},
		Bindings:      []retainedLineageBinding{{ID: "binding.example"}},
	}
	seen := make(map[string]struct{})
	appendImplementation := func(id string) {
		if _, duplicate := seen[id]; duplicate {
			return
		}
		seen[id] = struct{}{}
		lineage.Implementations = append(lineage.Implementations, retainedLineageImplementation{ID: id})
	}
	for _, implementation := range Implementations() {
		id := implementation.VariantID
		if strings.HasPrefix(id, "scheduler.main.") {
			id = "scheduler.main"
		}
		appendImplementation(id)
	}
	for _, implementation := range PromiseImplementations() {
		appendImplementation(implementation.VariantID)
	}
	appendImplementation("scheduler.libuv.native")

	entries, err := os.ReadDir("..")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() && (strings.HasPrefix(name, "alternate") || strings.HasPrefix(name, "promisealt") ||
			name == "gojabaseline" || name == "libuvbaseline") {
			lineage.Snapshots = append(lineage.Snapshots, retainedLineageSnapshot{
				ID:         "snapshot." + name,
				SourcePath: filepath.ToSlash(filepath.Join("eventloop", "internal", name)),
			})
		}
	}
	data, err := json.Marshal(lineage)
	if err != nil {
		t.Fatal(err)
	}
	checkManifestV5MatchesImplementations(t, data)
	checkManifestV5RetainedExecutables(t, map[string]retainedExecutable{
		"scheduler.main.auto":    {},
		"scheduler.libuv.native": {},
	}, data)
	checkManifestV5HistoricalSourcePackages(t, data)
	checkLineageV3SemanticFamilies(t, tournamentManifest{SchemaVersion: 5}, lineage)
}

func decodeRetainedLineage(t *testing.T, data []byte) retainedLineageCatalog {
	t.Helper()
	var lineage retainedLineageCatalog
	if err := json.Unmarshal(data, &lineage); err != nil {
		t.Fatalf("decode lineage: %v", err)
	}
	if lineage.SchemaVersion != 3 {
		t.Fatalf("lineage schema = %d, want 3", lineage.SchemaVersion)
	}
	return lineage
}

func TestManifestV5StructuralProjectionDecodes(t *testing.T) {
	data := manifestV5ExecutionFixture(t)
	manifest, err := decodeManifest(data)
	if err != nil {
		t.Fatalf("decode manifest v5 execution projection: %v", err)
	}
	if manifest.SchemaVersion != 5 {
		t.Fatalf("schema version = %d, want 5", manifest.SchemaVersion)
	}
	if manifest.Variants != nil || manifest.VariantGroups != nil {
		t.Fatal("manifest v5 unexpectedly decoded removed inference fields")
	}
	if len(manifest.Lanes) != 1 || len(manifest.Lanes[0].BuildCellIDs) != 1 ||
		len(manifest.Lanes[0].BenchmarkBindings) != 1 {
		t.Fatalf("manifest v5 lane projection = %+v", manifest.Lanes)
	}
	if manifest.Lanes[0].BenchmarkBindings[0].BindingID != "binding.example.product" {
		t.Fatalf("manifest v5 binding projection = %+v", manifest.Lanes[0].BenchmarkBindings[0])
	}
	if manifest.RootDispositions == nil {
		t.Fatal("manifest v5 root dispositions decoded as nil")
	}
}

func TestManifestV5StructuralProjectionRejectsInferenceAndNullFields(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "top-level v4 variants", mutate: func(root map[string]any) {
			root["variants"] = []any{}
		}},
		{name: "lane v4 benchmark", mutate: func(root map[string]any) {
			manifestV5FixtureLane(root)["benchmarks"] = []any{}
		}},
		{name: "null lanes", mutate: func(root map[string]any) {
			root["lanes"] = nil
		}},
		{name: "null build cells", mutate: func(root map[string]any) {
			manifestV5FixtureLane(root)["build_cell_ids"] = nil
		}},
		{name: "unknown binding field", mutate: func(root map[string]any) {
			manifestV5FixtureBinding(root)["workload_id"] = "workload.example.product"
		}},
		{name: "null binding field", mutate: func(root map[string]any) {
			manifestV5FixtureBinding(root)["implementation_id"] = nil
		}},
		{name: "null root dispositions", mutate: func(root map[string]any) {
			root["root_dispositions"] = nil
		}},
		{name: "v4 root disposition fields", mutate: func(root map[string]any) {
			root["root_dispositions"] = []any{map[string]any{
				"package": "example", "benchmark": "BenchmarkExample",
				"raw_root_id":    "raw-root.example.product",
				"disposition_id": "disposition.example.product",
			}}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := manifestV5ExecutionFixtureMap(t)
			test.mutate(root)
			if _, err := decodeManifest(marshalManifestExecutionFixture(t, root)); err == nil {
				t.Fatal("invalid manifest v5 execution projection unexpectedly passed")
			}
		})
	}
}

func manifestV5ExecutionFixture(t *testing.T) []byte {
	t.Helper()
	return marshalManifestExecutionFixture(t, manifestV5ExecutionFixtureMap(t))
}

func manifestV5ExecutionFixtureMap(t *testing.T) map[string]any {
	t.Helper()
	var root map[string]any
	if err := json.Unmarshal(manifestJSON, &root); err != nil {
		t.Fatalf("decode embedded manifest fixture: %v", err)
	}
	root["schema_version"] = 5
	delete(root, "variants")
	delete(root, "variant_groups")
	lineage := root["lineage"].(map[string]any)
	lineage["schema_version"] = 3
	lineageFloor := lineage["floor"].(map[string]any)
	lineageFloor["path"] = "lineagefloors/000003.json"
	lineageFloor["sequence"] = 3
	authority := root["source_authority"].(map[string]any)
	cell := authority["build_cells"].([]any)[0].(map[string]any)
	root["lanes"] = []any{map[string]any{
		"id": "product", "required": true,
		"build_cell_ids": []any{cell["id"]},
		"benchmark_bindings": []any{map[string]any{
			"binding_id":        "binding.example.product",
			"implementation_id": "implementation.example.product",
			"module_id":         cell["module_id"],
		}},
		"go_diagnostic_timeout_ns":          int64(3_300_000_000_000),
		"runner_watchdog_timeout_ns":        int64(3_600_000_000_000),
		"orchestration_watchdog_timeout_ns": int64(4_200_000_000_000),
	}}
	root["root_dispositions"] = []any{}
	return root
}

func manifestV5FixtureLane(root map[string]any) map[string]any {
	return root["lanes"].([]any)[0].(map[string]any)
}

func manifestV5FixtureBinding(root map[string]any) map[string]any {
	return manifestV5FixtureLane(root)["benchmark_bindings"].([]any)[0].(map[string]any)
}

func marshalManifestExecutionFixture(t *testing.T, root map[string]any) []byte {
	t.Helper()
	data, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
