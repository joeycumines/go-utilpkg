package tournament

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
)

//go:embed manifest.json
var manifestJSON []byte

//go:embed source_history.json
var sourceHistoryJSON []byte

//go:embed historyfloors/000002.json
var sourceHistoryFloorJSON []byte

//go:embed lineage.json
var lineageJSON []byte

//go:embed lineagefloors/000003.json
var lineageFloorJSON []byte

type tournamentManifest struct {
	SchemaVersion       int                       `json:"schema_version"`
	SourceHistory       manifestSourceHistory     `json:"source_history"`
	Lineage             manifestLineage           `json:"lineage"`
	SourceAuthority     manifestSourceAuthority   `json:"source_authority"`
	Measurement         manifestMeasurement       `json:"measurement"`
	Variants            []manifestVariant         `json:"variants"`
	VariantGroups       map[string][]string       `json:"variant_groups"`
	Lanes               []manifestLane            `json:"lanes"`
	Concepts            []manifestConcept         `json:"concepts"`
	RevisionVariants    []manifestRevision        `json:"revision_variants"`
	RevisionCheckpoints []string                  `json:"revision_checkpoints"`
	RootDispositions    []manifestRootDisposition `json:"root_dispositions"`
}

type manifestMeasurement struct {
	SampleCount        int               `json:"sample_count"`
	BenchmarkTime      manifestTime      `json:"benchmark_time"`
	Benchmem           bool              `json:"benchmem"`
	GoFlags            []string          `json:"go_flags"`
	CPUCardinality     int               `json:"cpu_cardinality"`
	PackageParallelism int               `json:"package_parallelism"`
	Environment        map[string]string `json:"environment"`
}

type manifestTime struct {
	Mode  string `json:"mode"`
	Value int64  `json:"value"`
}

type manifestSourceHistory struct {
	Path          string                     `json:"path"`
	SchemaVersion int                        `json:"schema_version"`
	SHA256        string                     `json:"sha256"`
	Floor         manifestSourceHistoryFloor `json:"floor"`
}

type manifestSourceHistoryFloor struct {
	Path                      string `json:"path"`
	SchemaVersion             int    `json:"schema_version"`
	Sequence                  int    `json:"sequence"`
	SHA256                    string `json:"sha256"`
	CumulativeRecordSetSHA256 string `json:"cumulative_record_set_sha256"`
}

type manifestLineage struct {
	Path          string               `json:"path"`
	SchemaVersion int                  `json:"schema_version"`
	SHA256        string               `json:"sha256"`
	Floor         manifestLineageFloor `json:"floor"`
}

type manifestLineageFloor struct {
	Path                      string `json:"path"`
	SchemaVersion             int    `json:"schema_version"`
	Sequence                  int    `json:"sequence"`
	SHA256                    string `json:"sha256"`
	CumulativeRecordSetSHA256 string `json:"cumulative_record_set_sha256"`
}

type manifestSourceAuthority struct {
	SchemaVersion  int                    `json:"schema_version"`
	Policy         string                 `json:"policy"`
	PhysicalPolicy manifestPhysicalPolicy `json:"physical_policy"`
	Modules        []manifestSourceModule `json:"modules"`
	BuildCells     []manifestSourceCell   `json:"build_cells"`
}

type manifestPhysicalPolicy struct {
	ID            string   `json:"id"`
	RootControls  []string `json:"root_controls"`
	Trees         []string `json:"trees"`
	RuntimeAssets []string `json:"runtime_assets"`
}

type manifestSourceModule struct {
	ID         string `json:"id"`
	Root       string `json:"root"`
	ModulePath string `json:"module_path"`
	Buildable  *bool  `json:"buildable"`
}

type manifestSourceCell struct {
	ID                  string                      `json:"id"`
	ModuleID            string                      `json:"module_id"`
	GOOS                string                      `json:"goos"`
	GOARCH              string                      `json:"goarch"`
	CGOEnabled          *bool                       `json:"cgo_enabled"`
	ArchitectureFeature manifestArchitectureFeature `json:"architecture_feature"`
	BuildTags           []string                    `json:"build_tags"`
	SelectionFlags      []string                    `json:"selection_flags"`
	PackagePatterns     []string                    `json:"package_patterns"`
}

type manifestArchitectureFeature struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type manifestVariant struct {
	Kind          string   `json:"kind"`
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	SourcePackage string   `json:"source_package"`
	Aliases       []string `json:"aliases"`
	OriginCommit  string   `json:"origin_commit"`
	OriginTree    string   `json:"origin_tree"`
	Capabilities  []string `json:"capabilities"`
}

