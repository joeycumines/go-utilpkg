package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash"
	"io"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const sourceMetadataPath = ".tournament/source.json"

var windowsVolumePattern = regexp.MustCompile(`^[A-Za-z]:`)

type sourceMetadata struct { // betteralign:ignore canonical JSON field order
	SchemaVersion          int                    `json:"schema_version"`
	SharedSourceID         string                 `json:"shared_source_id"`
	CaptureID              string                 `json:"capture_id"`
	CaptureAuthoritySHA256 string                 `json:"capture_authority_sha256"`
	LegacyV4Fingerprint    string                 `json:"legacy_v4_fingerprint"`
	LogicalAuthority       sourceLogicalAuthority `json:"logical_authority"`
	CaptureAuthority       sourceCaptureAuthority `json:"capture_authority"`
	FileCount              int                    `json:"file_count"`
	Files                  []sourceRecord         `json:"files"`
	Fingerprint            string                 `json:"-"`
	Authority              sourceAuthority        `json:"-"`
}

type sourceMetadataV4 struct { // betteralign:ignore canonical JSON field order
	SchemaVersion int               `json:"schema_version"`
	Fingerprint   string            `json:"fingerprint"`
	Authority     sourceAuthorityV4 `json:"authority"`
	FileCount     int               `json:"file_count"`
	Files         []sourceRecord    `json:"files"`
}

type sourceRecord struct { // betteralign:ignore canonical JSON field order
	Path          string `json:"path"`
	Mode          string `json:"mode"`
	Size          int64  `json:"size"`
	SHA256        string `json:"sha256"`
	SymlinkTarget string `json:"symlink_target,omitempty"`
}

