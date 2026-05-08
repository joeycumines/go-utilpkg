package tournament

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

type materializationArchivePatchFormat uint8

const (
	materializationArchivePatchAbbrev7 materializationArchivePatchFormat = iota + 1
	materializationArchivePatchFullIndex
)

type materializationArchiveFile struct {
	path   string
	mode   string
	blob   string
	sha256 string
	bytes  int64
}

type materializationArchiveSpec struct {
	files        []materializationArchiveFile
	roots        []string
	archive      timerReferenceMaterializationArchive
	id           string
	objectFormat string
	patchFormat  materializationArchivePatchFormat
	pathCount    int
	payloadBytes int64
}

type materializationArchiveTreeEntry struct {
	path       string
	mode       string
	objectType string
	object     string
}

type materializationArchiveGitOutput struct {
	stdout []byte
	stderr []byte
	err    error
}

type materializationArchiveLiveFile struct {
	path    string
	mode    string
	payload []byte
}

type materializationArchiveLiveDirectory struct {
	path string
	mode string
}

type materializationArchiveLive struct {
	directories []materializationArchiveLiveDirectory
	files       []materializationArchiveLiveFile
}

type materializationArchiveCapture struct {
	files          []materializationArchiveFile
	packageTrees   []string
	componentTrees []string
	patch          []byte
	emptyTree      string
	reconstructed  string
	payloadBytes   int64
}