type manifestLane struct {
	ID                          string                         `json:"id"`
	Package                     string                         `json:"package"`
	Required                    bool                           `json:"required"`
	Benchmarks                  []string                       `json:"benchmarks"`
	BenchmarkVariantGroups      map[string]string              `json:"benchmark_variant_groups"`
	BenchmarkGOOS               map[string][]string            `json:"benchmark_goos"`
	BenchmarkLeaves             map[string][]string            `json:"benchmark_leaves"`
	BenchmarkVariantExtraLeaves map[string]map[string][]string `json:"benchmark_variant_extra_leaves"`
	VariantIDs                  []string                       `json:"variant_ids"`
	DefaultVariantID            string                         `json:"default_variant_id"`
	GoDiagnosticTimeoutNS       int64                          `json:"go_diagnostic_timeout_ns"`
	RunnerWatchdogTimeoutNS     int64                          `json:"runner_watchdog_timeout_ns"`
	OrchestrationWatchdogNS     int64                          `json:"orchestration_watchdog_timeout_ns"`
	WorkloadDefinitions         map[string]manifestWorkload    `json:"workload_definitions"`
	BuildCellIDs                []string                       `json:"build_cell_ids"`
	BenchmarkBindings           []manifestBindingProjection    `json:"benchmark_bindings"`
}

type manifestBindingProjection struct {
	BindingID        string `json:"binding_id"`
	ImplementationID string `json:"implementation_id"`
	ModuleID         string `json:"module_id"`
}

type manifestWorkload struct {
	ID            string   `json:"id"`
	HarnessFiles  []string `json:"harness_files"`
	HarnessSHA256 string   `json:"harness_sha256"`
}

type manifestConcept struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Status         string `json:"status"`
	Benchmarkable  bool   `json:"benchmarkable"`
	SourceDocument string `json:"source_document"`
	Disposition    string `json:"disposition"`
}

type manifestRevision struct {
	ID     string `json:"id"`
	Commit string `json:"commit"`
	Name   string `json:"name"`
}

type manifestRootDisposition struct {
	Package       string `json:"package"`
	Benchmark     string `json:"benchmark"`
	RawRootID     string `json:"raw_root_id"`
	DispositionID string `json:"disposition_id"`
}

func TestManifestMatchesImplementations(t *testing.T) {
	manifest := loadManifest(t)
	if manifest.SchemaVersion == 5 {
		checkManifestV5MatchesImplementations(t, lineageJSON)
		return
	}
	want := make(map[string]manifestVariant)
	for _, implementation := range Implementations() {
		want[implementation.VariantID] = manifestVariant{
			Kind:          "scheduler",
			ID:            implementation.VariantID,
			Name:          implementation.Name,
			SourcePackage: implementation.SourcePackage,
			Aliases:       []string{implementation.Name},
			OriginCommit:  implementation.OriginCommit,
			OriginTree:    implementation.OriginTree,
			Capabilities:  implementation.Capabilities,
		}
	}
	for _, implementation := range PromiseImplementations() {
		capabilities := []string(nil)
		if implementation.Race != nil {
			capabilities = []string{"race"}
		}
		aliases := []string{implementation.Name}
		switch implementation.VariantID {
		case "promise.alt-four.main-snapshot":
			aliases = append(aliases, "promise.alt-four.main-snapshot-2026-01")
		case "promise.alt-five.original-chained":
			aliases = append(aliases, "promise.alt-five.original-chained-snapshot")
		}
		want[implementation.VariantID] = manifestVariant{
			Kind:          "promise",
			ID:            implementation.VariantID,
			Name:          implementation.Name,
			SourcePackage: implementation.SourcePackage,
			Aliases:       aliases,
			OriginCommit:  implementation.OriginCommit,
			OriginTree:    implementation.OriginTree,
			Capabilities:  capabilities,
		}
	}
	want["scheduler.libuv.native"] = manifestVariant{
		Kind:          "scheduler",
		ID:            "scheduler.libuv.native",
		Name:          "libuv",
		SourcePackage: eventloopPackage + "/internal/libuvbaseline",
		Aliases:       []string{},
		OriginCommit:  "system-library",
		OriginTree:    "system-library",
		Capabilities:  []string{"native-external-baseline"},
	}

	if len(manifest.Variants) != len(want) {
		t.Fatalf("manifest has %d variants, want %d", len(manifest.Variants), len(want))
	}
	seen := make(map[string]struct{}, len(manifest.Variants))
	aliases := make(map[string]string)
	for _, got := range manifest.Variants {
		if _, ok := seen[got.ID]; ok {
			t.Errorf("duplicate variant ID %q", got.ID)
			continue
		}
		seen[got.ID] = struct{}{}
		expected, ok := want[got.ID]
		if !ok {
			t.Errorf("unexpected variant ID %q", got.ID)
			continue
		}
		if got.Kind != expected.Kind || got.Name != expected.Name || got.SourcePackage != expected.SourcePackage ||
			got.OriginCommit != expected.OriginCommit || got.OriginTree != expected.OriginTree ||
			!slices.Equal(got.Aliases, expected.Aliases) ||
			!slices.Equal(got.Capabilities, expected.Capabilities) {
			t.Errorf("variant %q metadata = %+v, want %+v", got.ID, got, expected)
		}
		for _, alias := range got.Aliases {
			if owner, ok := aliases[alias]; ok {
				t.Errorf("alias %q belongs to both %q and %q", alias, owner, got.ID)
			} else {
				aliases[alias] = got.ID
			}
		}
	}
	for alias, owner := range aliases {
		if _, ok := seen[alias]; ok {
			t.Errorf("variant %q alias %q collides with a canonical variant ID", owner, alias)
		}
	}
}

