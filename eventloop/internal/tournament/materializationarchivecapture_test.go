package tournament

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestTimerMaterializationArchiveCaptureMatchesExistingAuthority(t *testing.T) {
	want := timerReferenceComponentArchiveSpec()
	got := captureMaterializationArchive(t, want.id, want.roots, want.patchFormat)
	if !reflect.DeepEqual(got.files, want.files) {
		t.Fatalf("captured files differ:\n got: %+v\nwant: %+v", got.files, want.files)
	}
	if got.emptyTree != want.archive.EmptyTree || got.reconstructed != want.archive.ReconstructedTree ||
		got.payloadBytes != want.payloadBytes {
		t.Fatalf(
			"capture identity = empty %s, tree %s, payload %d",
			got.emptyTree,
			got.reconstructed,
			got.payloadBytes,
		)
	}
	repository, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	patchPath := filepath.Join(
		repository,
		"eventloop",
		"internal",
		"tournament",
		filepath.FromSlash(want.archive.PatchPath),
	)
	patch, err := readMaterializationArchivePatch(patchPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.patch, patch.payload) {
		t.Fatal("capture patch differs from immutable 0004 authority")
	}
	if len(got.packageTrees) != len(want.roots) || len(got.componentTrees) != len(want.roots) {
		t.Fatalf("capture package/component tree counts = %d/%d", len(got.packageTrees), len(got.componentTrees))
	}
	for index := range want.roots {
		if !validMaterializationArchiveHex(got.packageTrees[index], 40) ||
			!validMaterializationArchiveHex(got.componentTrees[index], 64) {
			t.Fatalf("capture root %q identities = %q/%q", want.roots[index], got.packageTrees[index], got.componentTrees[index])
		}
	}
}

