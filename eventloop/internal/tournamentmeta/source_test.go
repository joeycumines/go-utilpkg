package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestSnapshotPreservesGovernedTopology(t *testing.T) {
	repository := t.TempDir()
	mustMkdirAll(t, filepath.Join(repository, "eventloop", "docs", "tournament", "2026-05-14"))
	mustMkdirAll(t, filepath.Join(repository, "goja-eventloop"))
	mustWriteFile(t, filepath.Join(repository, ".gitignore"), []byte("eventloop/ignored\n"), 0o644)
	mustWriteFile(t, filepath.Join(repository, "Makefile"), []byte("all:\n"), 0o644)
	mustWriteFile(t, filepath.Join(repository, "go.mod"), []byte("module example.invalid/tournament\n"), 0o640)
	mustWriteFile(t, filepath.Join(repository, "go.sum"), nil, 0o644)
	mustWriteFile(t, filepath.Join(repository, "go.work"), []byte("go 1.26.2\n\nuse .\n"), 0o644)
	mustWriteFile(t, filepath.Join(repository, "go.work.sum"), nil, 0o644)
	mustWriteFile(t, filepath.Join(repository, "project.mk"), []byte("all:\n"), 0o644)
	mustWriteFile(t, filepath.Join(repository, "eventloop", "source.go"), []byte("package eventloop\n"), 0o600)
	mustWriteFile(t, filepath.Join(repository, "goja-eventloop", "source.go"), []byte("package gojaeventloop\n"), 0o644)
	mustWriteFile(t, filepath.Join(repository, "eventloop", "ignored"), []byte("ignored\n"), 0o644)
	mustWriteFile(
		t,
		filepath.Join(repository, "eventloop", "docs", "tournament", "2026-05-14", "raw.log"),
		[]byte("historical evidence\n"),
		0o644,
	)
	if err := os.Symlink("source.go", filepath.Join(repository, "eventloop", "source-link")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	files, err := liveSourceFiles(repository)
	if err != nil {
		t.Fatalf("liveSourceFiles: %v", err)
	}
	want := []string{
		".gitignore",
		"Makefile",
		"eventloop/ignored",
		"eventloop/source-link",
		"eventloop/source.go",
		"go.mod",
		"go.sum",
		"go.work",
		"go.work.sum",
		"goja-eventloop/source.go",
		"project.mk",
	}
	if !slices.Equal(files, want) {
		t.Fatalf("files = %q, want %q", files, want)
	}

	before, err := fingerprintFiles(repository, files)
	if err != nil {
		t.Fatalf("fingerprintFiles: %v", err)
	}
	records, err := inspectSourceRecords(repository, files)
	if err != nil {
		t.Fatalf("inspectSourceRecords: %v", err)
	}
	wantSnapshotFingerprint, err := fingerprintSource(fixtureSourceAuthority(), records)
	if err != nil {
		t.Fatalf("fingerprintSource: %v", err)
	}
	snapshot := filepath.Join(t.TempDir(), "snapshot")
	metadata, err := createSnapshot(repository, snapshot)
	if err != nil {
		t.Fatalf("createSnapshot: %v", err)
	}
	if metadata.Fingerprint != wantSnapshotFingerprint || metadata.FileCount != len(want) {
		t.Fatalf("metadata = %+v, want fingerprint %s and %d files", metadata, wantSnapshotFingerprint, len(want))
	}
	if target, err := os.Readlink(filepath.Join(snapshot, "eventloop", "source-link")); err != nil || target != "source.go" {
		t.Fatalf("snapshot link = %q, %v", target, err)
	}
	if info, err := os.Stat(filepath.Join(snapshot, "eventloop", "source.go")); err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("snapshot source mode = %v, %v", info, err)
	}

	mustWriteFile(
		t,
		filepath.Join(repository, "eventloop", "docs", "tournament", "2026-05-14", "raw.log"),
		[]byte("changed evidence\n"),
		0o644,
	)
	afterEvidence, err := fingerprintFiles(repository, files)
	if err != nil {
		t.Fatalf("fingerprint after evidence: %v", err)
	}
	if afterEvidence != before {
		t.Fatalf("dated evidence changed fingerprint: %s != %s", afterEvidence, before)
	}

	mustWriteFile(t, filepath.Join(snapshot, "eventloop", "source.go"), []byte("package changed\n"), 0o600)
	if code := sourceFingerprintCommand([]string{
		"-root", snapshot,
		"-metadata", filepath.Join(snapshot, filepath.FromSlash(sourceMetadataPath)),
	}); code == 0 {
		t.Fatal("tampered snapshot fingerprint unexpectedly passed")
	}
}