func TestManifestRegistersHistoricalSourcePackages(t *testing.T) {
	manifest := loadManifest(t)
	if manifest.SchemaVersion == 5 {
		checkManifestV5HistoricalSourcePackages(t, lineageJSON)
		return
	}
	registered := make(map[string][]string, len(manifest.Variants))
	for _, variant := range manifest.Variants {
		if variant.SourcePackage == "" {
			t.Errorf("variant %q has no source package", variant.ID)
			continue
		}
		registered[variant.SourcePackage] = append(registered[variant.SourcePackage], variant.ID)
	}

	entries, err := os.ReadDir("..")
	if err != nil {
		t.Fatalf("read internal source packages: %v", err)
	}
	internalPrefix := eventloopPackage + "/internal/"
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || !(strings.HasPrefix(name, "alternate") ||
			strings.HasPrefix(name, "promisealt") || name == "gojabaseline" || name == "libuvbaseline") {
			continue
		}
		sourcePackage := internalPrefix + name
		if len(registered[sourcePackage]) == 0 {
			t.Errorf("historical implementation package %q has no manifest variant", sourcePackage)
		}
	}

	for sourcePackage, variantIDs := range registered {
		if !strings.HasPrefix(sourcePackage, internalPrefix) {
			continue
		}
		relative := strings.TrimPrefix(sourcePackage, internalPrefix)
		if relative == "" || strings.Contains(relative, "/") {
			t.Errorf("variants %v have invalid historical source package %q", variantIDs, sourcePackage)
			continue
		}
		info, err := os.Stat(filepath.Join("..", relative))
		if err != nil {
			t.Errorf("variants %v source package %q: %v", variantIDs, sourcePackage, err)
		} else if !info.IsDir() {
			t.Errorf("variants %v source package %q is not a directory", variantIDs, sourcePackage)
		}
	}
}

