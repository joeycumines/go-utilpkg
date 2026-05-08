package tournament

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type retainedLineageCatalog struct {
	SchemaVersion   int                             `json:"schema_version"`
	Introductions   []retainedLineageID             `json:"introductions"`
	Snapshots       []retainedLineageSnapshot       `json:"snapshots"`
	Implementations []retainedLineageImplementation `json:"implementations"`
	Concepts        []retainedLineageID             `json:"concepts"`
	RawRoots        []retainedLineageRawRoot        `json:"raw_roots"`
	Harnesses       []retainedLineageID             `json:"harnesses"`
	Workloads       []retainedLineageID             `json:"workloads"`
	Bindings        []retainedLineageBinding        `json:"bindings"`
	Aliases         []retainedLineageAlias          `json:"aliases"`
	Reconstructions []retainedLineageID             `json:"reconstructions"`
	Dispositions    []retainedLineageDisposition    `json:"dispositions"`
}

type retainedLineageID struct {
	ID string `json:"id"`
}

type retainedLineageImplementation struct {
	ID             string `json:"id"`
	IntroductionID string `json:"introduction_id"`
}

type retainedLineageSnapshot struct {
	ID         string `json:"id"`
	SourcePath string `json:"source_path"`
}

type retainedLineageRawRoot struct {
	ID         string   `json:"id"`
	ModuleID   string   `json:"module_id"`
	Package    string   `json:"package"`
	Benchmarks []string `json:"benchmarks"`
}

type retainedLineageBinding struct {
	ID               string `json:"id"`
	RawRootID        string `json:"raw_root_id"`
	Benchmark        string `json:"benchmark"`
	ImplementationID string `json:"implementation_id"`
	Applicability    string `json:"applicability"`
}

type retainedLineageDisposition struct {
	ID          string `json:"id"`
	SubjectKind string `json:"subject_kind"`
	SubjectID   string `json:"subject_id"`
}

type retainedLineageAlias struct {
	ID                 string `json:"id"`
	Kind               string `json:"kind"`
	AliasSubjectID     string `json:"alias_subject_id"`
	CanonicalSubjectID string `json:"canonical_subject_id"`
	Rerun              bool   `json:"rerun"`
}

type retainedExecutable struct {
	sourcePackage string
	originCommit  string
	originTree    string
}

