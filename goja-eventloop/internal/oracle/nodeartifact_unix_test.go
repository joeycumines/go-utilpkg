//go:build darwin || linux

package oracle

import (
	"archive/tar"
	"errors"
	"os"
	"testing"
)

type nodeCloseErrorSource struct {
	nodeArchiveSource
	err error
}

func (s *nodeCloseErrorSource) Close() error {
	return errors.Join(s.nodeArchiveSource.Close(), s.err)
}

func TestPrepareNodeArtifactSourceCloseFailureCleans(t *testing.T) {
	archive, path := nodeTestArchiveFile(t, []nodeTestArchiveEntry{{
		name: "node", typeflag: tar.TypeReg, data: []byte("authenticated node"),
	}})
	closeFailure := errors.New("injected source close failure")
	ops := defaultNodeArtifactOps()
	openSource := ops.openSource
	ops.openSource = func(path string) (nodeArchiveSource, error) {
		file, err := openSource(path)
		if err != nil {
			return nil, err
		}
		return &nodeCloseErrorSource{nodeArchiveSource: file, err: closeFailure}, nil
	}
	var removedRoot string
	ops.removeRoot = func(root string) error {
		removedRoot = root
		return os.RemoveAll(root)
	}
	ops.procFSAvailable = func() bool { return false }

	artifact, err := prepareNodeArtifactPinOps(path, nodeTestPin(archive, "node"), ops)
	if artifact != nil {
		_ = artifact.Close()
		t.Fatal("source close failure returned a nonnil artifact")
	}
	if !errors.Is(err, closeFailure) || !errors.Is(err, errNodeArchiveSource) {
		t.Fatalf("source close error = %v", err)
	}
	if removedRoot == "" {
		t.Fatal("source close failure did not clean the prepared artifact")
	}
	if _, statErr := os.Stat(removedRoot); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("prepared artifact root remains after source close failure: %v", statErr)
	}
}

func TestNodeArtifactCloseRetriesRemoval(t *testing.T) {
	archive, path := nodeTestArchiveFile(t, []nodeTestArchiveEntry{{
		name: "node", typeflag: tar.TypeReg, data: []byte("authenticated node"),
	}})
	removeFailure := errors.New("injected removal failure")
	removeCalls := 0
	ops := defaultNodeArtifactOps()
	ops.removeRoot = func(root string) error {
		removeCalls++
		if removeCalls == 1 {
			return removeFailure
		}
		return os.RemoveAll(root)
	}
	ops.procFSAvailable = func() bool { return false }

	artifact, err := prepareNodeArtifactPinOps(path, nodeTestPin(archive, "node"), ops)
	if err != nil {
		t.Fatal(err)
	}
	root := artifact.root
	t.Cleanup(func() {
		artifact.removeRoot = os.RemoveAll
		_ = artifact.Close()
	})
	if artifact.launchMode != nodeLaunchPrivatePath {
		t.Fatalf("fallback launch mode = %q, want %q", artifact.launchMode, nodeLaunchPrivatePath)
	}

	if err := artifact.Close(); !errors.Is(err, removeFailure) {
		t.Fatalf("first Close error = %v", err)
	}
	if artifact.root != root {
		t.Fatalf("failed Close cleared root = %q, want %q", artifact.root, root)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("failed Close removed root: %v", err)
	}

	if err := artifact.Close(); err != nil {
		t.Fatalf("retry Close: %v", err)
	}
	if artifact.root != "" {
		t.Fatalf("successful retry retained root %q", artifact.root)
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("successful retry retained artifact root: %v", err)
	}
	if removeCalls != 2 {
		t.Fatalf("remove calls = %d, want 2", removeCalls)
	}
	if err := artifact.Close(); err != nil {
		t.Fatalf("idempotent Close: %v", err)
	}
	if removeCalls != 2 {
		t.Fatalf("idempotent Close remove calls = %d, want 2", removeCalls)
	}
}
