package main

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

const historyArchiveDirectory = "eventloop/internal/tournament/revisions"

type historyArchiveRecord struct {
	OID               string
	PayloadSHA256     string
	Patch             string
	PatchSHA256       string
	PatchParent       string
	AliasTargetCommit string
	RootTree          string
	EventloopTree     string
	GojaEventloopTree string
	PayloadBytes      int
	LateDiscovery     int
}

var historyArchiveRecords = []historyArchiveRecord{
	{
		OID: "3249ec949f50e3743b34cdaf831728ec86e8796b", PayloadBytes: 283,
		PayloadSHA256:     "43f0ba8550c35e6b1a47200e66c3247622bf0d24a5f06067877362eddd826dc4",
		Patch:             "0001-index-on-no-branch-9b77ad1-Refresh-timer-poll-deadli.patch",
		PatchSHA256:       "34f0f29d9ac9f581b5e076bb45fb43bb8b197d8a6f3e1f29f440915e6580e865",
		PatchParent:       "9b77ad1d20f093759da7e0ff4a85fa50b5cf6f15",
		RootTree:          "7328ba8bdf18a3262a61761a2c13265e791659f4",
		EventloopTree:     "3d57c2de57c10cac02ea7f05f9a83c10ca9846f1",
		GojaEventloopTree: "69d8cf81666942396704d3d4bdb75208a0e523c6",
	},
	{
		OID: "537692fbb199d9e7e13255260ae878faff94fc02", PayloadBytes: 320,
		PayloadSHA256:     "1cd4d35604cae3f01df021aa7136d7b0a7a7080e4ca2e39045c55e658fd8cd70",
		AliasTargetCommit: "3249ec949f50e3743b34cdaf831728ec86e8796b",
	},
	{
		OID: "5f691abe5d4557c13eb7c16d42f60f203df24336", PayloadBytes: 261,
		PayloadSHA256:     "ab1c1b4beaf797cbcb78e134a30a408b48b3a903cce3528d8221b504830df9f7",
		AliasTargetCommit: "fa68be1139e0fac25349b3e7644ffd73a22d6616",
	},
	{
		OID: "60f89427c8b36ecb2b1c495309ac91187c26fd06", PayloadBytes: 261,
		PayloadSHA256:     "597ff77baa81e2109bf1475b06b4f75a4950551fc322438400462b038eedbfcd",
		Patch:             "0001-Temporary-WAKE-H2-candidate-43be6122.patch",
		PatchSHA256:       "5a5d995a3988240248f8b6f2b60ad623b23a4ba66c209ed87d0f28be264bf7dd",
		PatchParent:       "cd6a53322588420c9e2b5e19e5791b7b0696117f",
		RootTree:          "7d99eec56bf840cb4b832c29d1072b355a707910",
		EventloopTree:     "57b88dedc97ec680e4915cbf7b0181f3def008b7",
		GojaEventloopTree: "69d8cf81666942396704d3d4bdb75208a0e523c6",
	},
	{
		OID: "786998e8eb654d670dcf2174c1e362c784a08d3e", PayloadBytes: 403,
		PayloadSHA256:     "2b485f621d7ab4392b7edf904184e1f0a98d6e1e73d5469a6206bd5e70e9bcfb",
		AliasTargetCommit: "f7ef4c86843e1790bf0528975ab2c92ba3351702",
	},
	{
		OID: "819ee003aa8ff73d51e5a43dd35169db784f3111", PayloadBytes: 261,
		PayloadSHA256:     "e4bb6d29fac32e6f5c573b928b37c4bf909a61f01a307eb16ed92b96613eb25f",
		Patch:             "0001-Temporary-WAKE-H2-candidate-9e721d42.patch",
		PatchSHA256:       "434858ef0d7c4940859f0b618c1ba62d0d85e5f56af1d0dcdec4f9bfc43cd0ee",
		PatchParent:       "cd6a53322588420c9e2b5e19e5791b7b0696117f",
		RootTree:          "93f2b311557b83249ca59b5f9646f9a2677a3944",
		EventloopTree:     "35d7e1cfa4dc7d7485a13b39d7c55ad7fcffae83",
		GojaEventloopTree: "69d8cf81666942396704d3d4bdb75208a0e523c6",
	},
	{
		OID: "81caf61f96167e7b7f5ecf497af9110890ff6e03", PayloadBytes: 261,
		PayloadSHA256:     "b1c9083de73e77ff2f0c2825ec7753a77fa856dcc417fd5a06a82097ac91f6a9",
		Patch:             "0001-Temporary-WAKE-H2-candidate-f13eb33f.patch",
		PatchSHA256:       "d8839deb295d0c82ab7a0b8f9f87eb71308efcd6006b5637fe2f1e1164f008f9",
		PatchParent:       "cd6a53322588420c9e2b5e19e5791b7b0696117f",
		RootTree:          "7d387838addad98726792c47e30d4e3b7f824e88",
		EventloopTree:     "115d23747d2fe938c88b440e822c9bb40e2f61fe",
		GojaEventloopTree: "69d8cf81666942396704d3d4bdb75208a0e523c6",
	},
	{
		OID: "b3c342c981169d1b8b348c81a8e850a8a87911ee", PayloadBytes: 342,
		PayloadSHA256:     "48e18fca44ecc884e675d642cbd7800e105d964c19888cd30d4c44d55b328a5b",
		AliasTargetCommit: "8bbefe5623c5b94cd85aa8dda2f3ebe9007d3eba",
	},
	{
		OID: "bcadd4aaa61c7a9c1068a493c948bb88bf5fa038", PayloadBytes: 297,
		PayloadSHA256:     "5def94f6c0874461c531a6e2e71d490d9a33e57579795676a02b44b697951206",
		AliasTargetCommit: "8bbefe5623c5b94cd85aa8dda2f3ebe9007d3eba",
	},
	{
		OID: "f7ef4c86843e1790bf0528975ab2c92ba3351702", PayloadBytes: 403,
		PayloadSHA256:     "7ec816b4bb6c607ac2dc2105e65ff05800d3a3f593746d2ba657b2a47b8f4c83",
		Patch:             "0001-Test-E001-candidate-across-platforms.patch",
		PatchSHA256:       "1ae62529924d910ba7f038a8afdd7d5bbf54c2876100b033ccc3e152ab80f48a",
		PatchParent:       "c8e744e4867c351d5b83e438fd2cb438c9b04898",
		RootTree:          "c28e4b2a8cc0acf2f7795f3c82666273ec5dd6ec",
		EventloopTree:     "6eec43e57bfc858888a804e206b0d97335ba6d89",
		GojaEventloopTree: "69d8cf81666942396704d3d4bdb75208a0e523c6",
	},
	{
		OID: "f97fc084bb6a796dac41461a01f059a5845fbe47", PayloadBytes: 226,
		PayloadSHA256:     "5f54b38bfc32fd492f8231be1d625f7648c2954b1906a1e04ef26229b97e194c",
		AliasTargetCommit: "0def02e2ff987be01a38d237a5d84dae256a85ac",
	},
	{
		OID: "53e2f662adc245c9b63e06bb64977b0751dcff82", PayloadBytes: 305,
		PayloadSHA256:     "5757cc94cc79fcc877e8ff8d0b8626f6835fb5efa29cbc6ca0cf188420424c65",
		AliasTargetCommit: "0bc4ad0ae702ce2205615c31dcf37992d67ff9c8",
		LateDiscovery:     1,
	},
	{
		OID: "1396868d29689c659ff7782760e89423aa478cf4", PayloadBytes: 350,
		PayloadSHA256:     "f85df86a07aa8d4effc78cc3126ec493c2d5d9db5dc0da34fcd73628e4d04821",
		AliasTargetCommit: "0bc4ad0ae702ce2205615c31dcf37992d67ff9c8",
		LateDiscovery:     2,
	},
}