func TestTimerMaterializationArchiveComponentTreeIncludesEmptyDirectories(t *testing.T) {
	repository := t.TempDir()
	for _, directory := range []string{"root", "root/empty", "root/nested"} {
		if err := os.Mkdir(filepath.Join(repository, filepath.FromSlash(directory)), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for name, payload := range map[string][]byte{
		"root/a.go":        []byte("package root\n"),
		"root/nested/b.go": []byte("package nested\n"),
	} {
		if err := os.WriteFile(filepath.Join(repository, filepath.FromSlash(name)), payload, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	live, err := loadMaterializationArchiveLive(repository, []string{"root"})
	if err != nil {
		t.Fatal(err)
	}
	wantDirectories := []materializationArchiveLiveDirectory{
		{path: "root", mode: "040000"},
		{path: "root/empty", mode: "040000"},
		{path: "root/nested", mode: "040000"},
	}
	if !reflect.DeepEqual(live.directories, wantDirectories) {
		t.Fatalf("live directories = %+v, want %+v", live.directories, wantDirectories)
	}
	withEmpty, err := materializationArchiveComponentTree("root", live)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(repository, "root", "empty")); err != nil {
		t.Fatal(err)
	}
	withoutEmptyLive, err := loadMaterializationArchiveLive(repository, []string{"root"})
	if err != nil {
		t.Fatal(err)
	}
	withoutEmpty, err := materializationArchiveComponentTree("root", withoutEmptyLive)
	if err != nil {
		t.Fatal(err)
	}
	if withEmpty == withoutEmpty {
		t.Fatal("empty directory did not change the component-tree identity")
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(filepath.Join(repository, "root", "a.go"), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := loadMaterializationArchiveLive(repository, []string{"root"}); err == nil {
			t.Fatal("executable Go source was accepted")
		}
	}
}

func TestTimerMaterializationArchiveCaptureInputRejectsMutation(t *testing.T) {
	id := "0006"
	roots := []string{"a/root", "b/root"}
	format := materializationArchivePatchFullIndex
	if err := validateMaterializationArchiveCaptureInput(id, roots, format); err != nil {
		t.Fatalf("valid capture input: %v", err)
	}
	tests := map[string]struct {
		id     string
		roots  []string
		format materializationArchivePatchFormat
	}{
		"short-id":       {id: "006", roots: roots, format: format},
		"nondigit-id":    {id: "00x6", roots: roots, format: format},
		"empty-roots":    {id: id, format: format},
		"invalid-format": {id: id, roots: roots},
		"root-path":      {id: id, roots: []string{"../a"}, format: format},
		"root-order":     {id: id, roots: []string{"b/root", "a/root"}, format: format},
		"root-casefold":  {id: id, roots: []string{"A/root", "a/root"}, format: format},
		"root-overlap":   {id: id, roots: []string{"a", "a/root"}, format: format},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if err := validateMaterializationArchiveCaptureInput(test.id, test.roots, test.format); err == nil {
				t.Fatal("capture-input mutation was accepted")
			}
		})
	}
}

func TestTimerMaterializationArchiveGitEnvironmentIsIsolated(t *testing.T) {
	t.Setenv("GIT_ATTR_NOSYSTEM", "0")
	t.Setenv("GIT_ATTR_SYSTEM", "inherited-system")
	t.Setenv("GIT_ATTR_GLOBAL", "inherited-global")
	t.Setenv("GIT_OBJECT_DIRECTORY", "inherited-objects")
	t.Setenv("HOME", "inherited-home")
	t.Setenv("LANG", "inherited-lang")
	temporary := t.TempDir()
	environment, err := materializationArchiveGitEnvironment(temporary)
	if err != nil {
		t.Fatal(err)
	}
	got := make(map[string][]string)
	for _, entry := range environment {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			t.Fatalf("environment entry %q has no separator", entry)
		}
		if strings.HasPrefix(strings.ToUpper(key), "GIT_") {
			got[key] = append(got[key], value)
		}
	}
	want := map[string]string{
		"GIT_CONFIG_GLOBAL":      filepath.Join(temporary, "blank"),
		"GIT_CONFIG_NOSYSTEM":    "1",
		"GIT_ATTR_NOSYSTEM":      "1",
		"GIT_NO_REPLACE_OBJECTS": "1",
		"GIT_TERMINAL_PROMPT":    "0",
		"GIT_INDEX_FILE":         filepath.Join(temporary, "index"),
	}
	if len(got) != len(want) {
		t.Fatalf("isolated Git environment keys = %+v, want %+v", got, want)
	}
	for key, value := range want {
		if !reflect.DeepEqual(got[key], []string{value}) {
			t.Fatalf("isolated Git environment %s = %v, want %q", key, got[key], value)
		}
	}
}

func TestTimerMaterializationArchivePatchRequiresStableRegularFile(t *testing.T) {
	directory := t.TempDir()
	patchPath := filepath.Join(directory, "archive.patch")
	payload := []byte("patch bytes\n")
	if err := os.WriteFile(patchPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := readMaterializationArchivePatch(patchPath)
	if err != nil {
		t.Fatal(err)
	}
	unchanged, err := readMaterializationArchivePatch(patchPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateMaterializationArchivePatchStable(before, unchanged); err != nil {
		t.Fatalf("unchanged patch rejected: %v", err)
	}
	linkPath := filepath.Join(directory, "archive-link.patch")
	if err := os.Symlink(patchPath, linkPath); err == nil {
		if _, err := readMaterializationArchivePatch(linkPath); err == nil {
			t.Fatal("patch symlink was accepted")
		}
	}
	retainedPath := filepath.Join(directory, "retained.patch")
	if err := os.Rename(patchPath, retainedPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(patchPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	replaced, err := readMaterializationArchivePatch(patchPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateMaterializationArchivePatchStable(before, replaced); err == nil {
		t.Fatal("same-byte patch replacement was accepted")
	}
}

func TestTimerMaterializationArchivePathPortability(t *testing.T) {
	for _, value := range []string{
		"root/a.go",
		"root/a space.go",
		"root/.hidden.go",
		"root/COM10.go",
		"root/日本語.go",
	} {
		if err := validateMaterializationArchivePath(value); err != nil {
			t.Errorf("valid path %q: %v", value, err)
		}
	}
	for _, value := range []string{
		"",
		".",
		"..",
		"../root.go",
		"/root.go",
		"//server/root.go",
		"C:/root.go",
		`root\file.go`,
		"root/a:b.go",
		"root/a?b.go",
		"root/a/../b.go",
		"root//b.go",
		"root/trailing. ",
		"root/CON.go",
		"root/aux.txt",
		"root/COM1.go",
		"root/lpt9.log",
		"root/control\n.go",
		string([]byte{'r', 'o', 'o', 't', '/', 0xff}),
	} {
		if err := validateMaterializationArchivePath(value); err == nil {
			t.Errorf("nonportable path %q was accepted", value)
		}
	}
}