func TestManifestRetainsImmutableTournamentCatalog(t *testing.T) {
	manifest := loadManifest(t)

	wantExecutables := map[string]retainedExecutable{
		"scheduler.main.auto":                     {eventloopPackage, "current", "current"},
		"scheduler.main.forced":                   {eventloopPackage, "current", "current"},
		"scheduler.main.disabled":                 {eventloopPackage, "current", "current"},
		"scheduler.alternate-one.max-safety":      {eventloopPackage + "/internal/alternateone", "986e2378c1484aa917a1bb0fd13aef914bdce50f", "c7ba8255af6135c491e51020f4ea49c9498beb14"},
		"scheduler.alternate-two.max-performance": {eventloopPackage + "/internal/alternatetwo", "986e2378c1484aa917a1bb0fd13aef914bdce50f", "a5c1c9e9efd8ec2fd4f7ce92ca8808a7b3fc8a40"},
		"scheduler.alternate-three.original-main": {eventloopPackage + "/internal/alternatethree", "986e2378c1484aa917a1bb0fd13aef914bdce50f", "735bd94545f70aede5d543f4a89dea314ba2e655"},
		"scheduler.goja-nodejs.baseline":          {eventloopPackage + "/internal/gojabaseline", "986e2378c1484aa917a1bb0fd13aef914bdce50f", "bcb06de927695c6a51ed416d0abdfe67753984f1"},
		"scheduler.libuv.native":                  {eventloopPackage + "/internal/libuvbaseline", "system-library", "system-library"},
		"promise.main.chained":                    {eventloopPackage, "current", "current"},
		"promise.alt-one.embedded-first-handler":  {eventloopPackage + "/internal/promisealtone", "986e2378c1484aa917a1bb0fd13aef914bdce50f", "d21aa3b30d6bb98d0c217b12a69916e3bbcbd45b"},
		"promise.alt-two.treiber":                 {eventloopPackage + "/internal/promisealttwo", "986e2378c1484aa917a1bb0fd13aef914bdce50f", "e4d94e7fe925fcbdef676f60a291736921b678b3"},
		"promise.alt-three.pooled-treiber":        {eventloopPackage + "/internal/promisealtthree", "986e2378c1484aa917a1bb0fd13aef914bdce50f", "c6870653742d65e6b19a4c5d53713577c8c7be53"},
		"promise.alt-four.main-snapshot":          {eventloopPackage + "/internal/promisealtfour", "986e2378c1484aa917a1bb0fd13aef914bdce50f", "fd0ff142948d28aac4c9f5dc9248ba1314fc5ba6"},
		"promise.alt-five.original-chained":       {eventloopPackage + "/internal/promisealtfive", "986e2378c1484aa917a1bb0fd13aef914bdce50f", "16618a665a99a47a678630cb94381a9db47745f4"},
	}
	if manifest.SchemaVersion == 5 {
		checkManifestV5RetainedExecutables(t, wantExecutables, lineageJSON)
	} else {
		gotExecutables := make(map[string]manifestVariant, len(manifest.Variants))
		for _, variant := range manifest.Variants {
			gotExecutables[variant.ID] = variant
		}
		for id, want := range wantExecutables {
			got, ok := gotExecutables[id]
			if !ok {
				t.Errorf("immutable executable %q is absent", id)
				continue
			}
			if got.SourcePackage != want.sourcePackage || got.OriginCommit != want.originCommit || got.OriginTree != want.originTree {
				t.Errorf("immutable executable %q source = (%q, %q, %q), want (%q, %q, %q)", id, got.SourcePackage, got.OriginCommit, got.OriginTree, want.sourcePackage, want.originCommit, want.originTree)
			}
		}
	}

	wantConcepts := map[string]string{
		"scheduler.alternate-two-plus.dual-wake":      "docs/tournament/2026-01-18/ANALYSIS_ALTERNATETWO_HYBRID.md",
		"scheduler.alternate-two-plus.mini-fast-path": "docs/tournament/2026-01-18/ANALYSIS_ALTERNATETWO_HYBRID.md",
		"scheduler.alternate-three.fast-path":         "docs/tournament/2026-01-18/ANALYSIS_ALTERNATETHREE_LINUX_INVESTIGATION.md",
		"scheduler.main.task-arena.conservative":      "docs/tournament/2026-01-18/ANALYSIS_GC_PRESSURE_INVESTIGATION.md",
		"scheduler.main.task-arena.hybrid":            "docs/tournament/2026-01-18/ANALYSIS_GC_PRESSURE_INVESTIGATION.md",
		"scheduler.main.chunked-ingress.optimized":    "docs/tournament/2026-01-18/ANALYSIS_GC_PRESSURE_INVESTIGATION.md",
	}
	gotConcepts := make(map[string]manifestConcept, len(manifest.Concepts))
	for _, concept := range manifest.Concepts {
		gotConcepts[concept.ID] = concept
	}
	for id, sourceDocument := range wantConcepts {
		got, ok := gotConcepts[id]
		if !ok {
			t.Errorf("immutable concept %q is absent", id)
		} else if got.SourceDocument != sourceDocument {
			t.Errorf("immutable concept %q source document = %q, want %q", id, got.SourceDocument, sourceDocument)
		}
	}

	wantRevisions := map[string]string{
		"scheduler.revision.initial-go-native":             "506d6643cc1d45b1da156096870991ecb30b8847",
		"scheduler.revision.auto-exit-liveness":            "cc005d72b329fd91eee03aac62ba7188df7c91b9",
		"scheduler.revision.unconditional-microtask-drain": "3302f879a6cc3f52774a3d6ebf442b80e24a660b",
		"scheduler.revision.per-task-microtask-drain":      "8d3b3687e6e0110a2a34fc46229abedd358ecac4",
		"scheduler.revision.inter-phase-microtask-drain":   "d90b40eceb9e503c245895a83c40905b8d0d9c05",
		"scheduler.revision.exhaustive-microtask-drain":    "4ce34ac562bdbcbba477c299fe50139c4535ce8d",
		"scheduler.revision.internal-queue-tick-budget":    "9a051f4ca05b6a6bd59d36c4f8abb5dc04d30b86",
		"scheduler.revision.batched-microtask-drain":       "3f3384cc538f333032e73ca45c9b53bb46e2ef82",
		"scheduler.revision.node26-refactor":               "0def02e2ff987be01a38d237a5d84dae256a85ac",
		"scheduler.revision.tournament-snapshot":           "27b93ec32938ca838e1519bc8e17b6852d7df449",
		"scheduler.revision.unix-poller-hardened":          "986e2378c1484aa917a1bb0fd13aef914bdce50f",
		"scheduler.revision.current-candidate":             "current",
	}
	gotRevisions := make(map[string]manifestRevision, len(manifest.RevisionVariants))
	for _, revision := range manifest.RevisionVariants {
		gotRevisions[revision.ID] = revision
	}
	for id, commit := range wantRevisions {
		got, ok := gotRevisions[id]
		if !ok {
			t.Errorf("immutable revision %q is absent", id)
		} else if got.Commit != commit {
			t.Errorf("immutable revision %q commit = %q, want %q", id, got.Commit, commit)
		}
	}
}