func historyArchivedCommitIDs() []string {
	result := make([]string, len(historyArchiveRecords))
	for index, record := range historyArchiveRecords {
		result[index] = record.OID
	}
	return result
}

func buildHistoryArchiveRecords(sourceRoot string) ([]historyCommitArchive, []historyPatchArchive, error) {
	archiveRoot := filepath.Join(sourceRoot, filepath.FromSlash(historyArchiveDirectory))
	commits := make([]historyCommitArchive, 0, len(historyArchiveRecords))
	patches := make([]historyPatchArchive, 0, 5)
	for _, record := range historyArchiveRecords {
		commitPath := filepath.Join(archiveRoot, "commits", record.OID+".commit.b64")
		encoded, err := os.ReadFile(commitPath)
		if err != nil {
			return nil, nil, fmt.Errorf("read commit archive metadata for %s: %w", record.OID, err)
		}
		commits = append(commits, historyCommitArchive{
			ID:               historyCommitArchiveID(record.OID),
			DiscoveryOrdinal: len(commits) + 1,
			OccurrenceID:     historyOccurrenceID(record.OID),
			Path:             "revisions/commits/" + record.OID + ".commit.b64",
			Encoding:         "rfc4648-base64-standard-single-line-lf",
			EncodedBytes:     len(encoded),
			EncodedSHA256:    fmt.Sprintf("%x", sha256.Sum256(encoded)),
			PayloadBytes:     record.PayloadBytes,
			PayloadSHA256:    record.PayloadSHA256,
			GitObjectSHA1:    record.OID,
		})
		if record.Patch == "" {
			continue
		}
		patchPath := filepath.Join(archiveRoot, record.Patch)
		data, err := os.ReadFile(patchPath)
		if err != nil {
			return nil, nil, fmt.Errorf("read patch archive metadata for %s: %w", record.OID, err)
		}
		patches = append(patches, historyPatchArchive{
			ID:                historyPatchArchiveID(record.PatchSHA256),
			DiscoveryOrdinal:  len(patches) + 1,
			OccurrenceID:      historyOccurrenceID(record.OID),
			Path:              "revisions/" + record.Patch,
			Format:            "git-format-patch-full-index-binary-v1",
			PatchBytes:        len(data),
			PatchSHA256:       record.PatchSHA256,
			BaseCommit:        record.PatchParent,
			RootTree:          record.RootTree,
			EventloopTree:     record.EventloopTree,
			GojaEventloopTree: record.GojaEventloopTree,
		})
	}
	return commits, patches, nil
}

