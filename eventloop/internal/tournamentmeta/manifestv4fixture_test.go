package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManifestV4FrozenFixtureProductionLoaders(t *testing.T) {
	data := frozenManifestV4Fixture(t)
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		t.Fatalf("write frozen schema-4 manifest: %v", err)
	}
	if _, _, err := loadManifestSourceAuthority(manifestPath); err != nil {
		t.Fatalf("load frozen schema-4 source authority: %v", err)
	}
	profile, err := loadProfileManifest(manifestPath)
	if err != nil {
		t.Fatalf("load frozen schema-4 profile: %v", err)
	}
	if profile.SchemaVersion != manifestSchemaVersionV4 || len(profile.Lanes) != 4 {
		t.Fatalf("frozen schema-4 profile = schema %d, %d lanes", profile.SchemaVersion, len(profile.Lanes))
	}
}

func TestManifestV4FrozenFixtureRejectsSchemaFiveLaneFields(t *testing.T) {
	var root map[string]any
	if err := json.Unmarshal(frozenManifestV4Fixture(t), &root); err != nil {
		t.Fatal(err)
	}
	lane := root["lanes"].([]any)[0].(map[string]any)
	lane["benchmark_bindings"] = []any{map[string]any{
		"binding_id":        "binding.example.product",
		"implementation_id": "implementation.example.product",
		"module_id":         "eventloop",
	}}
	data, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateSourceManifestJSONShape(data); err == nil {
		t.Fatal("schema-4 manifest with schema-5 lane field unexpectedly passed")
	}
}

func TestManifestV4FrozenFixtureRejectsNoncanonicalLanes(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "case alias", mutate: func(lane map[string]any) {
			lane["Benchmarks"] = lane["benchmarks"]
			delete(lane, "benchmarks")
		}},
		{name: "null array", mutate: func(lane map[string]any) {
			lane["benchmarks"] = nil
		}},
		{name: "missing field", mutate: func(lane map[string]any) {
			delete(lane, "variant_ids")
		}},
		{name: "unknown field", mutate: func(lane map[string]any) {
			lane["unknown"] = true
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var root map[string]any
			if err := json.Unmarshal(frozenManifestV4Fixture(t), &root); err != nil {
				t.Fatal(err)
			}
			test.mutate(root["lanes"].([]any)[0].(map[string]any))
			data, err := json.Marshal(root)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateSourceManifestJSONShape(data); err == nil {
				t.Fatal("noncanonical schema-4 lane unexpectedly passed")
			}
		})
	}
}

func frozenManifestV4Fixture(t *testing.T) []byte {
	t.Helper()
	encoded, err := os.ReadFile("../tournament/testdata/manifest-v4.json.gz.b64")
	if err != nil {
		t.Fatalf("read frozen schema-4 manifest: %v", err)
	}
	compressed, err := base64.StdEncoding.Strict().DecodeString(strings.Join(strings.Fields(string(encoded)), ""))
	if err != nil {
		t.Fatalf("decode frozen schema-4 manifest base64: %v", err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("open frozen schema-4 manifest gzip: %v", err)
	}
	data, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if readErr != nil {
		t.Fatalf("read frozen schema-4 manifest gzip: %v", readErr)
	}
	if closeErr != nil {
		t.Fatalf("close frozen schema-4 manifest gzip: %v", closeErr)
	}
	if len(data) != 64739 {
		t.Fatalf("frozen schema-4 manifest bytes = %d, want 64739", len(data))
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != "118368c2c2d7714920d655145d530f0b17ca90dccae9a62dd43d6905e8d21049" {
		t.Fatalf("frozen schema-4 manifest SHA-256 = %s", got)
	}
	return data
}
