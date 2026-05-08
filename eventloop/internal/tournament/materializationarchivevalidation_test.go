package tournament

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestTimerMaterializationArchiveTreeParser(t *testing.T) {
	firstObject := "1111111111111111111111111111111111111111"
	secondObject := "2222222222222222222222222222222222222222"
	data := fmt.Appendf(nil,
		"100644 blob %s\ta space.go%c100644 blob %s\tz\tpath.go%c",
		firstObject,
		byte(0),
		secondObject,
		byte(0),
	)
	got, err := parseMaterializationArchiveTree(data)
	if err != nil {
		t.Fatal(err)
	}
	want := []materializationArchiveTreeEntry{
		{path: "a space.go", mode: "100644", objectType: "blob", object: firstObject},
		{path: "z\tpath.go", mode: "100644", objectType: "blob", object: secondObject},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tree entries = %+v, want %+v", got, want)
	}

	malformed := map[string][]byte{
		"empty":             nil,
		"missing-final-nul": data[:len(data)-1],
		"empty-record":      append(append([]byte(nil), data...), 0),
		"missing-tab":       []byte("100644 blob " + firstObject + " a.go\x00"),
		"missing-type":      []byte("100644\ta.go\x00"),
		"missing-object":    []byte("100644 blob\ta.go\x00"),
		"extra-header":      []byte("100644 blob " + firstObject + " extra\ta.go\x00"),
		"short-mode":        []byte("644 blob " + firstObject + "\ta.go\x00"),
		"non-octal-mode":    []byte("1006x4 blob " + firstObject + "\ta.go\x00"),
		"empty-type":        []byte("100644  " + firstObject + "\ta.go\x00"),
		"short-object":      []byte("100644 blob 1111111\ta.go\x00"),
		"uppercase-object":  []byte("100644 blob 111111111111111111111111111111111111111A\ta.go\x00"),
		"unsorted": fmt.Appendf(nil,
			"100644 blob %s\tz.go%c100644 blob %s\ta.go%c",
			firstObject,
			byte(0),
			secondObject,
			byte(0),
		),
		"duplicate": fmt.Appendf(nil,
			"100644 blob %s\ta.go%c100644 blob %s\ta.go%c",
			firstObject,
			byte(0),
			secondObject,
			byte(0),
		),
	}
	for name, input := range malformed {
		t.Run(name, func(t *testing.T) {
			if entries, err := parseMaterializationArchiveTree(input); err == nil {
				t.Fatalf("malformed tree parsed as %+v", entries)
			}
		})
	}
}

func TestTimerMaterializationArchiveValidationRejectsMutation(t *testing.T) {
	valid := validMaterializationArchiveValidationSpec()
	if err := validateMaterializationArchiveSpec(valid); err != nil {
		t.Fatalf("valid specification: %v", err)
	}
	mutations := map[string]func(*materializationArchiveSpec){
		"id-length":     func(value *materializationArchiveSpec) { value.id = "001" },
		"id-digit":      func(value *materializationArchiveSpec) { value.id = "00x1" },
		"object-format": func(value *materializationArchiveSpec) { value.objectFormat = "sha256" },
		"patch-format":  func(value *materializationArchiveSpec) { value.patchFormat = 0 },
		"patch-path":    func(value *materializationArchiveSpec) { value.archive.PatchPath = "../archive.patch" },
		"patch-sha":     func(value *materializationArchiveSpec) { value.archive.PatchSHA256 = "ABC" },
		"patch-bytes":   func(value *materializationArchiveSpec) { value.archive.PatchBytes = 0 },
		"empty-tree": func(value *materializationArchiveSpec) {
			value.archive.EmptyTree = "1111111111111111111111111111111111111111"
		},
		"reconstructed-tree": func(value *materializationArchiveSpec) { value.archive.ReconstructedTree = "short" },
		"path-count":         func(value *materializationArchiveSpec) { value.pathCount++ },
		"payload-bytes":      func(value *materializationArchiveSpec) { value.payloadBytes++ },
		"no-roots":           func(value *materializationArchiveSpec) { value.roots = nil },
		"root-path":          func(value *materializationArchiveSpec) { value.roots[0] = "root/../root" },
		"root-order": func(value *materializationArchiveSpec) {
			value.roots[0], value.roots[1] = value.roots[1], value.roots[0]
		},
		"root-casefold":  func(value *materializationArchiveSpec) { value.roots = []string{"ROOT", "root"} },
		"root-overlap":   func(value *materializationArchiveSpec) { value.roots = []string{"root", "root/sub"} },
		"root-empty":     func(value *materializationArchiveSpec) { value.roots = append(value.roots, "unused") },
		"file-path":      func(value *materializationArchiveSpec) { value.files[0].path = "../a.go" },
		"file-extension": func(value *materializationArchiveSpec) { value.files[0].path = "root/a.txt" },
		"file-order": func(value *materializationArchiveSpec) {
			value.files[0], value.files[1] = value.files[1], value.files[0]
		},
		"file-casefold": func(value *materializationArchiveSpec) {
			value.files[0].path, value.files[1].path = "root/A.go", "root/a.go"
		},
		"file-outside-root": func(value *materializationArchiveSpec) { value.files[0].path = "elsewhere/a.go" },
		"file-mode":         func(value *materializationArchiveSpec) { value.files[0].mode = "100755" },
		"file-blob":         func(value *materializationArchiveSpec) { value.files[0].blob = "short" },
		"file-sha":          func(value *materializationArchiveSpec) { value.files[0].sha256 = "short" },
		"file-bytes":        func(value *materializationArchiveSpec) { value.files[0].bytes = -1 },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			value := cloneMaterializationArchiveSpec(valid)
			mutate(&value)
			if err := validateMaterializationArchiveSpec(value); err == nil {
				t.Fatalf("mutation %q was accepted", name)
			}
		})
	}
}