func verifyMaterializationArchive(t *testing.T, spec materializationArchiveSpec) {
	t.Helper()
	if err := validateMaterializationArchiveSpec(spec); err != nil {
		t.Fatalf("materialization archive %q specification: %v", spec.id, err)
	}
	repository, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("materialization archive %q repository: %v", spec.id, err)
	}
	patchPath := filepath.Join(
		repository,
		"eventloop",
		"internal",
		"tournament",
		filepath.FromSlash(spec.archive.PatchPath),
	)
	patchSnapshot, err := readMaterializationArchivePatch(patchPath)
	if err != nil {
		t.Fatalf("materialization archive %q patch: %v", spec.id, err)
	}
	patch := patchSnapshot.payload
	if int64(len(patch)) != spec.archive.PatchBytes {
		t.Fatalf("materialization archive %q patch bytes = %d, want %d", spec.id, len(patch), spec.archive.PatchBytes)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(patch)); got != spec.archive.PatchSHA256 {
		t.Fatalf("materialization archive %q patch SHA-256 = %s, want %s", spec.id, got, spec.archive.PatchSHA256)
	}
	liveBefore, err := loadMaterializationArchiveLive(repository, spec.roots)
	if err != nil {
		t.Fatalf("materialization archive %q live census: %v", spec.id, err)
	}
	if err := validateMaterializationArchiveLive(spec, liveBefore); err != nil {
		t.Fatalf("materialization archive %q live authority: %v", spec.id, err)
	}

	temporary := t.TempDir()
	isolated := filepath.Join(temporary, "isolated.git")
	if err := os.Mkdir(isolated, 0o700); err != nil {
		t.Fatalf("materialization archive %q isolated repository: %v", spec.id, err)
	}
	environment, err := materializationArchiveGitEnvironment(temporary)
	if err != nil {
		t.Fatalf("materialization archive %q environment: %v", spec.id, err)
	}
	requireMaterializationArchiveGit(t, spec.id, isolated, environment, nil,
		"init", "--bare", "--quiet", "--object-format="+spec.objectFormat)
	if got := materializationArchiveScalar(t, spec.id, isolated, environment, "rev-parse", "--show-object-format"); got != spec.objectFormat {
		t.Fatalf("materialization archive %q object format = %s, want %s", spec.id, got, spec.objectFormat)
	}
	assertMaterializationArchiveNoAlternates(t, spec.id, isolated)
	emptyObject := requireMaterializationArchiveGit(t, spec.id, isolated, environment, nil,
		"hash-object", "-t", "tree", "-w", "--stdin")
	if got := materializationArchiveOutputScalar(t, spec.id, []string{"hash-object", "-t", "tree", "-w", "--stdin"}, emptyObject.stdout); got != spec.archive.EmptyTree {
		t.Fatalf("materialization archive %q written empty tree = %s, want %s", spec.id, got, spec.archive.EmptyTree)
	}
	requireMaterializationArchiveGit(t, spec.id, isolated, environment, nil, "read-tree", "--empty")
	if got := materializationArchiveScalar(t, spec.id, isolated, environment, "write-tree"); got != spec.archive.EmptyTree {
		t.Fatalf("materialization archive %q empty tree = %s, want %s", spec.id, got, spec.archive.EmptyTree)
	}
	requireMaterializationArchiveGit(t, spec.id, isolated, environment, patch,
		"apply", "--cached", "--binary", "--whitespace=error-all", "-")
	if got := materializationArchiveScalar(t, spec.id, isolated, environment, "write-tree"); got != spec.archive.ReconstructedTree {
		t.Fatalf("materialization archive %q reconstructed tree = %s, want %s", spec.id, got, spec.archive.ReconstructedTree)
	}

	refPrefix := "refs/tournament/materialization/" + spec.id + "/"
	requireMaterializationArchiveGit(t, spec.id, isolated, environment, nil,
		"update-ref", refPrefix+"empty", spec.archive.EmptyTree)
	requireMaterializationArchiveGit(t, spec.id, isolated, environment, nil,
		"update-ref", refPrefix+"reconstructed", spec.archive.ReconstructedTree)
	treeOutput := requireMaterializationArchiveGit(t, spec.id, isolated, environment, nil,
		"ls-tree", "-r", "-z", "--full-tree", spec.archive.ReconstructedTree)
	entries, err := parseMaterializationArchiveTree(treeOutput.stdout)
	if err != nil {
		t.Fatalf("materialization archive %q tree: %v", spec.id, err)
	}
	if len(entries) != len(spec.files) {
		t.Fatalf("materialization archive %q tree paths = %d, want %d", spec.id, len(entries), len(spec.files))
	}
	for index, file := range spec.files {
		entry := entries[index]
		if entry.path != file.path || entry.mode != file.mode || entry.objectType != "blob" || entry.object != file.blob {
			t.Fatalf("materialization archive %q tree entry %d = %+v, want %+v", spec.id, index, entry, file)
		}
		blobOutput := requireMaterializationArchiveGit(t, spec.id, isolated, environment, nil,
			"cat-file", "blob", file.blob)
		if int64(len(blobOutput.stdout)) != file.bytes {
			t.Fatalf("materialization archive %q path %s bytes = %d, want %d", spec.id, file.path, len(blobOutput.stdout), file.bytes)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(blobOutput.stdout)); got != file.sha256 {
			t.Fatalf("materialization archive %q path %s SHA-256 = %s, want %s", spec.id, file.path, got, file.sha256)
		}
		if got := materializationArchiveBlobSHA1(blobOutput.stdout); got != file.blob {
			t.Fatalf("materialization archive %q path %s framed blob = %s, want %s", spec.id, file.path, got, file.blob)
		}
		if !bytes.Equal(blobOutput.stdout, liveBefore.files[index].payload) {
			t.Fatalf("materialization archive %q path %s differs from live bytes", spec.id, file.path)
		}
	}

	patchOutput := requireMaterializationArchiveGit(t, spec.id, isolated, environment, nil,
		materializationArchivePatchArguments(spec)...)
	if !bytes.Equal(patchOutput.stdout, patch) {
		t.Fatalf("materialization archive %q canonical patch differs", spec.id)
	}
	fsck := materializationArchiveGit(isolated, environment, nil,
		"fsck", "--strict", "--full", "--no-reflogs", "--unreachable")
	if fsck.err != nil {
		t.Fatalf("materialization archive %q fsck: %v\nstdout:\n%s\nstderr:\n%s", spec.id, fsck.err, fsck.stdout, fsck.stderr)
	}
	if err := validateMaterializationArchiveFSCK(fsck.stdout, fsck.stderr); err != nil {
		t.Fatalf("materialization archive %q fsck: %v\nstdout:\n%s\nstderr:\n%s", spec.id, err, fsck.stdout, fsck.stderr)
	}
	assertMaterializationArchiveNoAlternates(t, spec.id, isolated)
	liveAfter, err := loadMaterializationArchiveLive(repository, spec.roots)
	if err != nil {
		t.Fatalf("materialization archive %q final live census: %v", spec.id, err)
	}
	if !reflect.DeepEqual(liveAfter, liveBefore) {
		t.Fatalf("materialization archive %q live bytes changed during verification", spec.id)
	}
	patchAfter, err := readMaterializationArchivePatch(patchPath)
	if err != nil {
		t.Fatalf("materialization archive %q final patch: %v", spec.id, err)
	}
	if err := validateMaterializationArchivePatchStable(patchSnapshot, patchAfter); err != nil {
		t.Fatalf("materialization archive %q: %v", spec.id, err)
	}
}

