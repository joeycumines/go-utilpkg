package tournament

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
)

//go:embed testdata/manifest-v4.json.gz.b64
var frozenManifestV4Base64 string

func TestManifestV4FrozenFixtureDecodes(t *testing.T) {
	data := frozenManifestV4(t)
	manifest, err := decodeManifest(data)
	if err != nil {
		t.Fatalf("decode frozen schema-4 manifest: %v", err)
	}
	if manifest.SchemaVersion != 4 || len(manifest.Lanes) != 4 || len(manifest.Variants) == 0 {
		t.Fatalf("frozen schema-4 manifest projection = schema %d, %d lanes, %d variants", manifest.SchemaVersion, len(manifest.Lanes), len(manifest.Variants))
	}
}

func TestManifestV4FrozenFixtureRejectsSchemaFiveLaneFields(t *testing.T) {
	var root map[string]any
	if err := json.Unmarshal(frozenManifestV4(t), &root); err != nil {
		t.Fatal(err)
	}
	lane := root["lanes"].([]any)[0].(map[string]any)
	lane["build_cell_ids"] = []any{"eventloop.darwin-amd64"}
	data, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeManifest(data); err == nil {
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
			lane["variant_ids"] = nil
		}},
		{name: "missing field", mutate: func(lane map[string]any) {
			delete(lane, "workload_definitions")
		}},
		{name: "unknown field", mutate: func(lane map[string]any) {
			lane["unknown"] = true
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var root map[string]any
			if err := json.Unmarshal(frozenManifestV4(t), &root); err != nil {
				t.Fatal(err)
			}
			test.mutate(root["lanes"].([]any)[0].(map[string]any))
			data, err := json.Marshal(root)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := decodeManifest(data); err == nil {
				t.Fatal("noncanonical schema-4 lane unexpectedly passed")
			}
		})
	}
}

func frozenManifestV4(t *testing.T) []byte {
	t.Helper()
	encoded := strings.Join(strings.Fields(frozenManifestV4Base64), "")
	compressed, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode frozen schema-4 manifest base64: %v", err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("open frozen schema-4 manifest gzip: %v", err)
	}
	data, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if err := errorsJoinManifestV4(readErr, closeErr); err != nil {
		t.Fatalf("decompress frozen schema-4 manifest: %v", err)
	}
	if len(data) != 64739 {
		t.Fatalf("frozen schema-4 manifest bytes = %d, want 64739", len(data))
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != "118368c2c2d7714920d655145d530f0b17ca90dccae9a62dd43d6905e8d21049" {
		t.Fatalf("frozen schema-4 manifest SHA-256 = %s", got)
	}
	return data
}

func errorsJoinManifestV4(values ...error) error {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}
