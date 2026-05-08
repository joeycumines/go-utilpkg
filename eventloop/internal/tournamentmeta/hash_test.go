package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSHA256Reader(t *testing.T) {
	for _, test := range []struct {
		name string
		data []byte
		want string
	}{
		{
			name: "abc",
			data: []byte("abc"),
			want: "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
		},
		{
			name: "binary",
			data: []byte{0, 1, 0, 255},
			want: "35405bce5dc7cacedd9c4373e68d01b369da4b5da34ecf90d0db265416797f71",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := sha256Reader(bytes.NewReader(test.data))
			if err != nil {
				t.Fatalf("sha256Reader: %v", err)
			}
			if got != test.want {
				t.Fatalf("digest = %s, want %s", got, test.want)
			}
		})
	}
}

func TestSHA256StableFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input")
	mustWriteFile(t, path, []byte("abc"), 0o644)
	if got, err := sha256StableFile(path, nil); err != nil || got != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatalf("sha256StableFile = %q, %v", got, err)
	}
	if err := os.Symlink("input", path+"-link"); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	if _, err := sha256StableFile(path+"-link", nil); err == nil {
		t.Fatal("symlink SHA-256 input unexpectedly passed")
	}
	if _, err := sha256StableFile(path, func() {
		mustWriteFile(t, path, []byte("changed"), 0o644)
	}); err == nil || !strings.Contains(err.Error(), "changed while hashing") {
		t.Fatalf("mutation error = %v", err)
	}
}
