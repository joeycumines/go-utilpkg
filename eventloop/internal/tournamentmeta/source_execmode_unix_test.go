//go:build unix

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFingerprintNormalizesRegularPermissions verifies that the source
// fingerprint distinguishes an executable file from a non-executable one while
// treating non-executable permission differences as equivalent. The execute bit
// is a POSIX concept: os.Chmod cannot toggle it on Windows (NTFS has no
// execute-permission bit), so this invariant can only be exercised on unix
// platforms.
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
