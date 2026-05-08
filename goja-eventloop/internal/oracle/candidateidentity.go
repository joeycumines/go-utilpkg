package oracle

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/mod/sumdb/dirhash"
	modzip "golang.org/x/mod/zip"
)

const eventloopCandidateFormat = "go-eventloop-candidate-module-xmodzip-v0.35.0-dirhash1-relative-v2"

type candidateModuleFile struct {
	ctx      context.Context
	info     os.FileInfo
	root     string
	relative string
}

func (f candidateModuleFile) Path() string {
	return f.relative
}

func (f candidateModuleFile) Lstat() (os.FileInfo, error) {
	return f.info, nil
}

func (f candidateModuleFile) Open() (io.ReadCloser, error) {
	file, err := os.Open(filepath.Join(f.root, filepath.FromSlash(f.relative)))
	if err != nil {
		return nil, err
	}
	return candidateContextReadCloser{ctx: f.ctx, ReadCloser: file}, nil
}

type candidateContextReadCloser struct {
	ctx context.Context
	io.ReadCloser
}

func (r candidateContextReadCloser) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	count, err := r.ReadCloser.Read(buffer)
	if err == nil {
		err = r.ctx.Err()
	}
	return count, err
}

// candidateModuleSHA256 hashes the publishable module members rooted at root.
// A Git worktree uses candidateproxy's Git-visible source contract. An
// unpacked source archive uses x/mod/zip's directory contract directly.
func candidateModuleSHA256(ctx context.Context, root string) (string, int, error) {
	if err := ctx.Err(); err != nil {
		return "", 0, err
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", 0, fmt.Errorf("resolve candidate root: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", 0, fmt.Errorf("resolve candidate root links: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", 0, err
	}
	if !info.IsDir() {
		return "", 0, fmt.Errorf("%s is not a directory", root)
	}

	files, worktree, err := candidateWorktreeFiles(ctx, root)
	if err != nil {
		return "", 0, err
	}
	var relatives []string
	if worktree {
		checked, checkErr := modzip.CheckFiles(files)
		if checkErr != nil {
			return "", 0, fmt.Errorf("check Git-visible module files: %w", checkErr)
		}
		relatives = checked.Valid
	} else {
		checked, checkErr := modzip.CheckDir(root)
		if checkErr != nil {
			return "", 0, fmt.Errorf("check module directory: %w", checkErr)
		}
		if err := ctx.Err(); err != nil {
			return "", 0, err
		}
		relatives = make([]string, 0, len(checked.Valid))
		for _, filename := range checked.Valid {
			relative, relativeErr := filepath.Rel(root, filename)
			if relativeErr != nil {
				return "", 0, fmt.Errorf("resolve candidate member %q: %w", filename, relativeErr)
			}
			relative = filepath.ToSlash(relative)
			if err := validateCandidateRelative(relative); err != nil {
				return "", 0, fmt.Errorf("candidate member %q: %w", filename, err)
			}
			relatives = append(relatives, relative)
		}
	}

	value, err := dirhash.Hash1(relatives, func(relative string) (io.ReadCloser, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := validateCandidateRelative(relative); err != nil {
			return nil, err
		}
		file, err := os.Open(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			return nil, err
		}
		return candidateContextReadCloser{ctx: ctx, ReadCloser: file}, nil
	})
	if err != nil {
		return "", 0, fmt.Errorf("hash candidate module files: %w", err)
	}
	encoded, ok := strings.CutPrefix(value, "h1:")
	if !ok {
		return "", 0, fmt.Errorf("unexpected candidate directory hash %q", value)
	}
	digest, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", 0, fmt.Errorf("decode candidate directory hash: %w", err)
	}
	if len(digest) != sha256.Size {
		return "", 0, fmt.Errorf("candidate directory hash has %d bytes, want %d", len(digest), sha256.Size)
	}
	return hex.EncodeToString(digest), len(relatives), nil
}

func candidateWorktreeFiles(ctx context.Context, root string) ([]modzip.File, bool, error) {
	marker, err := candidateGitMarker(root)
	if err != nil {
		return nil, false, fmt.Errorf("inspect candidate Git metadata: %w", err)
	}
	if !marker {
		return nil, false, nil
	}

	output, err := candidateGitOutput(ctx, root, "ls-files", "--cached", "--others", "--exclude-standard", "-z", "--", ".")
	if err != nil {
		return nil, false, fmt.Errorf("list Git-visible candidate files: %w", err)
	}
	files := make([]modzip.File, 0, bytes.Count(output, []byte{0}))
	rootGoMod := false
	for value := range bytes.SplitSeq(output, []byte{0}) {
		if len(value) == 0 {
			continue
		}
		relative := filepath.ToSlash(string(value))
		if err := validateCandidateRelative(relative); err != nil {
			return nil, false, fmt.Errorf("git candidate path %q: %w", value, err)
		}
		if candidateVCSParent(relative) {
			continue
		}
		filename := filepath.Join(root, filepath.FromSlash(relative))
		info, err := os.Lstat(filename)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, false, fmt.Errorf("inspect Git candidate path %q: %w", relative, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if !info.Mode().IsRegular() {
			return nil, false, fmt.Errorf("git-visible candidate path is not a regular file or symlink: %q", relative)
		}
		if relative == "go.mod" {
			rootGoMod = true
		}
		files = append(files, candidateModuleFile{ctx: ctx, info: info, root: root, relative: relative})
	}
	if !rootGoMod {
		return nil, false, errors.New("git-visible candidate does not contain a regular go.mod")
	}
	return files, true, nil
}

func candidateGitMarker(root string) (bool, error) {
	for directory := root; ; directory = filepath.Dir(directory) {
		_, err := os.Lstat(filepath.Join(directory, ".git"))
		switch {
		case err == nil:
			return true, nil
		case !errors.Is(err, os.ErrNotExist):
			return false, err
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return false, nil
		}
	}
}

func candidateGitOutput(ctx context.Context, root string, args ...string) ([]byte, error) {
	commandArgs := []string{"-C", root, "--literal-pathspecs"}
	commandArgs = append(commandArgs, args...)
	command := exec.CommandContext(ctx, "git", commandArgs...)
	output, err := command.Output()
	if err == nil {
		return output, nil
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if stderr := strings.TrimSpace(string(exitErr.Stderr)); stderr != "" {
			return nil, fmt.Errorf("%w: %s", err, stderr)
		}
	}
	return nil, err
}

func candidateVCSParent(relative string) bool {
	elements := strings.Split(relative, "/")
	for _, element := range elements[:len(elements)-1] {
		switch element {
		case ".bzr", ".git", ".hg", ".svn":
			return true
		}
	}
	return false
}

func validateCandidateRelative(relative string) error {
	if relative == "" || relative == "." {
		return errors.New("path is empty")
	}
	if relative != path.Clean(relative) {
		return errors.New("path is not clean")
	}
	if path.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, "../") {
		return errors.New("path is not relative to the module root")
	}
	return nil
}