func TestManifestCoverageContract(t *testing.T) {
	manifest := loadManifest(t)
	if manifest.SchemaVersion == 5 {
		checkManifestV5CoverageContract(t, manifest)
		return
	}
	if manifest.SchemaVersion != 4 {
		t.Errorf("schema version = %d, want 4", manifest.SchemaVersion)
	}
	checkManifestSourceAuthority(t, manifest.SourceAuthority)
	checkManifestMeasurement(t, manifest.Measurement)
	if manifest.SourceHistory.Path != "source_history.json" || manifest.SourceHistory.SchemaVersion != 2 {
		t.Errorf("source history reference = %+v", manifest.SourceHistory)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(sourceHistoryJSON)); got != manifest.SourceHistory.SHA256 {
		t.Errorf("source history SHA-256 = %s, want %s", got, manifest.SourceHistory.SHA256)
	}
	if manifest.SourceHistory.Floor.Path != "historyfloors/000002.json" ||
		manifest.SourceHistory.Floor.SchemaVersion != 1 || manifest.SourceHistory.Floor.Sequence != 2 ||
		manifest.SourceHistory.Floor.CumulativeRecordSetSHA256 != "04c4cfe87d6b527ceda6ff18e24a0cb614652f0cb86bba891ff5bf85e2c9cdeb" {
		t.Errorf("source history floor reference = %+v", manifest.SourceHistory.Floor)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(sourceHistoryFloorJSON)); got != manifest.SourceHistory.Floor.SHA256 {
		t.Errorf("source history floor SHA-256 = %s, want %s", got, manifest.SourceHistory.Floor.SHA256)
	}
	if manifest.Lineage.Path != "lineage.json" || manifest.Lineage.SchemaVersion != 2 {
		t.Errorf("lineage reference = %+v", manifest.Lineage)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(lineageJSON)); got != manifest.Lineage.SHA256 {
		t.Errorf("lineage SHA-256 = %s, want %s", got, manifest.Lineage.SHA256)
	}
	if manifest.Lineage.Floor.Path != "lineagefloors/000003.json" ||
		manifest.Lineage.Floor.SchemaVersion != 1 || manifest.Lineage.Floor.Sequence != 3 ||
		manifest.Lineage.Floor.CumulativeRecordSetSHA256 != "e85f6a46344c7311d4d6fe8cb9f89b3bcb0551f7529513039284b0e12d415865" {
		t.Errorf("lineage floor reference = %+v", manifest.Lineage.Floor)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(lineageFloorJSON)); got != manifest.Lineage.Floor.SHA256 {
		t.Errorf("lineage floor SHA-256 = %s, want %s", got, manifest.Lineage.Floor.SHA256)
	}

	stableID := regexp.MustCompile(`^[a-z0-9]+(?:[.-][a-z0-9]+)+$`)
	identities := make(map[string]string, len(manifest.Variants)+len(manifest.Concepts)+len(manifest.RevisionVariants))
	variants := make(map[string]struct{}, len(manifest.Variants))
	for _, variant := range manifest.Variants {
		checkManifestID(t, identities, stableID, variant.ID, "executable variant")
		variants[variant.ID] = struct{}{}
	}
	for _, concept := range manifest.Concepts {
		checkManifestID(t, identities, stableID, concept.ID, "concept")
		if concept.Name == "" || concept.SourceDocument == "" || concept.Disposition == "" {
			t.Errorf("incomplete concept metadata: %+v", concept)
		}
		if concept.Status != "concept-only" {
			t.Errorf("concept %q status = %q, want concept-only", concept.ID, concept.Status)
		}
		if concept.Benchmarkable {
			t.Errorf("concept %q is marked benchmarkable without an implementation", concept.ID)
		}
		if filepath.IsAbs(concept.SourceDocument) || strings.Contains(filepath.ToSlash(concept.SourceDocument), "../") {
			t.Errorf("concept %q has unsafe source document %q", concept.ID, concept.SourceDocument)
			continue
		}
		path := filepath.Join("..", "..", filepath.FromSlash(concept.SourceDocument))
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("concept %q source document %q: %v", concept.ID, concept.SourceDocument, err)
		} else if !info.Mode().IsRegular() {
			t.Errorf("concept %q source document %q is not a regular file", concept.ID, concept.SourceDocument)
		}
	}
	for _, revision := range manifest.RevisionVariants {
		checkManifestID(t, identities, stableID, revision.ID, "revision variant")
		if revision.Name == "" || revision.Commit == "" {
			t.Errorf("incomplete revision variant metadata: %+v", revision)
		}
	}
	variantGroups := make(map[string]map[string]struct{}, len(manifest.VariantGroups))
	groupID := regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	for group, variantIDs := range manifest.VariantGroups {
		if !groupID.MatchString(group) || len(variantIDs) == 0 {
			t.Errorf("invalid or empty variant group %q", group)
		}
		members := make(map[string]struct{}, len(variantIDs))
		for _, variantID := range variantIDs {
			if _, ok := variants[variantID]; !ok {
				t.Errorf("variant group %q references unknown variant %q", group, variantID)
			}
			if _, ok := members[variantID]; ok {
				t.Errorf("variant group %q repeats variant %q", group, variantID)
			}
			members[variantID] = struct{}{}
		}
		variantGroups[group] = members
	}
	lanes := make(map[string]struct{}, len(manifest.Lanes))
	workloads := make(map[string]string)
	workloadID := regexp.MustCompile(`^workload\.[a-z0-9]+(?:[.-][a-z0-9]+)+\.v[1-9][0-9]*$`)
	for _, lane := range manifest.Lanes {
		if lane.ID == "" || lane.Package == "" || len(lane.Benchmarks) == 0 {
			t.Errorf("incomplete lane metadata: %+v", lane)
		}
		if lane.GoDiagnosticTimeoutNS <= 0 ||
			lane.RunnerWatchdogTimeoutNS < lane.GoDiagnosticTimeoutNS ||
			lane.OrchestrationWatchdogNS <= lane.RunnerWatchdogTimeoutNS ||
			lane.OrchestrationWatchdogNS-lane.RunnerWatchdogTimeoutNS < 600_000_000_000 {
			t.Errorf(
				"lane %q timeout policy = diagnostic %d, runner %d, orchestration %d",
				lane.ID,
				lane.GoDiagnosticTimeoutNS,
				lane.RunnerWatchdogTimeoutNS,
				lane.OrchestrationWatchdogNS,
			)
		}
		if _, ok := lanes[lane.ID]; ok {
			t.Errorf("duplicate lane ID %q", lane.ID)
		}
		lanes[lane.ID] = struct{}{}
		benchmarks := make(map[string]struct{}, len(lane.Benchmarks))
		for _, benchmark := range lane.Benchmarks {
			if _, ok := benchmarks[benchmark]; ok {
				t.Errorf("lane %q repeats benchmark %q", lane.ID, benchmark)
			}
			benchmarks[benchmark] = struct{}{}
			definition, ok := lane.WorkloadDefinitions[benchmark]
			if !ok {
				t.Errorf("lane %q benchmark %q has no workload definition", lane.ID, benchmark)
				continue
			}
			checkManifestWorkload(t, lane.ID, benchmark, definition, workloadID)
			if owner, ok := workloads[definition.ID]; ok {
				t.Errorf("workload ID %q belongs to both %s and %s/%s", definition.ID, owner, lane.ID, benchmark)
			} else {
				workloads[definition.ID] = lane.ID + "/" + benchmark
			}
		}
		for benchmark := range lane.WorkloadDefinitions {
			if _, ok := benchmarks[benchmark]; !ok {
				t.Errorf("lane %q defines unknown workload %q", lane.ID, benchmark)
			}
		}
		laneVariants := make(map[string]struct{}, len(lane.VariantIDs))
		for _, variantID := range lane.VariantIDs {
			if _, ok := variants[variantID]; !ok {
				t.Errorf("lane %q references unknown variant %q", lane.ID, variantID)
			}
			if _, ok := laneVariants[variantID]; ok {
				t.Errorf("lane %q repeats variant %q", lane.ID, variantID)
			}
			laneVariants[variantID] = struct{}{}
		}
		if lane.DefaultVariantID != "" {
			if _, ok := variants[lane.DefaultVariantID]; !ok {
				t.Errorf("lane %q defaults to unknown variant %q", lane.ID, lane.DefaultVariantID)
			}
			if _, ok := laneVariants[lane.DefaultVariantID]; !ok {
				t.Errorf("lane %q default %q is not a lane variant", lane.ID, lane.DefaultVariantID)
			}
		}
		mappedVariants := make(map[string]struct{})
		for benchmark, group := range lane.BenchmarkVariantGroups {
			if _, ok := benchmarks[benchmark]; !ok {
				t.Errorf("lane %q maps unknown benchmark %q to variant group %q", lane.ID, benchmark, group)
			}
			members, ok := variantGroups[group]
			if !ok {
				t.Errorf("lane %q benchmark %q references unknown variant group %q", lane.ID, benchmark, group)
			}
			for variantID := range members {
				mappedVariants[variantID] = struct{}{}
				if _, ok := laneVariants[variantID]; !ok {
					t.Errorf("lane %q benchmark %q maps non-lane variant %q", lane.ID, benchmark, variantID)
				}
			}
		}
		if len(laneVariants) == 0 && len(lane.BenchmarkVariantGroups) != 0 {
			t.Errorf("lane %q has benchmark variant groups without variants", lane.ID)
		}
		if len(laneVariants) != 0 && len(lane.BenchmarkVariantGroups) != len(benchmarks) {
			t.Errorf("lane %q maps %d benchmark variant groups, want %d", lane.ID, len(lane.BenchmarkVariantGroups), len(benchmarks))
		}
		if !sameStringSet(mappedVariants, laneVariants) {
			t.Errorf("lane %q mapped variants %v != lane variants %v", lane.ID, sortedStringSet(mappedVariants), sortedStringSet(laneVariants))
		}
		for benchmark, goosValues := range lane.BenchmarkGOOS {
			if _, ok := benchmarks[benchmark]; !ok {
				t.Errorf("lane %q has GOOS policy for unknown benchmark %q", lane.ID, benchmark)
			}
			if len(goosValues) == 0 {
				t.Errorf("lane %q benchmark %q has empty GOOS policy", lane.ID, benchmark)
			}
			seenGOOS := make(map[string]struct{}, len(goosValues))
			for _, goos := range goosValues {
				if goos != "darwin" && goos != "linux" && goos != "windows" {
					t.Errorf("lane %q benchmark %q has unsupported GOOS %q", lane.ID, benchmark, goos)
				}
				if _, ok := seenGOOS[goos]; ok {
					t.Errorf("lane %q benchmark %q repeats GOOS %q", lane.ID, benchmark, goos)
				}
				seenGOOS[goos] = struct{}{}
			}
		}
		for benchmark, leaves := range lane.BenchmarkLeaves {
			if _, ok := benchmarks[benchmark]; !ok {
				t.Errorf("lane %q has leaves for unknown benchmark %q", lane.ID, benchmark)
			}
			checkManifestLeaves(t, lane.ID, benchmark, "common", leaves)
		}
		for benchmark, byVariant := range lane.BenchmarkVariantExtraLeaves {
			if _, ok := benchmarks[benchmark]; !ok {
				t.Errorf("lane %q has variant leaves for unknown benchmark %q", lane.ID, benchmark)
			}
			group := lane.BenchmarkVariantGroups[benchmark]
			members := variantGroups[group]
			for variantID, leaves := range byVariant {
				if _, ok := members[variantID]; !ok {
					t.Errorf("lane %q benchmark %q has extra leaves for inapplicable variant %q", lane.ID, benchmark, variantID)
				}
				checkManifestLeaves(t, lane.ID, benchmark, variantID, leaves)
				common := make(map[string]struct{}, len(lane.BenchmarkLeaves[benchmark]))
				for _, leaf := range lane.BenchmarkLeaves[benchmark] {
					common[leaf] = struct{}{}
				}
				for _, leaf := range leaves {
					if _, ok := common[leaf]; ok {
						t.Errorf("lane %q benchmark %q repeats common leaf %q for variant %q", lane.ID, benchmark, leaf, variantID)
					}
				}
			}
		}
	}

	sha1 := regexp.MustCompile(`^[0-9a-f]{40}$`)
	checkpoints := make(map[string]struct{}, len(manifest.RevisionCheckpoints))
	for _, checkpoint := range manifest.RevisionCheckpoints {
		if checkpoint != "current" && !sha1.MatchString(checkpoint) {
			t.Errorf("invalid revision checkpoint %q", checkpoint)
		}
		if _, ok := checkpoints[checkpoint]; ok {
			t.Errorf("duplicate revision checkpoint %q", checkpoint)
		}
		checkpoints[checkpoint] = struct{}{}
	}
	if _, ok := checkpoints["current"]; !ok {
		t.Error("revision checkpoints omit current")
	}
	if len(manifest.RevisionVariants) != len(checkpoints) {
		t.Errorf("manifest has %d revision variants, want %d checkpoints", len(manifest.RevisionVariants), len(checkpoints))
	}
	revisionCommits := make(map[string]string, len(manifest.RevisionVariants))
	for _, revision := range manifest.RevisionVariants {
		if revision.Commit != "current" && !sha1.MatchString(revision.Commit) {
			t.Errorf("revision variant %q has invalid commit %q", revision.ID, revision.Commit)
		}
		if owner, ok := revisionCommits[revision.Commit]; ok {
			t.Errorf("revision commit %q belongs to both %q and %q", revision.Commit, owner, revision.ID)
		} else {
			revisionCommits[revision.Commit] = revision.ID
		}
		if _, ok := checkpoints[revision.Commit]; !ok {
			t.Errorf("revision variant %q references ungoverned commit %q", revision.ID, revision.Commit)
		}
	}
	for checkpoint := range checkpoints {
		if _, ok := revisionCommits[checkpoint]; !ok {
			t.Errorf("revision checkpoint %q has no stable revision variant ID", checkpoint)
		}
	}
}