func TestLineageRetainsEveryKnownSemanticFamily(t *testing.T) {
	var lineage retainedLineageCatalog
	if err := json.Unmarshal(lineageJSON, &lineage); err != nil {
		t.Fatalf("decode lineage: %v", err)
	}
	if lineage.SchemaVersion == 3 {
		checkLineageV3SemanticFamilies(t, loadManifest(t), lineage)
		return
	}
	if lineage.SchemaVersion != 2 {
		t.Fatalf("lineage schema = %d, want 2 or 3", lineage.SchemaVersion)
	}
	if len(lineage.Introductions) != 25 || len(lineage.Snapshots) != 37 ||
		len(lineage.Implementations) != 25 || len(lineage.Concepts) != 12 ||
		len(lineage.Harnesses) != 0 || len(lineage.Workloads) != 0 || len(lineage.Bindings) != 0 ||
		len(lineage.Aliases) != 9 || len(lineage.Reconstructions) != 0 || len(lineage.Dispositions) != 42 {
		t.Fatalf("lineage census changed: introductions=%d snapshots=%d implementations=%d concepts=%d harnesses=%d workloads=%d bindings=%d aliases=%d reconstructions=%d dispositions=%d",
			len(lineage.Introductions), len(lineage.Snapshots), len(lineage.Implementations), len(lineage.Concepts),
			len(lineage.Harnesses), len(lineage.Workloads), len(lineage.Bindings), len(lineage.Aliases),
			len(lineage.Reconstructions), len(lineage.Dispositions))
	}
	wantImplementations := []string{
		"future.basic.channel-fanout",
		"goja.promise-job.exit-gated-closure",
		"goja.promise-job.native-internal-queue",
		"goja.promise-job.observed-ungated-closure",
		"goja.promise-job.unobserved-per-job-closure",
		"promise.alt-five.original-chained",
		"promise.alt-four.main-snapshot",
		"promise.alt-one.embedded-first-handler",
		"promise.alt-three.pooled-treiber",
		"promise.alt-two.treiber",
		"promise.main.chained",
		"scheduler.alternate-one.max-safety",
		"scheduler.alternate-three.original-main",
		"scheduler.alternate-two.max-performance",
		"scheduler.e001.disposable-branch",
		"scheduler.goja-nodejs.baseline",
		"scheduler.libuv.native",
		"scheduler.main",
		"scheduler.topology.command-ingress-owner-local",
		"scheduler.topology.node-phase-owner",
		"scheduler.topology.ring-terminal-linearized",
		"wake.fd-polling-capability",
		"wake.linearized-lifecycle",
		"wake.platform-selected-dedup",
		"wake.windows-native",
	}
	gotImplementations := make(map[string]retainedLineageImplementation, len(lineage.Implementations))
	for _, implementation := range lineage.Implementations {
		gotImplementations[implementation.ID] = implementation
		if implementation.IntroductionID == "" {
			t.Errorf("lineage implementation %q has no introduction", implementation.ID)
		}
	}
	for _, id := range wantImplementations {
		if _, ok := gotImplementations[id]; !ok {
			t.Errorf("known semantic implementation %q is absent", id)
		}
	}
	projection := map[string]string{
		"scheduler.main.auto":                     "scheduler.main",
		"scheduler.main.forced":                   "scheduler.main",
		"scheduler.main.disabled":                 "scheduler.main",
		"scheduler.alternate-one.max-safety":      "scheduler.alternate-one.max-safety",
		"scheduler.alternate-two.max-performance": "scheduler.alternate-two.max-performance",
		"scheduler.alternate-three.original-main": "scheduler.alternate-three.original-main",
		"scheduler.goja-nodejs.baseline":          "scheduler.goja-nodejs.baseline",
		"scheduler.libuv.native":                  "scheduler.libuv.native",
		"promise.main.chained":                    "promise.main.chained",
		"promise.alt-one.embedded-first-handler":  "promise.alt-one.embedded-first-handler",
		"promise.alt-two.treiber":                 "promise.alt-two.treiber",
		"promise.alt-three.pooled-treiber":        "promise.alt-three.pooled-treiber",
		"promise.alt-four.main-snapshot":          "promise.alt-four.main-snapshot",
		"promise.alt-five.original-chained":       "promise.alt-five.original-chained",
	}
	manifest := loadManifest(t)
	if len(manifest.Variants) != len(projection) {
		t.Fatalf("legacy projection has %d variants, want %d", len(manifest.Variants), len(projection))
	}
	for _, variant := range manifest.Variants {
		implementationID, ok := projection[variant.ID]
		if !ok {
			t.Errorf("legacy variant %q lacks a lineage projection", variant.ID)
			continue
		}
		if _, ok := gotImplementations[implementationID]; !ok {
			t.Errorf("legacy variant %q projects to missing implementation %q", variant.ID, implementationID)
		}
	}
	for _, alias := range lineage.Aliases {
		if alias.Rerun {
			t.Errorf("exact lineage alias %q is rerunnable", alias.ID)
		}
	}
}