func snapshotCommand(arguments []string) int {
	flags := flag.NewFlagSet("snapshot", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	root := flags.String("root", "", "live repository root")
	output := flags.String("output", "", "new snapshot directory")
	buildFlags := registerSourceBuildFlags(flags)
	if err := flags.Parse(arguments); err != nil {
		return commandError(err)
	}
	if flags.NArg() != 0 || *root == "" || *output == "" {
		return commandError(errors.New("snapshot requires -root and -output"))
	}
	buildConfig := buildFlags.config()
	metadata, err := createSnapshotBuild(*root, *output, buildConfig)
	if err != nil {
		return commandError(err)
	}
	fmt.Println(metadata.Fingerprint)
	return 0
}

func sourceFingerprintCommand(arguments []string) int {
	flags := flag.NewFlagSet("source-fingerprint", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	root := flags.String("root", "", "repository or snapshot root")
	metadataPath := flags.String("metadata", "", "snapshot metadata path")
	buildFlags := registerSourceBuildFlags(flags)
	if err := flags.Parse(arguments); err != nil {
		return commandError(err)
	}
	if flags.NArg() != 0 || *root == "" {
		return commandError(errors.New("source-fingerprint requires -root"))
	}
	buildConfig := buildFlags.config()
	if *metadataPath != "" {
		if buildConfig != (sourceBuildConfig{}) {
			return commandError(errors.New("snapshot metadata verification does not accept source-build flags"))
		}
		if err := validateSourceMetadataFile(*root, *metadataPath); err != nil {
			return commandError(err)
		}
	}

	var metadata sourceMetadata
	var fingerprint string
	var err error
	if *metadataPath == "" {
		var capture sourceCapture
		capture, err = governedSourceCapture(*root, buildConfig)
		if err == nil {
			metadata.Files, err = inspectSourceRecords(*root, capture.Files)
		}
		if err == nil {
			fingerprint, err = fingerprintSource(capture.Authority, metadata.Files)
		}
	} else {
		metadata, err = readSourceMetadata(*metadataPath)
		if err == nil {
			fingerprint, err = verifySourceRecords(*root, metadata.Authority, metadata.Files)
		}
	}
	if err != nil {
		return commandError(err)
	}
	if *metadataPath != "" && fingerprint != metadata.Fingerprint {
		return commandError(fmt.Errorf("snapshot fingerprint %s != recorded %s", fingerprint, metadata.Fingerprint))
	}
	fmt.Println(fingerprint)
	return 0
}

func createSnapshot(root, output string) (_ sourceMetadata, err error) {
	return createSnapshotWithEnumerator(root, output, fixtureSourceAuthority(), copySourcePath, liveSourceFiles)
}

func createSnapshotBuild(root, output string, config sourceBuildConfig) (_ sourceMetadata, err error) {
	capture, err := governedSourceCapture(root, config)
	if err != nil {
		return sourceMetadata{}, err
	}
	return createSnapshotWithEnumerator(
		root,
		output,
		capture.Authority,
		copySourcePath,
		func(root string) ([]string, error) {
			current, err := governedSourceCapture(root, config)
			if err != nil {
				return nil, err
			}
			if !reflect.DeepEqual(current.Authority, capture.Authority) {
				return nil, errors.New("governed source authority changed while snapshotting")
			}
			return current.Files, nil
		},
	)
}

func createSnapshotWithCopier(
	root,
	output string,
	copier func(string, string, string) error,
) (_ sourceMetadata, err error) {
	return createSnapshotWithEnumerator(root, output, fixtureSourceAuthority(), copier, liveSourceFiles)
}

func createSnapshotWithEnumerator(
	root,
	output string,
	authority sourceAuthority,
	copier func(string, string, string) error,
	enumerate func(string) ([]string, error),
) (_ sourceMetadata, err error) {
	if err := validateSourceAuthority(authority); err != nil {
		return sourceMetadata{}, err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return sourceMetadata{}, fmt.Errorf("resolve source root: %w", err)
	}
	output, err = filepath.Abs(output)
	if err != nil {
		return sourceMetadata{}, fmt.Errorf("resolve snapshot output: %w", err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return sourceMetadata{}, fmt.Errorf("resolve source root links: %w", err)
	}
	resolvedOutputParent, err := filepath.EvalSymlinks(filepath.Dir(output))
	if err != nil {
		return sourceMetadata{}, fmt.Errorf("resolve snapshot output parent: %w", err)
	}
	resolvedOutput := filepath.Join(resolvedOutputParent, filepath.Base(output))
	root = resolvedRoot
	output = resolvedOutput
	if containedPath(resolvedRoot, resolvedOutput) {
		return sourceMetadata{}, fmt.Errorf("snapshot output must be outside source root: %s", output)
	}
	if _, statErr := os.Lstat(output); !os.IsNotExist(statErr) {
		if statErr == nil {
			return sourceMetadata{}, fmt.Errorf("snapshot output already exists: %s", output)
		}
		return sourceMetadata{}, fmt.Errorf("inspect snapshot output: %w", statErr)
	}
	if err = os.Mkdir(output, 0o700); err != nil {
		return sourceMetadata{}, fmt.Errorf("create snapshot output: %w", err)
	}
	success := false
	defer func() {
		if !success {
			if cleanupErr := os.RemoveAll(output); cleanupErr != nil {
				err = errors.Join(err, fmt.Errorf("clean failed snapshot: %w", cleanupErr))
			}
		}
	}()

	files, err := enumerate(root)
	if err != nil {
		return sourceMetadata{}, err
	}
	startRecords, err := inspectSourceRecords(root, files)
	if err != nil {
		return sourceMetadata{}, fmt.Errorf("inspect live source before snapshot: %w", err)
	}
	startIdentity, err := identifySnapshotSource(authority, startRecords)
	if err != nil {
		return sourceMetadata{}, fmt.Errorf("fingerprint live source before snapshot: %w", err)
	}
	for _, relative := range files {
		if err := copier(root, output, relative); err != nil {
			return sourceMetadata{}, err
		}
	}
	endFiles, err := enumerate(root)
	if err != nil {
		return sourceMetadata{}, fmt.Errorf("enumerate live source after snapshot: %w", err)
	}
	if !slices.Equal(endFiles, files) {
		return sourceMetadata{}, fmt.Errorf("live source set changed while snapshotting: before %q, after %q", files, endFiles)
	}
	endRecords, err := inspectSourceRecords(root, endFiles)
	if err != nil {
		return sourceMetadata{}, fmt.Errorf("inspect live source after snapshot: %w", err)
	}
	endIdentity, err := identifySnapshotSource(authority, endRecords)
	if err != nil {
		return sourceMetadata{}, fmt.Errorf("fingerprint live source after snapshot: %w", err)
	}
	if !reflect.DeepEqual(endIdentity, startIdentity) {
		return sourceMetadata{}, errors.New("live source identity changed while snapshotting")
	}
	snapshotRecords, err := inspectSourceRecords(output, files)
	if err != nil {
		return sourceMetadata{}, fmt.Errorf("inspect snapshot: %w", err)
	}
	snapshotIdentity, err := identifySnapshotSource(authority, snapshotRecords)
	if err != nil {
		return sourceMetadata{}, fmt.Errorf("fingerprint snapshot: %w", err)
	}
	if !reflect.DeepEqual(snapshotIdentity, startIdentity) {
		return sourceMetadata{}, errors.New("snapshot identity differs from live source")
	}
	snapshotFiles, err := snapshotSourceFiles(output)
	if err != nil {
		return sourceMetadata{}, fmt.Errorf("enumerate snapshot source: %w", err)
	}
	if !slices.Equal(snapshotFiles, files) {
		return sourceMetadata{}, fmt.Errorf("snapshot source set %q != live source set %q", snapshotFiles, files)
	}

	metadata, err := newSourceMetadata(authority, snapshotRecords, snapshotIdentity)
	if err != nil {
		return sourceMetadata{}, fmt.Errorf("construct snapshot metadata: %w", err)
	}
	metadataPath := filepath.Join(output, filepath.FromSlash(sourceMetadataPath))
	if err := os.MkdirAll(filepath.Dir(metadataPath), 0o700); err != nil {
		return sourceMetadata{}, fmt.Errorf("create snapshot metadata directory: %w", err)
	}
	var persisted any = metadata
	if metadata.SchemaVersion == 4 {
		persisted = sourceMetadataV4{
			SchemaVersion: metadata.SchemaVersion,
			Fingerprint:   metadata.Fingerprint,
			Authority:     sourceAuthorityV4Value(metadata.Authority),
			FileCount:     metadata.FileCount,
			Files:         metadata.Files,
		}
	}
	data, err := json.MarshalIndent(persisted, "", "  ")
	if err != nil {
		return sourceMetadata{}, fmt.Errorf("encode snapshot metadata: %w", err)
	}
	data = append(data, '\n')
	if err := writeAtomicNew(metadataPath, data, 0o600); err != nil {
		return sourceMetadata{}, fmt.Errorf("write snapshot metadata: %w", err)
	}
	success = true
	return metadata, nil
}

func snapshotSourceFiles(root string) ([]string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve snapshot root: %w", err)
	}
	files := make([]string, 0)
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root || entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("resolve snapshot path %q: %w", path, err)
		}
		relative = filepath.ToSlash(relative)
		if relative == sourceMetadataPath {
			return nil
		}
		if err := validateSourceFilePath(relative); err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect snapshot source %q: %w", relative, err)
		}
		if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("snapshot source %q has unsupported mode %s", relative, info.Mode())
		}
		files = append(files, relative)
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.Sort(files)
	if len(files) == 0 {
		return nil, errors.New("snapshot source list is empty")
	}
	return files, nil
}

