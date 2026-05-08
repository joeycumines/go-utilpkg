package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

const sourcePathSetDomain = "go-utilpkg-eventloop-tournament-source-path-set-v1"

type sourcePathSet struct { // betteralign:ignore canonical JSON field order
	Count  int      `json:"count"`
	SHA256 string   `json:"sha256"`
	Paths  []string `json:"paths"`
}

type sourceCellAuthority struct { // betteralign:ignore canonical JSON field order
	ID              string        `json:"id"`
	Argv            []string      `json:"argv"`
	Environment     []string      `json:"environment"`
	RepositoryPaths sourcePathSet `json:"repository_paths"`
}

type sourceCapture struct {
	Authority sourceAuthority
	Files     []string
}

func governedSourceCapture(root string, config sourceBuildConfig) (sourceCapture, error) {
	root, config, err := validateSourceBuildConfig(root, config)
	if err != nil {
		return sourceCapture{}, err
	}
	manifestPath := filepath.Join(root, filepath.FromSlash(sourceManifestRelativePath))
	if err := validateSourceManifestFile(root, manifestPath); err != nil {
		return sourceCapture{}, err
	}
	manifest, authorityDigest, manifestDigest, err := loadManifestSourceAuthorityIdentity(manifestPath)
	if err != nil {
		return sourceCapture{}, err
	}
	physical, err := physicalSourceFiles(root, manifest.PhysicalPolicy.RuntimeAssets)
	if err != nil {
		return sourceCapture{}, err
	}
	if err := validateSourceModuleRegistry(root, manifest, physical); err != nil {
		return sourceCapture{}, err
	}
	startTool, err := inspectSourceGoTool(config)
	if err != nil {
		return sourceCapture{}, err
	}
	modules := make(map[string]manifestSourceModule, len(manifest.Modules))
	for _, module := range manifest.Modules {
		modules[module.ID] = module
	}
	cells := make([]sourceCellAuthority, 0, len(manifest.BuildCells))
	cellPaths := make([][]string, 0, len(manifest.BuildCells))
	for _, cell := range manifest.BuildCells {
		module := modules[cell.ModuleID]
		tokenArgv := sourceCellArgv(module, cell)
		tokenEnvironment := sourceCellEnvironment(cell)
		arguments, err := materializeSourceArgv(root, config, tokenArgv)
		if err != nil {
			return sourceCapture{}, fmt.Errorf("source cell %q argv: %w", cell.ID, err)
		}
		environment, err := materializeSourceEnvironment(config, tokenEnvironment)
		if err != nil {
			return sourceCapture{}, fmt.Errorf("source cell %q environment: %w", cell.ID, err)
		}
		output, err := runSourceList(arguments, environment)
		if err != nil {
			return sourceCapture{}, fmt.Errorf("list source cell %s: %w", sourceCellDescription(cell), err)
		}
		selected, err := parseSourceList(root, output)
		if err != nil {
			return sourceCapture{}, fmt.Errorf("parse source cell %s: %w", sourceCellDescription(cell), err)
		}
		pathSet, err := newSourcePathSet(selected)
		if err != nil {
			return sourceCapture{}, fmt.Errorf("source cell %q repository paths: %w", cell.ID, err)
		}
		cells = append(cells, sourceCellAuthority{
			ID:              cell.ID,
			Argv:            tokenArgv,
			Environment:     tokenEnvironment,
			RepositoryPaths: pathSet,
		})
		cellPaths = append(cellPaths, selected)
	}
	physicalSet, err := newSourcePathSet(physical)
	if err != nil {
		return sourceCapture{}, fmt.Errorf("physical source paths: %w", err)
	}
	buildSet, err := newSourcePathSet(mergeSourcePaths(cellPaths...))
	if err != nil {
		return sourceCapture{}, fmt.Errorf("build-selected source union: %w", err)
	}
	governed := mergeSourcePaths(physical, buildSet.Paths)
	governedSet, err := newSourcePathSet(governed)
	if err != nil {
		return sourceCapture{}, fmt.Errorf("governed source union: %w", err)
	}
	endTool, err := inspectSourceGoTool(config)
	if err != nil {
		return sourceCapture{}, err
	}
	if !reflect.DeepEqual(endTool, startTool) {
		return sourceCapture{}, errors.New("source Go tool changed while capturing cells")
	}
	endManifest, endAuthorityDigest, endManifestDigest, err := loadManifestSourceAuthorityIdentity(manifestPath)
	if err != nil {
		return sourceCapture{}, fmt.Errorf("reload source-authority manifest: %w", err)
	}
	if endAuthorityDigest != authorityDigest || endManifestDigest != manifestDigest || !reflect.DeepEqual(endManifest, manifest) {
		return sourceCapture{}, errors.New("source-authority manifest changed while capturing cells")
	}
	authority := sourceAuthority{
		EnumerationPolicy:             governedSourcePolicy,
		ManifestPath:                  sourceManifestRelativePath,
		ManifestSHA256:                manifestDigest,
		ManifestSourceAuthoritySHA256: authorityDigest,
		GoTool:                        startTool,
		BuildCells:                    cells,
		PhysicalPaths:                 physicalSet,
		BuildUnion:                    buildSet,
		GovernedUnion:                 governedSet,
		EnvironmentPolicy:             "tournamentmeta-go-list-hermetic-v2",
		ModuleMode:                    "readonly",
		WorkspaceMode:                 "off",
		ProxyMode:                     "off",
		ToolchainMode:                 "local",
		BuildVCS:                      false,
	}
	if err := validateSourceAuthorityManifest(authority, manifest, authorityDigest, manifestDigest); err != nil {
		return sourceCapture{}, err
	}
	return sourceCapture{Authority: authority, Files: slices.Clone(governed)}, nil
}

