package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

type sourceBuildConfig struct {
	GoExecutable string
	ModuleCache  string
	BuildCache   string
	ScratchRoot  string
}

type sourceBuildFlags struct {
	goExecutable *string
	moduleCache  *string
	buildCache   *string
	scratchRoot  *string
}

func registerSourceBuildFlags(flags *flag.FlagSet) *sourceBuildFlags {
	result := &sourceBuildFlags{}
	result.goExecutable = flags.String("go", "", "absolute Go executable for source closure")
	result.moduleCache = flags.String("gomodcache", "", "prefetched module cache")
	result.buildCache = flags.String("gocache", "", "private Go build cache")
	result.scratchRoot = flags.String("go-scratch", "", "private Go list scratch root")
	return result
}

func (value *sourceBuildFlags) config() sourceBuildConfig {
	return sourceBuildConfig{
		GoExecutable: *value.goExecutable,
		ModuleCache:  *value.moduleCache,
		BuildCache:   *value.buildCache,
		ScratchRoot:  *value.scratchRoot,
	}
}

type sourceListPackage struct {
	Module          *sourceListModule `json:"Module"`
	ImportPath      string            `json:"ImportPath"`
	ForTest         string            `json:"ForTest"`
	Dir             string            `json:"Dir"`
	GoFiles         []string          `json:"GoFiles"`
	CgoFiles        []string          `json:"CgoFiles"`
	CFiles          []string          `json:"CFiles"`
	CXXFiles        []string          `json:"CXXFiles"`
	MFiles          []string          `json:"MFiles"`
	HFiles          []string          `json:"HFiles"`
	FFiles          []string          `json:"FFiles"`
	SFiles          []string          `json:"SFiles"`
	SwigFiles       []string          `json:"SwigFiles"`
	SwigCXXFiles    []string          `json:"SwigCXXFiles"`
	SysoFiles       []string          `json:"SysoFiles"`
	EmbedFiles      []string          `json:"EmbedFiles"`
	TestGoFiles     []string          `json:"TestGoFiles"`
	TestEmbedFiles  []string          `json:"TestEmbedFiles"`
	XTestGoFiles    []string          `json:"XTestGoFiles"`
	XTestEmbedFiles []string          `json:"XTestEmbedFiles"`
}

type sourceListModule struct {
	Path    string            `json:"Path"`
	Version string            `json:"Version"`
	Main    bool              `json:"Main"`
	Replace *sourceListModule `json:"Replace"`
}

func validateSourceBuildConfig(root string, config sourceBuildConfig) (string, sourceBuildConfig, error) {
	if root == "" || !filepath.IsAbs(root) {
		return "", sourceBuildConfig{}, errors.New("source build root must be absolute")
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", sourceBuildConfig{}, fmt.Errorf("resolve source build root: %w", err)
	}
	rootInfo, err := os.Stat(resolvedRoot)
	if err != nil || !rootInfo.IsDir() {
		return "", sourceBuildConfig{}, fmt.Errorf("source build root is not a directory: %q", root)
	}
	type sourceBuildPath struct {
		name  string
		value string
	}
	paths := []sourceBuildPath{
		{name: "Go executable", value: config.GoExecutable},
		{name: "module cache", value: config.ModuleCache},
		{name: "build cache", value: config.BuildCache},
		{name: "scratch root", value: config.ScratchRoot},
	}
	for _, path := range paths {
		name, value := path.name, path.value
		if value == "" || !filepath.IsAbs(value) {
			return "", sourceBuildConfig{}, fmt.Errorf("source build %s must be absolute", name)
		}
		if name == "Go executable" {
			value = normalizeExecutablePath(value)
		}
		resolved, err := filepath.EvalSymlinks(value)
		if err != nil {
			return "", sourceBuildConfig{}, fmt.Errorf("resolve source build %s: %w", name, err)
		}
		if name == "Go executable" {
			info, err := os.Stat(resolved)
			if err != nil || !info.Mode().IsRegular() {
				return "", sourceBuildConfig{}, fmt.Errorf("source build Go executable is not a regular file: %q", value)
			}
			config.GoExecutable = resolved
			continue
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.IsDir() {
			return "", sourceBuildConfig{}, fmt.Errorf("source build %s is not a directory: %q", name, value)
		}
		switch name {
		case "module cache":
			config.ModuleCache = resolved
		case "build cache":
			config.BuildCache = resolved
		case "scratch root":
			config.ScratchRoot = resolved
		}
	}
	writableRoots := []sourceBuildPath{
		{name: "module cache", value: config.ModuleCache},
		{name: "build cache", value: config.BuildCache},
		{name: "scratch root", value: config.ScratchRoot},
	}
	for index, current := range writableRoots {
		if containedPath(resolvedRoot, current.value) || containedPath(current.value, resolvedRoot) {
			return "", sourceBuildConfig{}, fmt.Errorf("source build %s must not overlap source root", current.name)
		}
		for _, previous := range writableRoots[:index] {
			if containedPath(previous.value, current.value) || containedPath(current.value, previous.value) {
				return "", sourceBuildConfig{}, fmt.Errorf("source build %s and %s must not overlap", previous.name, current.name)
			}
		}
	}
	for _, path := range []string{
		filepath.Join(config.ScratchRoot, "gopath"),
		filepath.Join(config.ScratchRoot, "tmp"),
	} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return "", sourceBuildConfig{}, fmt.Errorf("create source build scratch %q: %w", path, err)
		}
	}
	return resolvedRoot, config, nil
}

func sourceTargetArchitecture(goarch string) (string, error) {
	switch goarch {
	case "amd64":
		return "GOAMD64=v1", nil
	case "arm64":
		return "GOARM64=v8.0", nil
	case "wasm":
		return "GOWASM=satconv,signext", nil
	default:
		return "", fmt.Errorf("source build architecture %q has no pinned feature policy", goarch)
	}
}

func sourcePackageFiles(pkg sourceListPackage) []string {
	count := len(pkg.GoFiles) + len(pkg.CgoFiles) + len(pkg.CFiles) + len(pkg.CXXFiles) +
		len(pkg.MFiles) + len(pkg.HFiles) + len(pkg.FFiles) + len(pkg.SFiles) +
		len(pkg.SwigFiles) + len(pkg.SwigCXXFiles) + len(pkg.SysoFiles) +
		len(pkg.EmbedFiles) + len(pkg.TestGoFiles) + len(pkg.TestEmbedFiles) +
		len(pkg.XTestGoFiles) + len(pkg.XTestEmbedFiles)
	result := make([]string, 0, count)
	for _, group := range [][]string{
		pkg.GoFiles,
		pkg.CgoFiles,
		pkg.CFiles,
		pkg.CXXFiles,
		pkg.MFiles,
		pkg.HFiles,
		pkg.FFiles,
		pkg.SFiles,
		pkg.SwigFiles,
		pkg.SwigCXXFiles,
		pkg.SysoFiles,
		pkg.EmbedFiles,
		pkg.TestGoFiles,
		pkg.TestEmbedFiles,
		pkg.XTestGoFiles,
		pkg.XTestEmbedFiles,
	} {
		result = append(result, group...)
	}
	return result
}