func fingerprintFiles(root string, files []string) (string, error) {
	records, err := inspectSourceRecords(root, files)
	if err != nil {
		return "", err
	}
	return fingerprintRecords(records)
}

func inspectSourceRecords(root string, files []string) ([]sourceRecord, error) {
	if len(files) == 0 {
		return nil, errors.New("cannot inspect an empty source list")
	}
	records := make([]sourceRecord, 0, len(files))
	previous := ""
	for index, relative := range files {
		if err := validateSourceFilePath(relative); err != nil {
			return nil, err
		}
		if index != 0 && relative <= previous {
			return nil, fmt.Errorf("source paths are not strictly sorted at %q", relative)
		}
		previous = relative
		path := filepath.Join(root, filepath.FromSlash(relative))
		info, err := os.Lstat(path)
		if err != nil {
			return nil, fmt.Errorf("inspect governed source %q: %w", relative, err)
		}
		record := sourceRecord{Path: relative, Mode: sourceMode(info.Mode())}
		switch {
		case info.Mode().IsRegular():
			record.Size = info.Size()
			file, err := os.Open(path)
			if err != nil {
				return nil, fmt.Errorf("open governed source %q: %w", relative, err)
			}
			record.SHA256, err = sha256Reader(file)
			closeErr := file.Close()
			if err != nil || closeErr != nil {
				return nil, errors.Join(
					annotateError("hash governed source "+relative, err),
					annotateError("close governed source "+relative, closeErr),
				)
			}
			after, err := os.Lstat(path)
			if err != nil {
				return nil, fmt.Errorf("reinspect governed source %q: %w", relative, err)
			}
			if !after.Mode().IsRegular() || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
				return nil, fmt.Errorf("governed source %q changed while hashing", relative)
			}
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return nil, fmt.Errorf("read governed symlink %q: %w", relative, err)
			}
			if err := validateSymlink(root, relative, target); err != nil {
				return nil, err
			}
			record.Size = int64(len(target))
			record.SymlinkTarget = target
			digest := sha256.Sum256([]byte(target))
			record.SHA256 = hex.EncodeToString(digest[:])
		default:
			return nil, fmt.Errorf("governed source %q has unsupported mode %s", relative, info.Mode())
		}
		records = append(records, record)
	}
	return records, nil
}

