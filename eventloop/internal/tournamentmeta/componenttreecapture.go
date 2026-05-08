package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"runtime"
	"slices"
)

type componentTreePass struct {
	records    []componentTreeRecord
	identities map[string]os.FileInfo
}

func captureComponentTree(rootPath string) (_ componentTree, err error) {
	if runtime.GOOS == "js" || runtime.GOOS == "plan9" {
		return componentTree{}, fmt.Errorf("component tree capture is unsupported on %s", runtime.GOOS)
	}
	pathIdentity, err := os.Stat(rootPath)
	if err != nil {
		return componentTree{}, fmt.Errorf("inspect component root path: %w", err)
	}
	if !pathIdentity.IsDir() || !freezeComponentIdentity(pathIdentity) {
		return componentTree{}, errors.New("component root path is not an identity-stable directory")
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return componentTree{}, fmt.Errorf("open component root: %w", err)
	}
	closed := false
	defer func() {
		if !closed {
			err = errors.Join(err, annotateError("close component root", root.Close()))
		}
	}()
	rootIdentity, err := componentRootIdentity(root, ".")
	if err != nil {
		return componentTree{}, err
	}
	if !os.SameFile(pathIdentity, rootIdentity) {
		return componentTree{}, errors.New("component root path and handle identities differ")
	}

	first, err := captureComponentTreePass(root, rootIdentity)
	if err != nil {
		return componentTree{}, fmt.Errorf("capture component tree first pass: %w", err)
	}
	second, err := captureComponentTreePass(root, rootIdentity)
	if err != nil {
		return componentTree{}, fmt.Errorf("capture component tree second pass: %w", err)
	}
	if !slices.Equal(first.records, second.records) || !sameComponentIdentities(first.identities, second.identities) {
		return componentTree{}, errors.New("component tree changed between complete passes")
	}
	finalRoot, err := componentRootIdentity(root, ".")
	if err != nil {
		return componentTree{}, err
	}
	finalPath, err := os.Stat(rootPath)
	if err != nil || !freezeComponentIdentity(finalPath) ||
		!os.SameFile(rootIdentity, finalRoot) || !os.SameFile(rootIdentity, finalPath) {
		return componentTree{}, errors.New("component root changed while capturing")
	}
	if err := root.Close(); err != nil {
		return componentTree{}, fmt.Errorf("close component root: %w", err)
	}
	closed = true
	return newComponentTree(first.records)
}

func captureComponentTreePass(root *os.Root, rootIdentity os.FileInfo) (componentTreePass, error) {
	result := componentTreePass{
		records:    []componentTreeRecord{{Path: ".", Mode: "040000"}},
		identities: map[string]os.FileInfo{".": rootIdentity},
	}
	if err := captureComponentDirectory(root, ".", rootIdentity, []os.FileInfo{rootIdentity}, &result); err != nil {
		return componentTreePass{}, err
	}
	slices.SortFunc(result.records, func(left, right componentTreeRecord) int {
		if left.Path < right.Path {
			return -1
		}
		if left.Path > right.Path {
			return 1
		}
		return 0
	})
	return result, nil
}

