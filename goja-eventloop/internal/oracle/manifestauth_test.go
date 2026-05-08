package oracle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManifestContractAuthenticatesCompleteInputs(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, string, *Manifest)
	}{
		{
			name: "harness bytes and declared hash",
			mutate: func(t *testing.T, root string, manifest *Manifest) {
				mutateManifestAsset(t, root, manifest.Harness.File, &manifest.Harness.SHA256)
			},
		},
		{
			name: "fixture bytes and declared hash",
			mutate: func(t *testing.T, root string, manifest *Manifest) {
				fixture := manifestFixture(t, manifest, "node-timers")
				mutateManifestAsset(t, root, fixture.File, &fixture.SHA256)
			},
		},
		{
			name: "declared hash",
			mutate: func(t *testing.T, _ string, manifest *Manifest) {
				fixture := manifestFixture(t, manifest, "node-timers")
				fixture.SHA256 = strings.Repeat("0", sha256.Size*2)
			},
		},
		{
			name: "semantic expectation",
			mutate: func(t *testing.T, _ string, manifest *Manifest) {
				fixture := manifestFixture(t, manifest, "web-events")
				fixture.Expected = json.RawMessage(`{"contractMutation":true}`)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, manifestPath, manifest := copyManifestContract(t)
			test.mutate(t, root, &manifest)
			writeManifestContract(t, manifestPath, manifest)

			_, err := LoadManifest(manifestPath)
			if err == nil || !strings.Contains(err.Error(), "manifest contract SHA-256") {
				t.Fatalf("LoadManifest error = %v, want manifest contract authentication failure", err)
			}
		})
	}
}

func TestManifestContractIgnoresJSONFormatting(t *testing.T) {
	_, manifestPath, manifest := copyManifestContract(t)
	writeManifestContract(t, manifestPath, manifest)
	if _, err := LoadManifest(manifestPath); err != nil {
		t.Fatalf("LoadManifest reformatted contract: %v", err)
	}
}

func copyManifestContract(t *testing.T) (string, string, Manifest) {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join("..", "..", "testdata", "oracle")
	if err := os.CopyFS(root, os.DirFS(source)); err != nil {
		t.Fatalf("copy manifest contract: %v", err)
	}
	manifestPath := filepath.Join(root, "surface.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest contract: %v", err)
	}
	var manifest Manifest
	if err := decodeStrict(data, &manifest); err != nil {
		t.Fatalf("decode manifest contract: %v", err)
	}
	if _, err := LoadManifest(manifestPath); err != nil {
		t.Fatalf("LoadManifest copied contract: %v", err)
	}
	return root, manifestPath, manifest
}

func mutateManifestAsset(t *testing.T, root, name string, declaredSHA256 *string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read asset %s: %v", name, err)
	}
	data = append(data, []byte("\n// authenticated contract mutation\n")...)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("mutate asset %s: %v", name, err)
	}
	sum := sha256.Sum256(data)
	*declaredSHA256 = hex.EncodeToString(sum[:])
}

func manifestFixture(t *testing.T, manifest *Manifest, id string) *Fixture {
	t.Helper()
	for index := range manifest.Fixtures {
		if manifest.Fixtures[index].ID == id {
			return &manifest.Fixtures[index]
		}
	}
	t.Fatalf("fixture %q is absent", id)
	return nil
}

func writeManifestContract(t *testing.T, path string, manifest Manifest) {
	t.Helper()
	data, err := json.MarshalIndent(manifest, "", "    ")
	if err != nil {
		t.Fatalf("encode manifest contract: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write manifest contract: %v", err)
	}
}