func TestManifestRejectsUnknownField(t *testing.T) {
	mutated := bytes.Replace(
		manifestJSON,
		[]byte(`"sample_count": 5,`),
		[]byte(`"sample_count": 5, "sample_count_typo": 5,`),
		1,
	)
	if _, err := decodeManifest(mutated); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("decode manifest unknown field error = %v", err)
	}
}

func checkManifestMeasurement(t *testing.T, measurement manifestMeasurement) {
	t.Helper()
	if measurement.SampleCount < 2 {
		t.Errorf("sample count = %d, want at least 2", measurement.SampleCount)
	}
	if measurement.BenchmarkTime.Mode != "duration" && measurement.BenchmarkTime.Mode != "iterations" {
		t.Errorf("benchmark time mode = %q, want duration or iterations", measurement.BenchmarkTime.Mode)
	}
	if measurement.BenchmarkTime.Value <= 0 {
		t.Errorf("benchmark time value = %d, want positive", measurement.BenchmarkTime.Value)
	}
	if !measurement.Benchmem {
		t.Error("canonical measurement must enable benchmem")
	}
	if !slices.Equal(measurement.GoFlags, []string{"-buildvcs=false"}) {
		t.Errorf("Go flags = %v, want [-buildvcs=false]", measurement.GoFlags)
	}
	if measurement.CPUCardinality != 1 {
		t.Errorf("CPU cardinality = %d, want 1", measurement.CPUCardinality)
	}
	if measurement.PackageParallelism != 1 {
		t.Errorf("package parallelism = %d, want 1", measurement.PackageParallelism)
	}
	wantEnvironment := map[string]string{
		"CGO_ENABLED":  "1",
		"GODEBUG":      "",
		"GOENV":        "off",
		"GOEXPERIMENT": "",
		"GOFLAGS":      "-buildvcs=false",
		"GOGC":         "100",
		"GOMEMLIMIT":   "off",
		"GOMAXPROCS":   "benchmark-cpu-flag",
		"GOPROXY":      "off",
		"GOTOOLCHAIN":  "local",
		"GOWORK":       "off",
		"LANG":         "C",
		"LC_ALL":       "C",
		"TZ":           "UTC",
	}
	if !maps.Equal(measurement.Environment, wantEnvironment) {
		t.Errorf("environment = %v, want %v", measurement.Environment, wantEnvironment)
	}
}