func TestSnapshotRejectsConcurrentSourceSetChange(t *testing.T) {
	repository := testSourceRepository(t)
	output := filepath.Join(t.TempDir(), "snapshot")
	created := false
	_, err := createSnapshotWithCopier(repository, output, func(root, snapshot, relative string) error {
		if err := copySourcePath(root, snapshot, relative); err != nil {
			return err
		}
		if !created {
			created = true
			mustWriteFile(t, filepath.Join(root, "eventloop", "concurrent.go"), []byte("package eventloop\n"), 0o644)
		}
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "source set changed") {
		t.Fatalf("createSnapshotWithCopier error = %v, want source-set change", err)
	}
	if _, statErr := os.Stat(output); !os.IsNotExist(statErr) {
		t.Fatalf("failed snapshot remains: %v", statErr)
	}
}

func TestSnapshotFingerprintRejectsInjectedSource(t *testing.T) {
	repository := testSourceRepository(t)
	snapshot := filepath.Join(t.TempDir(), "snapshot")
	if _, err := createSnapshot(repository, snapshot); err != nil {
		t.Fatalf("createSnapshot: %v", err)
	}
	mustWriteFile(t, filepath.Join(snapshot, "eventloop", "injected.go"), []byte("package eventloop\n"), 0o644)
	if code := sourceFingerprintCommand([]string{
		"-root", snapshot,
		"-metadata", filepath.Join(snapshot, filepath.FromSlash(sourceMetadataPath)),
	}); code == 0 {
		t.Fatal("snapshot with injected source unexpectedly passed")
	}
}

func TestSnapshotRejectsNestedOutput(t *testing.T) {
	repository := testSourceRepository(t)
	output := filepath.Join(repository, "eventloop", "snapshot")
	if _, err := createSnapshot(repository, output); err == nil || !strings.Contains(err.Error(), "outside source root") {
		t.Fatalf("createSnapshot error = %v, want outside-source-root rejection", err)
	}
}

func TestReadSourceMetadataRejectsFixtureAuthority(t *testing.T) {
	repository := testSourceRepository(t)
	snapshot := filepath.Join(t.TempDir(), "snapshot")
	metadata, err := createSnapshot(repository, snapshot)
	if err != nil {
		t.Fatalf("createSnapshot: %v", err)
	}
	if metadata.SchemaVersion != sourceMetadataLegacySchemaVersion || metadata.SharedSourceID != "" ||
		metadata.CaptureID != "" || metadata.CaptureAuthoritySHA256 != "" {
		t.Fatalf("fixture metadata = %+v, want legacy-only schema 4", metadata)
	}
	metadataPath := filepath.Join(snapshot, filepath.FromSlash(sourceMetadataPath))
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("ReadFile fixture source metadata: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("decode fixture source metadata: %v", err)
	}
	if _, exists := raw["fingerprint"]; !exists || raw["shared_source_id"] != nil {
		t.Fatalf("fixture metadata keys = %v, want schema-4 keys only", raw)
	}
	if _, err := readSourceMetadata(metadataPath); err == nil ||
		!strings.Contains(err.Error(), "null") {
		t.Fatalf("readSourceMetadata error = %v, want fixture-authority rejection", err)
	}
	if err := validatePersistedSourceAuthority(fixtureSourceAuthority()); err == nil ||
		!strings.Contains(err.Error(), "non-governed") {
		t.Fatalf("validatePersistedSourceAuthority error = %v, want non-governed policy rejection", err)
	}
}

func TestSourceBuildRejectsWritableRootOverlap(t *testing.T) {
	repository := testSourceRepository(t)
	goExecutable, err := exec.LookPath("go")
	if err != nil {
		t.Skipf("Go executable unavailable: %v", err)
	}
	goExecutable, err = filepath.Abs(goExecutable)
	if err != nil {
		t.Fatalf("Go executable path: %v", err)
	}
	outside := t.TempDir()
	base := sourceBuildConfig{
		GoExecutable: goExecutable,
		ModuleCache:  filepath.Join(outside, "module-cache"),
		BuildCache:   filepath.Join(outside, "build-cache"),
		ScratchRoot:  filepath.Join(outside, "scratch"),
	}
	for _, path := range []string{base.ModuleCache, base.BuildCache, base.ScratchRoot} {
		mustMkdirAll(t, path)
	}
	for _, test := range []struct {
		name   string
		mutate func(*sourceBuildConfig)
	}{
		{name: "source module cache", mutate: func(config *sourceBuildConfig) { config.ModuleCache = repository }},
		{name: "nested caches", mutate: func(config *sourceBuildConfig) {
			config.BuildCache = filepath.Join(config.ModuleCache, "nested")
			mustMkdirAll(t, config.BuildCache)
		}},
		{name: "scratch contains cache", mutate: func(config *sourceBuildConfig) { config.ScratchRoot = outside }},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := base
			test.mutate(&config)
			if _, _, err := validateSourceBuildConfig(repository, config); err == nil || !strings.Contains(err.Error(), "must not overlap") {
				t.Fatalf("validateSourceBuildConfig error = %v, want overlap rejection", err)
			}
		})
	}
}