func validateSourceManifestFile(root, manifestPath string) error {
	resolved, err := filepath.EvalSymlinks(manifestPath)
	if err != nil {
		return fmt.Errorf("resolve source-authority manifest: %w", err)
	}
	want := filepath.Join(root, filepath.FromSlash(sourceManifestRelativePath))
	if resolved != want {
		return fmt.Errorf("source-authority manifest resolves to %q, want %q", resolved, want)
	}
	info, err := os.Lstat(manifestPath)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("source-authority manifest is not a physical regular file: %q", manifestPath)
	}
	return nil
}

func newSourcePathSet(paths []string) (sourcePathSet, error) {
	paths = slices.Clone(paths)
	if len(paths) == 0 {
		return sourcePathSet{}, errors.New("source path set is empty")
	}
	if !slices.IsSorted(paths) || len(slices.Compact(slices.Clone(paths))) != len(paths) {
		return sourcePathSet{}, errors.New("source paths are not a strictly sorted set")
	}
	if err := validatePortableSourcePathSet(paths); err != nil {
		return sourcePathSet{}, err
	}
	digest := sha256.New()
	writeFingerprintFrame(digest, []byte(sourcePathSetDomain))
	writeFingerprintFrame(digest, []byte(strconv.Itoa(len(paths))))
	for _, relative := range paths {
		if err := validateSourceFilePath(relative); err != nil {
			return sourcePathSet{}, err
		}
		writeFingerprintFrame(digest, []byte(relative))
	}
	return sourcePathSet{Count: len(paths), SHA256: hex.EncodeToString(digest.Sum(nil)), Paths: paths}, nil
}

func validateSourcePathSet(value sourcePathSet, description string) error {
	expected, err := newSourcePathSet(value.Paths)
	if err != nil {
		return fmt.Errorf("%s: %w", description, err)
	}
	if value.Count != expected.Count || value.SHA256 != expected.SHA256 {
		return fmt.Errorf("%s count/digest is inconsistent", description)
	}
	return nil
}

func mergeSourcePaths(groups ...[]string) []string {
	count := 0
	for _, group := range groups {
		count += len(group)
	}
	result := make([]string, 0, count)
	for _, group := range groups {
		result = append(result, group...)
	}
	slices.Sort(result)
	return slices.Compact(result)
}

func validateSourceModuleRegistry(root string, manifest manifestSourceAuthority, physical []string) error {
	actual := make([]string, 0)
	for _, relative := range physical {
		if relative == "go.mod" || strings.HasSuffix(relative, "/go.mod") {
			actual = append(actual, relative)
		}
	}
	expected := make([]string, 0, len(manifest.Modules))
	moduleByFile := make(map[string]manifestSourceModule, len(manifest.Modules))
	for _, module := range manifest.Modules {
		relative := "go.mod"
		if module.Root != "." {
			relative = module.Root + "/go.mod"
		}
		expected = append(expected, relative)
		moduleByFile[relative] = module
	}
	slices.Sort(expected)
	if !slices.Equal(actual, expected) {
		return fmt.Errorf("physical module registry %q != manifest registry %q", actual, expected)
	}
	for _, relative := range expected {
		path := filepath.Join(root, filepath.FromSlash(relative))
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("source module file %q is not a physical regular file", relative)
		}
		modulePath, err := readModuleDirective(path)
		if err != nil {
			return fmt.Errorf("source module file %q: %w", relative, err)
		}
		if modulePath != moduleByFile[relative].ModulePath {
			return fmt.Errorf("source module file %q path = %q, want %q", relative, modulePath, moduleByFile[relative].ModulePath)
		}
	}
	return nil
}

func readModuleDirective(path string) (_ string, err error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, annotateError("close module file", file.Close())) }()
	scanner := bufio.NewScanner(file)
	modulePath := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "module") || (len(line) > len("module") && line[len("module")] != ' ' && line[len("module")] != '\t') {
			continue
		}
		if modulePath != "" {
			return "", errors.New("multiple module directives")
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, "module"))
		if strings.HasPrefix(value, "\"") || strings.HasPrefix(value, "`") {
			value, err = strconv.Unquote(value)
			if err != nil {
				return "", fmt.Errorf("unquote module path: %w", err)
			}
		} else if len(strings.Fields(value)) != 1 {
			return "", errors.New("malformed module directive")
		}
		modulePath = value
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scan module file: %w", err)
	}
	if modulePath == "" {
		return "", errors.New("module directive is absent")
	}
	return modulePath, nil
}
