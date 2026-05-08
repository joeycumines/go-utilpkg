package oracle

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

const (
	maxNodeArchiveBytes    = 128 << 20
	maxNodeExecutableBytes = 256 << 20

	nodeLaunchProcSelfFD    = "proc-self-fd"
	nodeLaunchPrivatePath   = "private-path-held-inode"
	nodeProcSelfFDDirectory = "/proc/self/fd"
)

var (
	errNodeArchiveSource    = errors.New("node archive source failure")
	errNodeArchiveIntegrity = errors.New("node archive integrity failure")
	errNodeArchiveFormat    = errors.New("node archive format failure")
	errNodeArchiveEntry     = errors.New("node archive executable entry failure")
)

var nodeArtifactPins = []NodeArtifactPin{
	{
		GOOS: "darwin", GOARCH: "arm64",
		File:   "node-v26.5.0-darwin-arm64.tar.gz",
		URL:    "https://nodejs.org/dist/v26.5.0/node-v26.5.0-darwin-arm64.tar.gz",
		SHA256: "ee920559aaa2391569cff4d737e3b83963430e3a14dedd91bfe0ff53171b5af9",
		Entry:  "node-v26.5.0-darwin-arm64/bin/node",
	},
	{
		GOOS: "darwin", GOARCH: "amd64",
		File:   "node-v26.5.0-darwin-x64.tar.gz",
		URL:    "https://nodejs.org/dist/v26.5.0/node-v26.5.0-darwin-x64.tar.gz",
		SHA256: "98293394c945a24e64e00b4177bf075ec963ea70b34d1d2e24bd4a71716d334f",
		Entry:  "node-v26.5.0-darwin-x64/bin/node",
	},
	{
		GOOS: "linux", GOARCH: "arm64",
		File:   "node-v26.5.0-linux-arm64.tar.gz",
		URL:    "https://nodejs.org/dist/v26.5.0/node-v26.5.0-linux-arm64.tar.gz",
		SHA256: "308e5fe89a82461ba5a6cf15ff5221b2cdbd7ae87600aa72bb3c3fbdc66412d1",
		Entry:  "node-v26.5.0-linux-arm64/bin/node",
	},
	{
		GOOS: "linux", GOARCH: "amd64",
		File:   "node-v26.5.0-linux-x64.tar.gz",
		URL:    "https://nodejs.org/dist/v26.5.0/node-v26.5.0-linux-x64.tar.gz",
		SHA256: "22b5f47ad6ae78837e4c2b846019965ce1a06ba143de176102294a1bf44fc677",
		Entry:  "node-v26.5.0-linux-x64/bin/node",
	},
}

type nodeArtifact struct {
	executableInfo   os.FileInfo
	executable       *os.File
	removeRoot       func(string) error
	pin              NodeArtifactPin
	root             string
	executablePath   string
	archiveSHA256    string
	executableSHA256 string
	launchMode       string
}

type nodeArchiveSource interface {
	io.Reader
	io.Closer
	Stat() (os.FileInfo, error)
}

type nodeArtifactOps struct {
	openSource      func(string) (nodeArchiveSource, error)
	removeRoot      func(string) error
	procFSAvailable func() bool
}

func defaultNodeArtifactOps() nodeArtifactOps {
	return nodeArtifactOps{
		openSource: func(path string) (nodeArchiveSource, error) {
			return os.Open(path)
		},
		removeRoot: os.RemoveAll,
		procFSAvailable: func() bool {
			info, err := os.Stat(nodeProcSelfFDDirectory)
			return err == nil && info.IsDir()
		},
	}
}

func selectedNodeArtifactPin(goos, goarch string) (NodeArtifactPin, error) {
	for _, pin := range nodeArtifactPins {
		if pin.GOOS == goos && pin.GOARCH == goarch {
			return pin, nil
		}
	}
	return NodeArtifactPin{}, fmt.Errorf("official Node %s has no authenticated artifact for %s/%s", NodeVersion, goos, goarch)
}

func currentNodeArtifactPin() (NodeArtifactPin, error) {
	return selectedNodeArtifactPin(runtime.GOOS, runtime.GOARCH)
}

func prepareNodeArtifact(path string) (*nodeArtifact, error) {
	pin, err := currentNodeArtifactPin()
	if err != nil {
		return nil, err
	}
	return prepareNodeArtifactPin(path, pin)
}

func prepareNodeArtifactPin(path string, pin NodeArtifactPin) (*nodeArtifact, error) {
	return prepareNodeArtifactPinOps(path, pin, defaultNodeArtifactOps())
}