func captureMaterializationArchive(
	t *testing.T,
	id string,
	roots []string,
	patchFormat materializationArchivePatchFormat,
) materializationArchiveCapture {
	t.Helper()
	if err := validateMaterializationArchiveCaptureInput(id, roots, patchFormat); err != nil {
		t.Fatalf("materialization archive capture: %v", err)
	}
	repository, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	liveBefore, err := loadMaterializationArchiveLive(repository, roots)
	if err != nil {
		t.Fatalf("materialization archive capture live census: %v", err)
	}
	files := make([]materializationArchiveFile, len(liveBefore.files))
	var payloadBytes int64
	for index, live := range liveBefore.files {
		files[index] = materializationArchiveFile{
			path:   live.path,
			mode:   live.mode,
			blob:   materializationArchiveBlobSHA1(live.payload),
			sha256: fmt.Sprintf("%x", sha256.Sum256(live.payload)),
			bytes:  int64(len(live.payload)),
		}
		payloadBytes += int64(len(live.payload))
	}
	temporary := t.TempDir()
	isolated := filepath.Join(temporary, "isolated.git")
	if err := os.Mkdir(isolated, 0o700); err != nil {
		t.Fatal(err)
	}
	environment, err := materializationArchiveGitEnvironment(temporary)
	if err != nil {
		t.Fatal(err)
	}
	requireMaterializationArchiveGit(t, id, isolated, environment, nil,
		"init", "--bare", "--quiet", "--object-format=sha1")
	assertMaterializationArchiveNoAlternates(t, id, isolated)
	emptyObject := requireMaterializationArchiveGit(t, id, isolated, environment, nil,
		"hash-object", "-t", "tree", "-w", "--stdin")
	emptyTree := materializationArchiveOutputScalar(t, id, []string{"hash-object", "-t", "tree", "-w", "--stdin"}, emptyObject.stdout)
	requireMaterializationArchiveGit(t, id, isolated, environment, nil, "read-tree", "--empty")
	indexInfo := make([]byte, 0, len(files)*128)
	for index, file := range files {
		blob := requireMaterializationArchiveGit(t, id, isolated, environment, liveBefore.files[index].payload,
			"hash-object", "-t", "blob", "-w", "--stdin")
		if got := materializationArchiveOutputScalar(t, id, []string{"hash-object", "-t", "blob", "-w", "--stdin"}, blob.stdout); got != file.blob {
			t.Fatalf("materialization archive capture path %q blob = %s, want %s", file.path, got, file.blob)
		}
		indexInfo = append(indexInfo, file.mode...)
		indexInfo = append(indexInfo, ' ')
		indexInfo = append(indexInfo, file.blob...)
		indexInfo = append(indexInfo, '\t')
		indexInfo = append(indexInfo, file.path...)
		indexInfo = append(indexInfo, 0)
	}
	requireMaterializationArchiveGit(t, id, isolated, environment, indexInfo,
		"update-index", "-z", "--index-info")
	reconstructed := materializationArchiveScalar(t, id, isolated, environment, "write-tree")
	packageTrees := make([]string, len(roots))
	componentTrees := make([]string, len(roots))
	for index, root := range roots {
		packageTrees[index] = materializationArchiveScalar(t, id, isolated, environment,
			"rev-parse", reconstructed+":"+root)
		componentTree, err := materializationArchiveComponentTree(root, liveBefore)
		if err != nil {
			t.Fatalf("materialization archive capture component %q: %v", root, err)
		}
		componentTrees[index] = componentTree
	}
	refPrefix := "refs/tournament/materialization/" + id + "/"
	requireMaterializationArchiveGit(t, id, isolated, environment, nil, "update-ref", refPrefix+"empty", emptyTree)
	requireMaterializationArchiveGit(t, id, isolated, environment, nil, "update-ref", refPrefix+"reconstructed", reconstructed)
	fsck := materializationArchiveGit(isolated, environment, nil,
		"fsck", "--strict", "--full", "--no-reflogs", "--unreachable")
	if fsck.err != nil {
		t.Fatalf("materialization archive capture fsck: %v\nstdout:\n%s\nstderr:\n%s", fsck.err, fsck.stdout, fsck.stderr)
	}
	if err := validateMaterializationArchiveFSCK(fsck.stdout, fsck.stderr); err != nil {
		t.Fatalf("materialization archive capture fsck: %v", err)
	}
	patchSpec := materializationArchiveSpec{
		archive: timerReferenceMaterializationArchive{
			EmptyTree:         emptyTree,
			ReconstructedTree: reconstructed,
		},
		patchFormat: patchFormat,
	}
	patch := requireMaterializationArchiveGit(t, id, isolated, environment, nil,
		materializationArchivePatchArguments(patchSpec)...).stdout
	assertMaterializationArchiveNoAlternates(t, id, isolated)
	liveAfter, err := loadMaterializationArchiveLive(repository, roots)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(liveAfter, liveBefore) {
		t.Fatal("materialization archive capture live bytes changed")
	}
	return materializationArchiveCapture{
		files:          files,
		packageTrees:   packageTrees,
		componentTrees: componentTrees,
		patch:          patch,
		emptyTree:      emptyTree,
		reconstructed:  reconstructed,
		payloadBytes:   payloadBytes,
	}
}

