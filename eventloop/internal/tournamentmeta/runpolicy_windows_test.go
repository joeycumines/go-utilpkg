//go:build windows

package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func testRunEnvironmentPlatform() []string {
	return []string{"SYSTEMROOT=" + os.Getenv("SYSTEMROOT")}
}

func sortRunTestEnvironment(environment []string) []string {
	slices.SortFunc(environment, func(left, right string) int {
		leftKey, _, _ := strings.Cut(left, "=")
		rightKey, _, _ := strings.Cut(right, "=")
		return strings.Compare(strings.ToUpper(leftKey), strings.ToUpper(rightKey))
	})
	return environment
}

func TestWindowsRunEnvironmentPolicy(t *testing.T) {
	for _, test := range []struct {
		name        string
		environment []string
	}{
		{name: "case duplicate", environment: []string{"Path=a", "PATH=b", "SYSTEMROOT=C:\\Windows"}},
		{name: "missing system root", environment: []string{"PATH=a"}},
		{name: "empty system root", environment: []string{"PATH=a", "SYSTEMROOT="}},
		{name: "unsorted", environment: []string{"SYSTEMROOT=C:\\Windows", "PATH=a"}},
		{name: "invalid UTF-8", environment: []string{"PATH=a", "SYSTEMROOT=C:\\Windows", "\xff=x"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := validateRunEnvironment(test.environment); err == nil {
				t.Fatal("invalid Windows environment unexpectedly passed")
			}
		})
	}
}

func TestWindowsExecutableNeedsNoUnixModeBits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runner.exe")
	if err := os.WriteFile(path, []byte("not executed"), 0o666); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !validRunExecutable(info) {
		t.Fatalf("regular Windows executable mode %v was rejected", info.Mode())
	}
}