func checkManifestWorkload(t *testing.T, lane, benchmark string, workload manifestWorkload, idPattern *regexp.Regexp) {
	t.Helper()
	if !idPattern.MatchString(workload.ID) {
		t.Errorf("lane %q benchmark %q workload ID %q is invalid", lane, benchmark, workload.ID)
	}
	if len(workload.HarnessFiles) == 0 {
		t.Errorf("lane %q benchmark %q has no harness files", lane, benchmark)
		return
	}
	if !slices.IsSorted(workload.HarnessFiles) {
		t.Errorf("lane %q benchmark %q harness files are not sorted: %v", lane, benchmark, workload.HarnessFiles)
	}
	sha256Pattern := regexp.MustCompile(`^[0-9a-f]{64}$`)
	if !sha256Pattern.MatchString(workload.HarnessSHA256) {
		t.Errorf("lane %q benchmark %q harness SHA-256 %q is invalid", lane, benchmark, workload.HarnessSHA256)
	}

	digest := sha256.New()
	declaration := regexp.MustCompile(`(?m)^func\s+` + regexp.QuoteMeta(benchmark) + `\s*\(`)
	declarationFound := false
	seen := make(map[string]struct{}, len(workload.HarnessFiles))
	for _, relative := range workload.HarnessFiles {
		if _, ok := seen[relative]; ok {
			t.Errorf("lane %q benchmark %q repeats harness file %q", lane, benchmark, relative)
			continue
		}
		seen[relative] = struct{}{}
		clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(relative)))
		if relative == "" || clean != relative || filepath.IsAbs(relative) || strings.HasPrefix(relative, "../") {
			t.Errorf("lane %q benchmark %q has unsafe harness file %q", lane, benchmark, relative)
			continue
		}
		path := filepath.Join("..", "..", filepath.FromSlash(relative))
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("lane %q benchmark %q read harness %q: %v", lane, benchmark, relative, err)
			continue
		}
		if declaration.Match(data) {
			declarationFound = true
		}
		writeFramedDigest(digest, []byte(relative))
		writeFramedDigest(digest, data)
	}
	if !declarationFound {
		t.Errorf("lane %q benchmark %q declaration is absent from harness files %v", lane, benchmark, workload.HarnessFiles)
	}
	if got := fmt.Sprintf("%x", digest.Sum(nil)); got != workload.HarnessSHA256 {
		t.Errorf("lane %q benchmark %q harness SHA-256 = %s, want %s", lane, benchmark, got, workload.HarnessSHA256)
	}
}

