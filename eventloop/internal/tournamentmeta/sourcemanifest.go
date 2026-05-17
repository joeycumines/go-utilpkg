package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	manifestSourceAuthorityPolicy = "manifest-build-cells-v1"
	manifestSchemaVersionV4       = 4
	manifestSchemaVersionV5       = 5
	sourceManifestRelativePath    = "eventloop/internal/tournament/manifest.json"
)

var sourceAuthorityIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:[.-][a-z0-9]+)*$`)
var sourceBuildTagPattern = regexp.MustCompile(`^[A-Za-z0-9_.]+$`)

type manifestSourceAuthority struct { // betteralign:ignore canonical JSON field order
	SchemaVersion  int                    `json:"schema_version"`
	Policy         string                 `json:"policy"`
	PhysicalPolicy manifestPhysicalPolicy `json:"physical_policy"`
	Modules        []manifestSourceModule `json:"modules"`
	BuildCells     []manifestSourceCell   `json:"build_cells"`
}

type manifestPhysicalPolicy struct { // betteralign:ignore canonical JSON field order
	ID            string   `json:"id"`
	RootControls  []string `json:"root_controls"`
	Trees         []string `json:"trees"`
	RuntimeAssets []string `json:"runtime_assets"`
}

type manifestSourceModule struct { // betteralign:ignore canonical JSON field order
	ID         string `json:"id"`
	Root       string `json:"root"`
	ModulePath string `json:"module_path"`
	Buildable  *bool  `json:"buildable"`
}

type manifestSourceCell struct { // betteralign:ignore canonical JSON field order
	ID                  string                      `json:"id"`
	ModuleID            string                      `json:"module_id"`
	GOOS                string                      `json:"goos"`
	GOARCH              string                      `json:"goarch"`
	CGOEnabled          *bool                       `json:"cgo_enabled"`
	ArchitectureFeature manifestArchitectureFeature `json:"architecture_feature"`
	BuildTags           []string                    `json:"build_tags"`
	SelectionFlags      []string                    `json:"selection_flags"`
	PackagePatterns     []string                    `json:"package_patterns"`
}

type manifestArchitectureFeature struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type manifestLineageReference struct { // betteralign:ignore canonical JSON field order
	Path          string                        `json:"path"`
	SchemaVersion int                           `json:"schema_version"`
	SHA256        string                        `json:"sha256"`
	Floor         manifestLineageFloorReference `json:"floor"`
}

type manifestLineageFloorReference struct { // betteralign:ignore canonical JSON field order
	Path                      string `json:"path"`
	SchemaVersion             int    `json:"schema_version"`
	Sequence                  int    `json:"sequence"`
	SHA256                    string `json:"sha256"`
	CumulativeRecordSetSHA256 string `json:"cumulative_record_set_sha256"`
}

type sourceManifestEnvelope struct { // betteralign:ignore canonical JSON field order
	SchemaVersion       int                      `json:"schema_version"`
	SourceHistory       json.RawMessage          `json:"source_history"`
	Lineage             manifestLineageReference `json:"lineage"`
	SourceAuthority     manifestSourceAuthority  `json:"source_authority"`
	Measurement         json.RawMessage          `json:"measurement"`
	Variants            json.RawMessage          `json:"variants"`
	VariantGroups       json.RawMessage          `json:"variant_groups"`
	Lanes               json.RawMessage          `json:"lanes"`
	Concepts            json.RawMessage          `json:"concepts"`
	RevisionVariants    json.RawMessage          `json:"revision_variants"`
	RevisionCheckpoints json.RawMessage          `json:"revision_checkpoints"`
	RootDispositions    json.RawMessage          `json:"root_dispositions,omitempty"`
}

func loadManifestSourceAuthority(path string) (manifestSourceAuthority, string, error) {
	authority, digest, _, err := loadManifestSourceAuthorityIdentity(path)
	return authority, digest, err
}

func loadManifestSourceAuthorityIdentity(path string) (manifestSourceAuthority, string, string, error) {
	data, err := readRegularStable(path, 0o644)
	if err != nil {
		return manifestSourceAuthority{}, "", "", fmt.Errorf("read source-authority manifest: %w", err)
	}
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return manifestSourceAuthority{}, "", "", fmt.Errorf("validate source-authority manifest JSON: %w", err)
	}
	if err := validateSourceManifestJSONShape(data); err != nil {
		return manifestSourceAuthority{}, "", "", fmt.Errorf("validate source-authority manifest shape: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest sourceManifestEnvelope
	if err := decoder.Decode(&manifest); err != nil {
		return manifestSourceAuthority{}, "", "", fmt.Errorf("decode source-authority manifest: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return manifestSourceAuthority{}, "", "", errors.New("source-authority manifest has trailing JSON")
	}
	if manifest.SchemaVersion != manifestSchemaVersionV4 && manifest.SchemaVersion != manifestSchemaVersionV5 {
		return manifestSourceAuthority{}, "", "", fmt.Errorf(
			"source-authority manifest schema = %d, want %d or %d",
			manifest.SchemaVersion,
			manifestSchemaVersionV4,
			manifestSchemaVersionV5,
		)
	}
	if err := validateManifestLineageReference(manifest.Lineage); err != nil {
		return manifestSourceAuthority{}, "", "", fmt.Errorf("source-authority manifest lineage: %w", err)
	}
	if err := validateManifestSourceAuthority(manifest.SourceAuthority); err != nil {
		return manifestSourceAuthority{}, "", "", fmt.Errorf("source-authority manifest: %w", err)
	}
	if manifest.SchemaVersion == manifestSchemaVersionV5 {
		if err := verifyManifestV5Lineage(path, manifest); err != nil {
			return manifestSourceAuthority{}, "", "", fmt.Errorf("source-authority manifest lineage authority: %w", err)
		}
	}
	digest, err := manifestSourceAuthorityDigest(manifest.SourceAuthority)
	if err != nil {
		return manifestSourceAuthority{}, "", "", err
	}
	manifestDigest := sha256.Sum256(data)
	return manifest.SourceAuthority, digest, hex.EncodeToString(manifestDigest[:]), nil
}

func validateSourceManifestJSONShape(data []byte) error {
	root, err := decodeSourceRawObject(data, "manifest")
	if err != nil {
		return err
	}
	var schemaVersion int
	if err := json.Unmarshal(root["schema_version"], &schemaVersion); err != nil {
		return errors.New("manifest schema_version must be an integer")
	}
	var rootKeys []string
	switch schemaVersion {
	case manifestSchemaVersionV4:
		rootKeys = []string{
			"concepts", "lanes", "lineage", "measurement", "revision_checkpoints", "revision_variants",
			"schema_version", "source_authority", "source_history", "variant_groups", "variants",
		}
	case manifestSchemaVersionV5:
		rootKeys = []string{
			"concepts", "lanes", "lineage", "measurement", "revision_checkpoints", "revision_variants",
			"root_dispositions", "schema_version", "source_authority", "source_history",
		}
	default:
		return fmt.Errorf("unsupported manifest schema %d", schemaVersion)
	}
	// root_dispositions is optional in v4 and required in v5; reject unknown
	// extra keys that are not part of either schema.
	allowed := make(map[string]bool, len(rootKeys)+1)
	for _, k := range rootKeys {
		allowed[k] = true
	}
	if schemaVersion == manifestSchemaVersionV4 {
		allowed["root_dispositions"] = true
	}
	actual := make([]string, 0, len(root))
	for key := range root {
		if !allowed[key] {
			actual = append(actual, key)
		}
	}
	if len(actual) > 0 {
		slices.Sort(actual)
		return fmt.Errorf("manifest keys = %q, want %q (root_dispositions optional in v4)", actual, rootKeys)
	}
	// Verify all required keys are present.
	for _, k := range rootKeys {
		if _, ok := root[k]; !ok {
			return fmt.Errorf("manifest missing required key %q", k)
		}
	}
	for key, value := range root {
		if string(bytes.TrimSpace(value)) == "null" {
			return fmt.Errorf("manifest field %q is null", key)
		}
	}
	if schemaVersion == manifestSchemaVersionV4 {
		if err := validateSourceManifestV4ProjectionShape(root); err != nil {
			return err
		}
	}
	if schemaVersion == manifestSchemaVersionV5 {
		if err := validateSourceManifestV5ProjectionShape(root); err != nil {
			return err
		}
	}
	lineage, err := decodeSourceRawObject(root["lineage"], "lineage")
	if err != nil {
		return err
	}
	if err := requireSourceRawKeys(lineage, []string{"floor", "path", "schema_version", "sha256"}, "lineage"); err != nil {
		return err
	}
	floor, err := decodeSourceRawObject(lineage["floor"], "lineage.floor")
	if err != nil {
		return err
	}
	if err := requireSourceRawKeys(floor, []string{
		"cumulative_record_set_sha256", "path", "schema_version", "sequence", "sha256",
	}, "lineage.floor"); err != nil {
		return err
	}
	authority, err := decodeSourceRawObject(root["source_authority"], "source_authority")
	if err != nil {
		return err
	}
	if err := requireSourceRawKeys(authority, []string{"build_cells", "modules", "physical_policy", "policy", "schema_version"}, "source_authority"); err != nil {
		return err
	}
	physical, err := decodeSourceRawObject(authority["physical_policy"], "source_authority.physical_policy")
	if err != nil {
		return err
	}
	if err := requireSourceRawKeys(physical, []string{"id", "root_controls", "runtime_assets", "trees"}, "source_authority.physical_policy"); err != nil {
		return err
	}
	var modules []json.RawMessage
	if err := json.Unmarshal(authority["modules"], &modules); err != nil || modules == nil {
		return errors.New("source_authority.modules must be a non-null array")
	}
	for index, raw := range modules {
		module, err := decodeSourceRawObject(raw, fmt.Sprintf("source_authority.modules[%d]", index))
		if err != nil {
			return err
		}
		if err := requireSourceRawKeys(module, []string{"buildable", "id", "module_path", "root"}, fmt.Sprintf("source_authority.modules[%d]", index)); err != nil {
			return err
		}
	}
	var cells []json.RawMessage
	if err := json.Unmarshal(authority["build_cells"], &cells); err != nil || cells == nil {
		return errors.New("source_authority.build_cells must be a non-null array")
	}
	for index, raw := range cells {
		description := fmt.Sprintf("source_authority.build_cells[%d]", index)
		cell, err := decodeSourceRawObject(raw, description)
		if err != nil {
			return err
		}
		if err := requireSourceRawKeys(cell, []string{
			"architecture_feature", "build_tags", "cgo_enabled", "goarch", "goos", "id",
			"module_id", "package_patterns", "selection_flags",
		}, description); err != nil {
			return err
		}
		feature, err := decodeSourceRawObject(cell["architecture_feature"], description+".architecture_feature")
		if err != nil {
			return err
		}
		if err := requireSourceRawKeys(feature, []string{"name", "value"}, description+".architecture_feature"); err != nil {
			return err
		}
	}
	return nil
}

func validateManifestLineageReference(reference manifestLineageReference) error {
	if reference.Path != "lineage.json" ||
		reference.SchemaVersion < lineageMinimumSchemaVersion || reference.SchemaVersion > lineageLatestSchemaVersion ||
		!historySHA256Pattern.MatchString(reference.SHA256) {
		return errors.New("invalid lineage inventory reference")
	}
	wantFloorPath := fmt.Sprintf("lineagefloors/%06d.json", reference.Floor.Sequence)
	if reference.Floor.Sequence <= 0 || reference.Floor.Sequence > 999999 ||
		reference.Floor.Path != wantFloorPath || reference.Floor.SchemaVersion != lineageFloorSchemaVersion ||
		!historySHA256Pattern.MatchString(reference.Floor.SHA256) ||
		!historySHA256Pattern.MatchString(reference.Floor.CumulativeRecordSetSHA256) {
		return errors.New("invalid lineage floor reference")
	}
	return nil
}

func decodeSourceRawObject(data []byte, description string) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil || object == nil {
		return nil, fmt.Errorf("%s must be an object", description)
	}
	return object, nil
}

func requireSourceRawKeys(object map[string]json.RawMessage, expected []string, description string) error {
	actual := make([]string, 0, len(object))
	for key := range object {
		actual = append(actual, key)
	}
	slices.Sort(actual)
	if !slices.Equal(actual, expected) {
		return fmt.Errorf("%s keys = %q, want %q", description, actual, expected)
	}
	return nil
}

func manifestSourceAuthorityDigest(authority manifestSourceAuthority) (string, error) {
	if err := validateManifestSourceAuthority(authority); err != nil {
		return "", err
	}
	data, err := json.Marshal(authority)
	if err != nil {
		return "", fmt.Errorf("encode manifest source authority: %w", err)
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func validateManifestSourceAuthority(authority manifestSourceAuthority) error {
	if authority.SchemaVersion != 1 || authority.Policy != manifestSourceAuthorityPolicy {
		return fmt.Errorf("schema/policy = %d/%q, want 1/%q", authority.SchemaVersion, authority.Policy, manifestSourceAuthorityPolicy)
	}
	if err := validateManifestPhysicalPolicy(authority.PhysicalPolicy); err != nil {
		return err
	}
	if len(authority.Modules) == 0 || len(authority.BuildCells) == 0 {
		return errors.New("source authority requires non-null modules and build cells")
	}
	modules := make(map[string]manifestSourceModule, len(authority.Modules))
	roots := make(map[string]string, len(authority.Modules))
	previous := ""
	for _, module := range authority.Modules {
		if !sourceAuthorityIDPattern.MatchString(module.ID) || module.ID <= previous {
			return fmt.Errorf("source modules are not a strictly sorted valid ID set at %q", module.ID)
		}
		previous = module.ID
		if module.Root != "." {
			if err := validateRelativePath(module.Root); err != nil {
				return fmt.Errorf("source module %q root: %w", module.ID, err)
			}
		}
		if module.ModulePath == "" || strings.ContainsAny(module.ModulePath, "\x00\r\n\\") {
			return fmt.Errorf("source module %q has invalid module path %q", module.ID, module.ModulePath)
		}
		if prior, exists := roots[module.Root]; exists {
			return fmt.Errorf("source modules %q and %q repeat root %q", prior, module.ID, module.Root)
		}
		roots[module.Root] = module.ID
		if module.Buildable == nil {
			return fmt.Errorf("source module %q omits buildable", module.ID)
		}
		modules[module.ID] = module
	}
	cellsByModule := make(map[string]int, len(modules))
	previous = ""
	for _, cell := range authority.BuildCells {
		if !sourceAuthorityIDPattern.MatchString(cell.ID) || cell.ID <= previous {
			return fmt.Errorf("source build cells are not a strictly sorted valid ID set at %q", cell.ID)
		}
		previous = cell.ID
		module, exists := modules[cell.ModuleID]
		if !exists || module.Buildable == nil || !*module.Buildable {
			return fmt.Errorf("source cell %q references missing or control-only module %q", cell.ID, cell.ModuleID)
		}
		if err := validateManifestSourceCell(cell); err != nil {
			return fmt.Errorf("source cell %q: %w", cell.ID, err)
		}
		cellsByModule[cell.ModuleID]++
	}
	for _, module := range authority.Modules {
		if module.Buildable != nil && *module.Buildable && cellsByModule[module.ID] == 0 {
			return fmt.Errorf("buildable source module %q has no cells", module.ID)
		}
		if module.Buildable != nil && !*module.Buildable && cellsByModule[module.ID] != 0 {
			return fmt.Errorf("control-only source module %q has build cells", module.ID)
		}
	}
	return nil
}

func validateManifestPhysicalPolicy(policy manifestPhysicalPolicy) error {
	if policy.ID != physicalSourcePolicy || policy.RootControls == nil || policy.Trees == nil || policy.RuntimeAssets == nil {
		return fmt.Errorf("physical source policy is incomplete or not %q", physicalSourcePolicy)
	}
	if !slices.Equal(policy.RootControls, physicalSourceControls) || !slices.Equal(policy.Trees, physicalSourceTrees) {
		return errors.New("physical source controls or trees changed")
	}
	if err := validateSortedSourcePaths(policy.RuntimeAssets, "runtime assets"); err != nil {
		return err
	}
	return nil
}

func validateManifestSourceCell(cell manifestSourceCell) error {
	if cell.ModuleID == "" || cell.GOOS == "" || cell.GOARCH == "" ||
		strings.ContainsAny(cell.ModuleID+cell.GOOS+cell.GOARCH, "\x00/\\\r\n") {
		return errors.New("module or target is invalid")
	}
	feature, err := sourceTargetArchitecture(cell.GOARCH)
	if err != nil {
		return err
	}
	name, value, ok := strings.Cut(feature, "=")
	if !ok || cell.ArchitectureFeature.Name != name || cell.ArchitectureFeature.Value != value {
		return fmt.Errorf("architecture feature = %+v, want %s=%s", cell.ArchitectureFeature, name, value)
	}
	if cell.BuildTags == nil || cell.SelectionFlags == nil || cell.PackagePatterns == nil {
		return errors.New("tags, flags, and package patterns must be non-null")
	}
	if cell.CGOEnabled == nil {
		return errors.New("cgo_enabled is missing")
	}
	if !slices.Equal(cell.SelectionFlags, []string{"-deps", "-test"}) {
		return fmt.Errorf("selection flags = %q, want [-deps -test]", cell.SelectionFlags)
	}
	if len(cell.PackagePatterns) == 0 || !slices.IsSorted(cell.PackagePatterns) ||
		len(slices.Compact(slices.Clone(cell.PackagePatterns))) != len(cell.PackagePatterns) {
		return errors.New("package patterns are not a nonempty sorted set")
	}
	for _, pattern := range cell.PackagePatterns {
		if !validSourcePackagePattern(pattern) {
			return fmt.Errorf("invalid local package pattern %q", pattern)
		}
	}
	if !slices.IsSorted(cell.BuildTags) || len(slices.Compact(slices.Clone(cell.BuildTags))) != len(cell.BuildTags) {
		return errors.New("build tags are not a sorted set")
	}
	for _, tag := range cell.BuildTags {
		if !sourceBuildTagPattern.MatchString(tag) {
			return fmt.Errorf("invalid build tag %q", tag)
		}
	}
	if slices.Contains(cell.BuildTags, "libuv") && (!*cell.CGOEnabled || cell.ModuleID != "eventloop" ||
		!slices.Equal(cell.PackagePatterns, []string{"./internal/libuvbaseline"})) {
		return errors.New("libuv cell must be cgo-enabled eventloop/libuvbaseline")
	}
	return nil
}

func validSourcePackagePattern(pattern string) bool {
	if !strings.HasPrefix(pattern, "./") || !utf8.ValidString(pattern) ||
		strings.ContainsAny(pattern, "\\:") || strings.IndexFunc(pattern, unicode.IsControl) >= 0 {
		return false
	}
	for component := range strings.SplitSeq(strings.TrimPrefix(pattern, "./"), "/") {
		if component == "" || component == "." || component == ".." {
			return false
		}
	}
	return true
}

func validateSortedSourcePaths(paths []string, description string) error {
	if !slices.IsSorted(paths) || len(slices.Compact(slices.Clone(paths))) != len(paths) {
		return fmt.Errorf("source %s are not a sorted set", description)
	}
	for _, relative := range paths {
		if err := validateRelativePath(relative); err != nil {
			return fmt.Errorf("source %s: %w", description, err)
		}
	}
	return nil
}
