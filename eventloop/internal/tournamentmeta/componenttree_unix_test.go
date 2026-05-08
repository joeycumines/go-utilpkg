//go:build unix

package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestComponentTreeRejectsSpecialFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "fifo")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("Mkfifo: %v", err)
	}
	defer os.Remove(path)
	if _, err := captureComponentTree(root); err == nil || !strings.Contains(err.Error(), "unsupported mode") {
		t.Fatalf("captureComponentTree error = %v, want special-file rejection", err)
	}
}
