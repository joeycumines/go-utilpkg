package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestSourceCellArgvBindsTagsAndPatterns(t *testing.T) {
	cgo := true
	module := manifestSourceModule{Root: "eventloop"}
	cell := manifestSourceCell{
		SelectionFlags:  []string{"-deps", "-test"},
		BuildTags:       []string{"integration", "libuv"},
		PackagePatterns: []string{"./internal/libuvbaseline"},
		CGOEnabled:      &cgo,
	}
	want := []string{
		"{go-executable}", "-C", "{source-root}/eventloop", "list", "-json",
		"-deps", "-test", "-tags=integration,libuv", "./internal/libuvbaseline",
	}
	if got := sourceCellArgv(module, cell); !slices.Equal(got, want) {
		t.Fatalf("sourceCellArgv = %q, want %q", got, want)
	}
}

func TestParseSourceListRetainsOnlyRepositoryPaths(t *testing.T) {
	repository := testSourceRepository(t)
	nested := filepath.Join(repository, "goja-eventloop", "nested.go")
	mustWriteFile(t, nested, []byte("package gojaeventloop\n"), 0o644)
	external := t.TempDir()
	mustWriteFile(t, filepath.Join(external, "external.go"), []byte("package external\n"), 0o644)
	packages := []sourceListPackage{
		{
			ImportPath: "example.invalid/eventloop",
			Dir:        filepath.Join(repository, "eventloop"),
			GoFiles:    []string{"source.go", filepath.Join(external, "external.go")},
		},
		{ImportPath: "example.invalid/goja", Dir: filepath.Join(repository, "goja-eventloop"), GoFiles: []string{"nested.go"}},
		{ImportPath: "example.invalid/external", Dir: external, GoFiles: []string{"external.go"}},
	}
	var input bytes.Buffer
	encoder := json.NewEncoder(&input)
	for _, pkg := range packages {
		if err := encoder.Encode(pkg); err != nil {
			t.Fatalf("Encode package: %v", err)
		}
	}
	paths, err := parseSourceList(repository, input.Bytes())
	if err != nil {
		t.Fatalf("parseSourceList: %v", err)
	}
	want := []string{"eventloop/source.go", "goja-eventloop/nested.go"}
	if !slices.Equal(paths, want) {
		t.Fatalf("parseSourceList paths = %q, want %q", paths, want)
	}
}

func TestSourcePathSetRejectsTampering(t *testing.T) {
	valid, err := newSourcePathSet([]string{"eventloop/a.go", "eventloop/b.go"})
	if err != nil {
		t.Fatalf("newSourcePathSet: %v", err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*sourcePathSet)
	}{
		{name: "count", mutate: func(value *sourcePathSet) { value.Count++ }},
		{name: "digest", mutate: func(value *sourcePathSet) { value.SHA256 = strings.Repeat("a", 64) }},
		{name: "order", mutate: func(value *sourcePathSet) { value.Paths[0], value.Paths[1] = value.Paths[1], value.Paths[0] }},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := valid
			changed.Paths = slices.Clone(valid.Paths)
			test.mutate(&changed)
			if err := validateSourcePathSet(changed, "test"); err == nil {
				t.Fatal("tampered source path set unexpectedly passed")
			}
		})
	}
}

func TestSourceModuleRegistryRejectsUndeclaredModule(t *testing.T) {
	repository, _ := testGovernedSourceRepository(t)
	manifest, _, err := loadManifestSourceAuthority(filepath.Join(repository, filepath.FromSlash(sourceManifestRelativePath)))
	if err != nil {
		t.Fatalf("loadManifestSourceAuthority: %v", err)
	}
	mustMkdirAll(t, filepath.Join(repository, "eventloop", "nested"))
	mustWriteFile(t, filepath.Join(repository, "eventloop", "nested", "go.mod"), []byte("module example.invalid/nested\n"), 0o644)
	physical, err := physicalSourceFiles(repository, manifest.PhysicalPolicy.RuntimeAssets)
	if err != nil {
		t.Fatalf("physicalSourceFiles: %v", err)
	}
	if err := validateSourceModuleRegistry(repository, manifest, physical); err == nil || !strings.Contains(err.Error(), "module registry") {
		t.Fatalf("validateSourceModuleRegistry error = %v, want registry mismatch", err)
	}
}