func TestGovernedSourceIncludesIgnoredBuildInput(t *testing.T) {
	repository, config := testGovernedSourceRepository(t)
	ignored := filepath.Join(repository, "eventloop", "ignored.go")
	capture, err := governedSourceCapture(repository, config)
	if err != nil {
		t.Fatalf("governedSourceCapture: %v", err)
	}
	if !slices.Contains(capture.Files, "eventloop/ignored.go") ||
		!slices.Contains(capture.Authority.PhysicalPaths.Paths, "eventloop/ignored.go") ||
		!slices.Contains(capture.Authority.BuildUnion.Paths, "eventloop/ignored.go") {
		t.Fatalf("governed capture omits ignored build input: %+v", capture)
	}
	records, err := inspectSourceRecords(repository, capture.Files)
	if err != nil {
		t.Fatalf("inspect governed source: %v", err)
	}
	governedFingerprint, err := fingerprintSource(capture.Authority, records)
	if err != nil {
		t.Fatalf("fingerprint governed source: %v", err)
	}
	changedAuthority := capture.Authority
	changedAuthority.BuildCells = slices.Clone(capture.Authority.BuildCells)
	changedAuthority.BuildCells[0].Argv = slices.Clone(capture.Authority.BuildCells[0].Argv)
	changedAuthority.BuildCells[0].Argv = append(changedAuthority.BuildCells[0].Argv, "./additional")
	changedFingerprint, err := fingerprintSource(changedAuthority, records)
	if err != nil {
		t.Fatalf("fingerprint changed authority: %v", err)
	}
	if changedFingerprint == governedFingerprint {
		t.Fatal("build-target authority change did not change source fingerprint")
	}
	snapshot := filepath.Join(t.TempDir(), "governed-snapshot")
	metadata, err := createSnapshotBuild(repository, snapshot, config)
	if err != nil {
		t.Fatalf("createSnapshotBuild: %v", err)
	}
	if metadata.SchemaVersion != sourceMetadataSchemaVersion ||
		metadata.SharedSourceID == "" || metadata.CaptureID == "" ||
		metadata.CaptureAuthoritySHA256 == "" || metadata.LegacyV4Fingerprint != metadata.Fingerprint ||
		metadata.LogicalAuthority.EnumerationPolicy != governedSourcePolicy ||
		metadata.CaptureAuthority.Policy != sourceCapturePolicy ||
		metadata.Authority.EnumerationPolicy != governedSourcePolicy ||
		len(metadata.Authority.BuildCells) != 2 || metadata.Authority.ManifestSHA256 == "" ||
		metadata.Authority.ManifestSourceAuthoritySHA256 == "" || metadata.Authority.GoTool.ExecutableSHA256 == "" {
		t.Fatalf("governed snapshot metadata = %+v", metadata)
	}
	loaded, err := readSourceMetadata(filepath.Join(snapshot, filepath.FromSlash(sourceMetadataPath)))
	if err != nil {
		t.Fatalf("readSourceMetadata: %v", err)
	}
	if !reflect.DeepEqual(loaded, metadata) {
		t.Fatalf("loaded governed metadata = %+v, want %+v", loaded, metadata)
	}
	before, err := fingerprintFiles(repository, capture.Files)
	if err != nil {
		t.Fatalf("fingerprint before ignored change: %v", err)
	}
	mustWriteFile(t, ignored, []byte("package eventloop\n\nconst ignoredBuildInput = 2\n"), 0o644)
	after, err := fingerprintFiles(repository, capture.Files)
	if err != nil {
		t.Fatalf("fingerprint after ignored change: %v", err)
	}
	if after == before {
		t.Fatal("ignored build-input change did not change fingerprint")
	}
}

