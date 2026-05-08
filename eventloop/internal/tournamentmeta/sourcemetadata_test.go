package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSourceMetadataSchemaCompatibility(t *testing.T) {
	repository, config := testGovernedSourceRepository(t)
	snapshot := filepath.Join(t.TempDir(), "snapshot")
	metadata, err := createSnapshotBuild(repository, snapshot, config)
	if err != nil {
		t.Fatalf("createSnapshotBuild: %v", err)
	}
	metadataPath := filepath.Join(snapshot, filepath.FromSlash(sourceMetadataPath))
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("ReadFile source metadata: %v", err)
	}

	t.Run("schema 5 round trip", func(t *testing.T) {
		if metadata.SchemaVersion != sourceMetadataSchemaVersion ||
			metadata.Fingerprint != metadata.LegacyV4Fingerprint ||
			!reflect.DeepEqual(metadata.LogicalAuthority, logicalSourceAuthority(metadata.Authority)) ||
			!reflect.DeepEqual(metadata.CaptureAuthority, sourceCaptureAuthority{
				Policy: sourceCapturePolicy,
				GoTool: metadata.Authority.GoTool,
			}) {
			t.Fatalf("source metadata = %+v, want normalized schema 5", metadata)
		}
		loaded, err := readSourceMetadata(metadataPath)
		if err != nil {
			t.Fatalf("readSourceMetadata: %v", err)
		}
		if !reflect.DeepEqual(loaded, metadata) {
			t.Fatalf("loaded source metadata = %+v, want %+v", loaded, metadata)
		}
		encoded, err := json.MarshalIndent(metadata, "", "  ")
		if err != nil {
			t.Fatalf("MarshalIndent source metadata: %v", err)
		}
		encoded = append(encoded, '\n')
		if !bytes.Equal(encoded, data) {
			t.Fatal("schema-5 source metadata is not canonical writer output")
		}
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(data, &raw); err != nil {
			t.Fatalf("Unmarshal source metadata object: %v", err)
		}
		if len(raw) != 9 || raw["authority"] != nil || raw["fingerprint"] != nil {
			t.Fatalf("schema-5 source metadata keys = %v", raw)
		}
	})

	t.Run("linked identity tampering", func(t *testing.T) {
		for _, test := range []struct {
			name   string
			mutate func(*sourceMetadata)
		}{
			{name: "shared source", mutate: func(value *sourceMetadata) { value.SharedSourceID = zeroSHA256 }},
			{name: "capture", mutate: func(value *sourceMetadata) { value.CaptureID = zeroSHA256 }},
			{name: "capture authority", mutate: func(value *sourceMetadata) {
				value.CaptureAuthoritySHA256 = zeroSHA256
			}},
			{name: "legacy v4", mutate: func(value *sourceMetadata) { value.LegacyV4Fingerprint = zeroSHA256 }},
			{name: "logical authority", mutate: func(value *sourceMetadata) {
				value.LogicalAuthority.ManifestSHA256 = zeroSHA256
			}},
			{name: "capture tool", mutate: func(value *sourceMetadata) {
				value.CaptureAuthority.GoTool.GOHostOS += "-changed"
			}},
			{name: "file record", mutate: func(value *sourceMetadata) { value.Files[0].SHA256 = zeroSHA256 }},
		} {
			t.Run(test.name, func(t *testing.T) {
				changed := cloneSourceMetadata(t, metadata)
				test.mutate(&changed)
				path := writeSourceMetadataCase(t, changed)
				if _, err := readSourceMetadata(path); err == nil {
					t.Fatal("tampered source metadata unexpectedly passed")
				}
			})
		}
		for _, field := range []struct {
			name string
			set  func(*sourceMetadata, string)
		}{
			{name: "shared source", set: func(value *sourceMetadata, digest string) { value.SharedSourceID = digest }},
			{name: "capture", set: func(value *sourceMetadata, digest string) { value.CaptureID = digest }},
			{name: "capture authority", set: func(value *sourceMetadata, digest string) {
				value.CaptureAuthoritySHA256 = digest
			}},
			{name: "legacy v4", set: func(value *sourceMetadata, digest string) {
				value.LegacyV4Fingerprint = digest
			}},
		} {
			for _, format := range []struct {
				name  string
				value string
			}{
				{name: "uppercase", value: strings.Repeat("A", 64)},
				{name: "short", value: strings.Repeat("0", 63)},
			} {
				t.Run(field.name+" "+format.name, func(t *testing.T) {
					changed := cloneSourceMetadata(t, metadata)
					field.set(&changed, format.value)
					path := writeSourceMetadataCase(t, changed)
					if _, err := readSourceMetadata(path); err == nil {
						t.Fatal("noncanonical source identity unexpectedly passed")
					}
				})
			}
		}
		t.Run("swapped identities", func(t *testing.T) {
			changed := cloneSourceMetadata(t, metadata)
			changed.SharedSourceID, changed.CaptureID = changed.CaptureID, changed.SharedSourceID
			path := writeSourceMetadataCase(t, changed)
			if _, err := readSourceMetadata(path); err == nil {
				t.Fatal("swapped source identities unexpectedly passed")
			}
		})
	})

	t.Run("schema dispatch", func(t *testing.T) {
		for _, test := range []struct {
			name        string
			replacement string
		}{
			{name: "legacy label", replacement: `"schema_version": 4`},
			{name: "unsupported old", replacement: `"schema_version": 3`},
			{name: "unsupported new", replacement: `"schema_version": 6`},
			{name: "string", replacement: `"schema_version": "5"`},
			{name: "null", replacement: `"schema_version": null`},
		} {
			t.Run(test.name, func(t *testing.T) {
				mutated := replaceSourceMetadataOnce(
					t,
					data,
					[]byte(`"schema_version": 5`),
					[]byte(test.replacement),
				)
				path := filepath.Join(t.TempDir(), "source.json")
				mustWriteFile(t, path, mutated, 0o600)
				if _, err := readSourceMetadata(path); err == nil {
					t.Fatal("mislabeled source metadata unexpectedly passed")
				}
			})
		}
	})

	t.Run("schema 5 exact fields", func(t *testing.T) {
		for _, test := range []struct {
			name        string
			needle      []byte
			replacement []byte
		}{
			{
				name:        "top case alias",
				needle:      []byte(`"capture_id": "`),
				replacement: []byte(`"Capture_ID": "`),
			},
			{
				name:        "top null",
				needle:      []byte(`"capture_id": "` + metadata.CaptureID + `"`),
				replacement: []byte(`"capture_id": null`),
			},
			{
				name:        "logical case alias",
				needle:      []byte(`"build_vcs": false`),
				replacement: []byte(`"Build_VCS": false`),
			},
			{
				name:        "logical null",
				needle:      []byte(`"build_vcs": false`),
				replacement: []byte(`"build_vcs": null`),
			},
			{
				name:        "capture case alias",
				needle:      []byte(`"policy": "` + sourceCapturePolicy + `"`),
				replacement: []byte(`"Policy": "` + sourceCapturePolicy + `"`),
			},
			{
				name:        "capture null",
				needle:      []byte(`"policy": "` + sourceCapturePolicy + `"`),
				replacement: []byte(`"policy": null`),
			},
			{
				name:        "Go tool case alias",
				needle:      []byte(`"go_host_os": "` + metadata.Authority.GoTool.GOHostOS + `"`),
				replacement: []byte(`"Go_Host_OS": "` + metadata.Authority.GoTool.GOHostOS + `"`),
			},
			{
				name:        "Go tool null",
				needle:      []byte(`"go_host_os": "` + metadata.Authority.GoTool.GOHostOS + `"`),
				replacement: []byte(`"go_host_os": null`),
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				mutated := replaceSourceMetadataOnce(t, data, test.needle, test.replacement)
				path := filepath.Join(t.TempDir(), "source.json")
				mustWriteFile(t, path, mutated, 0o600)
				if _, err := readSourceMetadata(path); err == nil ||
					(!strings.Contains(err.Error(), "keys") && !strings.Contains(err.Error(), "null")) {
					t.Fatalf("readSourceMetadata error = %v, want exact-field rejection", err)
				}
			})
		}
	})

	t.Run("governed schema 4 read only", func(t *testing.T) {
		legacy := sourceMetadataV4{
			SchemaVersion: sourceMetadataLegacySchemaVersion,
			Fingerprint:   metadata.Fingerprint,
			Authority:     sourceAuthorityV4Value(metadata.Authority),
			FileCount:     metadata.FileCount,
			Files:         metadata.Files,
		}
		legacyData, err := json.MarshalIndent(legacy, "", "  ")
		if err != nil {
			t.Fatalf("MarshalIndent schema-4 source metadata: %v", err)
		}
		legacyData = append(legacyData, '\n')
		if err := os.WriteFile(metadataPath, legacyData, 0o600); err != nil {
			t.Fatalf("WriteFile schema-4 source metadata: %v", err)
		}
		loaded, err := readSourceMetadata(metadataPath)
		if err != nil {
			t.Fatalf("readSourceMetadata schema 4: %v", err)
		}
		if loaded.SchemaVersion != sourceMetadataLegacySchemaVersion ||
			loaded.Fingerprint != metadata.Fingerprint ||
			!reflect.DeepEqual(loaded.Authority, metadata.Authority) ||
			!reflect.DeepEqual(loaded.Files, metadata.Files) {
			t.Fatalf("loaded schema-4 source metadata = %+v", loaded)
		}
		if code := sourceFingerprintCommand([]string{"-root", snapshot, "-metadata", metadataPath}); code != 0 {
			t.Fatalf("sourceFingerprintCommand schema 4 code = %d", code)
		}
		after, err := os.ReadFile(metadataPath)
		if err != nil {
			t.Fatalf("ReadFile schema-4 source metadata after verification: %v", err)
		}
		if !bytes.Equal(after, legacyData) {
			t.Fatal("schema-4 verification rewrote source metadata")
		}
		for _, test := range []struct {
			name        string
			needle      []byte
			replacement []byte
		}{
			{
				name:        "top case alias",
				needle:      []byte(`"fingerprint": "`),
				replacement: []byte(`"Fingerprint": "`),
			},
			{
				name:        "authority case alias",
				needle:      []byte(`"build_vcs": false`),
				replacement: []byte(`"Build_VCS": false`),
			},
			{
				name:        "authority null",
				needle:      []byte(`"build_vcs": false`),
				replacement: []byte(`"build_vcs": null`),
			},
			{
				name:   "authority missing",
				needle: []byte(",\n    \"build_vcs\": false"),
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				mutated := replaceSourceMetadataOnce(t, legacyData, test.needle, test.replacement)
				path := filepath.Join(t.TempDir(), "source.json")
				mustWriteFile(t, path, mutated, 0o600)
				if _, err := readSourceMetadata(path); err == nil ||
					(!strings.Contains(err.Error(), "keys") && !strings.Contains(err.Error(), "null")) {
					t.Fatalf("readSourceMetadata schema-4 shape error = %v", err)
				}
			})
		}
	})
}

func cloneSourceMetadata(t *testing.T, metadata sourceMetadata) sourceMetadata {
	t.Helper()
	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("Marshal source metadata clone: %v", err)
	}
	var result sourceMetadata
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Unmarshal source metadata clone: %v", err)
	}
	return result
}

func writeSourceMetadataCase(t *testing.T, metadata sourceMetadata) string {
	t.Helper()
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent source metadata case: %v", err)
	}
	path := filepath.Join(t.TempDir(), "source.json")
	mustWriteFile(t, path, append(data, '\n'), 0o600)
	return path
}

func replaceSourceMetadataOnce(t *testing.T, data, needle, replacement []byte) []byte {
	t.Helper()
	if count := bytes.Count(data, needle); count != 1 {
		t.Fatalf("source metadata mutation target %q count = %d, want 1", needle, count)
	}
	return bytes.Replace(data, needle, replacement, 1)
}

const zeroSHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
