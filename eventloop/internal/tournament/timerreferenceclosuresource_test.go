package tournament

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const (
	closureCCPackage   = "timerrefclosurecc"
	closureB77Package  = "timerrefclosureb77"
	closureCCRevision  = "cc005d72"
	closureB77Revision = "b77a13cf"
)

var closureCCB77Files = []string{
	"autoexit_test.go",
	"core.go",
	"core_test.go",
	"deadlock_test.go",
	"fd.go",
	"layout64_test.go",
	"lifecycle.go",
	"lifecycle_test.go",
	"queue_stability_test.go",
	"reference_model_test.go",
	"registration.go",
	"registration_test.go",
	"wake_test.go",
	"worker.go",
	"worker_test.go",
}

type closureSourceFile struct {
	mode    os.FileMode
	payload []byte
}

func TestTimerReferenceClosureCCB77SourceEquality(t *testing.T) {
	cc := loadClosureSourceTree(t, closureCCPackage)
	b77 := loadClosureSourceTree(t, closureB77Package)
	if err := compareClosureSourceTrees(cc, b77); err != nil {
		t.Fatal(err)
	}
}

func TestTimerReferenceClosureCCB77SourceEqualityRejectsDrift(t *testing.T) {
	cc := loadClosureSourceTree(t, closureCCPackage)
	b77 := loadClosureSourceTree(t, closureB77Package)
	if err := compareClosureSourceTrees(cc, b77); err != nil {
		t.Fatal(err)
	}

	mutations := map[string]func(map[string]closureSourceFile){
		"statement": func(tree map[string]closureSourceFile) {
			mutateClosureSource(t, tree, "core.go", "l.applyTimerRefChange(id, refed)", "l.applyTimerRefChange(id, !refed)")
		},
		"test assertion": func(tree map[string]closureSourceFile) {
			mutateClosureSource(t, tree, "wake_test.go", "value.submissionEpoch.Load() != 4", "value.submissionEpoch.Load() != 5")
		},
		"extra file": func(tree map[string]closureSourceFile) {
			tree["unexpected.go"] = closureSourceFile{mode: 0o644, payload: []byte("package timerrefclosureb77\n")}
		},
		"revision occurrence": func(tree map[string]closureSourceFile) {
			mutateClosureSource(t, tree, "core.go", closureB77Revision, "b77a13ce")
		},
		"owner import": func(tree map[string]closureSourceFile) {
			mutateClosureSource(t, tree, "lifecycle.go", "\t\"sync/atomic\"\n", "\t\"sync\"\n")
		},
		"owner helper": func(tree map[string]closureSourceFile) {
			mutateClosureSource(t, tree, "lifecycle.go", "return goroutineid.Get()", "return goroutineid.Slow(nil)")
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			candidate := cloneClosureSourceTree(b77)
			mutate(candidate)
			if err := compareClosureSourceTrees(cc, candidate); err == nil {
				t.Fatal("source equality accepted mutated b77 materialization")
			}
		})
	}
}

func loadClosureSourceTree(t *testing.T, packageName string) map[string]closureSourceFile {
	t.Helper()
	directory := filepath.Join("component", packageName)
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	tree := make(map[string]closureSourceFile, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		payload, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		tree[entry.Name()] = closureSourceFile{mode: info.Mode().Perm(), payload: payload}
	}
	return tree
}

func compareClosureSourceTrees(cc, b77 map[string]closureSourceFile) error {
	if err := validateClosureSourceFiles(cc); err != nil {
		return fmt.Errorf("cc file set: %w", err)
	}
	if err := validateClosureSourceFiles(b77); err != nil {
		return fmt.Errorf("b77 file set: %w", err)
	}
	for _, name := range closureCCB77Files {
		ccFile := cc[name]
		b77File := b77[name]
		// Platform file permissions vary (e.g. 0o644 on Unix, 0o666 on
		// Windows). Verify that corresponding files have identical modes
		// rather than asserting a specific permission value.
		if ccFile.mode != b77File.mode {
			return fmt.Errorf("%s modes differ: cc=%#o b77=%#o", name, ccFile.mode, b77File.mode)
		}
		ccPayload, err := normalizeClosureSource(closureCCPackage, closureCCRevision, name, ccFile.payload)
		if err != nil {
			return fmt.Errorf("normalize cc %s: %w", name, err)
		}
		b77Payload, err := normalizeClosureSource(closureB77Package, closureB77Revision, name, b77File.payload)
		if err != nil {
			return fmt.Errorf("normalize b77 %s: %w", name, err)
		}
		if !bytes.Equal(ccPayload, b77Payload) {
			return fmt.Errorf(
				"%s differs after identity-only normalization: cc=%x b77=%x",
				name,
				sha256.Sum256(ccPayload),
				sha256.Sum256(b77Payload),
			)
		}
	}
	return nil
}

