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
	replacements := []struct {
		token string
		value string
	}{
		{token: "{build-cache}", value: config.BuildCache},
		{token: "{go-executable-directory}", value: filepath.Dir(config.GoExecutable)},
		{token: "{host-system-root}", value: systemRoot},
		{token: "{module-cache}", value: config.ModuleCache},
		{token: "{scratch}", value: config.ScratchRoot},
	}
	result := make([]string, len(records))
	for index, record := range records {
		result[index] = record
		for _, replacement := range replacements {
			result[index] = strings.ReplaceAll(result[index], replacement.token, replacement.value)
		}
		if strings.Contains(result[index], "{") || strings.Contains(result[index], "}") {
			return nil, fmt.Errorf("source environment has unknown placeholder %q", record)
		}
	}
	if !slices.IsSorted(result) {
		return nil, errors.New("materialized source environment is not sorted")
	}
	return result, nil
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