func TestTimerMaterializationArchivePatchArguments(t *testing.T) {
	base := validMaterializationArchiveValidationSpec()
	wantPrefix := []string{"diff", "--binary", "--abbrev=7"}
	if got := materializationArchivePatchArguments(base); !reflect.DeepEqual(got[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("abbreviated patch arguments = %v", got)
	}
	base.patchFormat = materializationArchivePatchFullIndex
	wantPrefix = []string{"diff", "--binary", "--full-index"}
	if got := materializationArchivePatchArguments(base); !reflect.DeepEqual(got[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("full-index patch arguments = %v", got)
	}
	wantTail := []string{
		"--no-renames",
		"--no-ext-diff",
		"--no-textconv",
		"--no-color",
		"--src-prefix=a/",
		"--dst-prefix=b/",
		base.archive.EmptyTree,
		base.archive.ReconstructedTree,
		"--",
	}
	got := materializationArchivePatchArguments(base)
	if !reflect.DeepEqual(got[len(wantPrefix):], wantTail) {
		t.Fatalf("patch argument tail = %v, want %v", got[len(wantPrefix):], wantTail)
	}
	if err := validateMaterializationArchiveFSCK(nil, nil); err != nil {
		t.Fatalf("empty fsck diagnostics rejected: %v", err)
	}
	for name, output := range map[string][2][]byte{
		"stdout-notice":  {[]byte("notice: HEAD points to an unborn branch\n"), nil},
		"stderr-notice":  {nil, []byte("notice: HEAD points to an unborn branch\n")},
		"stdout-info":    {[]byte("info: checked objects\n"), nil},
		"stderr-warning": {nil, []byte("warning: checked objects\n")},
		"unreachable":    {[]byte("unreachable tree 123\n"), nil},
	} {
		if err := validateMaterializationArchiveFSCK(output[0], output[1]); err == nil {
			t.Fatalf("fsck output %q was accepted", name)
		}
	}
}

func TestTimerMaterializationArchiveLiveCensusRejectsMutation(t *testing.T) {
	repository := t.TempDir()
	root := filepath.Join(repository, "root")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	payload := []byte("package root\n")
	path := filepath.Join(root, "a.go")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	spec := materializationArchiveSpec{
		files: []materializationArchiveFile{{
			path:   "root/a.go",
			mode:   "100644",
			blob:   materializationArchiveBlobSHA1(payload),
			sha256: fmt.Sprintf("%x", sha256.Sum256(payload)),
			bytes:  int64(len(payload)),
		}},
		roots:        []string{"root"},
		pathCount:    1,
		payloadBytes: int64(len(payload)),
	}
	live, err := loadMaterializationArchiveLive(repository, spec.roots)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateMaterializationArchiveLive(spec, live); err != nil {
		t.Fatalf("valid live census: %v", err)
	}

	extra := filepath.Join(root, "extra.txt")
	if err := os.WriteFile(extra, []byte("extra"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadMaterializationArchiveLive(repository, spec.roots); err == nil {
		t.Fatal("non-Go live file was accepted")
	}
	if err := os.Remove(extra); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	live, err = loadMaterializationArchiveLive(repository, spec.roots)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateMaterializationArchiveLive(spec, live); err == nil {
		t.Fatal("changed live payload was accepted")
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(root, "link.go")
	if err := os.Symlink(path, link); err == nil {
		if _, err := loadMaterializationArchiveLive(repository, spec.roots); err == nil {
			t.Fatal("live symlink was accepted")
		}
		if err := os.Remove(link); err != nil {
			t.Fatal(err)
		}
	}
}

func validMaterializationArchiveValidationSpec() materializationArchiveSpec {
	first := []byte("package a\n")
	second := []byte("package b\n")
	return materializationArchiveSpec{
		files: []materializationArchiveFile{
			{path: "root/a.go", mode: "100644", blob: materializationArchiveBlobSHA1(first), sha256: fmt.Sprintf("%x", sha256.Sum256(first)), bytes: int64(len(first))},
			{path: "second/b.go", mode: "100644", blob: materializationArchiveBlobSHA1(second), sha256: fmt.Sprintf("%x", sha256.Sum256(second)), bytes: int64(len(second))},
		},
		roots: []string{"root", "second"},
		archive: timerReferenceMaterializationArchive{
			PatchPath:         "revisions/candidates/0001.patch",
			PatchSHA256:       "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			PatchBytes:        1,
			EmptyTree:         "4b825dc642cb6eb9a060e54bf8d69288fbee4904",
			ReconstructedTree: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
		id:           "0001",
		objectFormat: "sha1",
		patchFormat:  materializationArchivePatchAbbrev7,
		pathCount:    2,
		payloadBytes: int64(len(first) + len(second)),
	}
}

func cloneMaterializationArchiveSpec(value materializationArchiveSpec) materializationArchiveSpec {
	value.files = append([]materializationArchiveFile(nil), value.files...)
	value.roots = append([]string(nil), value.roots...)
	return value
}
