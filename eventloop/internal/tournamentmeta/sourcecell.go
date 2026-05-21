package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
)

func sourceCellArgv(module manifestSourceModule, cell manifestSourceCell) []string {
	moduleRoot := "{source-root}"
	if module.Root != "." {
		moduleRoot += "/" + module.Root
	}
	result := []string{"{go-executable}", "-C", moduleRoot, "list", "-json"}
	result = append(result, cell.SelectionFlags...)
	if len(cell.BuildTags) != 0 {
		result = append(result, "-tags="+strings.Join(cell.BuildTags, ","))
	}
	result = append(result, cell.PackagePatterns...)
	return result
}

func sourceCellEnvironment(cell manifestSourceCell) []string {
	cgo := "0"
	if cell.CGOEnabled != nil && *cell.CGOEnabled {
		cgo = "1"
	}
	result := []string{
		"CGO_ENABLED=" + cgo,
		"GOARCH=" + cell.GOARCH,
		"GOCACHE={build-cache}",
		"GODEBUG=",
		"GOENV=off",
		"GOEXPERIMENT=",
		"GOFLAGS=-buildvcs=false -mod=readonly",
		"GOGC=100",
		"GOMEMLIMIT=off",
		"GOMAXPROCS=1",
		"GOMODCACHE={module-cache}",
		"GONOPROXY=",
		"GONOSUMDB=",
		"GOOS=" + cell.GOOS,
		"GOPATH={scratch}/gopath",
		"GOPRIVATE=",
		"GOPROXY=off",
		"GOSUMDB=off",
		"GOTMPDIR={scratch}/tmp",
		"GOTOOLCHAIN=local",
		"GOWORK=off",
		"HOME={scratch}",
		"LANG=C",
		"LC_ALL=C",
		"PATH={go-executable-directory}",
		"SYSTEMROOT={host-system-root}",
		"TEMP={scratch}/tmp",
		"TMP={scratch}/tmp",
		"TMPDIR={scratch}/tmp",
		"TZ=UTC",
		"XDG_CONFIG_HOME={scratch}",
		cell.ArchitectureFeature.Name + "=" + cell.ArchitectureFeature.Value,
	}
	slices.Sort(result)
	return result
}

func materializeSourceArgv(root string, config sourceBuildConfig, arguments []string) ([]string, error) {
	result := make([]string, len(arguments))
	for index, argument := range arguments {
		switch {
		case argument == "{go-executable}":
			result[index] = config.GoExecutable
		case argument == "{source-root}":
			result[index] = root
		case strings.HasPrefix(argument, "{source-root}/"):
			relative := strings.TrimPrefix(argument, "{source-root}/")
			if err := validateRelativePath(relative); err != nil {
				return nil, fmt.Errorf("materialize source argv: %w", err)
			}
			result[index] = filepath.Join(root, filepath.FromSlash(relative))
		case strings.Contains(argument, "{") || strings.Contains(argument, "}"):
			return nil, fmt.Errorf("source argv has unknown placeholder %q", argument)
		default:
			result[index] = argument
		}
	}
	if len(result) == 0 || result[0] != config.GoExecutable {
		return nil, errors.New("source argv omits governed Go executable")
	}
	return result, nil
}

func materializeSourceEnvironment(config sourceBuildConfig, records []string) ([]string, error) {
	systemRoot := ""
	if runtime.GOOS == "windows" {
		systemRoot = sourceSystemRoot()
		if systemRoot == "" {
			return nil, errors.New("source environment requires SYSTEMROOT on Windows")
		}
	}
	result := make([]string, len(records))
	for index, record := range records {
		substituted, err := substituteEnvironmentPlaceholders(record, config, systemRoot)
		if err != nil {
			return nil, err
		}
		result[index] = substituted
		if strings.Contains(result[index], "{") || strings.Contains(result[index], "}") {
			return nil, fmt.Errorf("source environment has unknown placeholder %q", record)
		}
	}
	if !slices.IsSorted(result) {
		return nil, errors.New("materialized source environment is not sorted")
	}
	return result, nil
}

// substituteEnvironmentPlaceholders expands the placeholder tokens defined by
// sourceCellEnvironment for a single environment record. Path-bearing tokens
// ({scratch}, {build-cache}, {module-cache}, and {go-executable-directory})
// are joined to any trailing slash-relative suffix using filepath.Join, so the
// materialized path uses native separators on every platform even though the
// token templates themselves are platform-independent forward-slash text.
func substituteEnvironmentPlaceholders(record string, config sourceBuildConfig, systemRoot string) (string, error) {
	pathValues := map[string]string{
		"{build-cache}":             config.BuildCache,
		"{go-executable-directory}": filepath.Dir(config.GoExecutable),
		"{module-cache}":            config.ModuleCache,
		"{scratch}":                 config.ScratchRoot,
	}
	plainValues := map[string]string{
		"{host-system-root}": systemRoot,
	}
	tokens := []string{
		"{host-system-root}",
		"{build-cache}",
		"{go-executable-directory}",
		"{module-cache}",
		"{scratch}",
	}
	result := record
	for _, token := range tokens {
		pathValue, isPath := pathValues[token]
		value, ok := plainValues[token]
		if isPath {
			value = pathValue
			ok = true
		}
		if !ok {
			continue
		}
		result = replacePathToken(result, token, value, isPath)
	}
	return result, nil
}

