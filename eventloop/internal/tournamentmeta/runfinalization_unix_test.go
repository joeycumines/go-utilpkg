//go:build unix

package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRunStatusWaitsForScopeCloseAndArtifactFinalization(t *testing.T) {
	_, artifactRoot, arguments := runOperationFixture(t, "printf output; printf error >&2")
	operations := defaultRunOperations()
	closeEntered := make(chan struct{})
	closeRelease := make(chan struct{})
	var events []string
	operations.closeScope = func(scope *ownedProcess) error {
		events = append(events, "scope-close")
		close(closeEntered)
		<-closeRelease
		return scope.close()
	}
	originalSync := operations.syncFile
	operations.syncFile = func(file *os.File) error {
		events = append(events, "sync-"+filepath.Base(file.Name()))
		return originalSync(file)
	}
	originalClose := operations.closeFile
	operations.closeFile = func(file *os.File) error {
		events = append(events, "close-"+filepath.Base(file.Name()))
		return originalClose(file)
	}
	originalHash := operations.hashOutput
	operations.hashOutput = func(root *os.Root, name string) (runOutputArtifact, error) {
		events = append(events, "hash-"+name)
		return originalHash(root, name)
	}
	originalCloseRoot := operations.closeRoot
	operations.closeRoot = func(root *os.Root) error {
		events = append(events, "root-close")
		return originalCloseRoot(root)
	}
	originalWriteStatus := operations.writeStatus
	operations.writeStatus = func(path string, status runStatus) error {
		events = append(events, "status-write")
		return originalWriteStatus(path, status)
	}
	done := make(chan int, 1)
	go func() { done <- runCommandOperations(arguments, operations) }()
	<-closeEntered
	statusPath := filepath.Join(artifactRoot, runStatusName)
	if _, err := os.Lstat(statusPath); !os.IsNotExist(err) {
		t.Fatalf("status exists before scope close: %v", err)
	}
	close(closeRelease)
	if code := <-done; code != 0 {
		t.Fatalf("run exit = %d, want 0", code)
	}
	want := []string{
		"scope-close",
		"sync-" + runStdoutName,
		"sync-" + runStderrName,
		"close-" + runStdoutName,
		"close-" + runStderrName,
		"close-" + filepath.Base(os.DevNull),
		"hash-" + runStdoutName,
		"hash-" + runStderrName,
		"root-close",
		"status-write",
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("finalization events = %q, want %q", events, want)
	}
}

func TestRunFinalizationFailuresPublishRejectedStatus(t *testing.T) {
	sentinel := errors.New("forced finalization failure")
	for _, test := range []struct {
		name   string
		mutate func(*runOperations)
		check  func(*testing.T, runStatus)
	}{
		{
			name: "scope close",
			mutate: func(operations *runOperations) {
				original := operations.closeScope
				operations.closeScope = func(scope *ownedProcess) error {
					return errors.Join(original(scope), sentinel)
				}
			},
			check: func(t *testing.T, status runStatus) {
				t.Helper()
				if status.ContainmentOK || status.Reason != "containment-error" || status.Scope.Closed {
					t.Fatalf("scope-close status = %+v", status)
				}
			},
		},
		{
			name: "stdout sync",
			mutate: func(operations *runOperations) {
				original := operations.syncFile
				operations.syncFile = func(file *os.File) error {
					err := original(file)
					if filepath.Base(file.Name()) == runStdoutName {
						return errors.Join(err, sentinel)
					}
					return err
				}
			},
			check: assertArtifactRejected,
		},
		{
			name: "stderr close",
			mutate: func(operations *runOperations) {
				original := operations.closeFile
				operations.closeFile = func(file *os.File) error {
					err := original(file)
					if filepath.Base(file.Name()) == runStderrName {
						return errors.Join(err, sentinel)
					}
					return err
				}
			},
			check: assertArtifactRejected,
		},
		{
			name: "stdout hash",
			mutate: func(operations *runOperations) {
				original := operations.hashOutput
				operations.hashOutput = func(root *os.Root, name string) (runOutputArtifact, error) {
					if name == runStdoutName {
						return runOutputArtifact{Name: name}, sentinel
					}
					return original(root, name)
				}
			},
			check: assertArtifactRejected,
		},
		{
			name: "artifact root close",
			mutate: func(operations *runOperations) {
				original := operations.closeRoot
				operations.closeRoot = func(root *os.Root) error {
					return errors.Join(original(root), sentinel)
				}
			},
			check: assertArtifactRejected,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, artifactRoot, arguments := runOperationFixture(t, "exit 0")
			operations := defaultRunOperations()
			test.mutate(&operations)
			if code := runCommandOperations(arguments, operations); code != 125 {
				t.Fatalf("run exit = %d, want 125", code)
			}
			status := readRunStatus(t, artifactRoot)
			if !strings.Contains(status.Diagnostic, sentinel.Error()) {
				t.Fatalf("diagnostic = %q, want %q", status.Diagnostic, sentinel)
			}
			test.check(t, status)
		})
	}
}

func assertArtifactRejected(t *testing.T, status runStatus) {
	t.Helper()
	if status.ArtifactOK || status.Reason != "artifact-error" || !status.ContainmentOK {
		t.Fatalf("artifact status = %+v", status)
	}
}

func runOperationFixture(t *testing.T, script string) (string, string, []string) {
	t.Helper()
	sourceRoot := tempPhysicalDir(t)
	artifactRoot := tempPhysicalDir(t)
	writeRunInputs(t, artifactRoot, []string{resolvedShell(t), "-c", script}, testRunEnvironment())
	return sourceRoot, artifactRoot, runFlags(sourceRoot, sourceRoot, artifactRoot)
}

func readRunStatus(t *testing.T, artifactRoot string) runStatus {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(artifactRoot, runStatusName))
	if err != nil {
		t.Fatalf("ReadFile status: %v", err)
	}
	var status runStatus
	if err := json.Unmarshal(data, &status); err != nil {
		t.Fatalf("Unmarshal status: %v\n%s", err, data)
	}
	return status
}

func TestRunStatusLateCollisionPreservesExistingBytes(t *testing.T) {
	_, artifactRoot, arguments := runOperationFixture(t, "exit 0")
	operations := defaultRunOperations()
	operations.writeStatus = func(path string, _ runStatus) error {
		if err := writeAtomicNew(path, []byte("competing owner\n"), 0o640); err != nil {
			return err
		}
		return writeRunStatus(path, runStatus{})
	}
	if code := runCommandOperations(arguments, operations); code == 0 {
		t.Fatal("late status collision unexpectedly passed")
	}
	path := filepath.Join(artifactRoot, runStatusName)
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "competing owner\n" {
		t.Fatalf("competing status = %q, %v", data, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("competing status mode = %v, %v", info, err)
	}
}