func TestSourceListEnvironmentRejectsAmbientInfluence(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOAMD64", "v4")
	t.Setenv("GOEXPERIMENT", "fieldtrack")
	t.Setenv("GOFLAGS", "-tags=ambient")
	t.Setenv("CC", "/ambient/cc")
	t.Setenv("CGO_CFLAGS", "-Dambient")
	t.Setenv("LANG", "ambient")
	t.Setenv("TMPDIR", "/ambient/tmp")
	config := sourceBuildConfig{
		GoExecutable: filepath.Join(root, "toolchain", "bin", "go"),
		ModuleCache:  filepath.Join(root, "module-cache"),
		BuildCache:   filepath.Join(root, "build-cache"),
		ScratchRoot:  filepath.Join(root, "scratch"),
	}
	cgo := false
	cell := manifestSourceCell{
		GOOS:                "linux",
		GOARCH:              "amd64",
		CGOEnabled:          &cgo,
		ArchitectureFeature: manifestArchitectureFeature{Name: "GOAMD64", Value: "v1"},
	}
	tokenized := sourceCellEnvironment(cell)
	environment, err := materializeSourceEnvironment(config, tokenized)
	if err != nil {
		t.Fatalf("materializeSourceEnvironment: %v", err)
	}
	if !slices.IsSorted(environment) || len(slices.Compact(slices.Clone(environment))) != len(environment) {
		t.Fatalf("source-list environment is not a sorted set: %q", environment)
	}
	values := make(map[string]string, len(environment))
	for _, record := range environment {
		key, value, ok := strings.Cut(record, "=")
		if !ok {
			t.Fatalf("environment record %q has no separator", record)
		}
		values[key] = value
	}
	for _, key := range []string{"CC", "CGO_CFLAGS"} {
		if _, ok := values[key]; ok {
			t.Fatalf("ambient %s survived in source-list environment", key)
		}
	}
	for key, want := range map[string]string{
		"CGO_ENABLED":  "0",
		"GOAMD64":      "v1",
		"GOEXPERIMENT": "",
		"GOFLAGS":      "-buildvcs=false -mod=readonly",
		"LANG":         "C",
		"LC_ALL":       "C",
		"PATH":         filepath.Join(root, "toolchain", "bin"),
		"TMPDIR":       filepath.Join(root, "scratch", "tmp"),
		"TZ":           "UTC",
	} {
		if values[key] != want {
			t.Errorf("source-list %s = %q, want %q", key, values[key], want)
		}
	}
}