func newHistoryAuthority(source historyGit) (historyGit, func(), error) {
	root, err := os.MkdirTemp("", "eventloop-history-authority.")
	if err != nil {
		return historyGit{}, func() {}, fmt.Errorf("create history authority: %w", err)
	}
	cleanupRoot := func() { _ = os.RemoveAll(root) }
	repository := filepath.Join(root, "authority.git")
	if err := runHistoryCommand(source.executable, "", source.environment, nil,
		"init", "--bare", "--object-format=sha1", repository); err != nil {
		cleanupRoot()
		return historyGit{}, func() {}, fmt.Errorf("initialize history authority: %w", err)
	}
	authority, cleanupGit, err := newHistoryGit(source.executable, repository)
	if err != nil {
		cleanupRoot()
		return historyGit{}, func() {}, err
	}
	cleanup := func() {
		cleanupGit()
		cleanupRoot()
	}
	if err := authority.run("fetch", "--no-tags", "--no-write-fetch-head", source.repository, historyEnd); err != nil {
		cleanup()
		return historyGit{}, func() {}, fmt.Errorf("seed anchored history authority: %w", err)
	}
	for _, record := range historyArchiveRecords {
		if authority.objectExists(record.OID + "^{commit}") {
			cleanup()
			return historyGit{}, func() {}, fmt.Errorf("anchored-only authority unexpectedly contains archived commit %s", record.OID)
		}
	}
	if err := rehydrateHistoryArchives(source.repository, authority); err != nil {
		cleanup()
		return historyGit{}, func() {}, err
	}
	if err := authority.run("fsck", "--strict", "--no-reflogs", "--no-dangling"); err != nil {
		cleanup()
		return historyGit{}, func() {}, fmt.Errorf("verify rehydrated history authority: %w", err)
	}
	return authority, cleanup, nil
}

