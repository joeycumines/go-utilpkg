package tournament

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type benchmarkRoot struct {
	Package   string
	Benchmark string
}

func TestActiveBenchmarkRootsGovernedOrDisposed(t *testing.T) {
	manifest := loadManifest(t)
	repositoryRoot := filepath.Clean(filepath.Join("..", "..", ".."))
	if _, err := os.Stat(filepath.Join(repositoryRoot, "eventloop", "go.mod")); err != nil {
		// This test verifies the tournament manifest's benchmark-root governance
		// against the monorepo source tree (the eventloop and goja-eventloop
		// sibling modules). A standalone checkout of go-eventloop lacks that
		// layout by construction, so the check is skipped rather than failed.
		t.Skipf("repository root layout unavailable in standalone checkout: %v", err)
	}

	physical := discoverBenchmarkRoots(t, repositoryRoot)
	governed, disposed, err := projectBenchmarkRoots(manifest, lineageJSON)
	if err != nil {
		t.Fatalf("project benchmark-root governance: %v", err)
	}

	for _, root := range physical {
		_, isGoverned := governed[root]
		_, isDisposed := disposed[root]
		if !isGoverned && !isDisposed {
			t.Errorf("active benchmark root is neither governed nor disposed: %s %s", root.Package, root.Benchmark)
		}
	}
	for root, lane := range governed {
		if _, exists := slices.BinarySearchFunc(physical, root, compareBenchmarkRoot); !exists {
			t.Errorf("lane %q governs absent physical benchmark root %+v", lane, root)
		}
	}
	for root, disposition := range disposed {
		if _, exists := slices.BinarySearchFunc(physical, root, compareBenchmarkRoot); !exists {
			t.Errorf("disposition %q references absent physical benchmark root %+v", disposition, root)
		}
	}
}

func projectBenchmarkRoots(manifest tournamentManifest, lineageData []byte) (map[benchmarkRoot]string, map[benchmarkRoot]string, error) {
	switch manifest.SchemaVersion {
	case 4:
		return projectBenchmarkRootsV4(manifest)
	case 5:
		return projectBenchmarkRootsV5(manifest, lineageData)
	default:
		return nil, nil, fmt.Errorf("unsupported manifest schema %d", manifest.SchemaVersion)
	}
}

func projectBenchmarkRootsV4(manifest tournamentManifest) (map[benchmarkRoot]string, map[benchmarkRoot]string, error) {
	governed := make(map[benchmarkRoot]string)
	for _, lane := range manifest.Lanes {
		for _, benchmark := range lane.Benchmarks {
			root := benchmarkRoot{Package: lane.Package, Benchmark: benchmark}
			if owner, duplicate := governed[root]; duplicate {
				return nil, nil, fmt.Errorf("benchmark root %+v belongs to lanes %q and %q", root, owner, lane.ID)
			}
			governed[root] = lane.ID
		}
	}
	disposed := make(map[benchmarkRoot]string)
	for _, disposition := range manifest.RootDispositions {
		root := benchmarkRoot{Package: disposition.Package, Benchmark: disposition.Benchmark}
		if disposition.Package == "" || disposition.Benchmark == "" || disposition.DispositionID == "" {
			return nil, nil, fmt.Errorf("incomplete root disposition: %+v", disposition)
		}
		if lane, duplicate := governed[root]; duplicate {
			return nil, nil, fmt.Errorf("benchmark root %+v is both governed by lane %q and disposed by %q", root, lane, disposition.DispositionID)
		}
		if owner, duplicate := disposed[root]; duplicate {
			return nil, nil, fmt.Errorf("benchmark root %+v has dispositions %q and %q", root, owner, disposition.DispositionID)
		}
		disposed[root] = disposition.DispositionID
	}
	return governed, disposed, nil
}

