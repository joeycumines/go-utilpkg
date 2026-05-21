//go:build unix

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestComponentTreeExecutableModeChangesIdentity verifies that the execute bit
// is part of component identity. The execute bit is a POSIX concept: os.Chmod
// cannot toggle it on Windows (NTFS has no execute-permission bit), so this
// invariant can only be exercised on unix platforms.
func TestComponentTreeExecutableModeChangesIdentity(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "tool")
	mustWriteFile(t, path, []byte("tool\n"), 0o644)
	plain, err := captureComponentTree(root)
	if err != nil {
		t.Fatalf("capture plain: %v", err)
	}
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatalf("Chmod executable: %v", err)
	}
	executable, err := captureComponentTree(root)
	if err != nil {
		t.Fatalf("capture executable: %v", err)
	}
	if executable.SHA256 == plain.SHA256 || executable.Records[1].Mode != "100755" {
		t.Fatal("executable mode did not change component identity")
	}
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatalf("Chmod executable permissions: %v", err)
	}
	normalized, err := captureComponentTree(root)
	if err != nil {
		t.Fatalf("capture normalized executable: %v", err)
	}
	if normalized.SHA256 != executable.SHA256 {
		t.Fatal("ordinary executable permissions changed component identity")
	}
}