func TestFingerprintNormalizesRegularPermissions(t *testing.T) {
	repository := testSourceRepository(t)
	files, err := liveSourceFiles(repository)
	if err != nil {
		t.Fatalf("liveSourceFiles: %v", err)
	}
	path := filepath.Join(repository, "eventloop", "source.go")
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatalf("Chmod 0600: %v", err)
	}
	plain, err := fingerprintFiles(repository, files)
	if err != nil {
		t.Fatalf("fingerprint plain: %v", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("Chmod 0644: %v", err)
	}
	plainAgain, err := fingerprintFiles(repository, files)
	if err != nil {
		t.Fatalf("fingerprint plain again: %v", err)
	}
	if plainAgain != plain {
		t.Fatalf("non-executable permission changed fingerprint: %s != %s", plainAgain, plain)
	}
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatalf("Chmod 0755: %v", err)
	}
	executable, err := fingerprintFiles(repository, files)
	if err != nil {
		t.Fatalf("fingerprint executable: %v", err)
	}
	if executable == plain {
		t.Fatal("executable permission did not change fingerprint")
	}
}

func TestFingerprintRejectsUnsafeSymlinks(t *testing.T) {
	for _, test := range []struct {
		name   string
		target func(t *testing.T, repository string) string
	}{
		{
			name: "absolute",
			target: func(t *testing.T, _ string) string {
				return filepath.Join(t.TempDir(), "outside")
			},
		},
		{
			name: "escaping",
			target: func(t *testing.T, repository string) string {
				outside := filepath.Join(filepath.Dir(repository), "outside")
				mustWriteFile(t, outside, []byte("outside\n"), 0o644)
				return filepath.Join("..", "..", filepath.Base(outside))
			},
		},
		{
			name: "broken",
			target: func(_ *testing.T, _ string) string {
				return "missing.go"
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := testSourceRepository(t)
			link := filepath.Join(repository, "eventloop", "unsafe-link")
			if err := os.Symlink(test.target(t, repository), link); err != nil {
				t.Fatalf("Symlink: %v", err)
			}
			files, err := liveSourceFiles(repository)
			if err == nil {
				_, err = fingerprintFiles(repository, files)
			}
			if err == nil {
				t.Fatal("unsafe symlink fingerprint unexpectedly passed")
			}
		})
	}
}

func TestPhysicalSourceIgnoresGitVisibility(t *testing.T) {
	repository := testSourceRepository(t)
	ignored := filepath.Join(repository, "eventloop", "ignored.go")
	mustWriteFile(t, filepath.Join(repository, ".gitignore"), []byte("eventloop/ignored.go\n"), 0o644)
	mustWriteFile(t, ignored, []byte("package eventloop\n"), 0o644)
	baseline, err := liveSourceFiles(repository)
	if err != nil {
		t.Fatalf("liveSourceFiles: %v", err)
	}
	if !slices.Contains(baseline, "eventloop/ignored.go") {
		t.Fatalf("physical source omitted ignored input: %q", baseline)
	}
	mustMkdirAll(t, filepath.Join(repository, ".git", "info"))
	mustWriteFile(
		t,
		filepath.Join(repository, ".git", "info", "exclude"),
		[]byte("eventloop/source.go\neventloop/ignored.go\n"),
		0o644,
	)
	after, err := liveSourceFiles(repository)
	if err != nil {
		t.Fatalf("liveSourceFiles after Git exclude: %v", err)
	}
	if !slices.Equal(after, baseline) {
		t.Fatalf("Git visibility changed physical source: before %q, after %q", baseline, after)
	}
}

func TestPhysicalSourceEvidenceOverride(t *testing.T) {
	repository := testSourceRepository(t)
	relative := "eventloop/docs/tournament/2026-05-14/runtime.json"
	mustMkdirAll(t, filepath.Dir(filepath.Join(repository, filepath.FromSlash(relative))))
	mustWriteFile(t, filepath.Join(repository, filepath.FromSlash(relative)), []byte("{}\n"), 0o644)
	files, err := liveSourceFiles(repository)
	if err != nil {
		t.Fatalf("liveSourceFiles: %v", err)
	}
	if slices.Contains(files, relative) {
		t.Fatalf("undeclared dated evidence was included: %q", files)
	}
	files, err = physicalSourceFiles(repository, []string{relative})
	if err != nil {
		t.Fatalf("physicalSourceFiles override: %v", err)
	}
	if !slices.Contains(files, relative) {
		t.Fatalf("declared runtime evidence was omitted: %q", files)
	}
}

