package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

const physicalSourcePolicy = "physical-runtime-union-v1"

var datedEvidencePattern = regexp.MustCompile(`^eventloop/docs/tournament/[0-9]{4}-[0-9]{2}-[0-9]{2}/`)

var physicalSourceControls = []string{
	".gitignore",
	"Makefile",
	"go.mod",
	"go.sum",
	"go.work",
	"go.work.sum",
	"project.mk",
}

var physicalSourceTrees = []string{
	"eventloop",
	"goja-eventloop",
}

func liveSourceFiles(root string) ([]string, error) {
	return physicalSourceFiles(root, nil)
}

func physicalSourceFiles(root string, overrides []string) (_ []string, err error) {
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve physical source root: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("resolve physical source root links: %w", err)
	}
	physical, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open physical source root: %w", err)
	}
	defer func() {
		err = errors.Join(err, annotateError("close physical source root", physical.Close()))
	}()

	overrideSet := make(map[string]struct{}, len(overrides))
	for _, relative := range overrides {
		if err := validateSourceFilePath(relative); err != nil {
			return nil, fmt.Errorf("physical source override: %w", err)
		}
		if _, exists := overrideSet[relative]; exists {
			return nil, fmt.Errorf("duplicate physical source override %q", relative)
		}
		overrideSet[relative] = struct{}{}
	}

	files := make([]string, 0)
	for _, relative := range physicalSourceControls {
		info, statErr := physical.Lstat(relative)
		if statErr != nil {
			return nil, fmt.Errorf("inspect physical source control %q: %w", relative, statErr)
		}
		include, classifyErr := classifyPhysicalSource(relative, info, overrideSet)
		if classifyErr != nil {
			return nil, classifyErr
		}
		if include {
			files = append(files, relative)
		}
	}
	for _, tree := range physicalSourceTrees {
		info, statErr := physical.Lstat(tree)
		if statErr != nil {
			return nil, fmt.Errorf("inspect physical source tree %q: %w", tree, statErr)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("physical source tree %q is not a non-symlink directory", tree)
		}
		walkErr := fs.WalkDir(physical.FS(), tree, func(relative string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return fmt.Errorf("walk physical source %q: %w", relative, walkErr)
			}
			relative = filepath.ToSlash(relative)
			if entry.IsDir() {
				if relative != tree && datedEvidencePattern.MatchString(relative+"/") &&
					!physicalOverrideBelow(relative, overrideSet) {
					return fs.SkipDir
				}
				if secretPhysicalSource(relative) {
					return fmt.Errorf("physical source %q is a forbidden secret path", relative)
				}
				if relative != tree && excludedPhysicalDirectory(relative) {
					return fs.SkipDir
				}
				return nil
			}
			info, infoErr := entry.Info()
			if infoErr != nil {
				return fmt.Errorf("inspect physical source %q: %w", relative, infoErr)
			}
			include, classifyErr := classifyPhysicalSource(relative, info, overrideSet)
			if classifyErr != nil {
				return classifyErr
			}
			if include {
				files = append(files, relative)
			}
			return nil
		})
		if walkErr != nil {
			return nil, walkErr
		}
	}
	slices.Sort(files)
	files = slices.Compact(files)
	if len(files) == 0 {
		return nil, errors.New("physical source list is empty")
	}
	if err := validatePortableSourcePathSet(files); err != nil {
		return nil, err
	}
	sortedOverrides := slices.Clone(overrides)
	slices.Sort(sortedOverrides)
	if err := validatePhysicalSourceOverrides(physical, files, sortedOverrides); err != nil {
		return nil, err
	}
	if err := validatePhysicalSourceSymlinks(root, files); err != nil {
		return nil, err
	}
	return files, nil
}

func validatePortableSourcePathSet(paths []string) error {
	seen := make(map[string]string, len(paths))
	for _, relative := range paths {
		folded := strings.ToLower(relative)
		if prior, exists := seen[folded]; exists {
			return fmt.Errorf("physical source paths %q and %q collide case-insensitively", prior, relative)
		}
		seen[folded] = relative
	}
	return nil
}

func classifyPhysicalSource(relative string, info fs.FileInfo, overrides map[string]struct{}) (bool, error) {
	if err := validateSourceFilePath(relative); err != nil {
		return false, err
	}
	if secretPhysicalSource(relative) {
		return false, fmt.Errorf("physical source %q is a forbidden secret path", relative)
	}
	mode := info.Mode()
	if !mode.IsRegular() && mode&os.ModeSymlink == 0 {
		return false, fmt.Errorf("physical source %q has unsupported mode %s", relative, mode)
	}
	if _, override := overrides[relative]; override {
		return true, nil
	}
	return !excludedPhysicalSource(relative, mode), nil
}

func excludedPhysicalDirectory(relative string) bool {
	base := path.Base(relative)
	return base == ".git" || base == ".idea" || base == ".vscode" || base == "__pycache__" ||
		base == ".tournament"
}

func excludedPhysicalSource(relative string, mode fs.FileMode) bool {
	base := path.Base(relative)
	if datedEvidencePattern.MatchString(relative) || excludedPhysicalDirectory(relative) ||
		base == ".DS_Store" || base == "Thumbs.db" || base == "coverage.out" ||
		(strings.HasPrefix(base, "cover") && strings.HasSuffix(base, ".out")) {
		return true
	}
	return mode.IsRegular() && (strings.HasSuffix(base, ".test") || strings.HasSuffix(base, ".test.exe"))
}

func secretPhysicalSource(relative string) bool {
	for component := range strings.SplitSeq(relative, "/") {
		component = strings.ToLower(component)
		if component == ".env" || strings.HasPrefix(component, ".env.") || component == "id_rsa" ||
			strings.HasSuffix(component, ".pem") || strings.HasSuffix(component, ".key") {
			return true
		}
	}
	return false
}

func physicalOverrideBelow(directory string, overrides map[string]struct{}) bool {
	prefix := directory + "/"
	for relative := range overrides {
		if strings.HasPrefix(relative, prefix) {
			return true
		}
	}
	return false
}

func validatePhysicalSourceOverrides(root *os.Root, files, overrides []string) error {
	for _, relative := range overrides {
		if !slices.Contains(files, relative) {
			if _, err := root.Lstat(relative); err != nil {
				return fmt.Errorf("inspect physical source override %q: %w", relative, err)
			}
			return fmt.Errorf("physical source override %q was not included", relative)
		}
	}
	return nil
}

func validatePhysicalSourceSymlinks(root string, files []string) error {
	fileSet := make(map[string]struct{}, len(files))
	for _, relative := range files {
		fileSet[relative] = struct{}{}
	}
	for _, relative := range files {
		info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			return fmt.Errorf("inspect physical source %q: %w", relative, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		target, err := os.Readlink(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			return fmt.Errorf("read physical source symlink %q: %w", relative, err)
		}
		if err := validateSymlink(root, relative, target); err != nil {
			return err
		}
		resolved, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			return fmt.Errorf("resolve physical source symlink %q: %w", relative, err)
		}
		relativeTarget, err := filepath.Rel(root, resolved)
		if err != nil {
			return fmt.Errorf("resolve physical source symlink target %q: %w", relative, err)
		}
		relativeTarget = filepath.ToSlash(relativeTarget)
		if _, ok := fileSet[relativeTarget]; !ok {
			return fmt.Errorf("physical source symlink %q resolves to omitted source %q", relative, relativeTarget)
		}
	}
	return nil
}