func projectBenchmarkRootsV5(manifest tournamentManifest, lineageData []byte) (map[benchmarkRoot]string, map[benchmarkRoot]string, error) {
	var lineage retainedLineageCatalog
	if err := json.Unmarshal(lineageData, &lineage); err != nil {
		return nil, nil, fmt.Errorf("decode lineage: %w", err)
	}
	if lineage.SchemaVersion != 3 {
		return nil, nil, fmt.Errorf("lineage schema = %d, want 3", lineage.SchemaVersion)
	}
	rawRoots := make(map[string]retainedLineageRawRoot, len(lineage.RawRoots))
	for _, rawRoot := range lineage.RawRoots {
		if _, duplicate := rawRoots[rawRoot.ID]; duplicate {
			return nil, nil, fmt.Errorf("lineage repeats raw root %q", rawRoot.ID)
		}
		rawRoots[rawRoot.ID] = rawRoot
	}
	bindings := make(map[string]retainedLineageBinding, len(lineage.Bindings))
	for _, binding := range lineage.Bindings {
		if _, duplicate := bindings[binding.ID]; duplicate {
			return nil, nil, fmt.Errorf("lineage repeats binding %q", binding.ID)
		}
		bindings[binding.ID] = binding
	}
	dispositions := make(map[string]retainedLineageDisposition, len(lineage.Dispositions))
	for _, disposition := range lineage.Dispositions {
		if _, duplicate := dispositions[disposition.ID]; duplicate {
			return nil, nil, fmt.Errorf("lineage repeats disposition %q", disposition.ID)
		}
		dispositions[disposition.ID] = disposition
	}

	governed := make(map[benchmarkRoot]string)
	selectedBindings := make(map[string]string)
	govern := func(root benchmarkRoot, lane string) error {
		if owner, duplicate := governed[root]; duplicate && owner != lane {
			return fmt.Errorf("benchmark root %+v belongs to lanes %q and %q", root, owner, lane)
		}
		governed[root] = lane
		return nil
	}
	for _, lane := range manifest.Lanes {
		for _, projection := range lane.BenchmarkBindings {
			binding, ok := bindings[projection.BindingID]
			if !ok {
				return nil, nil, fmt.Errorf("lane %q references unknown binding %q", lane.ID, projection.BindingID)
			}
			if binding.ImplementationID != projection.ImplementationID {
				return nil, nil, fmt.Errorf("binding %q implementation = %q, projected %q", binding.ID, binding.ImplementationID, projection.ImplementationID)
			}
			if binding.Applicability != "executable" && binding.Applicability != "diagnostic" {
				return nil, nil, fmt.Errorf("lane %q projects ineligible binding %q with applicability %q", lane.ID, binding.ID, binding.Applicability)
			}
			rawRoot, ok := rawRoots[binding.RawRootID]
			if !ok {
				return nil, nil, fmt.Errorf("binding %q references unknown raw root %q", binding.ID, binding.RawRootID)
			}
			if rawRoot.ModuleID != projection.ModuleID {
				return nil, nil, fmt.Errorf("binding %q raw-root module = %q, projected %q", binding.ID, rawRoot.ModuleID, projection.ModuleID)
			}
			if _, found := slices.BinarySearch(rawRoot.Benchmarks, binding.Benchmark); !found {
				return nil, nil, fmt.Errorf("binding %q benchmark %q is absent from raw root %q", binding.ID, binding.Benchmark, rawRoot.ID)
			}
			root := benchmarkRoot{Package: rawRoot.Package, Benchmark: binding.Benchmark}
			if err := govern(root, lane.ID); err != nil {
				return nil, nil, err
			}
			selectedBindings[binding.ID] = lane.ID
		}
	}
	for _, binding := range lineage.Bindings {
		if binding.Applicability != "alias-only" {
			continue
		}
		for _, alias := range lineage.Aliases {
			if alias.Kind != "helper-identity" || alias.AliasSubjectID != binding.ID || alias.Rerun {
				continue
			}
			lane, selected := selectedBindings[alias.CanonicalSubjectID]
			if !selected {
				continue
			}
			rawRoot, ok := rawRoots[binding.RawRootID]
			if !ok {
				return nil, nil, fmt.Errorf("alias binding %q references unknown raw root %q", binding.ID, binding.RawRootID)
			}
			if _, found := slices.BinarySearch(rawRoot.Benchmarks, binding.Benchmark); !found {
				return nil, nil, fmt.Errorf("alias binding %q benchmark %q is absent from raw root %q", binding.ID, binding.Benchmark, rawRoot.ID)
			}
			if err := govern(benchmarkRoot{Package: rawRoot.Package, Benchmark: binding.Benchmark}, lane); err != nil {
				return nil, nil, err
			}
		}
	}

	disposed := make(map[benchmarkRoot]string)
	for _, projection := range manifest.RootDispositions {
		disposition, ok := dispositions[projection.DispositionID]
		if !ok {
			return nil, nil, fmt.Errorf("manifest references unknown disposition %q", projection.DispositionID)
		}
		if disposition.SubjectKind != "raw-root" || disposition.SubjectID != projection.RawRootID {
			return nil, nil, fmt.Errorf("disposition %q subject = %s %q, projected raw root %q", disposition.ID, disposition.SubjectKind, disposition.SubjectID, projection.RawRootID)
		}
		rawRoot, ok := rawRoots[projection.RawRootID]
		if !ok {
			return nil, nil, fmt.Errorf("disposition %q references unknown raw root %q", disposition.ID, projection.RawRootID)
		}
		for _, benchmark := range rawRoot.Benchmarks {
			root := benchmarkRoot{Package: rawRoot.Package, Benchmark: benchmark}
			if lane, duplicate := governed[root]; duplicate {
				return nil, nil, fmt.Errorf("benchmark root %+v is both governed by lane %q and disposed by %q", root, lane, disposition.ID)
			}
			if owner, duplicate := disposed[root]; duplicate {
				return nil, nil, fmt.Errorf("benchmark root %+v has dispositions %q and %q", root, owner, disposition.ID)
			}
			disposed[root] = disposition.ID
		}
	}
	return governed, disposed, nil
}

