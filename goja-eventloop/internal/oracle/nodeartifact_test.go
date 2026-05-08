package oracle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type nodeTestArchiveEntry struct {
	name     string
	typeflag byte
	data     []byte
}

func TestSelectedNodeArtifactPinMatrix(t *testing.T) {
	want := map[string]string{
		"darwin/arm64": "node-v26.5.0-darwin-arm64.tar.gz",
		"darwin/amd64": "node-v26.5.0-darwin-x64.tar.gz",
		"linux/arm64":  "node-v26.5.0-linux-arm64.tar.gz",
		"linux/amd64":  "node-v26.5.0-linux-x64.tar.gz",
	}
	if len(nodeArtifactPins) != len(want) {
		t.Fatalf("authenticated Node pin count = %d, want %d", len(nodeArtifactPins), len(want))
	}
	for platform, file := range want {
		goos, goarch, ok := strings.Cut(platform, "/")
		if !ok {
			t.Fatalf("invalid test platform %q", platform)
		}
		pin, err := selectedNodeArtifactPin(goos, goarch)
		if err != nil {
			t.Fatalf("select %s: %v", platform, err)
		}
		if pin.GOOS != goos || pin.GOARCH != goarch || pin.File != file {
			t.Errorf("select %s = %+v, want file %q", platform, pin, file)
		}
	}
	for _, platform := range []struct {
		goos   string
		goarch string
	}{
		{goos: "darwin", goarch: "386"},
		{goos: "linux", goarch: "ppc64le"},
		{goos: "windows", goarch: "amd64"},
		{goos: "freebsd", goarch: "arm64"},
	} {
		if _, err := selectedNodeArtifactPin(platform.goos, platform.goarch); err == nil {
			t.Errorf("unsupported platform %s/%s selected a pin", platform.goos, platform.goarch)
		}
	}
}

func TestSelectNodeLaunchMode(t *testing.T) {
	for _, test := range []struct {
		name   string
		goos   string
		procFS bool
		want   string
	}{
		{name: "Linux procfs", goos: "linux", procFS: true, want: nodeLaunchProcSelfFD},
		{name: "Linux fallback", goos: "linux", procFS: false, want: nodeLaunchPrivatePath},
		{name: "Darwin ignores procfs", goos: "darwin", procFS: true, want: nodeLaunchPrivatePath},
		{name: "Windows private path", goos: "windows", procFS: false, want: nodeLaunchPrivatePath},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := selectNodeLaunchMode(test.goos, test.procFS); got != test.want {
				t.Fatalf("selectNodeLaunchMode(%q, %t) = %q, want %q", test.goos, test.procFS, got, test.want)
			}
		})
	}
}

func TestValidateSelectedNodeIdentity(t *testing.T) {
	for _, pin := range nodeArtifactPins {
		wantArch := "arm64"
		if pin.GOARCH == "amd64" {
			wantArch = "x64"
		}
		identity := NodeIdentity{
			Platform:   pin.GOOS,
			Arch:       wantArch,
			Executable: filepath.Join("private", "node"),
		}
		if err := validateSelectedNodeIdentity(pin, identity); err != nil {
			t.Errorf("validate %s/%s: %v", pin.GOOS, pin.GOARCH, err)
		}
	}

	pin := nodeArtifactPins[0]
	valid := NodeIdentity{Platform: pin.GOOS, Arch: "arm64", Executable: filepath.Join("private", "node")}
	for _, test := range []struct {
		name     string
		pin      NodeArtifactPin
		identity NodeIdentity
		contains string
	}{
		{name: "platform", pin: pin, identity: NodeIdentity{Platform: "linux", Arch: valid.Arch, Executable: valid.Executable}, contains: "platform"},
		{name: "architecture", pin: pin, identity: NodeIdentity{Platform: valid.Platform, Arch: "x64", Executable: valid.Executable}, contains: "architecture"},
		{name: "executable", pin: pin, identity: NodeIdentity{Platform: valid.Platform, Arch: valid.Arch, Executable: filepath.Join("private", "not-node")}, contains: "executable"},
		{name: "empty executable", pin: pin, identity: NodeIdentity{Platform: valid.Platform, Arch: valid.Arch}, contains: "executable"},
		{name: "unsupported pin architecture", pin: nodePinWithArch(pin, "ppc64le"), identity: valid, contains: "unsupported"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateSelectedNodeIdentity(test.pin, test.identity)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("identity validation error = %v, want %q", err, test.contains)
			}
		})
	}
}

