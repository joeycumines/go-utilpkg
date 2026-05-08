package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
)

const (
	sourceMetadataLegacySchemaVersion = 4
	sourceMetadataSchemaVersion       = 5
)

type sourceAuthorityV4 struct { // betteralign:ignore canonical JSON field order
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

func sourceAuthorityV4Value(authority sourceAuthority) sourceAuthorityV4 {
	return sourceAuthorityV4(authority)
}

func (authority sourceAuthorityV4) current() sourceAuthority {
	return sourceAuthority(authority)
}

func newSourceMetadata(
	authority sourceAuthority,
	files []sourceRecord,
	identity sourceIdentity,
) (sourceMetadata, error) {
	if authority.EnumerationPolicy == testSourcePolicy {
		if identity.SchemaVersion != 0 || identity.SharedSourceID != "" || identity.CaptureID != "" ||
			identity.CaptureAuthoritySHA256 != "" {
			return sourceMetadata{}, errors.New("fixture source identity contains governed fields")
		}
		if err := validateCanonicalSHA256(identity.LegacyV4Fingerprint, "fixture source fingerprint"); err != nil {
			return sourceMetadata{}, err
		}
		return sourceMetadata{
			SchemaVersion:       sourceMetadataLegacySchemaVersion,
			LegacyV4Fingerprint: identity.LegacyV4Fingerprint,
			FileCount:           len(files),
			Files:               files,
			Fingerprint:         identity.LegacyV4Fingerprint,
			Authority:           authority,
		}, nil
	}
	if err := validatePersistedSourceAuthority(authority); err != nil {
		return sourceMetadata{}, err
	}
	want, err := identifySource(authority, files)
	if err != nil {
		return sourceMetadata{}, err
	}
	if !reflect.DeepEqual(identity, want) {
		return sourceMetadata{}, errors.New("source identity differs from authority and records")
	}
	return sourceMetadata{
		SchemaVersion:          sourceMetadataSchemaVersion,
		SharedSourceID:         identity.SharedSourceID,
		CaptureID:              identity.CaptureID,
		CaptureAuthoritySHA256: identity.CaptureAuthoritySHA256,
		LegacyV4Fingerprint:    identity.LegacyV4Fingerprint,
		LogicalAuthority:       logicalSourceAuthority(authority),
		CaptureAuthority: sourceCaptureAuthority{
			Policy: sourceCapturePolicy,
			GoTool: authority.GoTool,
		},
		FileCount:   len(files),
		Files:       files,
		Fingerprint: identity.LegacyV4Fingerprint,
		Authority:   authority,
	}, nil
}

func validateSourceMetadataFile(root, metadataPath string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve snapshot metadata root: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve snapshot metadata root links: %w", err)
	}
	metadataPath, err = filepath.Abs(metadataPath)
	if err != nil {
		return fmt.Errorf("resolve snapshot metadata path: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(metadataPath)
	if err != nil {
		return fmt.Errorf("resolve snapshot metadata links: %w", err)
	}
	want := filepath.Join(root, filepath.FromSlash(sourceMetadataPath))
	if resolved != want {
		return fmt.Errorf("snapshot metadata resolves to %q, want %q", resolved, want)
	}
	info, err := os.Lstat(metadataPath)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("snapshot metadata is not a physical regular file: %q", metadataPath)
	}
	return nil
}

func readSourceMetadata(path string) (sourceMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return sourceMetadata{}, fmt.Errorf("read source metadata: %w", err)
	}
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return sourceMetadata{}, fmt.Errorf("validate source metadata JSON: %w", err)
	}
	schemaVersion, err := validateSourceMetadataJSONShape(data)
	if err != nil {
		return sourceMetadata{}, fmt.Errorf("validate source metadata shape: %w", err)
	}
	switch schemaVersion {
	case sourceMetadataLegacySchemaVersion:
		return readSourceMetadataV4(data)
	case sourceMetadataSchemaVersion:
		return readSourceMetadataV5(data)
	default:
		return sourceMetadata{}, fmt.Errorf("source metadata schema = %d is unsupported", schemaVersion)
	}
}