func validateClosureSourceFiles(tree map[string]closureSourceFile) error {
	got := make([]string, 0, len(tree))
	for name := range tree {
		got = append(got, name)
	}
	sort.Strings(got)
	if len(got) != len(closureCCB77Files) {
		return fmt.Errorf("files = %v, want %v", got, closureCCB77Files)
	}
	for index, name := range closureCCB77Files {
		if got[index] != name {
			return fmt.Errorf("files = %v, want %v", got, closureCCB77Files)
		}
	}
	return nil
}

func normalizeClosureSource(packageName, revision, name string, payload []byte) ([]byte, error) {
	result := string(payload)
	packageCount := 1
	if name == "core.go" {
		packageCount = 2
	}
	var err error
	result, err = replaceClosureSourceCount(result, packageName, "timerrefclosuregeneration", packageCount)
	if err != nil {
		return nil, err
	}
	revisionCount := 0
	switch name {
	case "core.go":
		revisionCount = 4
	case "lifecycle.go":
		revisionCount = 3
	}
	result, err = replaceClosureSourceCount(result, revision, "historicalrevision", revisionCount)
	if err != nil {
		return nil, err
	}
	if name != "lifecycle.go" {
		return []byte(result), nil
	}

	const normalizedHelper = "/* normalized compiler-selected owner helper */"
	switch packageName {
	case closureCCPackage:
		result, err = replaceClosureSourceCount(result, "\t\"sync\"\n", "", 1)
		if err != nil {
			return nil, err
		}
		result, err = replaceClosureSourceCount(result, closureCCOwnerHelper, normalizedHelper, 1)
	case closureB77Package:
		if strings.Contains(result, "\t\"sync\"\n") {
			return nil, fmt.Errorf("unexpected b77 sync import")
		}
		result, err = replaceClosureSourceCount(result, closureB77OwnerHelper, normalizedHelper, 1)
	default:
		return nil, fmt.Errorf("unsupported closure package %q", packageName)
	}
	if err != nil {
		return nil, err
	}
	return []byte(result), nil
}

func replaceClosureSourceCount(source, old, replacement string, want int) (string, error) {
	if got := strings.Count(source, old); got != want {
		return "", fmt.Errorf("occurrences of %q = %d, want %d", old, got, want)
	}
	return strings.ReplaceAll(source, old, replacement), nil
}

func cloneClosureSourceTree(source map[string]closureSourceFile) map[string]closureSourceFile {
	clone := make(map[string]closureSourceFile, len(source))
	for name, file := range source {
		clone[name] = closureSourceFile{mode: file.mode, payload: bytes.Clone(file.payload)}
	}
	return clone
}

func mutateClosureSource(t *testing.T, tree map[string]closureSourceFile, name, old, replacement string) {
	t.Helper()
	file, ok := tree[name]
	if !ok {
		t.Fatalf("missing mutation target %s", name)
	}
	if !bytes.Contains(file.payload, []byte(old)) {
		t.Fatalf("mutation target %q missing from %s", old, name)
	}
	file.payload = bytes.Replace(file.payload, []byte(old), []byte(replacement), 1)
	tree[name] = file
}

const closureCCOwnerHelper = `var goroutineIDBuffers = sync.Pool{New: func() any {
	value := make([]byte, 64)
	return &value
}}

func currentGoroutineID() int64 {
	id := goroutineid.Fast()
	if id != -1 {
		return id
	}
	buffer := goroutineIDBuffers.Get().(*[]byte)
	id = goroutineid.Slow(*buffer)
	goroutineIDBuffers.Put(buffer)
	return id
}`

const closureB77OwnerHelper = `func currentGoroutineID() int64 {
	return goroutineid.Get()
}`