func captureComponentDirectory(
	root *os.Root,
	logical string,
	directoryIdentity os.FileInfo,
	ancestors []os.FileInfo,
	result *componentTreePass,
) error {
	names, err := listComponentDirectory(root, directoryIdentity)
	if err != nil {
		return fmt.Errorf("list component directory %q: %w", logical, err)
	}
	for _, name := range names {
		childLogical := name
		if logical != "." {
			childLogical = path.Join(logical, name)
		}
		identity, err := root.Lstat(name)
		if err != nil || !freezeComponentIdentity(identity) {
			return fmt.Errorf("inspect component path %q: %w", childLogical, err)
		}
		switch {
		case identity.IsDir():
			child, err := root.OpenRoot(name)
			if err != nil {
				return fmt.Errorf("open component directory %q: %w", childLogical, err)
			}
			childIdentity, identityErr := componentRootIdentity(child, ".")
			parentIdentity, parentErr := root.Lstat(name)
			if parentErr == nil && !freezeComponentIdentity(parentIdentity) {
				parentErr = errors.New("directory identity is unstable")
			}
			if identityErr != nil || parentErr != nil || !os.SameFile(identity, childIdentity) ||
				!os.SameFile(identity, parentIdentity) {
				return errors.Join(
					fmt.Errorf("component directory %q changed while opening", childLogical),
					identityErr,
					parentErr,
					annotateError("close changed component directory", child.Close()),
				)
			}
			for _, ancestor := range ancestors {
				if os.SameFile(ancestor, childIdentity) {
					return errors.Join(
						fmt.Errorf("component directory %q forms an identity cycle", childLogical),
						annotateError("close cyclic component directory", child.Close()),
					)
				}
			}
			result.records = append(result.records, componentTreeRecord{Path: childLogical, Mode: "040000"})
			result.identities[childLogical] = childIdentity
			if err := captureComponentDirectory(
				child,
				childLogical,
				childIdentity,
				append(slices.Clone(ancestors), childIdentity),
				result,
			); err != nil {
				return errors.Join(err, annotateError("close component directory", child.Close()))
			}
			finalChild, childErr := componentRootIdentity(child, ".")
			finalParent, parentErr := root.Lstat(name)
			if parentErr == nil && !freezeComponentIdentity(finalParent) {
				parentErr = errors.New("directory identity is unstable")
			}
			if childErr != nil || parentErr != nil || !os.SameFile(childIdentity, finalChild) ||
				!os.SameFile(childIdentity, finalParent) {
				return errors.Join(
					fmt.Errorf("component directory %q changed while traversing", childLogical),
					childErr,
					parentErr,
					annotateError("close changed component directory", child.Close()),
				)
			}
			if err := child.Close(); err != nil {
				return fmt.Errorf("close component directory %q: %w", childLogical, err)
			}
			closedIdentity, err := root.Lstat(name)
			if err != nil || !freezeComponentIdentity(closedIdentity) || !os.SameFile(childIdentity, closedIdentity) {
				return fmt.Errorf("component directory %q changed after close: %w", childLogical, err)
			}
		case identity.Mode().IsRegular():
			record, finalIdentity, err := captureComponentFile(root, name, childLogical, identity)
			if err != nil {
				return err
			}
			result.records = append(result.records, record)
			result.identities[childLogical] = finalIdentity
		case identity.Mode()&os.ModeSymlink != 0:
			record, finalIdentity, err := captureComponentSymlink(root, name, childLogical, identity)
			if err != nil {
				return err
			}
			result.records = append(result.records, record)
			result.identities[childLogical] = finalIdentity
		default:
			return fmt.Errorf("component path %q has unsupported mode %s", childLogical, identity.Mode())
		}
	}
	after, err := listComponentDirectory(root, directoryIdentity)
	if err != nil {
		return fmt.Errorf("relist component directory %q: %w", logical, err)
	}
	if !slices.Equal(names, after) {
		return fmt.Errorf("component directory %q membership changed", logical)
	}
	return nil
}

func listComponentDirectory(root *os.Root, identity os.FileInfo) (_ []string, err error) {
	directory, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, annotateError("close component directory listing", directory.Close())) }()
	handleIdentity, err := directory.Stat()
	if err != nil || !freezeComponentIdentity(handleIdentity) || !os.SameFile(identity, handleIdentity) {
		return nil, errors.Join(errors.New("component directory listing identity changed"), err)
	}
	entries, err := directory.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	if extra, err := directory.ReadDir(1); err != io.EOF || len(extra) != 0 {
		return nil, errors.New("component directory listing lacks an exact EOF")
	}
	names := make([]string, len(entries))
	for index, entry := range entries {
		names[index] = entry.Name()
	}
	slices.Sort(names)
	if len(slices.Compact(slices.Clone(names))) != len(names) {
		return nil, errors.New("component directory repeats a child name")
	}
	after, err := directory.Stat()
	if err != nil || !freezeComponentIdentity(after) || !os.SameFile(identity, after) {
		return nil, errors.Join(errors.New("component directory listing changed"), err)
	}
	return names, nil
}

