package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	runClaimName  = "runner.claim"
	runStdoutName = "stdout.raw"
	runStderrName = "stderr.raw"
	runStatusName = "status.json"
)

type runOutputArtifact struct { // betteralign:ignore canonical JSON field order
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type runArtifacts struct {
	root       *os.Root
	rootPath   string
	rootInfo   os.FileInfo
	stdin      *os.File
	stdout     *os.File
	stderr     *os.File
	statusPath string
}

type runOperations struct {
	closeScope  func(*ownedProcess) error
	syncFile    func(*os.File) error
	closeFile   func(*os.File) error
	hashOutput  func(*os.Root, string) (runOutputArtifact, error)
	closeRoot   func(*os.Root) error
	writeStatus func(string, runStatus) error
}

func defaultRunOperations() runOperations {
	return runOperations{
		closeScope:  func(scope *ownedProcess) error { return scope.close() },
		syncFile:    func(file *os.File) error { return file.Sync() },
		closeFile:   func(file *os.File) error { return file.Close() },
		hashOutput:  hashRunOutput,
		closeRoot:   func(root *os.Root) error { return root.Close() },
		writeStatus: writeRunStatus,
	}
}

func acquireRunArtifacts(rootPath string) (_ *runArtifacts, _ runInputs, err error) {
	rootInfo, err := os.Stat(rootPath)
	if err != nil {
		return nil, runInputs{}, fmt.Errorf("inspect run artifact root: %w", err)
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, runInputs{}, fmt.Errorf("open run artifact root: %w", err)
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, root.Close())
		}
	}()
	inputs, err := readRunInputs(root)
	if err != nil {
		return nil, runInputs{}, err
	}
	for _, name := range []string{runClaimName, runStdoutName, runStderrName, runStatusName} {
		if _, statErr := root.Lstat(name); !os.IsNotExist(statErr) {
			if statErr == nil {
				return nil, runInputs{}, fmt.Errorf("run artifact already exists: %s", name)
			}
			return nil, runInputs{}, fmt.Errorf("inspect run artifact %q: %w", name, statErr)
		}
	}
	claim, err := root.OpenFile(runClaimName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, runInputs{}, fmt.Errorf("claim run artifact root: %w", err)
	}
	if _, writeErr := io.WriteString(claim, "go-utilpkg-eventloop-tournament-run-v2\n"); writeErr != nil {
		err = errors.Join(fmt.Errorf("write run claim: %w", writeErr), claim.Close())
		return nil, runInputs{}, err
	}
	if err = errors.Join(claim.Sync(), claim.Close()); err != nil {
		return nil, runInputs{}, fmt.Errorf("seal run claim: %w", err)
	}
	stdout, err := root.OpenFile(runStdoutName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, runInputs{}, fmt.Errorf("create run stdout: %w", err)
	}
	stderr, err := root.OpenFile(runStderrName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		_ = stdout.Close()
		return nil, runInputs{}, fmt.Errorf("create run stderr: %w", err)
	}
	stdin, err := os.Open(os.DevNull)
	if err != nil {
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, runInputs{}, fmt.Errorf("open null run stdin: %w", err)
	}
	return &runArtifacts{
		root:       root,
		rootPath:   rootPath,
		rootInfo:   rootInfo,
		stdin:      stdin,
		stdout:     stdout,
		stderr:     stderr,
		statusPath: filepath.Join(rootPath, runStatusName),
	}, inputs, nil
}

func (artifacts *runArtifacts) finalize(operations runOperations) (runOutputArtifact, runOutputArtifact, error) {
	var errs []error
	if err := operations.syncFile(artifacts.stdout); err != nil {
		errs = append(errs, fmt.Errorf("sync run stdout: %w", err))
	}
	if err := operations.syncFile(artifacts.stderr); err != nil {
		errs = append(errs, fmt.Errorf("sync run stderr: %w", err))
	}
	if err := operations.closeFile(artifacts.stdout); err != nil {
		errs = append(errs, fmt.Errorf("close run stdout: %w", err))
	}
	if err := operations.closeFile(artifacts.stderr); err != nil {
		errs = append(errs, fmt.Errorf("close run stderr: %w", err))
	}
	if err := operations.closeFile(artifacts.stdin); err != nil {
		errs = append(errs, fmt.Errorf("close null run stdin: %w", err))
	}
	stdout, err := operations.hashOutput(artifacts.root, runStdoutName)
	if err != nil {
		errs = append(errs, err)
	}
	stderr, err := operations.hashOutput(artifacts.root, runStderrName)
	if err != nil {
		errs = append(errs, err)
	}
	current, err := os.Stat(artifacts.rootPath)
	if err != nil {
		errs = append(errs, fmt.Errorf("reinspect run artifact root: %w", err))
	} else if !os.SameFile(artifacts.rootInfo, current) {
		errs = append(errs, errors.New("run artifact root identity changed"))
	}
	if err := operations.closeRoot(artifacts.root); err != nil {
		errs = append(errs, fmt.Errorf("close run artifact root: %w", err))
	}
	return stdout, stderr, errors.Join(errs...)
}

func hashRunOutput(root *os.Root, name string) (runOutputArtifact, error) {
	data, _, err := readStableRunFile(root, name)
	if err != nil {
		return runOutputArtifact{Name: name}, fmt.Errorf("hash run output %q: %w", name, err)
	}
	return runOutputArtifact{
		Name:   name,
		Size:   int64(len(data)),
		SHA256: fmt.Sprintf("%x", sha256.Sum256(data)),
	}, nil
}

func readStableRunFile(root *os.Root, name string) ([]byte, os.FileInfo, error) {
	before, err := root.Lstat(name)
	if err != nil {
		return nil, nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("%q is not a physical regular file", name)
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, nil, err
	}
	data, readErr := io.ReadAll(file)
	after, statErr := file.Stat()
	closeErr := file.Close()
	if err := errors.Join(readErr, statErr, closeErr); err != nil {
		return nil, nil, err
	}
	if !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return nil, nil, fmt.Errorf("%q changed while reading", name)
	}
	return data, after, nil
}

func writeRunStatus(path string, status runStatus) error {
	data, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("encode run status: %w", err)
	}
	data = append(data, '\n')
	return writeAtomicNew(path, data, 0o600)
}