func writeFramedDigest(digest io.Writer, value []byte) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = digest.Write(length[:])
	_, _ = digest.Write(value)
}

func checkManifestLeaves(t *testing.T, lane, benchmark, owner string, leaves []string) {
	t.Helper()
	if len(leaves) == 0 {
		t.Errorf("lane %q benchmark %q has empty leaves for %q", lane, benchmark, owner)
	}
	seen := make(map[string]struct{}, len(leaves))
	for _, leaf := range leaves {
		if leaf == "" || strings.HasPrefix(leaf, "/") || strings.HasSuffix(leaf, "/") {
			t.Errorf("lane %q benchmark %q has invalid leaf %q for %q", lane, benchmark, leaf, owner)
		}
		if _, ok := seen[leaf]; ok {
			t.Errorf("lane %q benchmark %q repeats leaf %q for %q", lane, benchmark, leaf, owner)
		}
		seen[leaf] = struct{}{}
	}
}

func checkManifestSourceAuthority(t *testing.T, authority manifestSourceAuthority) {
	t.Helper()
	if authority.SchemaVersion != 1 || authority.Policy != "manifest-build-cells-v1" {
		t.Errorf("source authority schema/policy = %d/%q", authority.SchemaVersion, authority.Policy)
	}
	wantPhysical := manifestPhysicalPolicy{
		ID: "physical-runtime-union-v1",
		RootControls: []string{
			".gitignore", "Makefile", "go.mod", "go.sum", "go.work", "go.work.sum", "project.mk",
		},
		Trees: []string{"eventloop", "goja-eventloop"},
		RuntimeAssets: []string{
			"eventloop/docs/tournament/2026-01-18/ANALYSIS_ALTERNATETHREE_LINUX_INVESTIGATION.md",
			"eventloop/docs/tournament/2026-01-18/ANALYSIS_ALTERNATETWO_HYBRID.md",
			"eventloop/docs/tournament/2026-01-18/ANALYSIS_GC_PRESSURE_INVESTIGATION.md",
		},
	}
	if !reflect.DeepEqual(authority.PhysicalPolicy, wantPhysical) {
		t.Errorf("source physical policy = %+v, want %+v", authority.PhysicalPolicy, wantPhysical)
	}
	wantModules := []manifestSourceModule{
		{ID: "eventloop", Root: "eventloop", ModulePath: eventloopPackage, Buildable: new(true)},
		{ID: "goja-eventloop", Root: "goja-eventloop", ModulePath: "github.com/joeycumines/goja-eventloop", Buildable: new(true)},
		{ID: "gojabaseline", Root: "eventloop/internal/gojabaseline", ModulePath: eventloopPackage + "/internal/gojabaseline", Buildable: new(true)},
		{ID: "root-control", Root: ".", ModulePath: "github.com/joeycumines/go-utilpkg", Buildable: new(false)},
		{ID: "tournament", Root: "eventloop/internal/tournament", ModulePath: eventloopPackage + "/internal/tournament", Buildable: new(true)},
	}
	if !reflect.DeepEqual(authority.Modules, wantModules) {
		t.Errorf("source modules = %+v, want %+v", authority.Modules, wantModules)
	}
	wantCells := expectedManifestSourceCells()
	if !reflect.DeepEqual(authority.BuildCells, wantCells) {
		t.Errorf("source build cells = %+v, want %+v", authority.BuildCells, wantCells)
	}
}