func TestValidateRelativePathPortable(t *testing.T) {
	for _, value := range []string{"eventloop/source.go", "goja-eventloop/test data/value.json"} {
		if err := validateRelativePath(value); err != nil {
			t.Errorf("validateRelativePath(%q): %v", value, err)
		}
	}
	for _, value := range []string{
		"",
		".",
		"../source.go",
		"eventloop/../source.go",
		"/eventloop/source.go",
		`eventloop\source.go`,
		"C:/eventloop/source.go",
		"eventloop/source:alternate.go",
		`eventloop/source<alternate.go`,
		`eventloop/source>alternate.go`,
		`eventloop/source"alternate.go`,
		`eventloop/source|alternate.go`,
		`eventloop/source?alternate.go`,
		`eventloop/source*alternate.go`,
		"eventloop/CON.txt",
		"eventloop/source.go ",
		"eventloop/source.go\nextra",
		"eventloop/\xff.go",
	} {
		if err := validateSourceFilePath(value); err == nil {
			t.Errorf("validateSourceFilePath(%q) unexpectedly passed", value)
		}
	}
}

func TestSymlinkTargetRequiresCanonicalPortableText(t *testing.T) {
	for _, target := range []string{"", "/absolute", `C:\\absolute`, `dir\\file`, "value\n", "\xff"} {
		if err := validateSymlinkTarget(target); err == nil {
			t.Errorf("validateSymlinkTarget(%q) unexpectedly passed", target)
		}
	}
	for _, target := range []string{"source.go", "../AGENTS.md"} {
		if err := validateSymlinkTarget(target); err != nil {
			t.Errorf("validateSymlinkTarget(%q): %v", target, err)
		}
	}
}

func TestSourceRecordRejectsNoncanonicalDigest(t *testing.T) {
	digest := strings.Repeat("A", 64)
	record := sourceRecord{Path: "eventloop/source.go", Mode: "100644", SHA256: digest}
	if err := validateSourceRecord(record); err == nil {
		t.Fatal("uppercase source digest unexpectedly passed")
	}
}

func TestSourceRecordRejectsUnsafeSymlinkTarget(t *testing.T) {
	for _, target := range []string{"../../outside", "CON.txt", "target. "} {
		digest := sha256.Sum256([]byte(target))
		record := sourceRecord{
			Path:          "eventloop/link",
			Mode:          "120000",
			Size:          int64(len(target)),
			SHA256:        hex.EncodeToString(digest[:]),
			SymlinkTarget: target,
		}
		if err := validateSourceRecord(record); err == nil {
			t.Errorf("unsafe symlink target %q unexpectedly passed", target)
		}
	}
}

func testSourceRepository(t *testing.T) string {
	t.Helper()
	repository := t.TempDir()
	mustMkdirAll(t, filepath.Join(repository, "eventloop"))
	mustMkdirAll(t, filepath.Join(repository, "goja-eventloop"))
	mustWriteFile(t, filepath.Join(repository, ".gitignore"), nil, 0o644)
	mustWriteFile(t, filepath.Join(repository, "Makefile"), []byte("all:\n"), 0o644)
	mustWriteFile(t, filepath.Join(repository, "go.mod"), []byte("module example.invalid/tournament\n"), 0o644)
	mustWriteFile(t, filepath.Join(repository, "go.sum"), nil, 0o644)
	mustWriteFile(t, filepath.Join(repository, "go.work"), []byte("go 1.26.2\n\nuse .\n"), 0o644)
	mustWriteFile(t, filepath.Join(repository, "go.work.sum"), nil, 0o644)
	mustWriteFile(t, filepath.Join(repository, "project.mk"), []byte("all:\n"), 0o644)
	mustWriteFile(t, filepath.Join(repository, "eventloop", "source.go"), []byte("package eventloop\n"), 0o644)
	return repository
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}