func rehydrateHistoryArchives(sourceRoot string, authority historyGit) error {
	archiveRoot := filepath.Join(sourceRoot, filepath.FromSlash(historyArchiveDirectory))
	payloads, err := readHistoryCommitArchives(archiveRoot)
	if err != nil {
		return err
	}
	if err := reconstructHistoryPatchArchives(archiveRoot, authority); err != nil {
		return err
	}
	for _, record := range historyArchiveRecords {
		payload := payloads[record.OID]
		written, err := authority.input(payload, "hash-object", "-w", "-t", "commit", "--stdin")
		if err != nil {
			return fmt.Errorf("write archived commit %s: %w", record.OID, err)
		}
		if written != record.OID {
			return fmt.Errorf("archived commit object = %s, want %s", written, record.OID)
		}
	}
	for _, record := range historyArchiveRecords {
		root, err := authority.output("rev-parse", record.OID+"^{tree}")
		if err != nil {
			return err
		}
		expected, err := historyArchiveRecordRoot(record, payloads[record.OID])
		if err != nil {
			return err
		}
		if root != expected {
			return fmt.Errorf("archived commit %s root = %s, want %s", record.OID, root, expected)
		}
		if _, err := authority.output("rev-parse", root+":eventloop"); err != nil {
			return err
		}
		if _, err := authority.output("rev-parse", root+":goja-eventloop"); err != nil {
			return err
		}
	}
	return nil
}

func readHistoryCommitArchives(archiveRoot string) (map[string][]byte, error) {
	directory := filepath.Join(archiveRoot, "commits")
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("read commit archive directory: %w", err)
	}
	want := make(map[string]historyArchiveRecord, len(historyArchiveRecords))
	for _, record := range historyArchiveRecords {
		want[record.OID+".commit.b64"] = record
	}
	if len(entries) != len(want) {
		return nil, fmt.Errorf("commit archive files = %d, want %d", len(entries), len(want))
	}
	result := make(map[string][]byte, len(want))
	for _, entry := range entries {
		record, ok := want[entry.Name()]
		if !ok {
			return nil, fmt.Errorf("unexpected commit archive %q", entry.Name())
		}
		path := filepath.Join(directory, entry.Name())
		if err := requireHistoryRegularFile(path, 0o644); err != nil {
			return nil, err
		}
		encoded, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read commit archive %q: %w", entry.Name(), err)
		}
		if len(encoded) < 2 || encoded[len(encoded)-1] != '\n' || bytes.Count(encoded, []byte{'\n'}) != 1 {
			return nil, fmt.Errorf("commit archive %q is not one LF-terminated base64 line", entry.Name())
		}
		payload, err := base64.StdEncoding.Strict().DecodeString(string(encoded[:len(encoded)-1]))
		if err != nil {
			return nil, fmt.Errorf("decode commit archive %q: %w", entry.Name(), err)
		}
		canonical := append([]byte(base64.StdEncoding.EncodeToString(payload)), '\n')
		if !bytes.Equal(encoded, canonical) {
			return nil, fmt.Errorf("commit archive %q is not canonical base64", entry.Name())
		}
		if len(payload) != record.PayloadBytes || fmt.Sprintf("%x", sha256.Sum256(payload)) != record.PayloadSHA256 {
			return nil, fmt.Errorf("commit archive %q payload identity changed", entry.Name())
		}
		hash := sha1.New()
		fmt.Fprintf(hash, "commit %d%c", len(payload), 0)
		_, _ = hash.Write(payload)
		if got := fmt.Sprintf("%x", hash.Sum(nil)); got != record.OID {
			return nil, fmt.Errorf("commit archive %q object = %s, want %s", entry.Name(), got, record.OID)
		}
		result[record.OID] = payload
	}
	return result, nil
}

