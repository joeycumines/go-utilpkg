package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
)

type stableDirectoryFile struct {
	Name string
	Data []byte
}

func readRegularStable(path string, permissions os.FileMode) (_ []byte, err error) {
	resolved, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve regular file path: %w", err)
	}
	directoryPath := filepath.Dir(resolved)
	name := filepath.Base(resolved)
	if name == "." || name == string(filepath.Separator) {
		return nil, errors.New("regular file path has no leaf")
	}
	directoryPathIdentity, err := os.Stat(directoryPath)
	if err != nil || !freezeComponentIdentity(directoryPathIdentity) || !directoryPathIdentity.IsDir() {
		return nil, fmt.Errorf("inspect regular file directory: %w", err)
	}
	directory, err := os.OpenRoot(directoryPath)
	if err != nil {
		return nil, fmt.Errorf("open regular file directory: %w", err)
	}
	directoryClosed := false
	defer func() {
		if !directoryClosed {
			err = errors.Join(err, annotateError("close regular file directory", directory.Close()))
		}
	}()
	directoryIdentity, err := componentRootIdentity(directory, ".")
	if err != nil || !os.SameFile(directoryPathIdentity, directoryIdentity) {
		return nil, errors.Join(errors.New("regular file directory changed while opening"), err)
	}

	pathIdentity, err := directory.Lstat(name)
	if err != nil || !freezeComponentIdentity(pathIdentity) || !pathIdentity.Mode().IsRegular() ||
		pathIdentity.Mode().Perm() != permissions.Perm() {
		return nil, fmt.Errorf("regular file %q has invalid identity, mode, or permissions: %w", name, err)
	}
	file, err := directory.Open(name)
	if err != nil {
		return nil, fmt.Errorf("open regular file %q: %w", name, err)
	}
	fileClosed := false
	defer func() {
		if !fileClosed {
			err = errors.Join(err, annotateError("close regular file", file.Close()))
		}
	}()
	handleIdentity, err := file.Stat()
	if err != nil || !freezeComponentIdentity(handleIdentity) || !os.SameFile(pathIdentity, handleIdentity) {
		return nil, errors.Join(fmt.Errorf("regular file %q changed while opening", name), err)
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read regular file %q: %w", name, err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("rewind regular file %q: %w", name, err)
	}
	confirmation, err := io.ReadAll(file)
	if err != nil || !bytes.Equal(data, confirmation) {
		return nil, errors.Join(fmt.Errorf("regular file %q changed between reads", name), err)
	}
	finalHandle, handleErr := file.Stat()
	finalPath, pathErr := directory.Lstat(name)
	if handleErr != nil || pathErr != nil || !freezeComponentIdentity(finalHandle) || !freezeComponentIdentity(finalPath) ||
		!os.SameFile(pathIdentity, finalHandle) || !os.SameFile(pathIdentity, finalPath) ||
		finalHandle.Size() != int64(len(data)) || finalPath.Mode().Perm() != permissions.Perm() {
		return nil, errors.Join(fmt.Errorf("regular file %q changed while reading", name), handleErr, pathErr)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close regular file %q: %w", name, err)
	}
	fileClosed = true
	finalDirectory, handleErr := componentRootIdentity(directory, ".")
	finalDirectoryPath, pathErr := os.Stat(directoryPath)
	if handleErr != nil || pathErr != nil || !freezeComponentIdentity(finalDirectoryPath) ||
		!os.SameFile(directoryIdentity, finalDirectory) || !os.SameFile(directoryIdentity, finalDirectoryPath) {
		return nil, errors.Join(errors.New("regular file directory changed while reading"), handleErr, pathErr)
	}
	if err := directory.Close(); err != nil {
		return nil, fmt.Errorf("close regular file directory: %w", err)
	}
	directoryClosed = true
	return data, nil
}

func readStableDirectoryFiles(path string, permissions os.FileMode) (_ []stableDirectoryFile, err error) {
	resolved, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve stable directory path: %w", err)
	}
	pathIdentity, err := os.Stat(resolved)
	if err != nil || !freezeComponentIdentity(pathIdentity) || !pathIdentity.IsDir() {
		return nil, fmt.Errorf("inspect stable directory: %w", err)
	}
	root, err := os.OpenRoot(resolved)
	if err != nil {
		return nil, fmt.Errorf("open stable directory: %w", err)
	}
	closed := false
	defer func() {
		if !closed {
			err = errors.Join(err, annotateError("close stable directory", root.Close()))
		}
	}()
	identity, err := componentRootIdentity(root, ".")
	if err != nil || !os.SameFile(pathIdentity, identity) {
		return nil, errors.Join(errors.New("stable directory changed while opening"), err)
	}
	names, err := listComponentDirectory(root, identity)
	if err != nil {
		return nil, fmt.Errorf("list stable directory: %w", err)
	}
	files := make([]stableDirectoryFile, 0, len(names))
	for _, name := range names {
		data, err := readRootRegularStable(root, name, permissions)
		if err != nil {
			return nil, err
		}
		files = append(files, stableDirectoryFile{Name: name, Data: data})
	}
	finalNames, err := listComponentDirectory(root, identity)
	if err != nil || !slices.Equal(names, finalNames) {
		return nil, errors.Join(errors.New("stable directory membership changed while reading"), err)
	}
	finalIdentity, handleErr := componentRootIdentity(root, ".")
	finalPath, pathErr := os.Stat(resolved)
	if handleErr != nil || pathErr != nil || !freezeComponentIdentity(finalPath) ||
		!os.SameFile(identity, finalIdentity) || !os.SameFile(identity, finalPath) {
		return nil, errors.Join(errors.New("stable directory changed while reading"), handleErr, pathErr)
	}
	if err := root.Close(); err != nil {
		return nil, fmt.Errorf("close stable directory: %w", err)
	}
	closed = true
	return files, nil
}

func readRootRegularStable(root *os.Root, name string, permissions os.FileMode) (_ []byte, err error) {
	identity, err := root.Lstat(name)
	if err != nil || !freezeComponentIdentity(identity) || !identity.Mode().IsRegular() ||
		identity.Mode().Perm() != permissions.Perm() {
		return nil, fmt.Errorf("stable directory entry %q has invalid identity, mode, or permissions: %w", name, err)
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, fmt.Errorf("open stable directory entry %q: %w", name, err)
	}
	defer func() {
		err = errors.Join(err, annotateError("close stable directory entry", file.Close()))
	}()
	handleIdentity, err := file.Stat()
	if err != nil || !freezeComponentIdentity(handleIdentity) || !os.SameFile(identity, handleIdentity) {
		return nil, errors.Join(fmt.Errorf("stable directory entry %q changed while opening", name), err)
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read stable directory entry %q: %w", name, err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("rewind stable directory entry %q: %w", name, err)
	}
	confirmation, err := io.ReadAll(file)
	if err != nil || !bytes.Equal(data, confirmation) {
		return nil, errors.Join(fmt.Errorf("stable directory entry %q changed between reads", name), err)
	}
	finalHandle, handleErr := file.Stat()
	finalPath, pathErr := root.Lstat(name)
	if handleErr != nil || pathErr != nil || !freezeComponentIdentity(finalHandle) || !freezeComponentIdentity(finalPath) ||
		!os.SameFile(identity, finalHandle) || !os.SameFile(identity, finalPath) ||
		finalHandle.Size() != int64(len(data)) || finalPath.Mode().Perm() != permissions.Perm() {
		return nil, errors.Join(fmt.Errorf("stable directory entry %q changed while reading", name), handleErr, pathErr)
	}
	return data, nil
}