func TestNodeArchiveFailureTaxonomy(t *testing.T) {
	t.Run("source", func(t *testing.T) {
		_, err := prepareNodeArtifactPin(filepath.Join(t.TempDir(), "missing.tar.gz"), nodeTestPin(nil, "node"))
		if !errors.Is(err, errNodeArchiveSource) {
			t.Fatalf("source error = %v", err)
		}
	})

	t.Run("integrity", func(t *testing.T) {
		archive, path := nodeTestArchiveFile(t, []nodeTestArchiveEntry{{name: "node", typeflag: tar.TypeReg, data: []byte("node")}})
		pin := nodeTestPin(archive, "node")
		pin.SHA256 = strings.Repeat("0", sha256.Size*2)
		artifact, err := prepareNodeArtifactPin(path, pin)
		if artifact != nil {
			_ = artifact.Close()
			t.Fatal("checksum failure returned a Node artifact")
		}
		if !errors.Is(err, errNodeArchiveIntegrity) {
			t.Fatalf("integrity error = %v", err)
		}
	})

	t.Run("format", func(t *testing.T) {
		archive := filepath.Join(t.TempDir(), "invalid.tar.gz")
		if err := os.WriteFile(archive, []byte("not gzip"), 0o600); err != nil {
			t.Fatal(err)
		}
		file, err := os.Open(archive)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		_, err = extractNodeExecutable(file, filepath.Join(t.TempDir(), "node"), "node")
		if !errors.Is(err, errNodeArchiveFormat) {
			t.Fatalf("format error = %v", err)
		}
	})

	for _, test := range []struct {
		name    string
		entries []nodeTestArchiveEntry
	}{
		{name: "missing", entries: []nodeTestArchiveEntry{{name: "other", typeflag: tar.TypeReg, data: []byte("node")}}},
		{name: "duplicate", entries: []nodeTestArchiveEntry{
			{name: "node", typeflag: tar.TypeReg, data: []byte("first")},
			{name: "node", typeflag: tar.TypeReg, data: []byte("second")},
		}},
		{name: "nonregular", entries: []nodeTestArchiveEntry{{name: "node", typeflag: tar.TypeDir}}},
		{name: "empty", entries: []nodeTestArchiveEntry{{name: "node", typeflag: tar.TypeReg}}},
	} {
		t.Run("entry "+test.name, func(t *testing.T) {
			_, archive := nodeTestArchiveFile(t, test.entries)
			file, err := os.Open(archive)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			_, err = extractNodeExecutable(file, filepath.Join(t.TempDir(), "node"), "node")
			if !errors.Is(err, errNodeArchiveEntry) {
				t.Fatalf("entry error = %v", err)
			}
		})
	}
}

func nodePinWithArch(pin NodeArtifactPin, goarch string) NodeArtifactPin {
	pin.GOARCH = goarch
	return pin
}

func nodeTestPin(archive []byte, entry string) NodeArtifactPin {
	hash := sha256.Sum256(archive)
	return NodeArtifactPin{
		GOOS:   "darwin",
		GOARCH: "arm64",
		File:   "node.tar.gz",
		URL:    "https://nodejs.org/node.tar.gz",
		SHA256: hex.EncodeToString(hash[:]),
		Entry:  entry,
	}
}

func nodeTestArchive(t *testing.T, entries []nodeTestArchiveEntry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, entry := range entries {
		header := &tar.Header{
			Name:     entry.name,
			Mode:     0o700,
			Size:     int64(len(entry.data)),
			Typeflag: entry.typeflag,
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		if len(entry.data) != 0 {
			if _, err := tarWriter.Write(entry.data); err != nil {
				t.Fatalf("write tar entry: %v", err)
			}
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}
	return buffer.Bytes()
}

func nodeTestArchiveFile(t *testing.T, entries []nodeTestArchiveEntry) ([]byte, string) {
	t.Helper()
	archive := nodeTestArchive(t, entries)
	path := filepath.Join(t.TempDir(), "node.tar.gz")
	if err := os.WriteFile(path, archive, 0o600); err != nil {
		t.Fatal(err)
	}
	return archive, path
}