func prepareNodeArtifactPinOps(path string, pin NodeArtifactPin, ops nodeArtifactOps) (result *nodeArtifact, resultErr error) {
	if path == "" {
		return nil, fmt.Errorf("%w: path is empty", errNodeArchiveSource)
	}
	if pin.GOOS == "" || pin.GOARCH == "" || pin.File == "" || pin.URL == "" || pin.SHA256 == "" || pin.Entry == "" {
		return nil, fmt.Errorf("%w: Node artifact pin is incomplete", errNodeArchiveIntegrity)
	}
	if ops.openSource == nil || ops.removeRoot == nil || ops.procFSAvailable == nil {
		return nil, errors.New("node artifact operations are incomplete")
	}
	source, err := ops.openSource(path)
	if err != nil {
		return nil, fmt.Errorf("%w: open Node archive: %w", errNodeArchiveSource, err)
	}
	var artifact *nodeArtifact
	defer func() {
		if closeErr := source.Close(); closeErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("%w: close Node archive source: %w", errNodeArchiveSource, closeErr))
		}
		if resultErr != nil && artifact != nil {
			resultErr = errors.Join(resultErr, artifact.Close())
			result = nil
		}
	}()
	info, err := source.Stat()
	if err != nil {
		return nil, fmt.Errorf("%w: stat Node archive: %w", errNodeArchiveSource, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: Node archive is not a regular file", errNodeArchiveSource)
	}

	root, err := os.MkdirTemp("", "goja-eventloop-node-")
	if err != nil {
		return nil, fmt.Errorf("%w: create private Node directory: %w", errNodeArchiveSource, err)
	}
	artifact = &nodeArtifact{pin: pin, root: root, removeRoot: ops.removeRoot}

	archivePath := filepath.Join(root, "node.tar.gz")
	archive, err := os.OpenFile(archivePath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("%w: create private Node archive: %w", errNodeArchiveSource, err)
	}
	archiveOpen := true
	defer func() {
		if archiveOpen {
			resultErr = errors.Join(resultErr, nodeArchiveCloseError("close private Node archive", archive.Close()))
		}
	}()
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(archive, hash), io.LimitReader(source, maxNodeArchiveBytes+1))
	if copyErr != nil {
		return nil, fmt.Errorf("%w: copy Node archive: %w", errNodeArchiveSource, copyErr)
	}
	if written > maxNodeArchiveBytes {
		return nil, fmt.Errorf("%w: Node archive exceeds %d bytes", errNodeArchiveIntegrity, maxNodeArchiveBytes)
	}
	artifact.archiveSHA256 = hex.EncodeToString(hash.Sum(nil))
	if artifact.archiveSHA256 != pin.SHA256 {
		return nil, fmt.Errorf("%w: Node archive SHA-256 is %s, want %s", errNodeArchiveIntegrity, artifact.archiveSHA256, pin.SHA256)
	}
	if _, err := archive.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("%w: rewind private Node archive: %w", errNodeArchiveSource, err)
	}

	executablePath := filepath.Join(root, "node")
	executableHash, err := extractNodeExecutable(archive, executablePath, pin.Entry)
	if err != nil {
		return nil, err
	}
	closeErr := archive.Close()
	archiveOpen = false
	if closeErr != nil {
		return nil, nodeArchiveCloseError("close private Node archive", closeErr)
	}
	if err := os.Chmod(executablePath, 0o500); err != nil {
		return nil, fmt.Errorf("%w: make private Node executable read-only: %w", errNodeArchiveSource, err)
	}
	executable, err := os.Open(executablePath)
	if err != nil {
		return nil, fmt.Errorf("%w: open private Node executable: %w", errNodeArchiveSource, err)
	}
	artifact.executable = executable
	artifact.executablePath = executablePath
	artifact.executableSHA256 = executableHash
	artifact.executableInfo, err = executable.Stat()
	if err != nil {
		return nil, fmt.Errorf("%w: stat private Node executable: %w", errNodeArchiveSource, err)
	}
	if err := os.Chmod(root, 0o500); err != nil {
		return nil, fmt.Errorf("%w: make private Node directory read-only: %w", errNodeArchiveSource, err)
	}
	artifact.launchMode = selectNodeLaunchMode(pin.GOOS, ops.procFSAvailable())
	if err := artifact.verify(); err != nil {
		return nil, err
	}
	return artifact, nil
}

func nodeArchiveCloseError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %s: %w", errNodeArchiveSource, operation, err)
}

