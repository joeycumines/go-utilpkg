package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadManifestSourceAuthority(t *testing.T) {
	authority, digest, err := loadManifestSourceAuthority("../tournament/manifest.json")
	if err != nil {
		t.Fatalf("loadManifestSourceAuthority: %v", err)
	}
	if len(authority.Modules) != 5 || len(authority.BuildCells) != 40 || len(authority.PhysicalPolicy.RuntimeAssets) != 3 {
		t.Fatalf("source authority shape = %d modules, %d cells, %d runtime assets", len(authority.Modules), len(authority.BuildCells), len(authority.PhysicalPolicy.RuntimeAssets))
	}
	if len(digest) != 64 || strings.ToLower(digest) != digest {
		t.Fatalf("source authority digest = %q", digest)
	}
	changed := cloneManifestSourceAuthority(t, authority)
	changed.BuildCells[0].PackagePatterns = []string{"./internal/alternateone"}
	changedDigest, err := manifestSourceAuthorityDigest(changed)
	if err != nil {
		t.Fatalf("manifestSourceAuthorityDigest changed: %v", err)
	}
	if changedDigest == digest {
		t.Fatal("source-authority mutation did not change digest")
	}
}

func TestManifestSourceAuthorityRejectsMissingFalseValues(t *testing.T) {
	authority, _, err := loadManifestSourceAuthority("../tournament/manifest.json")
	if err != nil {
		t.Fatalf("loadManifestSourceAuthority: %v", err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*manifestSourceAuthority)
		want   string
	}{
		{
			name: "module buildable",
			mutate: func(value *manifestSourceAuthority) {
				value.Modules[0].Buildable = nil
			},
			want: "omits buildable",
		},
		{
			name: "cell cgo enabled",
			mutate: func(value *manifestSourceAuthority) {
				value.BuildCells[0].CGOEnabled = nil
			},
			want: "cgo_enabled is missing",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := cloneManifestSourceAuthority(t, authority)
			test.mutate(&changed)
			if err := validateManifestSourceAuthority(changed); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateManifestSourceAuthority error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestManifestSourceAuthorityRejectsInvalidRelations(t *testing.T) {
	authority, _, err := loadManifestSourceAuthority("../tournament/manifest.json")
	if err != nil {
		t.Fatalf("loadManifestSourceAuthority: %v", err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*manifestSourceAuthority)
		want   string
	}{
		{
			name: "null tags",
			mutate: func(value *manifestSourceAuthority) {
				value.BuildCells[0].BuildTags = nil
			},
			want: "must be non-null",
		},
		{
			name: "unknown module",
			mutate: func(value *manifestSourceAuthority) {
				value.BuildCells[0].ModuleID = "missing"
			},
			want: "missing or control-only",
		},
		{
			name: "architecture",
			mutate: func(value *manifestSourceAuthority) {
				value.BuildCells[0].ArchitectureFeature.Value = "v4"
			},
			want: "architecture feature",
		},
		{
			name: "unsorted cells",
			mutate: func(value *manifestSourceAuthority) {
				value.BuildCells[0], value.BuildCells[1] = value.BuildCells[1], value.BuildCells[0]
			},
			want: "strictly sorted",
		},
		{
			name: "libuv without cgo",
			mutate: func(value *manifestSourceAuthority) {
				for index := range value.BuildCells {
					if strings.HasSuffix(value.BuildCells[index].ID, "-libuv") {
						value.BuildCells[index].CGOEnabled = new(false)
						return
					}
				}
			},
			want: "libuv cell",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := cloneManifestSourceAuthority(t, authority)
			test.mutate(&changed)
			if err := validateManifestSourceAuthority(changed); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateManifestSourceAuthority error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestLoadManifestSourceAuthorityRejectsUnknownAndDuplicateJSON(t *testing.T) {
	data, err := os.ReadFile("../tournament/manifest.json")
	if err != nil {
		t.Fatalf("ReadFile manifest: %v", err)
	}
	for _, test := range []struct {
		name string
		data []byte
		want string
	}{
		{
			name: "unknown",
			data: []byte(strings.Replace(string(data), `"policy": "manifest-build-cells-v1",`, `"policy": "manifest-build-cells-v1", "unknown": true,`, 1)),
			want: "keys =",
		},
		{
			name: "case alias",
			data: []byte(strings.Replace(string(data), `"policy": "manifest-build-cells-v1",`, `"Policy": "manifest-build-cells-v1",`, 1)),
			want: "keys =",
		},
		{
			name: "duplicate",
			data: []byte(strings.Replace(string(data), `"policy": "manifest-build-cells-v1",`, `"policy": "manifest-build-cells-v1", "policy": "manifest-build-cells-v1",`, 1)),
			want: "duplicate key",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "manifest.json")
			mustWriteFile(t, path, test.data, 0o644)
			if _, _, err := loadManifestSourceAuthority(path); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("loadManifestSourceAuthority error = %v, want %q", err, test.want)
			}
		})
	}
}

func cloneManifestSourceAuthority(t *testing.T, authority manifestSourceAuthority) manifestSourceAuthority {
	t.Helper()
	data, err := json.Marshal(authority)
	if err != nil {
		t.Fatalf("Marshal source authority: %v", err)
	}
	var result manifestSourceAuthority
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Unmarshal source authority: %v", err)
	}
	return result
}