func fingerprintRecords(records []sourceRecord) (string, error) {
	if len(records) == 0 {
		return "", errors.New("cannot fingerprint empty source records")
	}
	digest := sha256.New()
	writeFingerprintFrame(digest, []byte("go-utilpkg-eventloop-tournament-source-v2"))
	previous := ""
	portablePaths := make(map[string]string, len(records))
	for index, record := range records {
		if err := validateSourceRecord(record); err != nil {
			return "", err
		}
		if index != 0 && record.Path <= previous {
			return "", fmt.Errorf("source records are not strictly sorted at %q", record.Path)
		}
		previous = record.Path
		folded := strings.ToLower(record.Path)
		if prior, exists := portablePaths[folded]; exists {
			return "", fmt.Errorf("source paths %q and %q collide case-insensitively", prior, record.Path)
		}
		portablePaths[folded] = record.Path
		writeFingerprintFrame(digest, []byte(record.Path))
		writeFingerprintFrame(digest, []byte(record.Mode))
		writeFingerprintFrame(digest, []byte(strconv.FormatInt(record.Size, 10)))
		writeFingerprintFrame(digest, []byte(record.SHA256))
		writeFingerprintFrame(digest, []byte(record.SymlinkTarget))
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func fingerprintSource(authority sourceAuthority, records []sourceRecord) (string, error) {
	if err := validateSourceAuthority(authority); err != nil {
		return "", err
	}
	if authority.EnumerationPolicy == governedSourcePolicy {
		paths := make([]string, len(records))
		for index, record := range records {
			paths[index] = record.Path
		}
		if !slices.Equal(paths, authority.GovernedUnion.Paths) {
			return "", errors.New("governed source records differ from authority union")
		}
	}
	authorityData, err := json.Marshal(sourceAuthorityV4Value(authority))
	if err != nil {
		return "", fmt.Errorf("encode source authority: %w", err)
	}
	recordFingerprint, err := fingerprintRecords(records)
	if err != nil {
		return "", err
	}
	digest := sha256.New()
	writeFingerprintFrame(digest, []byte("go-utilpkg-eventloop-tournament-source-v4"))
	writeFingerprintFrame(digest, authorityData)
	writeFingerprintFrame(digest, []byte(recordFingerprint))
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func verifySourceRecords(root string, authority sourceAuthority, expected []sourceRecord) (string, error) {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve snapshot verification root: %w", err)
	}
	root = resolvedRoot
	if authority.EnumerationPolicy == governedSourcePolicy {
		manifestPath := filepath.Join(root, filepath.FromSlash(sourceManifestRelativePath))
		if err := validateSourceManifestFile(root, manifestPath); err != nil {
			return "", err
		}
		manifest, authorityDigest, manifestDigest, err := loadManifestSourceAuthorityIdentity(manifestPath)
		if err != nil {
			return "", err
		}
		if err := validateSourceAuthorityManifest(authority, manifest, authorityDigest, manifestDigest); err != nil {
			return "", err
		}
		physical, err := physicalSourceFiles(root, manifest.PhysicalPolicy.RuntimeAssets)
		if err != nil {
			return "", err
		}
		physicalSet, err := newSourcePathSet(physical)
		if err != nil || !reflect.DeepEqual(physicalSet, authority.PhysicalPaths) {
			return "", errors.New("snapshot physical source paths differ from recorded authority")
		}
		if err := validateSourceModuleRegistry(root, manifest, physical); err != nil {
			return "", err
		}
	}
	physical, err := snapshotSourceFiles(root)
	if err != nil {
		return "", err
	}
	expectedPaths := make([]string, len(expected))
	for index, record := range expected {
		expectedPaths[index] = record.Path
	}
	if !slices.Equal(physical, expectedPaths) {
		return "", fmt.Errorf("snapshot source set %q != recorded %q", physical, expectedPaths)
	}
	actual, err := inspectSourceRecords(root, physical)
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "windows" {
		for index := range actual {
			if actual[index].Mode == "100644" && expected[index].Mode == "100755" {
				actual[index].Mode = "100755"
			}
		}
	}
	if !slices.Equal(actual, expected) {
		return "", errors.New("snapshot source records do not match recorded topology and payloads")
	}
	return fingerprintSource(authority, expected)
}

func copySourcePath(root, output, relative string) error {
	source := filepath.Join(root, filepath.FromSlash(relative))
	target := filepath.Join(output, filepath.FromSlash(relative))
	info, err := os.Lstat(source)
	if err != nil {
		return fmt.Errorf("inspect snapshot source %q: %w", relative, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		linkTarget, err := os.Readlink(source)
		if err != nil {
			return fmt.Errorf("read snapshot symlink %q: %w", relative, err)
		}
		if err := validateSymlink(root, relative, linkTarget); err != nil {
			return err
		}
		if err := ensureSnapshotParent(output, relative); err != nil {
			return err
		}
		if err := os.Symlink(linkTarget, target); err != nil {
			return fmt.Errorf("copy snapshot symlink %q: %w", relative, err)
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("snapshot source %q has unsupported mode %s", relative, info.Mode())
	}
	if err := ensureSnapshotParent(output, relative); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open snapshot source %q: %w", relative, err)
	}
	mode := os.FileMode(0o644)
	if info.Mode().Perm()&0o111 != 0 {
		mode = 0o755
	}
	outputFile, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return errors.Join(
			fmt.Errorf("create snapshot file %q: %w", relative, err),
			closeError("snapshot source", relative, input),
		)
	}
	if _, err := io.Copy(outputFile, input); err != nil {
		return errors.Join(
			fmt.Errorf("copy snapshot file %q: %w", relative, err),
			closeError("snapshot output", relative, outputFile),
			closeError("snapshot source", relative, input),
		)
	}
	if err := outputFile.Close(); err != nil {
		return errors.Join(
			fmt.Errorf("close snapshot file %q: %w", relative, err),
			closeError("snapshot source", relative, input),
		)
	}
	if err := input.Close(); err != nil {
		return fmt.Errorf("close snapshot source %q: %w", relative, err)
	}
	return nil
}

func validateRelativePath(relative string) error {
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(relative)))
	if relative == "" || clean != relative || filepath.IsAbs(relative) || filepath.VolumeName(relative) != "" ||
		windowsVolumePattern.MatchString(relative) || strings.HasPrefix(relative, "//") ||
		!utf8.ValidString(relative) || strings.ContainsAny(relative, "\\:<>\"|?*") ||
		strings.IndexFunc(relative, unicode.IsControl) >= 0 ||
		relative == ".." || strings.HasPrefix(relative, "../") {
		return fmt.Errorf("unsafe governed source path %q", relative)
	}
	if err := validatePortablePathComponents(relative); err != nil {
		return err
	}
	return nil
}