func extractNodeExecutable(archive *os.File, destination, entry string) (_ string, resultErr error) {
	gzipReader, err := gzip.NewReader(archive)
	if err != nil {
		return "", fmt.Errorf("%w: open Node gzip stream: %w", errNodeArchiveFormat, err)
	}
	defer func() {
		if closeErr := gzipReader.Close(); closeErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("%w: close Node gzip stream: %w", errNodeArchiveFormat, closeErr))
		}
	}()
	tarReader := tar.NewReader(gzipReader)
	found := false
	var executableHash string
	for {
		header, nextErr := tarReader.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return "", fmt.Errorf("%w: read Node tar stream: %w", errNodeArchiveFormat, nextErr)
		}
		if header.Name != entry {
			continue
		}
		if found {
			return "", fmt.Errorf("%w: Node archive contains duplicate executable entry %q", errNodeArchiveEntry, entry)
		}
		found = true
		if !header.FileInfo().Mode().IsRegular() {
			return "", fmt.Errorf("%w: Node executable entry %q is not a regular file", errNodeArchiveEntry, entry)
		}
		if header.Size <= 0 || header.Size > maxNodeExecutableBytes {
			return "", fmt.Errorf("%w: Node executable size %d is outside the permitted range", errNodeArchiveEntry, header.Size)
		}
		output, openErr := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
		if openErr != nil {
			return "", fmt.Errorf("%w: create private Node executable: %w", errNodeArchiveSource, openErr)
		}
		hash := sha256.New()
		copied, copyErr := io.CopyN(io.MultiWriter(output, hash), tarReader, header.Size)
		closeErr := output.Close()
		if copyErr != nil || copied != header.Size {
			var extractErr error
			if copyErr != nil {
				extractErr = fmt.Errorf("%w: extract Node executable: copied %d of %d bytes: %w", errNodeArchiveSource, copied, header.Size, copyErr)
			} else {
				extractErr = fmt.Errorf("%w: extract Node executable: copied %d of %d bytes", errNodeArchiveSource, copied, header.Size)
			}
			return "", errors.Join(extractErr, nodeArchiveCloseError("close private Node executable", closeErr))
		}
		if closeErr != nil {
			return "", nodeArchiveCloseError("close private Node executable", closeErr)
		}
		executableHash = hex.EncodeToString(hash.Sum(nil))
	}
	if !found {
		return "", fmt.Errorf("%w: Node archive is missing executable entry %q", errNodeArchiveEntry, entry)
	}
	return executableHash, nil
}

func selectNodeLaunchMode(goos string, procFSAvailable bool) string {
	if goos == "linux" && procFSAvailable {
		return nodeLaunchProcSelfFD
	}
	return nodeLaunchPrivatePath
}

func validateSelectedNodeIdentity(pin NodeArtifactPin, identity NodeIdentity) error {
	if identity.Platform != pin.GOOS {
		return fmt.Errorf("node platform is %q, selected artifact requires %q", identity.Platform, pin.GOOS)
	}
	wantArch, ok := map[string]string{"amd64": "x64", "arm64": "arm64"}[pin.GOARCH]
	if !ok {
		return fmt.Errorf("selected Node artifact architecture %q is unsupported", pin.GOARCH)
	}
	if identity.Arch != wantArch {
		return fmt.Errorf("node architecture is %q, selected artifact requires %q", identity.Arch, wantArch)
	}
	wantExecutable := filepath.Base(filepath.FromSlash(pin.Entry))
	gotExecutable := filepath.Base(filepath.Clean(identity.Executable))
	if identity.Executable == "" || wantExecutable == "." || gotExecutable != wantExecutable {
		return fmt.Errorf("node executable is %q, selected artifact requires %q", identity.Executable, wantExecutable)
	}
	return nil
}

func (a *nodeArtifact) verify() error {
	if a == nil || a.executable == nil || a.executableInfo == nil || a.executablePath == "" {
		return errors.New("node artifact is unavailable")
	}
	if a.launchMode != nodeLaunchProcSelfFD && a.launchMode != nodeLaunchPrivatePath {
		return fmt.Errorf("node artifact launch mode %q is invalid", a.launchMode)
	}
	info, err := os.Stat(a.executablePath)
	if err != nil {
		return fmt.Errorf("stat private Node executable path: %w", err)
	}
	if !os.SameFile(a.executableInfo, info) {
		return errors.New("private Node executable path no longer names the authenticated inode")
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o222 != 0 {
		return fmt.Errorf("private Node executable mode is %s, want a non-writable regular file", info.Mode())
	}
	rootInfo, err := os.Stat(a.root)
	if err != nil {
		return fmt.Errorf("stat private Node directory: %w", err)
	}
	if !rootInfo.IsDir() || rootInfo.Mode().Perm()&0o222 != 0 {
		return fmt.Errorf("private Node directory mode is %s, want a non-writable directory", rootInfo.Mode())
	}
	return nil
}

func (a *nodeArtifact) identity() NodeArtifact {
	return NodeArtifact{
		GOOS:                  a.pin.GOOS,
		GOARCH:                a.pin.GOARCH,
		URL:                   a.pin.URL,
		File:                  a.pin.File,
		Entry:                 a.pin.Entry,
		ExpectedArchiveSHA256: a.pin.SHA256,
		ArchiveSHA256:         a.archiveSHA256,
		ExecutableSHA256:      a.executableSHA256,
		LaunchMode:            a.launchMode,
	}
}

func (a *nodeArtifact) Close() error {
	if a == nil {
		return nil
	}
	var result error
	if a.executable != nil {
		if err := a.executable.Close(); err != nil {
			result = errors.Join(result, fmt.Errorf("close private Node executable: %w", err))
		}
		a.executable = nil
	}
	if a.root != "" {
		if err := os.Chmod(a.root, 0o700); err != nil {
			result = errors.Join(result, fmt.Errorf("make private Node directory removable: %w", err))
		}
		removeRoot := a.removeRoot
		if removeRoot == nil {
			removeRoot = os.RemoveAll
		}
		if err := removeRoot(a.root); err != nil {
			result = errors.Join(result, fmt.Errorf("remove private Node directory: %w", err))
		} else {
			a.root = ""
		}
	}
	return result
}