func TestSourceFingerprintRejectsLegacySubsetFlags(t *testing.T) {
	if code := sourceFingerprintCommand([]string{"-root", t.TempDir(), "-build-module=eventloop"}); code == 0 {
		t.Fatal("legacy build-module flag unexpectedly passed")
	}
	if code := sourceFingerprintCommand([]string{"-root", t.TempDir(), "-build-target=darwin/arm64/cgo0"}); code == 0 {
		t.Fatal("legacy build-target flag unexpectedly passed")
	}
}

func TestGovernedSnapshotRootVerification(t *testing.T) {
	repository, config := testGovernedSourceRepository(t)
	snapshot := filepath.Join(t.TempDir(), "snapshot")
	metadata, err := createSnapshotBuild(repository, snapshot, config)
	if err != nil {
		t.Fatalf("createSnapshotBuild: %v", err)
	}
	fingerprint, err := verifySourceRecords(snapshot, metadata.Authority, metadata.Files)
	if err != nil {
		t.Fatalf("verifySourceRecords: %v", err)
	}
	if fingerprint != metadata.Fingerprint {
		t.Fatalf("verified fingerprint = %s, want %s", fingerprint, metadata.Fingerprint)
	}
	manifestPath := filepath.Join(snapshot, filepath.FromSlash(sourceManifestRelativePath))
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile manifest: %v", err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("Unmarshal manifest: %v", err)
	}
	manifest["source_authority"].(map[string]any)["policy"] = "changed"
	changed, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent changed manifest: %v", err)
	}
	mustWriteFile(t, manifestPath, append(changed, '\n'), 0o644)
	if _, err := verifySourceRecords(snapshot, metadata.Authority, metadata.Files); err == nil {
		t.Fatal("changed snapshot manifest unexpectedly verified")
	}
}

func TestSnapshotMetadataVerificationRejectsExternalAuthority(t *testing.T) {
	repository, config := testGovernedSourceRepository(t)
	snapshot := filepath.Join(t.TempDir(), "snapshot")
	if _, err := createSnapshotBuild(repository, snapshot, config); err != nil {
		t.Fatalf("createSnapshotBuild: %v", err)
	}
	metadataPath := filepath.Join(snapshot, filepath.FromSlash(sourceMetadataPath))
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("ReadFile metadata: %v", err)
	}
	external := filepath.Join(t.TempDir(), "source.json")
	mustWriteFile(t, external, data, 0o600)
	if code := sourceFingerprintCommand([]string{
		"-root", snapshot,
		"-metadata", external,
	}); code == 0 {
		t.Fatal("external snapshot metadata unexpectedly verified")
	}
	if code := sourceFingerprintCommand([]string{
		"-root", snapshot,
		"-metadata", metadataPath,
		"-go", config.GoExecutable,
	}); code == 0 {
		t.Fatal("ignored source-build flags unexpectedly passed metadata verification")
	}
}

func TestGovernedCaptureMatchesAuthorityUnion(t *testing.T) {
	repository, config := testGovernedSourceRepository(t)
	capture, err := governedSourceCapture(repository, config)
	if err != nil {
		t.Fatalf("governedSourceCapture: %v", err)
	}
	if !slices.Equal(capture.Files, capture.Authority.GovernedUnion.Paths) {
		t.Fatalf("capture files = %q, authority union = %q", capture.Files, capture.Authority.GovernedUnion.Paths)
	}
	manifest, authorityDigest, manifestDigest, err := loadManifestSourceAuthorityIdentity(
		filepath.Join(repository, filepath.FromSlash(sourceManifestRelativePath)),
	)
	if err != nil {
		t.Fatalf("loadManifestSourceAuthorityIdentity: %v", err)
	}
	if err := validateSourceAuthorityManifest(capture.Authority, manifest, authorityDigest, manifestDigest); err != nil {
		t.Fatalf("validateSourceAuthorityManifest: %v", err)
	}
	if reflect.DeepEqual(capture.Authority.PhysicalPaths, sourcePathSet{}) || reflect.DeepEqual(capture.Authority.BuildUnion, sourcePathSet{}) {
		t.Fatal("capture omitted physical or build path relation")
	}
}

