package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestPhysicalSourceRequiresGovernedTopology(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, string)
		want   string
	}{
		{
			name: "missing control",
			mutate: func(t *testing.T, root string) {
				t.Helper()
				if err := os.Remove(filepath.Join(root, "Makefile")); err != nil {
					t.Fatalf("Remove Makefile: %v", err)
				}
			},
			want: "physical source control",
		},
		{
			name: "missing tree",
			mutate: func(t *testing.T, root string) {
				t.Helper()
				if err := os.Remove(filepath.Join(root, "goja-eventloop")); err != nil {
					t.Fatalf("Remove goja-eventloop: %v", err)
				}
			},
			want: "physical source tree",
		},
		{
			name: "tree symlink",
			mutate: func(t *testing.T, root string) {
				t.Helper()
				if err := os.Remove(filepath.Join(root, "goja-eventloop")); err != nil {
					t.Fatalf("Remove goja-eventloop: %v", err)
				}
				if err := os.Symlink("eventloop", filepath.Join(root, "goja-eventloop")); err != nil {
					t.Fatalf("Symlink goja-eventloop: %v", err)
				}
			},
			want: "non-symlink directory",
		},
		{
			name: "tree regular file",
			mutate: func(t *testing.T, root string) {
				t.Helper()
				if err := os.Remove(filepath.Join(root, "goja-eventloop")); err != nil {
					t.Fatalf("Remove goja-eventloop: %v", err)
				}
				mustWriteFile(t, filepath.Join(root, "goja-eventloop"), []byte("not a directory\n"), 0o644)
			},
			want: "non-symlink directory",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := testSourceRepository(t)
			test.mutate(t, repository)
			if _, err := liveSourceFiles(repository); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("liveSourceFiles error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestPhysicalSourceSecretPolicy(t *testing.T) {
	repository := testSourceRepository(t)
	secretDirectory := filepath.Join(repository, "eventloop", ".env")
	mustMkdirAll(t, secretDirectory)
	mustWriteFile(t, filepath.Join(secretDirectory, "value.txt"), []byte("secret\n"), 0o644)
	if _, err := liveSourceFiles(repository); err == nil || !strings.Contains(err.Error(), "forbidden secret path") {
		t.Fatalf("liveSourceFiles error = %v, want secret rejection", err)
	}
}

func TestPhysicalSourcePrunesUndeclaredDatedTree(t *testing.T) {
	repository := testSourceRepository(t)
	directory := filepath.Join(repository, "eventloop", "docs", "tournament", "2026-05-14", ".env")
	mustMkdirAll(t, directory)
	relative := "eventloop/docs/tournament/2026-05-14/.env/value.txt"
	mustWriteFile(t, filepath.Join(repository, filepath.FromSlash(relative)), []byte("excluded\n"), 0o644)
	files, err := liveSourceFiles(repository)
	if err != nil {
		t.Fatalf("liveSourceFiles: %v", err)
	}
	if slices.Contains(files, relative) {
		t.Fatalf("undeclared dated secret was included: %q", files)
	}
	if _, err := physicalSourceFiles(repository, []string{relative}); err == nil ||
		!strings.Contains(err.Error(), "forbidden secret path") {
		t.Fatalf("physicalSourceFiles override error = %v, want secret rejection", err)
	}
}

func TestPhysicalSourceDatedOverrideSurvivesSnapshot(t *testing.T) {
	repository := testSourceRepository(t)
	relative := "eventloop/docs/tournament/2026-05-14/runtime.json"
	mustMkdirAll(t, filepath.Dir(filepath.Join(repository, filepath.FromSlash(relative))))
	mustWriteFile(t, filepath.Join(repository, filepath.FromSlash(relative)), []byte("{}\n"), 0o644)
	output := filepath.Join(t.TempDir(), "snapshot")
	metadata, err := createSnapshotWithEnumerator(
		repository,
		output,
		fixtureSourceAuthority(),
		copySourcePath,
		func(root string) ([]string, error) {
			return physicalSourceFiles(root, []string{relative})
		},
	)
	if err != nil {
		t.Fatalf("createSnapshotWithEnumerator: %v", err)
	}
	if !slices.ContainsFunc(metadata.Files, func(record sourceRecord) bool { return record.Path == relative }) {
		t.Fatalf("snapshot metadata omitted dated runtime asset: %+v", metadata.Files)
	}
	if data, err := os.ReadFile(filepath.Join(output, filepath.FromSlash(relative))); err != nil || string(data) != "{}\n" {
		t.Fatalf("snapshot dated runtime asset = %q, %v", data, err)
	}
}

func TestPortableSourcePathSetRejectsCaseCollision(t *testing.T) {
	if err := validatePortableSourcePathSet([]string{"eventloop/A.go", "eventloop/a.go"}); err == nil {
		t.Fatal("case-fold collision unexpectedly passed")
	}
}
