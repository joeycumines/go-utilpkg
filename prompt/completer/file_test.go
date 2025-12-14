package completer

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	prompt "github.com/joeycumines/go-prompt"
)

func TestCleanFilePath(t *testing.T) {
	dir, base, err := cleanFilePath("")
	if err != nil || dir != "." || base != "" {
		t.Fatalf("empty path: dir=%q base=%q err=%v", dir, base, err)
	}

	dir, base, err = cleanFilePath("/tmp/example/")
	if err != nil || base != "" || dir != filepath.FromSlash("/tmp/example/") {
		t.Fatalf("trailing slash: dir=%q base=%q err=%v", dir, base, err)
	}

	dir, base, err = cleanFilePath(filepath.Join("/tmp", "nested", "file.txt"))
	if err != nil || base != "file.txt" {
		t.Fatalf("normal path: dir=%q base=%q err=%v", dir, base, err)
	}
}

func TestCleanFilePathTildeExpansion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("tilde expansion not supported on Windows")
	}

	// Test ~/ expansion to home directory
	dir, base, err := cleanFilePath("~/testfile")
	if err != nil {
		t.Fatalf("tilde expansion error: %v", err)
	}
	if dir == "" || base != "testfile" {
		t.Fatalf("tilde expansion: dir=%q base=%q", dir, base)
	}
	// The dir should not start with ~ anymore
	if len(dir) > 0 && dir[0] == '~' {
		t.Fatalf("tilde was not expanded: dir=%q", dir)
	}

	// Test ~/trailing/ expansion
	dir, base, err = cleanFilePath("~/some/path/")
	if err != nil {
		t.Fatalf("tilde expansion with trailing slash error: %v", err)
	}
	if base != "" {
		t.Fatalf("trailing slash should make base empty, got %q", base)
	}
	// The dir should not start with ~ anymore
	if len(dir) > 0 && dir[0] == '~' {
		t.Fatalf("tilde was not expanded in path with trailing slash: dir=%q", dir)
	}
}

func TestCleanFilePathEnvExpansion(t *testing.T) {
	// Test $VAR expansion
	t.Setenv("TEST_COMPLETER_DIR", "/test/path")
	dir, base, err := cleanFilePath("$TEST_COMPLETER_DIR/file.txt")
	if err != nil {
		t.Fatalf("env expansion error: %v", err)
	}
	if base != "file.txt" {
		t.Fatalf("env expansion: expected base=file.txt, got %q", base)
	}
	if dir != filepath.FromSlash("/test/path") {
		t.Fatalf("env expansion: expected dir=%q, got %q", filepath.FromSlash("/test/path"), dir)
	}
}

func TestFilePathCompleterCompleteCachesAndFilters(t *testing.T) {
	tmpDir := t.TempDir()
	// Create files
	names := []string{"apple.txt", "Banana.txt", "carrot"}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(tmpDir, n), []byte("test"), 0o600); err != nil {
			t.Fatalf("write file %s: %v", n, err)
		}
	}

	buf := prompt.NewBuffer()
	buf.InsertTextMoveCursor(tmpDir+string(os.PathSeparator), 80, 24, false)
	doc := *buf.Document()

	c := &FilePathCompleter{}
	res := c.Complete(doc)
	if len(res) != 3 {
		t.Fatalf("expected 3 suggestions, got %d", len(res))
	}

	// Cache hit with IgnoreCase filtering
	buf2 := prompt.NewBuffer()
	buf2.InsertTextMoveCursor(filepath.Join(tmpDir, "bA"), 80, 24, false)
	doc2 := *buf2.Document()
	c.IgnoreCase = true
	res2 := c.Complete(doc2)
	if len(res2) != 1 {
		t.Fatalf("expected 1 suggestion for prefix bA, got %d", len(res2))
	}
	if got := res2[0].Text; got == "" {
		t.Fatalf("suggestion text should be populated")
	}

	// Filter out carrot using Filter
	c.fileListCache = nil
	c.Filter = func(fi os.FileInfo) bool { return fi.Name() != "carrot" }
	buf3 := prompt.NewBuffer()
	buf3.InsertTextMoveCursor(tmpDir+string(os.PathSeparator), 80, 24, false)
	doc3 := *buf3.Document()
	res3 := c.Complete(doc3)
	if len(res3) != 2 {
		t.Fatalf("expected 2 suggestions after filter, got %d", len(res3))
	}
}

func TestFilePathCompleterNonExistentDirectory(t *testing.T) {
	c := &FilePathCompleter{}
	buf := prompt.NewBuffer()
	buf.InsertTextMoveCursor("/nonexistent/path/that/does/not/exist/file", 80, 24, false)
	doc := *buf.Document()
	res := c.Complete(doc)
	if res != nil {
		t.Fatalf("expected nil for nonexistent directory, got %v", res)
	}
}

func TestFilePathCompleterEmptyPath(t *testing.T) {
	c := &FilePathCompleter{}
	buf := prompt.NewBuffer()
	// Empty buffer means empty path
	doc := *buf.Document()
	res := c.Complete(doc)
	// Should return suggestions for current directory (.)
	// We don't need to check specific results, just that it doesn't crash
	_ = res
}

func TestFilePathCompletionSeparator(t *testing.T) {
	// FilePathCompletionSeparator should contain space and path separator
	if len(FilePathCompletionSeparator) < 2 {
		t.Fatalf("FilePathCompletionSeparator should have at least 2 chars, got %q", FilePathCompletionSeparator)
	}
	// First char is space
	if FilePathCompletionSeparator[0] != ' ' {
		t.Fatalf("first char should be space, got %q", FilePathCompletionSeparator[0])
	}
	// Second char is path separator
	if FilePathCompletionSeparator[1] != os.PathSeparator {
		t.Fatalf("second char should be os.PathSeparator, got %q", FilePathCompletionSeparator[1])
	}
}
