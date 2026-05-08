package main

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	runArgvName        = "argv.nul"
	runEnvironmentName = "environment.nul"
)

type runInputArtifact struct { // betteralign:ignore canonical JSON field order
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
	RecordCount int    `json:"record_count"`
}

type runInputs struct {
	argv                []string
	environment         []string
	argvArtifact        runInputArtifact
	environmentArtifact runInputArtifact
}

func readRunInputs(root *os.Root) (runInputs, error) {
	argvData, argv, err := readNULInput(root, runArgvName)
	if err != nil {
		return runInputs{}, fmt.Errorf("read run argv: %w", err)
	}
	if len(argv) == 0 || argv[0] == "" {
		return runInputs{}, errors.New("run argv must contain a nonempty executable")
	}
	if _, err := validateRunExecutable(argv[0]); err != nil {
		return runInputs{}, err
	}

	environmentData, environment, err := readNULInput(root, runEnvironmentName)
	if err != nil {
		return runInputs{}, fmt.Errorf("read run environment: %w", err)
	}
	if err := validateRunEnvironment(environment); err != nil {
		return runInputs{}, err
	}
	return runInputs{
		argv:        argv,
		environment: environment,
		argvArtifact: runInputArtifact{
			Name:        runArgvName,
			Size:        int64(len(argvData)),
			SHA256:      fmt.Sprintf("%x", sha256.Sum256(argvData)),
			RecordCount: len(argv),
		},
		environmentArtifact: runInputArtifact{
			Name:        runEnvironmentName,
			Size:        int64(len(environmentData)),
			SHA256:      fmt.Sprintf("%x", sha256.Sum256(environmentData)),
			RecordCount: len(environment),
		},
	}, nil
}

func readNULInput(root *os.Root, name string) ([]byte, []string, error) {
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
	if readErr == nil {
		_, readErr = file.Seek(0, io.SeekStart)
	}
	var repeated []byte
	if readErr == nil {
		repeated, readErr = io.ReadAll(file)
	}
	after, statErr := file.Stat()
	closeErr := file.Close()
	if err := errors.Join(readErr, statErr, closeErr); err != nil {
		return nil, nil, fmt.Errorf("read stable input %q: %w", name, err)
	}
	if !os.SameFile(before, after) || before.Size() != after.Size() ||
		!before.ModTime().Equal(after.ModTime()) || !bytes.Equal(data, repeated) {
		return nil, nil, fmt.Errorf("run input %q changed while reading", name)
	}
	if len(data) == 0 || data[len(data)-1] != 0 {
		return nil, nil, fmt.Errorf("run input %q must be nonempty and end with NUL", name)
	}
	rawRecords := bytes.Split(data[:len(data)-1], []byte{0})
	records := make([]string, len(rawRecords))
	for index, record := range rawRecords {
		records[index] = string(record)
	}
	return data, records, nil
}

func validateRunExecutable(path string) (string, error) {
	if path == "" || !isAbsoluteCleanPath(path) {
		return "", fmt.Errorf("run executable must be an absolute clean path: %q", path)
	}
	resolved, err := evalPhysicalPath(path)
	if err != nil {
		return "", fmt.Errorf("resolve run executable: %w", err)
	}
	if !sameRunPath(path, resolved) {
		return "", fmt.Errorf("run executable must be its resolved path: %q != %q", path, resolved)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("inspect run executable: %w", err)
	}
	if !validRunExecutable(info) {
		return "", fmt.Errorf("run executable %q is not an executable regular file", path)
	}
	return resolved, nil
}

func validateRunEnvironment(environment []string) error {
	if len(environment) == 0 {
		return errors.New("run environment must not be empty")
	}
	keys := make([]string, 0, len(environment))
	for _, record := range environment {
		key, _, found := strings.Cut(record, "=")
		if !found || key == "" || strings.ContainsAny(key, "\x00\r\n=") {
			return fmt.Errorf("invalid run environment record %q", record)
		}
		for _, previous := range keys {
			equal, err := sameRunEnvironmentKey(previous, key)
			if err != nil {
				return fmt.Errorf("compare run environment keys %q and %q: %w", previous, key, err)
			}
			if equal {
				return fmt.Errorf("duplicate run environment keys %q and %q", previous, key)
			}
		}
		keys = append(keys, key)
	}
	return validateRunEnvironmentPlatform(environment)
}

func isAbsoluteCleanPath(path string) bool {
	return filepath.IsAbs(path) && filepath.Clean(path) == path
}

func evalPhysicalPath(path string) (string, error) {
	return filepath.EvalSymlinks(path)
}
