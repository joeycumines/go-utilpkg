package oracle

import (
	"archive/zip"
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/mod/module"
	"golang.org/x/mod/sumdb/dirhash"
	modzip "golang.org/x/mod/zip"
)

const candidateIdentityVector = "a2fc9660b1905247a42da9968e09ef5a531bc07902b913df2f0f104f2b607da3"

func TestCandidateModuleSHA256Vector(t *testing.T) {
	root := t.TempDir()
	writeCandidateTestFile(t, root, "go.mod", "module example.com/m\n\ngo 1.26.2\n")
	writeCandidateTestFile(t, root, "m.go", "package m\n")
	assertCandidateIdentity(t, root, candidateIdentityVector, 2)
}

func TestCandidateModuleSHA256ArchiveParity(t *testing.T) {
	root := t.TempDir()
	writeCandidateTestFile(t, root, "go.mod", "module example.com/m\n\ngo 1.26.2\n")
	writeCandidateTestFile(t, root, "m.go", "package m\n")
	writeCandidateTestFile(t, root, ".config/kept.txt", "included\n")
	writeCandidateTestFile(t, root, "nested/go.mod", "module example.com/nested\n")
	writeCandidateTestFile(t, root, "nested/nested.go", "package nested\n")
	writeCandidateTestFile(t, root, "vendor/example.com/dependency/dependency.go", "package dependency\n")

	digest, records, err := candidateModuleSHA256(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(t.TempDir(), "candidate.zip")
	archive, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	version := module.Version{Path: "example.com/m", Version: "v1.2.3"}
	createErr := modzip.CreateFromDir(archive, version, root)
	closeErr := archive.Close()
	if err := errors.Join(createErr, closeErr); err != nil {
		t.Fatal(err)
	}

	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	prefix := version.Path + "@" + version.Version + "/"
	files := make(map[string]*zip.File, len(reader.File))
	relatives := make([]string, 0, len(reader.File))
	for _, file := range reader.File {
		relative, ok := strings.CutPrefix(file.Name, prefix)
		if !ok || relative == "" {
			t.Fatalf("archive member %q lacks prefix %q", file.Name, prefix)
		}
		files[relative] = file
		relatives = append(relatives, relative)
	}
	archiveHash, err := dirhash.Hash1(relatives, func(relative string) (io.ReadCloser, error) {
		file := files[relative]
		if file == nil {
			return nil, os.ErrNotExist
		}
		return file.Open()
	})
	if err != nil {
		t.Fatal(err)
	}
	digestBytes, err := hex.DecodeString(digest)
	if err != nil {
		t.Fatal(err)
	}
	wantArchiveHash := "h1:" + base64.StdEncoding.EncodeToString(digestBytes)
	if archiveHash != wantArchiveHash || records != len(relatives) {
		t.Fatalf("identity = %s / %d records; archive = %s / %d records", wantArchiveHash, records, archiveHash, len(relatives))
	}
}

func TestCandidateModuleSHA256UnpackedMembership(t *testing.T) {
	root := t.TempDir()
	writeCandidateTestFile(t, root, "go.mod", "module example.com/m\n\ngo 1.26.2\n")
	source := filepath.Join(root, "m.go")
	writeCandidateTestFile(t, root, "m.go", "package m\n")

	writeCandidateTestFile(t, root, "nested/go.mod", "module example.com/nested\n")
	writeCandidateTestFile(t, root, "nested/nested.go", "package nested\n")
	writeCandidateTestFile(t, root, "vendor/example.com/dependency/dependency.go", "package dependency\n")
	writeCandidateTestFile(t, root, "vendor/modules.txt", "ignored\n")
	writeCandidateTestFile(t, root, ".hg/store", "ignored\n")
	writeCandidateTestFile(t, root, ".svn/entries", "ignored\n")
	if err := os.Mkdir(filepath.Join(root, "empty"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("m.go", filepath.Join(root, "link.go")); err != nil {
		t.Logf("symlink omission not exercised: %v", err)
	}
	assertCandidateIdentity(t, root, candidateIdentityVector, 2)

	if err := os.Chmod(source, 0o755); err != nil {
		t.Fatal(err)
	}
	assertCandidateIdentity(t, root, candidateIdentityVector, 2)

	writeCandidateTestFile(t, root, ".config/kept.txt", "included\n")
	digest, records, err := candidateModuleSHA256(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if digest == candidateIdentityVector || records != 3 {
		t.Fatalf("hidden ordinary member identity = %s / %d records", digest, records)
	}
}

func TestCandidateModuleSHA256GitVisibleMembership(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}

	repository := t.TempDir()
	runCandidateGit(t, repository, "init", "-q")
	root := filepath.Join(repository, "module")
	writeCandidateTestFile(t, root, "go.mod", "module example.com/m\n\ngo 1.26.2\n")
	source := filepath.Join(root, "m.go")
	writeCandidateTestFile(t, root, "m.go", "package m\n")
	runCandidateGit(t, repository, "add", "module/go.mod", "module/m.go")
	writeCandidateTestFile(t, filepath.Join(repository, ".git"), "info/exclude", "ignored.out\nignored-module/\n")

	writeCandidateTestFile(t, root, "ignored.out", "ignored one\n")
	writeCandidateTestFile(t, root, "nested/go.mod", "module example.com/nested\n")
	writeCandidateTestFile(t, root, "nested/nested.go", "package nested\n")
	writeCandidateTestFile(t, root, "vendor/example.com/dependency/dependency.go", "package dependency\n")
	writeCandidateTestFile(t, root, ".hg/store", "ignored\n")
	if err := os.Symlink("m.go", filepath.Join(root, "link.go")); err != nil {
		t.Logf("symlink omission not exercised: %v", err)
	}
	if err := os.Chmod(source, 0o755); err != nil {
		t.Fatal(err)
	}
	assertCandidateIdentity(t, root, candidateIdentityVector, 2)

	writeCandidateTestFile(t, root, "ignored.out", "ignored two\n")
	writeCandidateTestFile(t, root, "nested/nested.go", "package nested\n// changed\n")
	writeCandidateTestFile(t, root, "vendor/example.com/dependency/dependency.go", "package dependency\n// changed\n")
	writeCandidateTestFile(t, root, ".hg/store", "changed\n")
	assertCandidateIdentity(t, root, candidateIdentityVector, 2)

	untracked := filepath.Join(repository, "untracked")
	writeCandidateTestFile(t, untracked, "go.mod", "module example.com/m\n\ngo 1.26.2\n")
	writeCandidateTestFile(t, untracked, "m.go", "package m\n")
	writeCandidateTestFile(t, untracked, "ignored.out", "ignored\n")
	assertCandidateIdentity(t, untracked, candidateIdentityVector, 2)

	ignoredModule := filepath.Join(repository, "ignored-module")
	writeCandidateTestFile(t, ignoredModule, "go.mod", "module example.com/ignored\n")
	if _, _, err := candidateModuleSHA256(context.Background(), ignoredModule); err == nil || !strings.Contains(err.Error(), "does not contain a regular go.mod") {
		t.Fatalf("ignored go.mod identity error = %v", err)
	}

	visible := filepath.Join(root, "visible.txt")
	writeCandidateTestFile(t, root, "visible.txt", "visible one\n")
	firstVisible, records, err := candidateModuleSHA256(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if firstVisible == candidateIdentityVector || records != 3 {
		t.Fatalf("untracked member identity = %s / %d records", firstVisible, records)
	}
	writeCandidateTestFile(t, root, "visible.txt", "visible two\n")
	secondVisible, records, err := candidateModuleSHA256(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if secondVisible == firstVisible || records != 3 {
		t.Fatalf("changed untracked member identity = %s / %d records", secondVisible, records)
	}
	if err := os.Remove(visible); err != nil {
		t.Fatal(err)
	}
	assertCandidateIdentity(t, root, candidateIdentityVector, 2)

	runCandidateGit(t, repository, "add", "-f", "module/ignored.out")
	trackedIgnored, records, err := candidateModuleSHA256(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if trackedIgnored == candidateIdentityVector || records != 3 {
		t.Fatalf("tracked ignored member identity = %s / %d records", trackedIgnored, records)
	}
	if err := os.Remove(filepath.Join(root, "ignored.out")); err != nil {
		t.Fatal(err)
	}
	assertCandidateIdentity(t, root, candidateIdentityVector, 2)

	writeCandidateTestFile(t, root, "m.go", "package m\n// changed\n")
	changed, records, err := candidateModuleSHA256(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if changed == candidateIdentityVector || records != 2 {
		t.Fatalf("changed tracked member identity = %s / %d records", changed, records)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := candidateModuleSHA256(cancelled, root); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled identity error = %v, want context.Canceled", err)
	}

	t.Run("Git unavailable fails closed", func(t *testing.T) {
		t.Setenv("PATH", "")
		if _, _, err := candidateModuleSHA256(context.Background(), root); err == nil || !strings.Contains(err.Error(), "Git-visible candidate files") {
			t.Fatalf("Git-unavailable identity error = %v", err)
		}
	})
}

func TestCandidateModuleSHA256Errors(t *testing.T) {
	t.Run("cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, _, err := candidateModuleSHA256(ctx, t.TempDir()); !errors.Is(err, context.Canceled) {
			t.Fatalf("identity error = %v, want context.Canceled", err)
		}
	})

	t.Run("root is not a directory", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "candidate")
		writeCandidateTestFile(t, filepath.Dir(root), filepath.Base(root), "not a directory\n")
		if _, _, err := candidateModuleSHA256(context.Background(), root); err == nil || !strings.Contains(err.Error(), "not a directory") {
			t.Fatalf("identity error = %v", err)
		}
	})

	t.Run("invalid module file path", func(t *testing.T) {
		root := t.TempDir()
		writeCandidateTestFile(t, root, "go.mod", "module example.com/m\n")
		writeCandidateTestFile(t, root, "bad😀", "invalid\n")
		if _, _, err := candidateModuleSHA256(context.Background(), root); err == nil || !strings.Contains(err.Error(), "invalid char") {
			t.Fatalf("identity error = %v", err)
		}
	})

	t.Run("case-fold collision", func(t *testing.T) {
		root := t.TempDir()
		writeCandidateTestFile(t, root, "go.mod", "module example.com/m\n")
		writeCandidateTestFile(t, root, "m.go", "package m\n")
		writeCandidateTestFile(t, root, "M.GO", "package m\n")
		lower, lowerErr := os.Stat(filepath.Join(root, "m.go"))
		upper, upperErr := os.Stat(filepath.Join(root, "M.GO"))
		if lowerErr != nil || upperErr != nil {
			t.Fatal(errors.Join(lowerErr, upperErr))
		}
		if os.SameFile(lower, upper) {
			t.Skip("case-insensitive filesystem cannot represent the collision")
		}
		if _, _, err := candidateModuleSHA256(context.Background(), root); err == nil || !strings.Contains(err.Error(), "case-insensitive file name collision") {
			t.Fatalf("identity error = %v", err)
		}
	})

	t.Run("oversized go.mod", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "go.mod")
		writeCandidateTestFile(t, root, "go.mod", "module example.com/m\n")
		if err := os.Truncate(path, int64(modzip.MaxGoMod)+1); err != nil {
			t.Fatal(err)
		}
		if _, _, err := candidateModuleSHA256(context.Background(), root); err == nil || !strings.Contains(err.Error(), "go.mod file too large") {
			t.Fatalf("identity error = %v", err)
		}
	})
}

func assertCandidateIdentity(t *testing.T, root, wantDigest string, wantRecords int) {
	t.Helper()
	digest, records, err := candidateModuleSHA256(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if digest != wantDigest || records != wantRecords {
		t.Fatalf("candidate identity = %s / %d records, want %s / %d", digest, records, wantDigest, wantRecords)
	}
}

func writeCandidateTestFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func runCandidateGit(t *testing.T, directory string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = directory
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}