func testGovernedSourceRepository(t *testing.T) (string, sourceBuildConfig) {
	t.Helper()
	repository := testSourceRepository(t)
	mustWriteFile(t, filepath.Join(repository, "go.mod"), []byte("module example.invalid/root\n\ngo 1.26.2\n"), 0o644)
	mustWriteFile(t, filepath.Join(repository, ".gitignore"), []byte("eventloop/ignored.go\n"), 0o644)
	mustWriteFile(t, filepath.Join(repository, "eventloop", "go.mod"), []byte("module example.invalid/eventloop\n\ngo 1.26.2\n"), 0o644)
	mustWriteFile(t, filepath.Join(repository, "eventloop", "ignored.go"), []byte("package eventloop\n\nconst ignoredBuildInput = 1\n"), 0o644)
	mustWriteFile(t, filepath.Join(repository, "goja-eventloop", "go.mod"), []byte("module example.invalid/goja-eventloop\n\ngo 1.26.2\n"), 0o644)
	mustWriteFile(t, filepath.Join(repository, "goja-eventloop", "source.go"), []byte("package gojaeventloop\n"), 0o644)

	feature, err := sourceTargetArchitecture(runtime.GOARCH)
	if err != nil {
		t.Fatalf("sourceTargetArchitecture: %v", err)
	}
	name, value, _ := strings.Cut(feature, "=")
	cgo := false
	cell := func(id, module string) manifestSourceCell {
		return manifestSourceCell{
			ID:                  id,
			ModuleID:            module,
			GOOS:                runtime.GOOS,
			GOARCH:              runtime.GOARCH,
			CGOEnabled:          &cgo,
			ArchitectureFeature: manifestArchitectureFeature{Name: name, Value: value},
			BuildTags:           []string{},
			SelectionFlags:      []string{"-deps", "-test"},
			PackagePatterns:     []string{"./..."},
		}
	}
	buildable := true
	controlOnly := false
	authority := manifestSourceAuthority{
		SchemaVersion: 1,
		Policy:        manifestSourceAuthorityPolicy,
		PhysicalPolicy: manifestPhysicalPolicy{
			ID:            physicalSourcePolicy,
			RootControls:  physicalSourceControls,
			Trees:         physicalSourceTrees,
			RuntimeAssets: []string{},
		},
		Modules: []manifestSourceModule{
			{ID: "eventloop", Root: "eventloop", ModulePath: "example.invalid/eventloop", Buildable: &buildable},
			{ID: "goja-eventloop", Root: "goja-eventloop", ModulePath: "example.invalid/goja-eventloop", Buildable: &buildable},
			{ID: "root-control", Root: ".", ModulePath: "example.invalid/root", Buildable: &controlOnly},
		},
		BuildCells: []manifestSourceCell{
			cell("eventloop.host", "eventloop"),
			cell("goja-eventloop.host", "goja-eventloop"),
		},
	}
	manifest := sourceManifestEnvelope{
		SchemaVersion: 4,
		SourceHistory: json.RawMessage(`{}`),
		Lineage: manifestLineageReference{
			Path: "lineage.json", SchemaVersion: 2, SHA256: strings.Repeat("a", 64),
			Floor: manifestLineageFloorReference{
				Path: "lineagefloors/000002.json", SchemaVersion: 1, Sequence: 2,
				SHA256: strings.Repeat("b", 64), CumulativeRecordSetSHA256: strings.Repeat("c", 64),
			},
		},
		SourceAuthority: authority,
		Measurement:     json.RawMessage(`{}`),
		Variants:        json.RawMessage(`[]`),
		VariantGroups:   json.RawMessage(`{}`),
		Lanes: json.RawMessage(`[{
      "id": "test",
      "package": "./eventloop",
      "required": true,
      "benchmarks": [],
      "variant_ids": [],
      "go_diagnostic_timeout_ns": 3300000000000,
      "runner_watchdog_timeout_ns": 3600000000000,
      "orchestration_watchdog_timeout_ns": 4200000000000,
      "workload_definitions": {}
    }]`),
		Concepts:            json.RawMessage(`[]`),
		RevisionVariants:    json.RawMessage(`[]`),
		RevisionCheckpoints: json.RawMessage(`[]`),
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent manifest: %v", err)
	}
	data = append(data, '\n')
	manifestPath := filepath.Join(repository, filepath.FromSlash(sourceManifestRelativePath))
	mustMkdirAll(t, filepath.Dir(manifestPath))
	mustWriteFile(t, manifestPath, data, 0o644)

	goExecutable, err := exec.LookPath("go")
	if err != nil {
		t.Skipf("Go executable unavailable: %v", err)
	}
	goExecutable, err = filepath.Abs(goExecutable)
	if err != nil {
		t.Fatalf("Go executable path: %v", err)
	}
	return repository, sourceBuildConfig{
		GoExecutable: goExecutable,
		ModuleCache:  t.TempDir(),
		BuildCache:   t.TempDir(),
		ScratchRoot:  t.TempDir(),
	}
}