func captureComponentFile(
	root *os.Root,
	name,
	logical string,
	identity os.FileInfo,
) (_ componentTreeRecord, _ os.FileInfo, err error) {
	file, err := root.Open(name)
	if err != nil {
		return componentTreeRecord{}, nil, fmt.Errorf("open component file %q: %w", logical, err)
	}
	defer func() { err = errors.Join(err, annotateError("close component file", file.Close())) }()
	handleIdentity, err := file.Stat()
	pathIdentity, pathErr := root.Lstat(name)
	if pathErr == nil && !freezeComponentIdentity(pathIdentity) {
		pathErr = errors.New("file identity is unstable")
	}
	if err != nil || !freezeComponentIdentity(handleIdentity) || pathErr != nil ||
		!os.SameFile(identity, handleIdentity) || !os.SameFile(identity, pathIdentity) {
		return componentTreeRecord{}, nil, errors.Join(fmt.Errorf("component file %q changed while opening", logical), err, pathErr)
	}
	if handleIdentity.Size() < 0 {
		return componentTreeRecord{}, nil, fmt.Errorf("component file %q has negative size", logical)
	}
	first, err := hashExactComponentFile(file, handleIdentity.Size())
	if err != nil {
		return componentTreeRecord{}, nil, fmt.Errorf("hash component file %q: %w", logical, err)
	}
	second, err := hashExactComponentFile(file, handleIdentity.Size())
	if err != nil {
		return componentTreeRecord{}, nil, fmt.Errorf("rehash component file %q: %w", logical, err)
	}
	finalHandle, err := file.Stat()
	finalPath, pathErr := root.Lstat(name)
	if pathErr == nil && !freezeComponentIdentity(finalPath) {
		pathErr = errors.New("file identity is unstable")
	}
	if err != nil || !freezeComponentIdentity(finalHandle) || pathErr != nil || first != second ||
		!os.SameFile(handleIdentity, finalHandle) || !os.SameFile(handleIdentity, finalPath) {
		return componentTreeRecord{}, nil, errors.Join(fmt.Errorf("component file %q changed while hashing", logical), err, pathErr)
	}
	mode := "100644"
	if handleIdentity.Mode().Perm()&0o111 != 0 {
		mode = "100755"
	}
	return componentTreeRecord{
		Path:   logical,
		Mode:   mode,
		Size:   uint64(handleIdentity.Size()),
		SHA256: first,
	}, finalHandle, nil
}

func captureComponentSymlink(
	root *os.Root,
	name,
	logical string,
	identity os.FileInfo,
) (componentTreeRecord, os.FileInfo, error) {
	first, err := root.Readlink(name)
	if err != nil {
		return componentTreeRecord{}, nil, fmt.Errorf("read component symlink %q: %w", logical, err)
	}
	middle, err := root.Lstat(name)
	if err != nil || !freezeComponentIdentity(middle) || !os.SameFile(identity, middle) {
		return componentTreeRecord{}, nil, fmt.Errorf("component symlink %q changed after first read: %w", logical, err)
	}
	second, err := root.Readlink(name)
	if err != nil {
		return componentTreeRecord{}, nil, fmt.Errorf("reread component symlink %q: %w", logical, err)
	}
	final, err := root.Lstat(name)
	if err != nil || !freezeComponentIdentity(final) || !os.SameFile(identity, final) || first != second {
		return componentTreeRecord{}, nil, fmt.Errorf("component symlink %q changed while reading: %w", logical, err)
	}
	if err := validateComponentSymlink(logical, first); err != nil {
		return componentTreeRecord{}, nil, err
	}
	digest := sha256.Sum256([]byte(first))
	return componentTreeRecord{
		Path:          logical,
		Mode:          "120000",
		Size:          uint64(len(first)),
		SHA256:        hex.EncodeToString(digest[:]),
		SymlinkTarget: first,
	}, final, nil
}

func hashExactComponentFile(file *os.File, size int64) (string, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	digest := sha256.New()
	if _, err := io.CopyN(digest, file, size); err != nil {
		return "", err
	}
	var extra [1]byte
	if count, err := file.Read(extra[:]); count != 0 || err != io.EOF {
		return "", fmt.Errorf("component file EOF = %d bytes, %v", count, err)
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func componentRootIdentity(root *os.Root, name string) (os.FileInfo, error) {
	identity, err := root.Stat(name)
	if err != nil {
		return nil, err
	}
	if !identity.IsDir() || !freezeComponentIdentity(identity) {
		return nil, errors.New("component root handle is not an identity-stable directory")
	}
	return identity, nil
}

func freezeComponentIdentity(identity os.FileInfo) bool {
	return identity != nil && os.SameFile(identity, identity)
}

func sameComponentIdentities(left, right map[string]os.FileInfo) bool {
	if len(left) != len(right) {
		return false
	}
	for logical, first := range left {
		second, exists := right[logical]
		if !exists || !os.SameFile(first, second) {
			return false
		}
	}
	return true
}