func readSourceMetadataV4(data []byte) (sourceMetadata, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var legacy sourceMetadataV4
	if err := decoder.Decode(&legacy); err != nil {
		return sourceMetadata{}, fmt.Errorf("decode source metadata schema 4: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return sourceMetadata{}, errors.New("source metadata has trailing JSON")
	}
	if legacy.SchemaVersion != sourceMetadataLegacySchemaVersion ||
		legacy.FileCount != len(legacy.Files) || len(legacy.Files) == 0 {
		return sourceMetadata{}, errors.New("source metadata has inconsistent schema or file count")
	}
	authority := legacy.Authority.current()
	if err := validatePersistedSourceAuthority(authority); err != nil {
		return sourceMetadata{}, fmt.Errorf("source metadata authority: %w", err)
	}
	if err := validateCanonicalSHA256(legacy.Fingerprint, "source metadata fingerprint"); err != nil {
		return sourceMetadata{}, err
	}
	fingerprint, err := fingerprintSource(authority, legacy.Files)
	if err != nil {
		return sourceMetadata{}, fmt.Errorf("validate source metadata records: %w", err)
	}
	if fingerprint != legacy.Fingerprint {
		return sourceMetadata{}, fmt.Errorf(
			"source metadata fingerprint %s != records %s",
			legacy.Fingerprint,
			fingerprint,
		)
	}
	return sourceMetadata{
		SchemaVersion:       sourceMetadataLegacySchemaVersion,
		LegacyV4Fingerprint: legacy.Fingerprint,
		FileCount:           legacy.FileCount,
		Files:               legacy.Files,
		Fingerprint:         legacy.Fingerprint,
		Authority:           authority,
	}, nil
}

func readSourceMetadataV5(data []byte) (sourceMetadata, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var metadata sourceMetadata
	if err := decoder.Decode(&metadata); err != nil {
		return sourceMetadata{}, fmt.Errorf("decode source metadata schema 5: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return sourceMetadata{}, errors.New("source metadata has trailing JSON")
	}
	if metadata.SchemaVersion != sourceMetadataSchemaVersion ||
		metadata.FileCount != len(metadata.Files) || len(metadata.Files) == 0 {
		return sourceMetadata{}, errors.New("source metadata has inconsistent schema or file count")
	}
	if err := validateSourceCaptureAuthority(metadata.CaptureAuthority); err != nil {
		return sourceMetadata{}, fmt.Errorf("source metadata capture authority: %w", err)
	}
	authority := combineSourceAuthority(metadata.LogicalAuthority, metadata.CaptureAuthority)
	if err := validatePersistedSourceAuthority(authority); err != nil {
		return sourceMetadata{}, fmt.Errorf("source metadata authority: %w", err)
	}
	identity, err := identifySource(authority, metadata.Files)
	if err != nil {
		return sourceMetadata{}, fmt.Errorf("validate source metadata records: %w", err)
	}
	if err := validateRecordedSourceIdentity(metadata, identity); err != nil {
		return sourceMetadata{}, err
	}
	metadata.Fingerprint = metadata.LegacyV4Fingerprint
	metadata.Authority = authority
	return metadata, nil
}

func validateRecordedSourceIdentity(metadata sourceMetadata, want sourceIdentity) error {
	fields := []struct {
		description string
		got         string
		want        string
	}{
		{"shared source ID", metadata.SharedSourceID, want.SharedSourceID},
		{"capture ID", metadata.CaptureID, want.CaptureID},
		{"capture authority SHA-256", metadata.CaptureAuthoritySHA256, want.CaptureAuthoritySHA256},
		{"legacy v4 fingerprint", metadata.LegacyV4Fingerprint, want.LegacyV4Fingerprint},
	}
	for _, field := range fields {
		if err := validateCanonicalSHA256(field.got, "source metadata "+field.description); err != nil {
			return err
		}
		if field.got != field.want {
			return fmt.Errorf("source metadata %s %s != records %s", field.description, field.got, field.want)
		}
	}
	return nil
}

func validateSourceMetadataJSONShape(data []byte) (int, error) {
	metadata, err := decodeSourceRawObject(data, "source metadata")
	if err != nil {
		return 0, err
	}
	rawSchema, exists := metadata["schema_version"]
	if !exists {
		return 0, errors.New("source metadata keys omit schema_version")
	}
	if bytes.Equal(bytes.TrimSpace(rawSchema), []byte("null")) {
		return 0, errors.New("source metadata field \"schema_version\" is null")
	}
	var schemaVersion int
	if err := json.Unmarshal(rawSchema, &schemaVersion); err != nil {
		return 0, errors.New("source metadata schema_version must be an integer")
	}
	var authorityKey string
	switch schemaVersion {
	case sourceMetadataLegacySchemaVersion:
		if err := requireSourceRawKeys(metadata, []string{
			"authority", "file_count", "files", "fingerprint", "schema_version",
		}, "source metadata"); err != nil {
			return 0, err
		}
		authorityKey = "authority"
	case sourceMetadataSchemaVersion:
		if err := requireSourceRawKeys(metadata, []string{
			"capture_authority", "capture_authority_sha256", "capture_id", "file_count", "files",
			"legacy_v4_fingerprint", "logical_authority", "schema_version", "shared_source_id",
		}, "source metadata"); err != nil {
			return 0, err
		}
		authorityKey = "logical_authority"
	default:
		return 0, fmt.Errorf("source metadata schema = %d is unsupported", schemaVersion)
	}
	if err := rejectNullSourceFields(metadata, "source metadata"); err != nil {
		return 0, err
	}
	if err := validateSourceAuthorityJSONShape(
		metadata[authorityKey],
		"source metadata "+authorityKey,
		schemaVersion == sourceMetadataLegacySchemaVersion,
	); err != nil {
		return 0, err
	}
	if schemaVersion == sourceMetadataSchemaVersion {
		if err := validateSourceCaptureAuthorityJSONShape(metadata["capture_authority"]); err != nil {
			return 0, err
		}
	}
	if err := validateSourceRecordsJSONShape(metadata["files"]); err != nil {
		return 0, err
	}
	return schemaVersion, nil
}

func validateSourceAuthorityJSONShape(data []byte, description string, includesGoTool bool) error {
	authority, err := decodeSourceRawObject(data, description)
	if err != nil {
		return err
	}
	expected := []string{
		"build_cells", "build_union", "build_vcs", "enumeration_policy",
		"environment_policy", "governed_union", "manifest_path", "manifest_sha256",
		"manifest_source_authority_sha256", "module_mode", "physical_paths", "proxy_mode",
		"toolchain_mode", "workspace_mode",
	}
	if includesGoTool {
		expected = []string{
			"build_cells", "build_union", "build_vcs", "enumeration_policy",
			"environment_policy", "go_tool", "governed_union", "manifest_path",
			"manifest_sha256", "manifest_source_authority_sha256", "module_mode",
			"physical_paths", "proxy_mode", "toolchain_mode", "workspace_mode",
		}
	}
	if err := requireSourceRawKeys(authority, expected, description); err != nil {
		return err
	}
	if err := rejectNullSourceFields(authority, description); err != nil {
		return err
	}
	if includesGoTool {
		if err := validateSourceGoToolJSONShape(authority["go_tool"], description+" Go tool"); err != nil {
			return err
		}
	}
	var cells []json.RawMessage
	if err := json.Unmarshal(authority["build_cells"], &cells); err != nil || cells == nil {
		return fmt.Errorf("%s build_cells must be a non-null array", description)
	}
	for index, raw := range cells {
		cellDescription := fmt.Sprintf("%s build_cells[%d]", description, index)
		cell, err := decodeSourceRawObject(raw, cellDescription)
		if err != nil {
			return err
		}
		if err := requireSourceRawKeys(cell, []string{
			"argv", "environment", "id", "repository_paths",
		}, cellDescription); err != nil {
			return err
		}
		if err := rejectNullSourceFields(cell, cellDescription); err != nil {
			return err
		}
		if err := validateSourcePathSetJSONShape(
			cell["repository_paths"],
			cellDescription+" repository_paths",
		); err != nil {
			return err
		}
	}
	for _, name := range []string{"physical_paths", "build_union", "governed_union"} {
		if err := validateSourcePathSetJSONShape(authority[name], description+" "+name); err != nil {
			return err
		}
	}
	return nil
}

func validateSourceCaptureAuthorityJSONShape(data []byte) error {
	const description = "source metadata capture_authority"
	authority, err := decodeSourceRawObject(data, description)
	if err != nil {
		return err
	}
	if err := requireSourceRawKeys(authority, []string{"go_tool", "policy"}, description); err != nil {
		return err
	}
	if err := rejectNullSourceFields(authority, description); err != nil {
		return err
	}
	return validateSourceGoToolJSONShape(authority["go_tool"], description+" Go tool")
}

func validateSourceGoToolJSONShape(data []byte, description string) error {
	tool, err := decodeSourceRawObject(data, description)
	if err != nil {
		return err
	}
	if err := requireSourceRawKeys(tool, []string{
		"executable_sha256", "go_host_arch", "go_host_os", "go_version", "version_output",
	}, description); err != nil {
		return err
	}
	return rejectNullSourceFields(tool, description)
}

func validateSourceRecordsJSONShape(data []byte) error {
	var records []json.RawMessage
	if err := json.Unmarshal(data, &records); err != nil || records == nil {
		return errors.New("source metadata files must be a non-null array")
	}
	for index, raw := range records {
		description := fmt.Sprintf("source metadata files[%d]", index)
		record, err := decodeSourceRawObject(raw, description)
		if err != nil {
			return err
		}
		expected := []string{"mode", "path", "sha256", "size"}
		if _, exists := record["symlink_target"]; exists {
			expected = append(expected, "symlink_target")
		}
		if err := requireSourceRawKeys(record, expected, description); err != nil {
			return err
		}
		if err := rejectNullSourceFields(record, description); err != nil {
			return err
		}
	}
	return nil
}

func validateSourcePathSetJSONShape(data []byte, description string) error {
	pathSet, err := decodeSourceRawObject(data, description)
	if err != nil {
		return err
	}
	if err := requireSourceRawKeys(pathSet, []string{"count", "paths", "sha256"}, description); err != nil {
		return err
	}
	return rejectNullSourceFields(pathSet, description)
}

func rejectNullSourceFields(object map[string]json.RawMessage, description string) error {
	for key, value := range object {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("%s field %q is null", description, key)
		}
	}
	return nil
}