func TestBenchmarkRootProjectionSchema5UsesLineageAndDeduplicatesBindings(t *testing.T) {
	manifest, lineage := benchmarkRootProjectionV5Fixture()
	lineageData, err := json.Marshal(lineage)
	if err != nil {
		t.Fatal(err)
	}
	governed, disposed, err := projectBenchmarkRoots(manifest, lineageData)
	if err != nil {
		t.Fatalf("project schema-5 roots: %v", err)
	}
	if len(governed) != 1 || governed[benchmarkRoot{Package: eventloopPackage, Benchmark: "BenchmarkActive"}] != "lane-a" {
		t.Errorf("governed roots = %v", governed)
	}
	if len(disposed) != 1 || disposed[benchmarkRoot{Package: eventloopPackage, Benchmark: "BenchmarkRetired"}] != "disposition.retired" {
		t.Errorf("disposed roots = %v", disposed)
	}
}

func TestBenchmarkRootProjectionSchema5RejectsAuthorityMismatch(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*tournamentManifest, *retainedLineageCatalog)
	}{
		{name: "lineage schema", mutate: func(_ *tournamentManifest, lineage *retainedLineageCatalog) {
			lineage.SchemaVersion = 2
		}},
		{name: "unknown binding", mutate: func(manifest *tournamentManifest, _ *retainedLineageCatalog) {
			manifest.Lanes[0].BenchmarkBindings[0].BindingID = "binding.unknown"
		}},
		{name: "implementation", mutate: func(manifest *tournamentManifest, _ *retainedLineageCatalog) {
			manifest.Lanes[0].BenchmarkBindings[0].ImplementationID = "implementation.wrong"
		}},
		{name: "module", mutate: func(manifest *tournamentManifest, _ *retainedLineageCatalog) {
			manifest.Lanes[0].BenchmarkBindings[0].ModuleID = "wrong"
		}},
		{name: "ineligible binding", mutate: func(_ *tournamentManifest, lineage *retainedLineageCatalog) {
			lineage.Bindings[0].Applicability = "alias-only"
		}},
		{name: "disposition subject", mutate: func(_ *tournamentManifest, lineage *retainedLineageCatalog) {
			lineage.Dispositions[0].SubjectID = "raw-root.active"
		}},
		{name: "cross-lane physical root", mutate: func(manifest *tournamentManifest, _ *retainedLineageCatalog) {
			manifest.Lanes = append(manifest.Lanes, manifestLane{
				ID: "lane-b",
				BenchmarkBindings: []manifestBindingProjection{{
					BindingID: "binding.root.cell-a", ImplementationID: "implementation.a", ModuleID: "eventloop",
				}},
			})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			manifest, lineage := benchmarkRootProjectionV5Fixture()
			test.mutate(&manifest, &lineage)
			lineageData, err := json.Marshal(lineage)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := projectBenchmarkRoots(manifest, lineageData); err == nil {
				t.Fatal("invalid schema-5 root authority unexpectedly passed")
			}
		})
	}
}

func TestBenchmarkRootProjectionSchema5GovernsAliasRootTransitively(t *testing.T) {
	manifest, lineage := benchmarkRootProjectionV5Fixture()
	lineage.RawRoots = append(lineage.RawRoots, retainedLineageRawRoot{
		ID: "raw-root.alias", ModuleID: "eventloop", Package: eventloopPackage, Benchmarks: []string{"BenchmarkAlias"},
	})
	lineage.Bindings = append(lineage.Bindings, retainedLineageBinding{
		ID: "binding.root.alias", RawRootID: "raw-root.alias", Benchmark: "BenchmarkAlias",
		ImplementationID: "implementation.a", Applicability: "alias-only",
	})
	lineage.Aliases = append(lineage.Aliases, retainedLineageAlias{
		ID: "alias.root", Kind: "helper-identity", AliasSubjectID: "binding.root.alias",
		CanonicalSubjectID: "binding.root.cell-a", Rerun: false,
	})
	lineageData, err := json.Marshal(lineage)
	if err != nil {
		t.Fatal(err)
	}
	governed, _, err := projectBenchmarkRoots(manifest, lineageData)
	if err != nil {
		t.Fatalf("project alias-governed root: %v", err)
	}
	root := benchmarkRoot{Package: eventloopPackage, Benchmark: "BenchmarkAlias"}
	if governed[root] != "lane-a" {
		t.Errorf("alias root owner = %q, want lane-a", governed[root])
	}
}