func validateMaterializationArchiveCaptureInput(
	id string,
	roots []string,
	patchFormat materializationArchivePatchFormat,
) error {
	if len(id) != 4 {
		return fmt.Errorf("ID %q is not four digits", id)
	}
	for _, value := range id {
		if value < '0' || value > '9' {
			return fmt.Errorf("ID %q is not four digits", id)
		}
	}
	if patchFormat != materializationArchivePatchAbbrev7 && patchFormat != materializationArchivePatchFullIndex {
		return fmt.Errorf("patch format %d is invalid", patchFormat)
	}
	folded := make(map[string]struct{}, len(roots))
	for index, root := range roots {
		if err := validateMaterializationArchivePath(root); err != nil {
			return fmt.Errorf("root %d: %w", index, err)
		}
		if index != 0 && roots[index-1] >= root {
			return fmt.Errorf("roots are not strictly sorted at %q", root)
		}
		key := strings.ToLower(root)
		if _, exists := folded[key]; exists {
			return fmt.Errorf("root %q has a case-fold collision", root)
		}
		folded[key] = struct{}{}
		for prior := range roots[:index] {
			if strings.HasPrefix(root, roots[prior]+"/") {
				return fmt.Errorf("root %q overlaps %q", root, roots[prior])
			}
		}
	}
	if len(roots) == 0 {
		return fmt.Errorf("roots are empty")
	}
	return nil
}

