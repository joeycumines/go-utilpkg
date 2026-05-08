package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"slices"
	"strings"
)

const (
	governedSourcePolicy = "manifest-physical-plus-go-list-cells-v1"
	testSourcePolicy     = "physical-test-fixture-v1"
)

type sourceAuthority struct { // betteralign:ignore canonical JSON field order
	EnumerationPolicy             string                `json:"enumeration_policy"`
	ManifestPath                  string                `json:"manifest_path"`
	ManifestSHA256                string                `json:"manifest_sha256"`
	ManifestSourceAuthoritySHA256 string                `json:"manifest_source_authority_sha256"`
	GoTool                        sourceGoTool          `json:"go_tool"`
	BuildCells                    []sourceCellAuthority `json:"build_cells"`
	PhysicalPaths                 sourcePathSet         `json:"physical_paths"`
	BuildUnion                    sourcePathSet         `json:"build_union"`
	GovernedUnion                 sourcePathSet         `json:"governed_union"`
	EnvironmentPolicy             string                `json:"environment_policy"`
	ModuleMode                    string                `json:"module_mode"`
	WorkspaceMode                 string                `json:"workspace_mode"`
	ProxyMode                     string                `json:"proxy_mode"`
	ToolchainMode                 string                `json:"toolchain_mode"`
	BuildVCS                      bool                  `json:"build_vcs"`
}

type sourceGoTool struct { // betteralign:ignore canonical JSON field order
	ExecutableSHA256 string `json:"executable_sha256"`
	VersionOutput    string `json:"version_output"`
	GOVersion        string `json:"go_version"`
	GOHostOS         string `json:"go_host_os"`
	GOHostArch       string `json:"go_host_arch"`
}

type sourceGoEnvironment struct { // betteralign:ignore canonical JSON field order
	GOVersion  string `json:"GOVERSION"`
	GOHostOS   string `json:"GOHOSTOS"`
	GOHostArch string `json:"GOHOSTARCH"`
}