func TestTournamentHistoricalDirectoriesRemainPhysical(t *testing.T) {
	want := []string{
		"alternateone",
		"alternatetwo",
		"alternatethree",
		"gojabaseline",
		"libuvbaseline",
		"promisealtone",
		"promisealttwo",
		"promisealtthree",
		"promisealtfour",
		"promisealtfive",
		"promisetournament",
		"tournament",
	}
	for _, directory := range want {
		info, err := os.Stat(filepath.Join("..", directory))
		if err != nil {
			t.Errorf("historical tournament directory %q: %v", directory, err)
		} else if !info.IsDir() {
			t.Errorf("historical tournament path %q is not a directory", directory)
		}
	}
}

func TestTournamentWorkspaceEntriesRemainRegistered(t *testing.T) {
	workspacePath := filepath.Join("..", "..", "..", "go.work")
	data, err := os.ReadFile(workspacePath)
	if os.IsNotExist(err) {
		t.Skip("monorepo go.work is absent from this standalone module")
	}
	if err != nil {
		t.Fatalf("read monorepo workspace: %v", err)
	}
	workspace := string(data)
	for _, module := range []string{"./eventloop/internal/gojabaseline", "./eventloop/internal/tournament"} {
		if !strings.Contains(workspace, "\n\t"+module+"\n") {
			t.Errorf("monorepo workspace omits immutable tournament module %q", module)
		}
	}
}
