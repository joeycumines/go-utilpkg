package libuvbaseline

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
)

func TestLegacySourceRetention(t *testing.T) {
	wants := map[string]string{
		"libuv_cgo.go":        "402a3a26e9105c62da472a0f86e9f731a51f5e138048bbe12a1c75b2f956cc14",
		"libuv_bench_test.go": "a21dff7131a910dfe83d0df81ead3e06031848f2d13f6ee35a317ee33cea485e",
	}
	for name, want := range wants {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Errorf("read legacy libuv source %s: %v", name, err)
			continue
		}
		digest := sha256.Sum256(data)
		if got := hex.EncodeToString(digest[:]); got != want {
			t.Errorf("legacy libuv source %s SHA-256 = %s, want %s", name, got, want)
		}
	}
}