// replacePathToken substitutes a single placeholder token in record. When the
// token is path-bearing and immediately followed by a slash-relative suffix
// (e.g. "{scratch}/tmp"), the suffix is appended with filepath.Join so the
// separators match the host platform; otherwise the token is replaced by value
// verbatim.
func replacePathToken(record, token, value string, pathAware bool) string {
	if !pathAware {
		return strings.ReplaceAll(record, token, value)
	}
	var builder strings.Builder
	remaining := record
	for {
		index := strings.Index(remaining, token)
		if index < 0 {
			builder.WriteString(remaining)
			break
		}
		builder.WriteString(remaining[:index])
		after := remaining[index+len(token):]
		trimmed, rest := splitPathSuffix(after)
		if trimmed == "" {
			builder.WriteString(value)
		} else {
			builder.WriteString(filepath.Join(value, trimmed))
		}
		remaining = rest
	}
	return builder.String()
}

// splitPathSuffix separates a leading slash-relative path component (the part
// following a path placeholder token) from the remainder of the record. It
// returns the relative path without its leading separator and the unconsumed
// record tail. A non-empty relative path beginning with any other character
// means the token was not path-qualified and the whole input is the tail.
func splitPathSuffix(input string) (relative string, tail string) {
	if input == "" || input[0] != '/' {
		return "", input
	}
	rest := input[1:]
	end := len(rest)
	if cut := strings.IndexAny(rest, "{}"); cut >= 0 {
		end = cut
	}
	return rest[:end], input[1+end:]
}

func runSourceList(arguments, environment []string) ([]byte, error) {
	if len(arguments) == 0 {
		return nil, errors.New("source list argv is empty")
	}
	command := exec.Command(arguments[0], arguments[1:]...)
	command.Env = environment
	output, err := command.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return nil, fmt.Errorf("go list: %w: %s", err, exit.Stderr)
		}
		return nil, fmt.Errorf("go list: %w", err)
	}
	return output, nil
}

func parseSourceList(root string, data []byte) ([]string, error) {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("resolve source-list root: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	files := make([]string, 0)
	packageCount := 0
	for {
		var pkg sourceListPackage
		if err := decoder.Decode(&pkg); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("decode go package: %w", err)
		}
		packageCount++
		if pkg.ImportPath == "" || strings.ContainsAny(pkg.ImportPath, "\x00\r\n") {
			return nil, errors.New("go package has invalid import path")
		}
		if pkg.Dir == "" || !filepath.IsAbs(pkg.Dir) {
			return nil, fmt.Errorf("go package %q has invalid directory %q", pkg.ImportPath, pkg.Dir)
		}
		resolvedDirectory, err := filepath.EvalSymlinks(pkg.Dir)
		if err != nil {
			return nil, fmt.Errorf("resolve go package %q directory: %w", pkg.ImportPath, err)
		}
		if !containedPath(root, resolvedDirectory) {
			continue
		}
		for _, name := range sourcePackageFiles(pkg) {
			if name == "" || strings.ContainsAny(name, "\x00\r\n") {
				return nil, fmt.Errorf("go package %q has invalid file %q", pkg.ImportPath, name)
			}
			filePath := name
			if filepath.IsAbs(name) || filepath.VolumeName(name) != "" {
				filePath = filepath.Clean(name)
				if !containedPath(root, filePath) {
					continue
				}
			} else {
				filePath = filepath.Join(resolvedDirectory, filepath.FromSlash(name))
			}
			relative, err := filepath.Rel(root, filePath)
			if err != nil {
				return nil, fmt.Errorf("resolve build-selected source %q: %w", filePath, err)
			}
			relative = filepath.ToSlash(relative)
			if err := validateSourceFilePath(relative); err != nil {
				return nil, fmt.Errorf("build-selected source %q: %w", filePath, err)
			}
			info, err := os.Lstat(filePath)
			if err != nil {
				return nil, fmt.Errorf("inspect build-selected source %q: %w", relative, err)
			}
			if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
				return nil, fmt.Errorf("build-selected source %q has unsupported mode %s", relative, info.Mode())
			}
			files = append(files, relative)
		}
	}
	if packageCount == 0 {
		return nil, errors.New("go source-list package set is empty")
	}
	slices.Sort(files)
	return slices.Compact(files), nil
}

func sourceCellDescription(cell manifestSourceCell) string {
	cgo := false
	if cell.CGOEnabled != nil {
		cgo = *cell.CGOEnabled
	}
	return cell.ID + " at " + cell.GOOS + "/" + cell.GOARCH + "/cgo=" + strconv.FormatBool(cgo)
}