func reconstructHistoryPatchArchives(archiveRoot string, authority historyGit) error {
	entries, err := os.ReadDir(archiveRoot)
	if err != nil {
		return fmt.Errorf("read patch archive directory: %w", err)
	}
	want := make(map[string]historyArchiveRecord)
	for _, record := range historyArchiveRecords {
		if record.Patch != "" {
			want[record.Patch] = record
		}
	}
	found := make([]string, 0, len(want))
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".patch") {
			found = append(found, entry.Name())
		}
	}
	slices.Sort(found)
	wantNames := make([]string, 0, len(want))
	for name := range want {
		wantNames = append(wantNames, name)
	}
	slices.Sort(wantNames)
	if !slices.Equal(found, wantNames) {
		return fmt.Errorf("patch archive set = %q, want %q", found, wantNames)
	}
	scratch, err := os.MkdirTemp("", "eventloop-history-patches.")
	if err != nil {
		return fmt.Errorf("create patch reconstruction scratch: %w", err)
	}
	defer os.RemoveAll(scratch)
	for _, name := range wantNames {
		record := want[name]
		path := filepath.Join(archiveRoot, name)
		if err := requireHistoryRegularFile(path, 0o644); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read patch archive %q: %w", name, err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != record.PatchSHA256 {
			return fmt.Errorf("patch archive %q SHA-256 = %s, want %s", name, got, record.PatchSHA256)
		}
		indexPath := filepath.Join(scratch, record.OID+".index")
		environment := append(slices.Clone(authority.environment), "GIT_INDEX_FILE="+indexPath)
		if err := runHistoryCommand(authority.executable, authority.repository, environment, nil,
			"read-tree", record.PatchParent+"^{tree}"); err != nil {
			return fmt.Errorf("read patch parent for %s: %w", record.OID, err)
		}
		if err := runHistoryCommand(authority.executable, authority.repository, environment, nil,
			"apply", "--cached", "--binary", "--whitespace=error-all", path); err != nil {
			return fmt.Errorf("apply patch for %s: %w", record.OID, err)
		}
		root, err := historyCommandOutput(authority.executable, authority.repository, environment, nil, "write-tree")
		if err != nil {
			return fmt.Errorf("write patch tree for %s: %w", record.OID, err)
		}
		if root != record.RootTree {
			return fmt.Errorf("patch root for %s = %s, want %s", record.OID, root, record.RootTree)
		}
		eventloopTree, err := authority.output("rev-parse", root+":eventloop")
		if err != nil {
			return err
		}
		gojaTree, err := authority.output("rev-parse", root+":goja-eventloop")
		if err != nil {
			return err
		}
		if eventloopTree != record.EventloopTree || gojaTree != record.GojaEventloopTree {
			return fmt.Errorf("patch component trees for %s changed", record.OID)
		}
	}
	return nil
}

func historyArchiveRecordRoot(record historyArchiveRecord, payload []byte) (string, error) {
	commit, err := parseHistoryCommit(record.OID, payload)
	if err != nil {
		return "", err
	}
	return commit.rootTree, nil
}

func requireHistoryRegularFile(path string, permissions os.FileMode) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect history archive %q: %w", path, err)
	}
	if !info.Mode().IsRegular() || authorityPerm(info.Mode()) != authorityPerm(permissions) {
		return fmt.Errorf("history archive %q mode = %s, want regular %04o", path, info.Mode(), permissions)
	}
	return nil
}

func runHistoryCommand(executable, directory string, environment []string, input []byte, arguments ...string) error {
	_, err := historyCommandOutput(executable, directory, environment, input, arguments...)
	return err
}

func historyCommandOutput(executable, directory string, environment []string, input []byte, arguments ...string) (string, error) {
	command := exec.Command(executable, arguments...)
	if directory != "" {
		command.Dir = directory
	}
	command.Env = environment
	command.Stdin = bytes.NewReader(input)
	output, err := command.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return "", fmt.Errorf("git %v: %w: %s", arguments, err, exit.Stderr)
		}
		return "", fmt.Errorf("git %v: %w", arguments, err)
	}
	return strings.TrimSpace(string(output)), nil
}