func expectedManifestSourceCells() []manifestSourceCell {
	targets := []struct {
		ID      string
		GOOS    string
		GOARCH  string
		Feature manifestArchitectureFeature
	}{
		{ID: "darwin-amd64", GOOS: "darwin", GOARCH: "amd64", Feature: manifestArchitectureFeature{Name: "GOAMD64", Value: "v1"}},
		{ID: "darwin-arm64", GOOS: "darwin", GOARCH: "arm64", Feature: manifestArchitectureFeature{Name: "GOARM64", Value: "v8.0"}},
		{ID: "js-wasm", GOOS: "js", GOARCH: "wasm", Feature: manifestArchitectureFeature{Name: "GOWASM", Value: "satconv,signext"}},
		{ID: "linux-amd64", GOOS: "linux", GOARCH: "amd64", Feature: manifestArchitectureFeature{Name: "GOAMD64", Value: "v1"}},
		{ID: "linux-arm64", GOOS: "linux", GOARCH: "arm64", Feature: manifestArchitectureFeature{Name: "GOARM64", Value: "v8.0"}},
		{ID: "plan9-amd64", GOOS: "plan9", GOARCH: "amd64", Feature: manifestArchitectureFeature{Name: "GOAMD64", Value: "v1"}},
		{ID: "wasip1-wasm", GOOS: "wasip1", GOARCH: "wasm", Feature: manifestArchitectureFeature{Name: "GOWASM", Value: "satconv,signext"}},
		{ID: "windows-amd64", GOOS: "windows", GOARCH: "amd64", Feature: manifestArchitectureFeature{Name: "GOAMD64", Value: "v1"}},
		{ID: "windows-arm64", GOOS: "windows", GOARCH: "arm64", Feature: manifestArchitectureFeature{Name: "GOARM64", Value: "v8.0"}},
	}
	modules := []string{"eventloop", "goja-eventloop", "gojabaseline", "tournament"}
	cells := make([]manifestSourceCell, 0, len(modules)*len(targets)+4)
	for _, module := range modules {
		for _, target := range targets {
			cells = append(cells, manifestSourceCell{
				ID:                  module + "." + target.ID,
				ModuleID:            module,
				GOOS:                target.GOOS,
				GOARCH:              target.GOARCH,
				CGOEnabled:          new(false),
				ArchitectureFeature: target.Feature,
				BuildTags:           []string{},
				SelectionFlags:      []string{"-deps", "-test"},
				PackagePatterns:     []string{"./..."},
			})
			if module == "eventloop" && (target.GOOS == "darwin" || target.GOOS == "linux") {
				cells = append(cells, manifestSourceCell{
					ID:                  module + "." + target.ID + "-libuv",
					ModuleID:            module,
					GOOS:                target.GOOS,
					GOARCH:              target.GOARCH,
					CGOEnabled:          new(true),
					ArchitectureFeature: target.Feature,
					BuildTags:           []string{"libuv"},
					SelectionFlags:      []string{"-deps", "-test"},
					PackagePatterns:     []string{"./internal/libuvbaseline"},
				})
			}
		}
	}
	slices.SortFunc(cells, func(left, right manifestSourceCell) int {
		return strings.Compare(left.ID, right.ID)
	})
	return cells
}

func sameStringSet(left, right map[string]struct{}) bool {
	return len(left) == len(right) && slices.Equal(sortedStringSet(left), sortedStringSet(right))
}

func sortedStringSet(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	slices.Sort(result)
	return result
}

func checkManifestID(t *testing.T, seen map[string]string, pattern *regexp.Regexp, id, kind string) {
	t.Helper()
	if !pattern.MatchString(id) {
		t.Errorf("%s ID %q is not stable-ID syntax", kind, id)
	}
	if owner, ok := seen[id]; ok {
		t.Errorf("manifest ID %q belongs to both %s and %s", id, owner, kind)
		return
	}
	seen[id] = kind
}

func loadManifest(t *testing.T) tournamentManifest {
	t.Helper()
	manifest, err := decodeManifest(manifestJSON)
	if err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	return manifest
}

func decodeManifest(data []byte) (tournamentManifest, error) {
	if err := rejectTournamentManifestDuplicateKeys(data); err != nil {
		return tournamentManifest{}, err
	}
	if err := validateTournamentManifestJSONShape(data); err != nil {
		return tournamentManifest{}, err
	}
	var manifest tournamentManifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return tournamentManifest{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return tournamentManifest{}, err
	}
	return manifest, nil
}
