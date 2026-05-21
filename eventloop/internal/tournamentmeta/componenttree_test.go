package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestComponentTreeDigestFixture(t *testing.T) {
	digest := sha256.Sum256([]byte("abc"))
	records := []componentTreeRecord{
		{Path: ".", Mode: "040000"},
		{Path: "a", Mode: "100644", Size: 3, SHA256: fmt.Sprintf("%x", digest)},
	}
	tree, err := newComponentTree(records)
	if err != nil {
		t.Fatalf("newComponentTree: %v", err)
	}
	const want = "4f1f3825bc2a9959bca13ff68a119e9f761d1cac42dffd843e011c9899ef37a4"
	if tree.SHA256 != want || tree.RecordCount != 2 || tree.PayloadBytes != 3 {
		t.Fatalf("component tree = %+v, want digest %s", tree, want)
	}
	if err := validateComponentTree(tree); err != nil {
		t.Fatalf("validateComponentTree: %v", err)
	}
}

func TestCaptureComponentTreeRelocatesExactly(t *testing.T) {
	first := componentTreeFixture(t)
	second := filepath.Join(t.TempDir(), "relocated")
	copyComponentFixture(t, first, second)
	firstTree, err := captureComponentTree(first)
	if err != nil {
		t.Fatalf("captureComponentTree first: %v", err)
	}
	secondTree, err := captureComponentTree(second)
	if err != nil {
		t.Fatalf("captureComponentTree second: %v", err)
	}
	if !slices.Equal(firstTree.Records, secondTree.Records) || firstTree.SHA256 != secondTree.SHA256 {
		t.Fatalf("relocated trees differ: first %+v, second %+v", firstTree, secondTree)
	}
	wantPaths := []string{".", "a", "a/empty", "a/file", "link"}
	gotPaths := make([]string, len(firstTree.Records))
	for index, record := range firstTree.Records {
		gotPaths[index] = record.Path
	}
	if !slices.Equal(gotPaths, wantPaths) {
		t.Fatalf("component paths = %q, want %q", gotPaths, wantPaths)
	}
}

func TestComponentTreeValidationRejectsTopologyAndPayloadTampering(t *testing.T) {
	root := componentTreeFixture(t)
	tree, err := captureComponentTree(root)
	if err != nil {
		t.Fatalf("captureComponentTree: %v", err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*componentTree)
	}{
		{name: "count", mutate: func(value *componentTree) { value.RecordCount++ }},
		{name: "payload", mutate: func(value *componentTree) { value.PayloadBytes++ }},
		{name: "digest", mutate: func(value *componentTree) { value.SHA256 = fmt.Sprintf("%064x", 1) }},
		{name: "missing parent", mutate: func(value *componentTree) { value.Records[1].Path = "missing/file" }},
		{name: "record digest", mutate: func(value *componentTree) { value.Records[3].SHA256 = fmt.Sprintf("%064x", 1) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := tree
			changed.Records = slices.Clone(tree.Records)
			test.mutate(&changed)
			if err := validateComponentTree(changed); err == nil {
				t.Fatal("tampered component tree unexpectedly passed")
			}
		})
	}
}

func componentTreeFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "a", "empty"))
	mustWriteFile(t, filepath.Join(root, "a", "file"), []byte("payload\n"), 0o644)
	if err := os.Symlink("a/file", filepath.Join(root, "link")); err != nil {
		t.Skipf("component symlink unavailable: %v", err)
	}
	return root
}

func copyComponentFixture(t *testing.T, source, target string) {
	t.Helper()
	mustMkdirAll(t, filepath.Join(target, "a", "empty"))
	data, err := os.ReadFile(filepath.Join(source, "a", "file"))
	if err != nil {
		t.Fatalf("ReadFile fixture: %v", err)
	}
	mustWriteFile(t, filepath.Join(target, "a", "file"), data, 0o644)
	link, err := os.Readlink(filepath.Join(source, "link"))
	if err != nil {
		t.Fatalf("Readlink fixture: %v", err)
	}
	if err := os.Symlink(link, filepath.Join(target, "link")); err != nil {
		t.Fatalf("Symlink fixture: %v", err)
	}
}