func materializationArchiveComponentTree(root string, live materializationArchiveLive) (string, error) {
	type record struct {
		path   string
		mode   string
		size   int64
		sha256 string
	}
	records := make([]record, 0, len(live.directories)+len(live.files))
	rootFound := false
	var payloadBytes int64
	for _, directory := range live.directories {
		relative := ""
		switch {
		case directory.path == root:
			relative = "."
			rootFound = true
		case strings.HasPrefix(directory.path, root+"/"):
			relative = strings.TrimPrefix(directory.path, root+"/")
		default:
			continue
		}
		records = append(records, record{path: relative, mode: directory.mode})
	}
	for _, file := range live.files {
		if !strings.HasPrefix(file.path, root+"/") {
			continue
		}
		relative := strings.TrimPrefix(file.path, root+"/")
		records = append(records, record{
			path:   relative,
			mode:   file.mode,
			size:   int64(len(file.payload)),
			sha256: fmt.Sprintf("%x", sha256.Sum256(file.payload)),
		})
		payloadBytes += int64(len(file.payload))
	}
	if !rootFound {
		return "", fmt.Errorf("root directory is absent")
	}
	sort.Slice(records, func(left, right int) bool { return records[left].path < records[right].path })
	fields := []string{
		"os-root-complete-v1",
		strconv.Itoa(len(records)),
		strconv.FormatInt(payloadBytes, 10),
	}
	for _, record := range records {
		fields = append(fields,
			record.path,
			record.mode,
			strconv.FormatInt(record.size, 10),
			record.sha256,
			"",
		)
	}
	digest := framedTimerSeal("go-utilpkg-eventloop-tournament-component-tree-v1", fields...)
	return fmt.Sprintf("%x", digest), nil
}

func validateMaterializationArchiveSpec(spec materializationArchiveSpec) error {
	if len(spec.id) != 4 {
		return fmt.Errorf("ID %q is not four digits", spec.id)
	}
	for _, value := range spec.id {
		if value < '0' || value > '9' {
			return fmt.Errorf("ID %q is not four digits", spec.id)
		}
	}
	if spec.objectFormat != "sha1" {
		return fmt.Errorf("object format %q is not sha1", spec.objectFormat)
	}
	if spec.patchFormat != materializationArchivePatchAbbrev7 && spec.patchFormat != materializationArchivePatchFullIndex {
		return fmt.Errorf("patch format %d is invalid", spec.patchFormat)
	}
	if err := validateMaterializationArchivePath(spec.archive.PatchPath); err != nil {
		return fmt.Errorf("patch path: %w", err)
	}
	if !validMaterializationArchiveHex(spec.archive.PatchSHA256, sha256.Size*2) {
		return fmt.Errorf("patch SHA-256 %q is invalid", spec.archive.PatchSHA256)
	}
	if spec.archive.PatchBytes <= 0 {
		return fmt.Errorf("patch bytes %d is not positive", spec.archive.PatchBytes)
	}
	if spec.archive.EmptyTree != "4b825dc642cb6eb9a060e54bf8d69288fbee4904" {
		return fmt.Errorf("empty tree %q is invalid", spec.archive.EmptyTree)
	}
	if !validMaterializationArchiveHex(spec.archive.ReconstructedTree, sha1.Size*2) {
		return fmt.Errorf("reconstructed tree %q is invalid", spec.archive.ReconstructedTree)
	}
	if spec.pathCount != len(spec.files) || spec.pathCount <= 0 {
		return fmt.Errorf("path count %d differs from %d files", spec.pathCount, len(spec.files))
	}
	if len(spec.roots) == 0 {
		return fmt.Errorf("roots are empty")
	}
	rootCounts := make([]int, len(spec.roots))
	rootFolded := make(map[string]struct{}, len(spec.roots))
	for index, root := range spec.roots {
		if err := validateMaterializationArchivePath(root); err != nil {
			return fmt.Errorf("root %d: %w", index, err)
		}
		if index != 0 && spec.roots[index-1] >= root {
			return fmt.Errorf("roots are not strictly sorted at %q", root)
		}
		folded := strings.ToLower(root)
		if _, exists := rootFolded[folded]; exists {
			return fmt.Errorf("root %q has a case-fold collision", root)
		}
		rootFolded[folded] = struct{}{}
		for prior := range spec.roots[:index] {
			if strings.HasPrefix(root, spec.roots[prior]+"/") {
				return fmt.Errorf("root %q overlaps %q", root, spec.roots[prior])
			}
		}
	}
	total := int64(0)
	pathFolded := make(map[string]struct{}, len(spec.files))
	for index, file := range spec.files {
		if err := validateMaterializationArchivePath(file.path); err != nil {
			return fmt.Errorf("file %d: %w", index, err)
		}
		if path.Ext(file.path) != ".go" {
			return fmt.Errorf("file %q is not Go source", file.path)
		}
		if index != 0 && spec.files[index-1].path >= file.path {
			return fmt.Errorf("files are not strictly sorted at %q", file.path)
		}
		folded := strings.ToLower(file.path)
		if _, exists := pathFolded[folded]; exists {
			return fmt.Errorf("file %q has a case-fold collision", file.path)
		}
		pathFolded[folded] = struct{}{}
		if file.mode != "100644" {
			return fmt.Errorf("file %q mode = %q", file.path, file.mode)
		}
		if !validMaterializationArchiveHex(file.blob, sha1.Size*2) {
			return fmt.Errorf("file %q blob = %q", file.path, file.blob)
		}
		if !validMaterializationArchiveHex(file.sha256, sha256.Size*2) {
			return fmt.Errorf("file %q SHA-256 = %q", file.path, file.sha256)
		}
		if file.bytes < 0 {
			return fmt.Errorf("file %q bytes = %d", file.path, file.bytes)
		}
		matched := -1
		for rootIndex, root := range spec.roots {
			if strings.HasPrefix(file.path, root+"/") {
				if matched != -1 {
					return fmt.Errorf("file %q belongs to multiple roots", file.path)
				}
				matched = rootIndex
			}
		}
		if matched == -1 {
			return fmt.Errorf("file %q is outside the declared roots", file.path)
		}
		rootCounts[matched]++
		total += file.bytes
	}
	for index, count := range rootCounts {
		if count == 0 {
			return fmt.Errorf("root %q has no files", spec.roots[index])
		}
	}
	if total != spec.payloadBytes {
		return fmt.Errorf("payload bytes %d differs from file total %d", spec.payloadBytes, total)
	}
	return nil
}

