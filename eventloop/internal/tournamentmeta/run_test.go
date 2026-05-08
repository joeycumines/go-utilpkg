package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestReadRunInputsPreservesExactRecords(t *testing.T) {
	rootPath := t.TempDir()
	executable := currentTestExecutable(t)
	argv := []string{executable, "", "spaces quotes ' * ? [", "line\nbreak", "世界"}
	environment := testRunEnvironment("VALUE=spaces quotes ' * ? [", "EMPTY=")
	mustWriteNULFile(t, filepath.Join(rootPath, runArgvName), argv)
	mustWriteNULFile(t, filepath.Join(rootPath, runEnvironmentName), environment)
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	defer root.Close()
	inputs, err := readRunInputs(root)
	if err != nil {
		t.Fatalf("readRunInputs: %v", err)
	}
	if !slices.Equal(inputs.argv, argv) {
		t.Fatalf("argv = %q, want %q", inputs.argv, argv)
	}
	if !slices.Equal(inputs.environment, environment) {
		t.Fatalf("environment = %q, want %q", inputs.environment, environment)
	}
	if inputs.argvArtifact.RecordCount != len(argv) || inputs.environmentArtifact.RecordCount != len(environment) {
		t.Fatalf("input artifacts = %+v, %+v", inputs.argvArtifact, inputs.environmentArtifact)
	}
}

func TestValidateRunEnvironmentRejectsInvalidRecords(t *testing.T) {
	for _, test := range []struct {
		name        string
		environment []string
	}{
		{name: "empty"},
		{name: "duplicate", environment: []string{"PATH=/bin", "PATH=/usr/bin"}},
		{name: "missing equals", environment: []string{"PATH"}},
		{name: "empty key", environment: []string{"=/bin"}},
		{name: "newline key", environment: []string{"BAD\nKEY=value"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := validateRunEnvironment(test.environment); err == nil {
				t.Fatal("invalid environment unexpectedly passed")
			}
		})
	}
}

func TestReadNULInputRejectsInvalidFramingAndSymlink(t *testing.T) {
	for _, test := range []struct {
		name string
		data []byte
	}{
		{name: "empty"},
		{name: "missing final NUL", data: []byte("value")},
	} {
		t.Run(test.name, func(t *testing.T) {
			rootPath := t.TempDir()
			mustWriteFile(t, filepath.Join(rootPath, runArgvName), test.data, 0o600)
			root, err := os.OpenRoot(rootPath)
			if err != nil {
				t.Fatalf("OpenRoot: %v", err)
			}
			defer root.Close()
			if _, _, err := readNULInput(root, runArgvName); err == nil {
				t.Fatal("invalid NUL input unexpectedly passed")
			}
		})
	}
	t.Run("symlink", func(t *testing.T) {
		rootPath := t.TempDir()
		target := filepath.Join(rootPath, "target")
		mustWriteFile(t, target, []byte("value\x00"), 0o600)
		if err := os.Symlink(target, filepath.Join(rootPath, runArgvName)); err != nil {
			t.Skipf("Symlink unavailable: %v", err)
		}
		root, err := os.OpenRoot(rootPath)
		if err != nil {
			t.Fatalf("OpenRoot: %v", err)
		}
		defer root.Close()
		if _, _, err := readNULInput(root, runArgvName); err == nil {
			t.Fatal("symlink NUL input unexpectedly passed")
		}
	})
}

func mustWriteNULFile(t *testing.T, path string, records []string) {
	t.Helper()
	var data []byte
	for _, record := range records {
		data = append(data, record...)
		data = append(data, 0)
	}
	mustWriteFile(t, path, data, 0o600)
}

func currentTestExecutable(t *testing.T) string {
	t.Helper()
	path, err := os.Executable()
	if err != nil {
		t.Fatalf("Executable: %v", err)
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	return path
}

func testRunEnvironment(extra ...string) []string {
	environment := []string{"LANG=C", "LC_ALL=C", "PATH=/usr/bin:/bin"}
	environment = append(environment, testRunEnvironmentPlatform()...)
	environment = append(environment, extra...)
	return sortRunTestEnvironment(environment)
}

func TestWindowsLaunchSourceRetainsExactCreateProcessOwnership(t *testing.T) {
	data, err := os.ReadFile("processlaunch_windows.go")
	if err != nil {
		t.Fatalf("ReadFile Windows process launch: %v", err)
	}
	source := string(data)
	for _, required := range []string{
		"windows.CreateProcess(",
		"windows.AssignProcessToJobObject(job, information.Process)",
		"windows.ResumeThread(information.Thread)",
		"windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("Windows process launch lacks %q", required)
		}
	}
	for _, forbidden := range []string{
		"CreateToolhelp32Snapshot",
		"OpenThread",
		"OpenProcess",
		"GenerateConsoleCtrlEvent",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("Windows process launch contains forbidden ownership recovery %q", forbidden)
		}
	}
}