func inspectSourceGoTool(config sourceBuildConfig) (sourceGoTool, error) {
	file, err := os.Open(config.GoExecutable)
	if err != nil {
		return sourceGoTool{}, fmt.Errorf("open source-authority Go executable: %w", err)
	}
	digest, digestErr := sha256Reader(file)
	closeErr := file.Close()
	if digestErr != nil || closeErr != nil {
		return sourceGoTool{}, errors.Join(
			annotateError("hash source-authority Go executable", digestErr),
			annotateError("close source-authority Go executable", closeErr),
		)
	}
	feature, err := sourceTargetArchitecture(runtime.GOARCH)
	if err != nil {
		return sourceGoTool{}, err
	}
	name, value, _ := strings.Cut(feature, "=")
	cgo := false
	cell := manifestSourceCell{
		GOOS:                runtime.GOOS,
		GOARCH:              runtime.GOARCH,
		CGOEnabled:          &cgo,
		ArchitectureFeature: manifestArchitectureFeature{Name: name, Value: value},
	}
	environment, err := materializeSourceEnvironment(config, sourceCellEnvironment(cell))
	if err != nil {
		return sourceGoTool{}, err
	}
	versionOutput, err := sourceToolOutput(config.GoExecutable, environment, "version")
	if err != nil {
		return sourceGoTool{}, err
	}
	environmentOutput, err := sourceToolOutput(
		config.GoExecutable,
		environment,
		"env", "-json", "GOVERSION", "GOHOSTOS", "GOHOSTARCH",
	)
	if err != nil {
		return sourceGoTool{}, err
	}
	decoder := json.NewDecoder(bytes.NewBufferString(environmentOutput))
	decoder.DisallowUnknownFields()
	var goEnvironment sourceGoEnvironment
	if err := decoder.Decode(&goEnvironment); err != nil {
		return sourceGoTool{}, fmt.Errorf("decode source-authority Go environment: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return sourceGoTool{}, errors.New("source-authority Go environment has trailing JSON")
	}
	result := sourceGoTool{
		ExecutableSHA256: digest,
		VersionOutput:    versionOutput,
		GOVersion:        goEnvironment.GOVersion,
		GOHostOS:         goEnvironment.GOHostOS,
		GOHostArch:       goEnvironment.GOHostArch,
	}
	if err := validateSourceGoTool(result); err != nil {
		return sourceGoTool{}, err
	}
	return result, nil
}

func fixtureSourceAuthority() sourceAuthority {
	return sourceAuthority{EnumerationPolicy: testSourcePolicy}
}

func validateSourceAuthority(authority sourceAuthority) error {
	if authority.EnumerationPolicy == testSourcePolicy {
		fixture := fixtureSourceAuthority()
		if !reflect.DeepEqual(authority, fixture) {
			return errors.New("test source authority has governed-build fields")
		}
		return nil
	}
	if authority.EnumerationPolicy != governedSourcePolicy || authority.ManifestPath != sourceManifestRelativePath ||
		authority.EnvironmentPolicy != "tournamentmeta-go-list-hermetic-v2" || authority.ModuleMode != "readonly" ||
		authority.WorkspaceMode != "off" || authority.ProxyMode != "off" || authority.ToolchainMode != "local" || authority.BuildVCS {
		return errors.New("governed source authority policy changed")
	}
	if err := validateCanonicalSHA256(authority.ManifestSHA256, "manifest"); err != nil {
		return err
	}
	if err := validateCanonicalSHA256(authority.ManifestSourceAuthoritySHA256, "manifest source authority"); err != nil {
		return err
	}
	if err := validateSourceGoTool(authority.GoTool); err != nil {
		return err
	}
	if len(authority.BuildCells) == 0 {
		return errors.New("source authority build cells are empty")
	}
	previous := ""
	cellPaths := make([][]string, 0, len(authority.BuildCells))
	for _, cell := range authority.BuildCells {
		if !sourceAuthorityIDPattern.MatchString(cell.ID) || cell.ID <= previous || cell.Argv == nil ||
			len(cell.Argv) == 0 || cell.Environment == nil || !slices.IsSorted(cell.Environment) {
			return fmt.Errorf("source authority cell %q is invalid or unsorted", cell.ID)
		}
		previous = cell.ID
		if err := validateSourcePathSet(cell.RepositoryPaths, "source cell "+cell.ID); err != nil {
			return err
		}
		cellPaths = append(cellPaths, cell.RepositoryPaths.Paths)
	}
	if err := validateSourcePathSet(authority.PhysicalPaths, "physical source paths"); err != nil {
		return err
	}
	if err := validateSourcePathSet(authority.BuildUnion, "build-selected source union"); err != nil {
		return err
	}
	if err := validateSourcePathSet(authority.GovernedUnion, "governed source union"); err != nil {
		return err
	}
	wantBuild, err := newSourcePathSet(mergeSourcePaths(cellPaths...))
	if err != nil || !reflect.DeepEqual(wantBuild, authority.BuildUnion) {
		return errors.New("source authority build union does not equal its cells")
	}
	wantGoverned, err := newSourcePathSet(mergeSourcePaths(authority.PhysicalPaths.Paths, authority.BuildUnion.Paths))
	if err != nil || !reflect.DeepEqual(wantGoverned, authority.GovernedUnion) {
		return errors.New("source authority governed union does not equal physical plus build paths")
	}
	return nil
}

func validateSourceAuthorityManifest(
	authority sourceAuthority,
	manifest manifestSourceAuthority,
	authorityDigest,
	manifestDigest string,
) error {
	if err := validateSourceAuthority(authority); err != nil {
		return err
	}
	if authority.ManifestSourceAuthoritySHA256 != authorityDigest || authority.ManifestSHA256 != manifestDigest ||
		len(authority.BuildCells) != len(manifest.BuildCells) {
		return errors.New("source authority does not match manifest identity or cell count")
	}
	modules := make(map[string]manifestSourceModule, len(manifest.Modules))
	for _, module := range manifest.Modules {
		modules[module.ID] = module
	}
	for index, expected := range manifest.BuildCells {
		actual := authority.BuildCells[index]
		if actual.ID != expected.ID || !slices.Equal(actual.Argv, sourceCellArgv(modules[expected.ModuleID], expected)) ||
			!slices.Equal(actual.Environment, sourceCellEnvironment(expected)) {
			return fmt.Errorf("source authority cell %q differs from manifest", expected.ID)
		}
	}
	return nil
}

func validatePersistedSourceAuthority(authority sourceAuthority) error {
	if err := validateSourceAuthority(authority); err != nil {
		return err
	}
	if authority.EnumerationPolicy != governedSourcePolicy {
		return fmt.Errorf("persisted source authority uses non-governed policy %q", authority.EnumerationPolicy)
	}
	return nil
}

func validateSourceGoTool(tool sourceGoTool) error {
	if err := validateCanonicalSHA256(tool.ExecutableSHA256, "Go executable"); err != nil {
		return err
	}
	if tool.VersionOutput == "" || tool.GOVersion == "" || tool.GOHostOS == "" || tool.GOHostArch == "" ||
		strings.ContainsAny(tool.VersionOutput+tool.GOVersion+tool.GOHostOS+tool.GOHostArch, "\x00\r\n") {
		return errors.New("source authority Go tool identity is incomplete")
	}
	return nil
}

func validateCanonicalSHA256(value, description string) error {
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size || hex.EncodeToString(decoded) != value {
		return fmt.Errorf("source authority %s SHA-256 is invalid", description)
	}
	return nil
}

func sourceSystemRoot() string {
	if value := os.Getenv("SYSTEMROOT"); value != "" {
		return value
	}
	return os.Getenv("SystemRoot")
}

func sourceToolOutput(executable string, environment []string, arguments ...string) (string, error) {
	command := exec.Command(executable, arguments...)
	command.Env = environment
	output, err := command.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return "", fmt.Errorf("go %v: %w: %s", arguments, err, exit.Stderr)
		}
		return "", fmt.Errorf("go %v: %w", arguments, err)
	}
	result := strings.TrimSpace(string(output))
	if result == "" || strings.ContainsRune(result, '\x00') {
		return "", fmt.Errorf("go %v returned invalid identity output", arguments)
	}
	return result, nil
}