func parseMaterializationArchiveTree(data []byte) ([]materializationArchiveTreeEntry, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("tree output is empty")
	}
	entries := make([]materializationArchiveTreeEntry, 0)
	for offset := 0; offset < len(data); {
		end := bytes.IndexByte(data[offset:], 0)
		if end == -1 {
			return nil, fmt.Errorf("tree output lacks a final NUL")
		}
		end += offset
		record := data[offset:end]
		if len(record) == 0 {
			return nil, fmt.Errorf("tree output contains an empty record")
		}
		tab := bytes.IndexByte(record, '\t')
		if tab == -1 || tab == len(record)-1 {
			return nil, fmt.Errorf("tree record %q has no path separator", record)
		}
		header := record[:tab]
		firstSpace := bytes.IndexByte(header, ' ')
		if firstSpace == -1 {
			return nil, fmt.Errorf("tree record %q has no type separator", record)
		}
		secondRelative := bytes.IndexByte(header[firstSpace+1:], ' ')
		if secondRelative == -1 {
			return nil, fmt.Errorf("tree record %q has no object separator", record)
		}
		secondSpace := firstSpace + 1 + secondRelative
		if bytes.IndexByte(header[secondSpace+1:], ' ') != -1 {
			return nil, fmt.Errorf("tree record %q has extra header fields", record)
		}
		entry := materializationArchiveTreeEntry{
			mode:       string(header[:firstSpace]),
			objectType: string(header[firstSpace+1 : secondSpace]),
			object:     string(header[secondSpace+1:]),
			path:       string(record[tab+1:]),
		}
		if len(entry.mode) != 6 {
			return nil, fmt.Errorf("tree path %q mode %q is invalid", entry.path, entry.mode)
		}
		if _, err := strconv.ParseUint(entry.mode, 8, 32); err != nil {
			return nil, fmt.Errorf("tree path %q mode %q is invalid", entry.path, entry.mode)
		}
		if entry.objectType == "" || !validMaterializationArchiveHex(entry.object, sha1.Size*2) {
			return nil, fmt.Errorf("tree path %q object identity is invalid", entry.path)
		}
		if len(entries) != 0 && entries[len(entries)-1].path >= entry.path {
			return nil, fmt.Errorf("tree paths are not strictly sorted at %q", entry.path)
		}
		entries = append(entries, entry)
		offset = end + 1
	}
	return entries, nil
}