func validateSourceFilePath(relative string) error {
	if relative == "." {
		return errors.New("governed source file path is dot")
	}
	return validateRelativePath(relative)
}

func validatePortablePathComponents(relative string) error {
	for component := range strings.SplitSeq(relative, "/") {
		if component == "" || strings.TrimRight(component, ". ") != component {
			return fmt.Errorf("governed source path %q has a nonportable component", relative)
		}
		stem, _, _ := strings.Cut(component, ".")
		upper := strings.ToUpper(stem)
		if upper == "CON" || upper == "PRN" || upper == "AUX" || upper == "NUL" ||
			(len(upper) == 4 && (strings.HasPrefix(upper, "COM") || strings.HasPrefix(upper, "LPT")) &&
				upper[3] >= '1' && upper[3] <= '9') {
			return fmt.Errorf("governed source path %q uses a reserved Windows component", relative)
		}
	}
	return nil
}

func sourceMode(mode os.FileMode) string {
	if mode&os.ModeSymlink != 0 {
		return "120000"
	}
	if mode.IsRegular() {
		if mode.Perm()&0o111 != 0 {
			return "100755"
		}
		return "100644"
	}
	return "unsupported"
}

func validateSourceRecord(record sourceRecord) error {
	if err := validateSourceFilePath(record.Path); err != nil {
		return err
	}
	if record.Mode != "100644" && record.Mode != "100755" && record.Mode != "120000" {
		return fmt.Errorf("source record %q has invalid mode %q", record.Path, record.Mode)
	}
	if record.Size < 0 {
		return fmt.Errorf("source record %q has negative size %d", record.Path, record.Size)
	}
	decoded, err := hex.DecodeString(record.SHA256)
	if err != nil || len(decoded) != sha256.Size || hex.EncodeToString(decoded) != record.SHA256 {
		return fmt.Errorf("source record %q has invalid SHA-256 %q", record.Path, record.SHA256)
	}
	if record.Mode == "120000" {
		if record.SymlinkTarget == "" || record.Size != int64(len(record.SymlinkTarget)) {
			return fmt.Errorf("symlink source record %q has inconsistent target", record.Path)
		}
		if err := validatePersistedSymlinkTarget(record.Path, record.SymlinkTarget); err != nil {
			return fmt.Errorf("symlink source record %q: %w", record.Path, err)
		}
		digest := sha256.Sum256([]byte(record.SymlinkTarget))
		if record.SHA256 != hex.EncodeToString(digest[:]) {
			return fmt.Errorf("symlink source record %q has inconsistent target digest", record.Path)
		}
	} else if record.SymlinkTarget != "" {
		return fmt.Errorf("regular source record %q has a symlink target", record.Path)
	}
	return nil
}