func benchmarkRootProjectionV5Fixture() (tournamentManifest, retainedLineageCatalog) {
	manifest := tournamentManifest{
		SchemaVersion: 5,
		Lanes: []manifestLane{{
			ID: "lane-a",
			BenchmarkBindings: []manifestBindingProjection{
				{BindingID: "binding.root.cell-a", ImplementationID: "implementation.a", ModuleID: "eventloop"},
				{BindingID: "binding.root.cell-b", ImplementationID: "implementation.b", ModuleID: "eventloop"},
			},
		}},
		RootDispositions: []manifestRootDisposition{{RawRootID: "raw-root.retired", DispositionID: "disposition.retired"}},
	}
	lineage := retainedLineageCatalog{
		SchemaVersion: 3,
		RawRoots: []retainedLineageRawRoot{
			{ID: "raw-root.active", ModuleID: "eventloop", Package: eventloopPackage, Benchmarks: []string{"BenchmarkActive"}},
			{ID: "raw-root.retired", ModuleID: "eventloop", Package: eventloopPackage, Benchmarks: []string{"BenchmarkRetired"}},
		},
		Bindings: []retainedLineageBinding{
			{ID: "binding.root.cell-a", RawRootID: "raw-root.active", Benchmark: "BenchmarkActive", ImplementationID: "implementation.a", Applicability: "executable"},
			{ID: "binding.root.cell-b", RawRootID: "raw-root.active", Benchmark: "BenchmarkActive", ImplementationID: "implementation.b", Applicability: "diagnostic"},
		},
		Dispositions: []retainedLineageDisposition{{ID: "disposition.retired", SubjectKind: "raw-root", SubjectID: "raw-root.retired"}},
	}
	return manifest, lineage
}

func discoverBenchmarkRoots(t *testing.T, repositoryRoot string) []benchmarkRoot {
	t.Helper()
	var roots []benchmarkRoot
	for _, module := range []struct {
		path       string
		modulePath string
	}{
		{path: "eventloop", modulePath: eventloopPackage},
		{path: "goja-eventloop", modulePath: "github.com/joeycumines/goja-eventloop"},
	} {
		moduleRoot := filepath.Join(repositoryRoot, module.path)
		err := filepath.WalkDir(moduleRoot, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				switch entry.Name() {
				case ".git", "docs", "examples", "node_modules", "testdata", "vendor":
					if path != moduleRoot {
						return filepath.SkipDir
					}
				}
				return nil
			}
			if !strings.HasSuffix(entry.Name(), "_test.go") {
				return nil
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
			if err != nil {
				return fmt.Errorf("parse %s: %w", path, err)
			}
			relativeDirectory, err := filepath.Rel(moduleRoot, filepath.Dir(path))
			if err != nil {
				return fmt.Errorf("resolve package for %s: %w", path, err)
			}
			packagePath := module.modulePath
			if relativeDirectory != "." {
				packagePath += "/" + filepath.ToSlash(relativeDirectory)
			}
			for _, declaration := range parsed.Decls {
				function, ok := declaration.(*ast.FuncDecl)
				if !ok || !benchmarkFunction(function) {
					continue
				}
				roots = append(roots, benchmarkRoot{Package: packagePath, Benchmark: function.Name.Name})
			}
			return nil
		})
		if err != nil {
			t.Fatalf("discover benchmark roots under %s: %v", module.path, err)
		}
	}
	slices.SortFunc(roots, compareBenchmarkRoot)
	for index := 1; index < len(roots); index++ {
		if roots[index] == roots[index-1] {
			t.Fatalf("duplicate physical benchmark root %+v", roots[index])
		}
	}
	return roots
}

func benchmarkFunction(function *ast.FuncDecl) bool {
	if function.Recv != nil || !strings.HasPrefix(function.Name.Name, "Benchmark") || function.Type.Results != nil ||
		function.Type.Params == nil || len(function.Type.Params.List) != 1 {
		return false
	}
	parameter := function.Type.Params.List[0]
	if len(parameter.Names) != 1 {
		return false
	}
	pointer, ok := parameter.Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	selector, ok := pointer.X.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "B" {
		return false
	}
	packageName, ok := selector.X.(*ast.Ident)
	return ok && packageName.Name == "testing"
}

func compareBenchmarkRoot(left, right benchmarkRoot) int {
	if result := strings.Compare(left.Package, right.Package); result != 0 {
		return result
	}
	return strings.Compare(left.Benchmark, right.Benchmark)
}