func loadMaterializationArchiveLive(repository string, roots []string) (materializationArchiveLive, error) {
	live := materializationArchiveLive{}
	for _, root := range roots {
		if err := validateMaterializationArchivePath(root); err != nil {
			return materializationArchiveLive{}, fmt.Errorf("root %q: %w", root, err)
		}
		rootPath := filepath.Join(repository, filepath.FromSlash(root))
		info, err := os.Lstat(rootPath)
		if err != nil {
			return materializationArchiveLive{}, fmt.Errorf("root %q: %w", root, err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return materializationArchiveLive{}, fmt.Errorf("root %q is not a physical directory", root)
		}
		err = filepath.WalkDir(rootPath, func(filePath string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("path %q is a symlink", filePath)
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(repository, filePath)
			if err != nil {
				return err
			}
			repositoryPath := filepath.ToSlash(relative)
			if err := validateMaterializationArchivePath(repositoryPath); err != nil {
				return err
			}
			if entry.IsDir() {
				live.directories = append(live.directories, materializationArchiveLiveDirectory{
					path: repositoryPath,
					mode: "040000",
				})
				return nil
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("path %q is not a regular file", filePath)
			}
			if path.Ext(repositoryPath) != ".go" {
				return fmt.Errorf("path %q is not Go source", repositoryPath)
			}
			if info.Mode().Perm()&0o111 != 0 {
				return fmt.Errorf("path %q is executable Go source", repositoryPath)
			}
			payload, err := os.ReadFile(filePath)
			if err != nil {
				return err
			}
			live.files = append(live.files, materializationArchiveLiveFile{
				path:    repositoryPath,
				mode:    "100644",
				payload: payload,
			})
			return nil
		})
		if err != nil {
			return materializationArchiveLive{}, fmt.Errorf("root %q: %w", root, err)
		}
	}
	sort.Slice(live.directories, func(left, right int) bool {
		return live.directories[left].path < live.directories[right].path
	})
	sort.Slice(live.files, func(left, right int) bool { return live.files[left].path < live.files[right].path })
	folded := make(map[string]string, len(live.directories)+len(live.files))
	for _, directory := range live.directories {
		key := strings.ToLower(directory.path)
		if prior, exists := folded[key]; exists {
			return materializationArchiveLive{}, fmt.Errorf("live paths %q and %q collide case-insensitively", prior, directory.path)
		}
		folded[key] = directory.path
	}
	for _, file := range live.files {
		key := strings.ToLower(file.path)
		if prior, exists := folded[key]; exists {
			return materializationArchiveLive{}, fmt.Errorf("live paths %q and %q collide case-insensitively", prior, file.path)
		}
		folded[key] = file.path
	}
	return live, nil
}

func validateMaterializationArchiveLive(spec materializationArchiveSpec, live materializationArchiveLive) error {
	if len(live.files) != len(spec.files) {
		return fmt.Errorf("live paths = %d, want %d", len(live.files), len(spec.files))
	}
	for index, file := range spec.files {
		if live.files[index].path != file.path {
			return fmt.Errorf("live path %d = %q, want %q", index, live.files[index].path, file.path)
		}
		if live.files[index].mode != file.mode {
			return fmt.Errorf("live path %q mode = %s, want %s", file.path, live.files[index].mode, file.mode)
		}
		if int64(len(live.files[index].payload)) != file.bytes {
			return fmt.Errorf("live path %q bytes = %d, want %d", file.path, len(live.files[index].payload), file.bytes)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(live.files[index].payload)); got != file.sha256 {
			return fmt.Errorf("live path %q SHA-256 = %s, want %s", file.path, got, file.sha256)
		}
		if got := materializationArchiveBlobSHA1(live.files[index].payload); got != file.blob {
			return fmt.Errorf("live path %q blob = %s, want %s", file.path, got, file.blob)
		}
	}
	return nil
}