func validateSymlink(root, relative, target string) error {
	if err := validatePersistedSymlinkTarget(relative, target); err != nil {
		return fmt.Errorf("governed symlink %q has unsafe target %q", relative, target)
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve governed source root: %w", err)
	}
	link := filepath.Join(root, filepath.FromSlash(relative))
	resolved, err := filepath.EvalSymlinks(link)
	if err != nil {
		return fmt.Errorf("resolve governed symlink %q target %q: %w", relative, target, err)
	}
	contained, err := filepath.Rel(root, resolved)
	if err != nil || contained == ".." || strings.HasPrefix(contained, ".."+string(filepath.Separator)) || filepath.IsAbs(contained) {
		return fmt.Errorf("governed symlink %q target %q escapes source root", relative, target)
	}
	return nil
}

func validateSymlinkTarget(target string) error {
	if target == "" || !utf8.ValidString(target) || filepath.IsAbs(target) || filepath.VolumeName(target) != "" ||
		windowsVolumePattern.MatchString(target) || strings.HasPrefix(target, "//") ||
		strings.ContainsAny(target, "\\:") || strings.IndexFunc(target, unicode.IsControl) >= 0 {
		return fmt.Errorf("unsafe symlink target %q", target)
	}
	return nil
}

func validatePersistedSymlinkTarget(relative, target string) error {
	if err := validateSymlinkTarget(target); err != nil {
		return err
	}
	joined := path.Clean(path.Join(path.Dir(relative), target))
	if joined == ".." || strings.HasPrefix(joined, "../") {
		return fmt.Errorf("symlink target %q escapes governed source", target)
	}
	if err := validateSourceFilePath(joined); err != nil {
		return fmt.Errorf("symlink target %q resolves nonportably: %w", target, err)
	}
	return nil
}

func containedPath(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func ensureSnapshotParent(root, relative string) error {
	parent := filepath.Dir(filepath.FromSlash(relative))
	if parent == "." {
		return nil
	}
	current := root
	for component := range strings.SplitSeq(filepath.ToSlash(parent), "/") {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			if err := os.Mkdir(current, 0o755); err != nil {
				return fmt.Errorf("create snapshot directory %q: %w", current, err)
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect snapshot directory %q: %w", current, err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("snapshot parent %q is not a physical directory", current)
		}
	}
	return nil
}

func writeFingerprintFrame(writer hash.Hash, value []byte) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	if _, err := writer.Write(length[:]); err != nil {
		panic(fmt.Sprintf("hash length frame: %v", err))
	}
	if _, err := writer.Write(value); err != nil {
		panic(fmt.Sprintf("hash value frame: %v", err))
	}
}

func closeError(kind, relative string, closer io.Closer) error {
	if err := closer.Close(); err != nil {
		return fmt.Errorf("close %s %q: %w", kind, relative, err)
	}
	return nil
}
