package gojaeventloop_test

import (
	"path/filepath"
	"testing"

	"github.com/joeycumines/goja-eventloop/internal/oracle"
)

func TestOracleManifestAuthenticatesEveryDeclaredCase(t *testing.T) {
	manifest, err := oracle.LoadManifest(filepath.Join("testdata", "oracle", "surface.json"))
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if len(manifest.Manifest.Fixtures) == 0 || len(manifest.Manifest.Surfaces) == 0 {
		t.Fatal("oracle manifest has an empty declared catalog")
	}
	for _, fixture := range manifest.Manifest.Fixtures {
		t.Run(fixture.ID, func(t *testing.T) {
			if len(manifest.Fixtures[fixture.ID]) == 0 {
				t.Fatal("declared fixture has no authenticated bytes")
			}
		})
	}
}
