package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

const (
	historySchemaVersion = 2
	historyBoundary      = "9a54e44550496dd5e9c460e5fcdb4d08aca97497"
	historyStart         = "506d6643cc1d45b1da156096870991ecb30b8847"
	historyEnd           = "469fd952ed251edc7ea1d2bb0faf4e04fc94dd88"
)

var historyArchivedCommits = historyArchivedCommitIDs()

var historyOIDPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
var historySHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func historyCommand(arguments []string) int {
	if len(arguments) == 0 {
		return commandError(errors.New("history requires generate, floor-generate, verify, or audit-live"))
	}
	switch arguments[0] {
	case "generate":
		return historyGenerateCommand(arguments[1:])
	case "floor-generate":
		return historyFloorGenerateCommand(arguments[1:])
	case "verify":
		return historyVerifyCommand(arguments[1:])
	case "audit-live":
		return historyAuditCommand(arguments[1:])
	default:
		return commandError(fmt.Errorf("unknown history operation %q", arguments[0]))
	}
}

func historyGenerateCommand(arguments []string) int {
	flags := flag.NewFlagSet("history generate", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	gitPath := flags.String("git", "", "absolute Git executable")
	repository := flags.String("repository", "", "absolute repository root")
	output := flags.String("output", "", "new inventory path")
	if err := flags.Parse(arguments); err != nil {
		return commandError(err)
	}
	if flags.NArg() != 0 || *output == "" || !filepath.IsAbs(*output) {
		return commandError(errors.New("history generate requires an absolute new -output"))
	}
	inventory, err := generateHistory(*gitPath, *repository)
	if err != nil {
		return commandError(err)
	}
	data, err := encodeHistory(inventory)
	if err != nil {
		return commandError(err)
	}
	if err := writeAtomicNew(*output, data, 0o644); err != nil {
		return commandError(fmt.Errorf("write history inventory: %w", err))
	}
	return 0
}

func historyVerifyCommand(arguments []string) int {
	flags := flag.NewFlagSet("history verify", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	inventoryPath := flags.String("inventory", "", "history inventory")
	if err := flags.Parse(arguments); err != nil {
		return commandError(err)
	}
	if flags.NArg() != 0 || *inventoryPath == "" {
		return commandError(errors.New("history verify requires -inventory"))
	}
	inventory, err := loadHistory(*inventoryPath)
	if err == nil {
		_, err = validateHistoryFloors(inventory, *inventoryPath)
	}
	return commandError(err)
}

func historyAuditCommand(arguments []string) int {
	flags := flag.NewFlagSet("history audit-live", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	gitPath := flags.String("git", "", "absolute Git executable")
	repository := flags.String("repository", "", "absolute repository root")
	inventoryPath := flags.String("inventory", "", "history inventory")
	if err := flags.Parse(arguments); err != nil {
		return commandError(err)
	}
	if flags.NArg() != 0 || *inventoryPath == "" {
		return commandError(errors.New("history audit-live requires -inventory"))
	}
	stored, err := loadHistory(*inventoryPath)
	if err != nil {
		return commandError(err)
	}
	if _, err := validateHistoryFloors(stored, *inventoryPath); err != nil {
		return commandError(err)
	}
	live, err := generateHistory(*gitPath, *repository)
	if err != nil {
		return commandError(err)
	}
	storedData, err := encodeHistory(stored)
	if err != nil {
		return commandError(err)
	}
	liveData, err := encodeHistory(live)
	if err != nil {
		return commandError(err)
	}
	if !bytes.Equal(storedData, liveData) {
		return commandError(errors.New("history inventory differs from the fixed Git closure"))
	}
	return 0
}

func generateHistory(gitPath, repository string) (historyInventory, error) {
	source, cleanup, err := newHistoryGit(gitPath, repository)
	if err != nil {
		return historyInventory{}, err
	}
	defer cleanup()
	git, cleanupAuthority, err := newHistoryAuthority(source)
	if err != nil {
		return historyInventory{}, err
	}
	defer cleanupAuthority()
	objectFormat, err := git.output("rev-parse", "--show-object-format")
	if err != nil {
		return historyInventory{}, err
	}
	if objectFormat != "sha1" {
		return historyInventory{}, fmt.Errorf("git object format = %q, want sha1", objectFormat)
	}
	ancestryOutput, err := git.output("rev-list", "--ancestry-path", historyBoundary+".."+historyEnd)
	if err != nil {
		return historyInventory{}, err
	}
	ancestry := nonemptyLines(ancestryOutput)
	if len(ancestry) != 135 {
		return historyInventory{}, fmt.Errorf("anchored history commits = %d, want 135", len(ancestry))
	}
	anchored := make(map[string]struct{}, len(ancestry))
	commitIDs := make([]string, 0, len(ancestry)+len(historyArchivedCommits))
	for _, oid := range ancestry {
		if _, exists := anchored[oid]; exists {
			return historyInventory{}, fmt.Errorf("anchored history repeats commit %s", oid)
		}
		anchored[oid] = struct{}{}
		commitIDs = append(commitIDs, oid)
	}
	if _, ok := anchored[historyStart]; !ok {
		return historyInventory{}, fmt.Errorf("anchored history omits start %s", historyStart)
	}
	for _, oid := range historyArchivedCommits {
		if _, exists := anchored[oid]; exists {
			return historyInventory{}, fmt.Errorf("archived commit %s is also anchored", oid)
		}
		commitIDs = append(commitIDs, oid)
	}
	commits := make(map[string]*historyCommit, len(commitIDs))
	archiveRecords := make(map[string]historyArchiveRecord, len(historyArchiveRecords))
	for _, record := range historyArchiveRecords {
		archiveRecords[record.OID] = record
	}
	for _, oid := range commitIDs {
		payload, err := git.bytes("cat-file", "commit", oid)
		if err != nil {
			return historyInventory{}, err
		}
		commit, err := parseHistoryCommit(oid, payload)
		if err != nil {
			return historyInventory{}, err
		}
		_, commit.anchored = anchored[oid]
		if record, ok := archiveRecords[oid]; ok {
			commit.lateDiscovery = record.LateDiscovery
		}
		commit.eventloopTree, err = git.output("rev-parse", oid+":eventloop")
		if err != nil {
			return historyInventory{}, err
		}
		commit.gojaTree, err = git.output("rev-parse", oid+":goja-eventloop")
		if err != nil {
			return historyInventory{}, err
		}
		commits[oid] = commit
	}
	ordered, err := orderHistoryCommits(commits)
	if err != nil {
		return historyInventory{}, err
	}
	return buildHistoryInventory(ordered, source.repository)
}

func newHistoryGit(gitPath, repository string) (historyGit, func(), error) {
	if gitPath == "" || repository == "" || !filepath.IsAbs(gitPath) || !filepath.IsAbs(repository) {
		return historyGit{}, func() {}, errors.New("history requires absolute -git and -repository paths")
	}
	gitPath = normalizeExecutablePath(gitPath)
	resolvedGit, err := filepath.EvalSymlinks(gitPath)
	if err != nil {
		return historyGit{}, func() {}, fmt.Errorf("resolve Git executable: %w", err)
	}
	resolvedRepository, err := filepath.EvalSymlinks(repository)
	if err != nil {
		return historyGit{}, func() {}, fmt.Errorf("resolve repository: %w", err)
	}
	info, err := os.Stat(resolvedRepository)
	if err != nil || !info.IsDir() {
		return historyGit{}, func() {}, fmt.Errorf("repository is not a directory: %q", repository)
	}
	configuration, err := os.MkdirTemp("", "eventloop-history-git.")
	if err != nil {
		return historyGit{}, func() {}, fmt.Errorf("create Git configuration root: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(configuration) }
	globalConfig := filepath.Join(configuration, "global.gitconfig")
	if err := os.WriteFile(globalConfig, nil, 0o600); err != nil {
		cleanup()
		return historyGit{}, func() {}, fmt.Errorf("create empty Git configuration: %w", err)
	}
	environment := filteredGitEnvironment([]string{
		"GIT_CONFIG_GLOBAL=" + globalConfig,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_EXTERNAL_DIFF=",
		"GIT_LITERAL_PATHSPECS=1",
		"GIT_NO_REPLACE_OBJECTS=1",
		"GIT_OPTIONAL_LOCKS=0",
		"HOME=" + configuration,
		"XDG_CONFIG_HOME=" + configuration,
	})
	return historyGit{executable: resolvedGit, repository: resolvedRepository, environment: environment}, cleanup, nil
}

func filteredGitEnvironment(overrides []string) []string {
	result := make([]string, 0, len(os.Environ())+len(overrides))
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		if strings.HasPrefix(key, "GIT_") || key == "HOME" || key == "XDG_CONFIG_HOME" {
			continue
		}
		result = append(result, value)
	}
	return append(result, overrides...)
}

func (value historyGit) bytes(arguments ...string) ([]byte, error) {
	command := exec.Command(value.executable, append([]string{"-C", value.repository}, arguments...)...)
	command.Env = value.environment
	output, err := command.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return nil, fmt.Errorf("git %v: %w: %s", arguments, err, exit.Stderr)
		}
		return nil, fmt.Errorf("git %v: %w", arguments, err)
	}
	return output, nil
}

func (value historyGit) output(arguments ...string) (string, error) {
	data, err := value.bytes(arguments...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func (value historyGit) run(arguments ...string) error {
	_, err := value.bytes(arguments...)
	return err
}

func (value historyGit) input(data []byte, arguments ...string) (string, error) {
	return historyCommandOutput(value.executable, value.repository, value.environment, data, arguments...)
}

func (value historyGit) objectExists(object string) bool {
	command := exec.Command(value.executable, "-C", value.repository, "cat-file", "-e", object)
	command.Env = value.environment
	return command.Run() == nil
}

func parseHistoryCommit(oid string, payload []byte) (*historyCommit, error) {
	if !validHistoryOID(oid) {
		return nil, fmt.Errorf("invalid commit ID %q", oid)
	}
	before, after, ok := bytes.Cut(payload, []byte("\n\n"))
	if !ok {
		return nil, fmt.Errorf("commit %s has no message separator", oid)
	}
	commit := &historyCommit{oid: oid}
	for line := range strings.SplitSeq(string(before), "\n") {
		key, value, ok := strings.Cut(line, " ")
		if !ok {
			return nil, fmt.Errorf("commit %s has malformed header %q", oid, line)
		}
		switch key {
		case "tree":
			if commit.rootTree != "" {
				return nil, fmt.Errorf("commit %s repeats tree header", oid)
			}
			commit.rootTree = value
		case "parent":
			commit.parents = append(commit.parents, value)
		case "author":
			epoch, err := historySignatureEpoch(value)
			if err != nil {
				return nil, fmt.Errorf("commit %s author: %w", oid, err)
			}
			commit.authorEpoch = epoch
		case "committer":
			epoch, err := historySignatureEpoch(value)
			if err != nil {
				return nil, fmt.Errorf("commit %s committer: %w", oid, err)
			}
			commit.commitEpoch = epoch
		}
	}
	if !validHistoryOID(commit.rootTree) || commit.authorEpoch == 0 || commit.commitEpoch == 0 {
		return nil, fmt.Errorf("commit %s has incomplete required headers", oid)
	}
	for _, parent := range commit.parents {
		if !validHistoryOID(parent) {
			return nil, fmt.Errorf("commit %s has invalid parent %q", oid, parent)
		}
	}
	message := after
	if newline := bytes.IndexByte(message, '\n'); newline >= 0 {
		message = message[:newline]
	}
	commit.subject = slices.Clone(message)
	return commit, nil
}

func historySignatureEpoch(value string) (int64, error) {
	fields := strings.Fields(value)
	if len(fields) < 3 {
		return 0, errors.New("malformed signature")
	}
	epoch, err := strconv.ParseInt(fields[len(fields)-2], 10, 64)
	if err != nil || epoch <= 0 {
		return 0, fmt.Errorf("invalid epoch %q", fields[len(fields)-2])
	}
	return epoch, nil
}

func orderHistoryCommits(commits map[string]*historyCommit) ([]*historyCommit, error) {
	lateDiscoveries := make(map[int]string)
	maxLateDiscovery := 0
	for oid, commit := range commits {
		if commit.lateDiscovery < 0 {
			return nil, fmt.Errorf("commit %s has negative late-discovery order", oid)
		}
		if commit.lateDiscovery == 0 {
			continue
		}
		if other, duplicate := lateDiscoveries[commit.lateDiscovery]; duplicate {
			return nil, fmt.Errorf("commits %s and %s share late-discovery order %d", other, oid, commit.lateDiscovery)
		}
		lateDiscoveries[commit.lateDiscovery] = oid
		maxLateDiscovery = max(maxLateDiscovery, commit.lateDiscovery)
	}
	if maxLateDiscovery != len(lateDiscoveries) {
		return nil, errors.New("late-discovery order is not contiguous")
	}
	indegree := make(map[string]int, len(commits))
	children := make(map[string][]string, len(commits))
	for oid, commit := range commits {
		for _, parent := range commit.parents {
			if _, ok := commits[parent]; !ok {
				continue
			}
			indegree[oid]++
			children[parent] = append(children[parent], oid)
		}
	}
	ready := make([]*historyCommit, 0, len(commits))
	for oid, commit := range commits {
		if indegree[oid] == 0 {
			ready = append(ready, commit)
		}
	}
	ordered := make([]*historyCommit, 0, len(commits))
	for len(ready) != 0 {
		slices.SortFunc(ready, compareHistoryCommits)
		commit := ready[0]
		ready = ready[1:]
		ordered = append(ordered, commit)
		for _, child := range children[commit.oid] {
			indegree[child]--
			if indegree[child] == 0 {
				ready = append(ready, commits[child])
			}
		}
	}
	if len(ordered) != len(commits) {
		return nil, errors.New("fixed commit closure is cyclic")
	}
	seenLate := false
	wantLate := 1
	for _, commit := range ordered {
		if commit.lateDiscovery == 0 {
			if seenLate {
				return nil, errors.New("late-discovered commit is an ancestor of the frozen discovery order")
			}
			continue
		}
		seenLate = true
		if commit.lateDiscovery != wantLate {
			return nil, fmt.Errorf("late-discovered commit %s has order %d after %d", commit.oid, commit.lateDiscovery, wantLate-1)
		}
		wantLate++
	}
	return ordered, nil
}

func compareHistoryCommits(left, right *historyCommit) int {
	if left.lateDiscovery != 0 || right.lateDiscovery != 0 {
		switch {
		case left.lateDiscovery == 0:
			return -1
		case right.lateDiscovery == 0:
			return 1
		default:
			return intCompare(int64(left.lateDiscovery), int64(right.lateDiscovery))
		}
	}
	if left.commitEpoch != right.commitEpoch {
		return intCompare(left.commitEpoch, right.commitEpoch)
	}
	if left.authorEpoch != right.authorEpoch {
		return intCompare(left.authorEpoch, right.authorEpoch)
	}
	return strings.Compare(left.oid, right.oid)
}

func intCompare(left, right int64) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func buildHistoryInventory(ordered []*historyCommit, repository string) (historyInventory, error) {
	occurrences := make([]historyOccurrence, 0, len(ordered))
	archiveByOID := make(map[string]historyArchiveRecord, len(historyArchiveRecords))
	for _, record := range historyArchiveRecords {
		archiveByOID[record.OID] = record
	}
	ancestryID := historyAncestryID(historyBoundary, historyEnd)
	for index, commit := range ordered {
		snapshotID := historySnapshotID(commit.eventloopTree)
		occurrenceID := historyOccurrenceID(commit.oid)
		authorityKind := "ancestry-segment"
		authorityID := ancestryID
		materializationKind := "anchored-object"
		materializationID := occurrenceID
		if commit.anchored {
			// The anchored object is materialized by the immutable ancestry segment.
		} else {
			record, ok := archiveByOID[commit.oid]
			if !ok {
				return historyInventory{}, fmt.Errorf("archived commit %s has no archive authority", commit.oid)
			}
			authorityKind = "commit-archive"
			authorityID = historyCommitArchiveID(commit.oid)
			if record.Patch != "" {
				materializationKind = "tree-patch"
				materializationID = historyPatchArchiveID(record.PatchSHA256)
			} else {
				materializationKind = "root-alias"
				materializationID = historyOccurrenceID(record.AliasTargetCommit)
			}
		}
		occurrences = append(occurrences, historyOccurrence{
			ID:                      occurrenceID,
			DiscoveryOrdinal:        index + 1,
			SnapshotID:              snapshotID,
			Commit:                  commit.oid,
			RootTree:                commit.rootTree,
			EventloopTree:           commit.eventloopTree,
			GojaEventloopTree:       commit.gojaTree,
			ParentCommits:           slices.Clone(commit.parents),
			AuthorEpoch:             commit.authorEpoch,
			CommitterEpoch:          commit.commitEpoch,
			SubjectBase64:           base64.StdEncoding.EncodeToString(commit.subject),
			AuthorityKind:           authorityKind,
			AuthorityID:             authorityID,
			TreeMaterializationKind: materializationKind,
			TreeMaterializationID:   materializationID,
		})
	}
	snapshots := make([]historySnapshot, 0)
	seenTrees := make(map[string]struct{})
	for _, commit := range ordered {
		if _, ok := seenTrees[commit.eventloopTree]; ok {
			continue
		}
		seenTrees[commit.eventloopTree] = struct{}{}
		snapshots = append(snapshots, historySnapshot{
			ID:               historySnapshotID(commit.eventloopTree),
			DiscoveryOrdinal: len(snapshots) + 1,
			EventloopTree:    commit.eventloopTree,
		})
	}
	transitions := buildHistoryTransitions(occurrences)
	commitArchives, patchArchives, err := buildHistoryArchiveRecords(repository)
	if err != nil {
		return historyInventory{}, err
	}
	inventory := historyInventory{
		SchemaVersion: historySchemaVersion,
		ObjectFormat:  "sha1",
		AncestrySegments: []historyAncestry{{
			ID:               ancestryID,
			DiscoveryOrdinal: 1,
			BoundaryCommit:   historyBoundary,
			AncestryStart:    historyStart,
			AncestryEnd:      historyEnd,
			OccurrenceCount:  135,
		}},
		SourceSnapshots:   snapshots,
		SourceOccurrences: occurrences,
		SourceTransitions: transitions,
		CommitArchives:    commitArchives,
		TreePatchArchives: patchArchives,
		DynamicCurrent: historyCurrent{
			ID:             "source.eventloop.current",
			Kind:           "dynamic-frozen-filesystem",
			BaseCommit:     historyEnd,
			IdentityPolicy: "tournamentmeta-source-fingerprint-v4",
		},
	}
	if err := validateHistory(inventory); err != nil {
		return historyInventory{}, err
	}
	return inventory, nil
}

func buildHistoryTransitions(occurrences []historyOccurrence) []historyTransition {
	commits := make(map[string]historyOccurrence, len(occurrences))
	for _, occurrence := range occurrences {
		commits[occurrence.Commit] = occurrence
	}
	transitions := make([]historyTransition, 0)
	seen := make(map[string]struct{})
	for _, child := range occurrences {
		for parentIndex, parentCommit := range child.ParentCommits {
			parent, ok := commits[parentCommit]
			if !ok || parent.SnapshotID == child.SnapshotID {
				continue
			}
			id := historyTransitionID(parent.EventloopTree, child.EventloopTree)
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			transitions = append(transitions, historyTransition{
				ID:                        id,
				DiscoveryOrdinal:          len(transitions) + 1,
				PredecessorSnapshotID:     parent.SnapshotID,
				SuccessorSnapshotID:       child.SnapshotID,
				WitnessParentOccurrenceID: parent.ID,
				WitnessChildOccurrenceID:  child.ID,
				WitnessParentIndex:        parentIndex,
			})
		}
	}
	return transitions
}

func loadHistory(path string) (historyInventory, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return historyInventory{}, fmt.Errorf("read history inventory: %w", err)
	}
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return historyInventory{}, fmt.Errorf("validate history JSON: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var inventory historyInventory
	if err := decoder.Decode(&inventory); err != nil {
		return historyInventory{}, fmt.Errorf("decode history inventory: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return historyInventory{}, errors.New("history inventory has trailing JSON")
	}
	if err := validateHistory(inventory); err != nil {
		return historyInventory{}, err
	}
	canonical, err := encodeHistory(inventory)
	if err != nil {
		return historyInventory{}, err
	}
	if !bytes.Equal(data, canonical) {
		return historyInventory{}, errors.New("history inventory is not canonical JSON")
	}
	return inventory, nil
}

func encodeHistory(inventory historyInventory) ([]byte, error) {
	data, err := json.MarshalIndent(inventory, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode history inventory: %w", err)
	}
	return append(data, '\n'), nil
}

func validateHistory(inventory historyInventory) error {
	if inventory.SchemaVersion != historySchemaVersion || inventory.ObjectFormat != "sha1" {
		return fmt.Errorf("history schema/object format = %d/%q, want %d/sha1", inventory.SchemaVersion, inventory.ObjectFormat, historySchemaVersion)
	}
	wantAncestry := []historyAncestry{{
		ID:               historyAncestryID(historyBoundary, historyEnd),
		DiscoveryOrdinal: 1,
		BoundaryCommit:   historyBoundary,
		AncestryStart:    historyStart,
		AncestryEnd:      historyEnd,
		OccurrenceCount:  135,
	}}
	if !slices.Equal(inventory.AncestrySegments, wantAncestry) {
		return errors.New("history ancestry authority changed")
	}
	if len(inventory.SourceSnapshots) != 81 || len(inventory.SourceOccurrences) != 148 ||
		len(inventory.SourceTransitions) != 82 || len(inventory.CommitArchives) != 13 ||
		len(inventory.TreePatchArchives) != 5 {
		return fmt.Errorf(
			"history inventory = %d snapshots/%d occurrences/%d transitions/%d commits/%d patches, want 81/148/82/13/5",
			len(inventory.SourceSnapshots), len(inventory.SourceOccurrences), len(inventory.SourceTransitions),
			len(inventory.CommitArchives), len(inventory.TreePatchArchives),
		)
	}
	if inventory.DynamicCurrent != (historyCurrent{
		ID:             "source.eventloop.current",
		Kind:           "dynamic-frozen-filesystem",
		BaseCommit:     historyEnd,
		IdentityPolicy: "tournamentmeta-source-fingerprint-v4",
	}) {
		return errors.New("dynamic current source policy changed")
	}
	archiveRecords := make(map[string]historyArchiveRecord, len(historyArchiveRecords))
	for _, record := range historyArchiveRecords {
		archiveRecords[record.OID] = record
	}
	archivedCommits := make(map[string]historyCommitArchive, len(inventory.CommitArchives))
	for index, archive := range inventory.CommitArchives {
		record, ok := archiveRecords[archive.GitObjectSHA1]
		if !ok || archive.DiscoveryOrdinal != index+1 || archive.ID != historyCommitArchiveID(record.OID) ||
			archive.OccurrenceID != historyOccurrenceID(record.OID) ||
			archive.Path != "revisions/commits/"+record.OID+".commit.b64" ||
			archive.Encoding != "rfc4648-base64-standard-single-line-lf" || archive.EncodedBytes <= 0 ||
			!historySHA256Pattern.MatchString(archive.EncodedSHA256) ||
			archive.PayloadBytes != record.PayloadBytes || archive.PayloadSHA256 != record.PayloadSHA256 {
			return fmt.Errorf("invalid commit archive at discovery ordinal %d", index+1)
		}
		if _, duplicate := archivedCommits[record.OID]; duplicate {
			return fmt.Errorf("duplicate commit archive %q", archive.ID)
		}
		archivedCommits[record.OID] = archive
	}
	patchesByOccurrence := make(map[string]historyPatchArchive, len(inventory.TreePatchArchives))
	for index, archive := range inventory.TreePatchArchives {
		var record historyArchiveRecord
		found := false
		for _, candidate := range historyArchiveRecords {
			if candidate.PatchSHA256 == archive.PatchSHA256 && candidate.Patch != "" {
				record = candidate
				found = true
				break
			}
		}
		if !found || archive.DiscoveryOrdinal != index+1 || archive.ID != historyPatchArchiveID(record.PatchSHA256) ||
			archive.OccurrenceID != historyOccurrenceID(record.OID) || archive.Path != "revisions/"+record.Patch ||
			archive.Format != "git-format-patch-full-index-binary-v1" || archive.PatchBytes <= 0 ||
			archive.BaseCommit != record.PatchParent || archive.RootTree != record.RootTree ||
			archive.EventloopTree != record.EventloopTree || archive.GojaEventloopTree != record.GojaEventloopTree {
			return fmt.Errorf("invalid patch archive at discovery ordinal %d", index+1)
		}
		if _, duplicate := patchesByOccurrence[archive.OccurrenceID]; duplicate {
			return fmt.Errorf("duplicate patch archive occurrence %q", archive.OccurrenceID)
		}
		patchesByOccurrence[archive.OccurrenceID] = archive
	}
	occurrences := make(map[string]historyOccurrence, len(inventory.SourceOccurrences))
	commits := make(map[string]historyOccurrence, len(inventory.SourceOccurrences))
	groups := make(map[string][]historyOccurrence, len(inventory.SourceSnapshots))
	anchoredCount := 0
	gojaTrees := make(map[string]struct{})
	for index, occurrence := range inventory.SourceOccurrences {
		if occurrence.DiscoveryOrdinal != index+1 || occurrence.ID != historyOccurrenceID(occurrence.Commit) ||
			occurrence.SnapshotID != historySnapshotID(occurrence.EventloopTree) {
			return fmt.Errorf("invalid occurrence identity at ordinal %d", index+1)
		}
		if !validHistoryOID(occurrence.Commit) || !validHistoryOID(occurrence.RootTree) ||
			!validHistoryOID(occurrence.EventloopTree) || !validHistoryOID(occurrence.GojaEventloopTree) {
			return fmt.Errorf("occurrence %q has invalid object ID", occurrence.ID)
		}
		if occurrence.AuthorEpoch <= 0 || occurrence.CommitterEpoch <= 0 {
			return fmt.Errorf("occurrence %q has invalid timestamps", occurrence.ID)
		}
		subject, err := base64.StdEncoding.Strict().DecodeString(occurrence.SubjectBase64)
		if err != nil {
			return fmt.Errorf("occurrence %q has invalid subject: %w", occurrence.ID, err)
		}
		if base64.StdEncoding.EncodeToString(subject) != occurrence.SubjectBase64 {
			return fmt.Errorf("occurrence %q subject is not canonical base64", occurrence.ID)
		}
		archive, archived := archivedCommits[occurrence.Commit]
		if !archived {
			if occurrence.AuthorityKind != "ancestry-segment" || occurrence.AuthorityID != wantAncestry[0].ID ||
				occurrence.TreeMaterializationKind != "anchored-object" || occurrence.TreeMaterializationID != occurrence.ID {
				return fmt.Errorf("anchored occurrence %q has invalid authority", occurrence.ID)
			}
			anchoredCount++
		} else {
			if occurrence.AuthorityKind != "commit-archive" || occurrence.AuthorityID != archive.ID {
				return fmt.Errorf("archived occurrence %q has invalid authority", occurrence.ID)
			}
			record := archiveRecords[occurrence.Commit]
			if record.Patch != "" {
				patch := patchesByOccurrence[occurrence.ID]
				if occurrence.TreeMaterializationKind != "tree-patch" || occurrence.TreeMaterializationID != patch.ID ||
					occurrence.RootTree != patch.RootTree || occurrence.EventloopTree != patch.EventloopTree ||
					occurrence.GojaEventloopTree != patch.GojaEventloopTree {
					return fmt.Errorf("archived patch occurrence %q has invalid materialization", occurrence.ID)
				}
			} else if occurrence.TreeMaterializationKind != "root-alias" ||
				occurrence.TreeMaterializationID != historyOccurrenceID(record.AliasTargetCommit) {
				return fmt.Errorf("archived alias occurrence %q has invalid materialization", occurrence.ID)
			}
		}
		for _, parent := range occurrence.ParentCommits {
			if !validHistoryOID(parent) {
				return fmt.Errorf("occurrence %q has invalid parent %q", occurrence.ID, parent)
			}
		}
		if _, ok := occurrences[occurrence.ID]; ok {
			return fmt.Errorf("duplicate occurrence %q", occurrence.ID)
		}
		if _, ok := commits[occurrence.Commit]; ok {
			return fmt.Errorf("duplicate occurrence commit %q", occurrence.Commit)
		}
		occurrences[occurrence.ID] = occurrence
		commits[occurrence.Commit] = occurrence
		groups[occurrence.SnapshotID] = append(groups[occurrence.SnapshotID], occurrence)
		gojaTrees[occurrence.GojaEventloopTree] = struct{}{}
	}
	if anchoredCount != 135 || len(gojaTrees) != 24 {
		return fmt.Errorf("history has %d anchored commits/%d Goja trees, want 135/24", anchoredCount, len(gojaTrees))
	}
	if len(archivedCommits) != len(historyArchiveRecords) {
		return errors.New("history commit archive set differs from archived occurrences")
	}
	for _, record := range historyArchiveRecords {
		occurrence := commits[record.OID]
		if occurrence.ID == "" {
			return fmt.Errorf("history omits archived occurrence %s", record.OID)
		}
		if record.AliasTargetCommit != "" {
			target := commits[record.AliasTargetCommit]
			if target.ID == "" || target.RootTree != occurrence.RootTree {
				return fmt.Errorf("archived occurrence %s has invalid root alias %s", record.OID, record.AliasTargetCommit)
			}
		}
	}
	if err := validateHistoryCommitGraph(inventory.SourceOccurrences, commits); err != nil {
		return err
	}
	snapshotIDs := make(map[string]historySnapshot, len(inventory.SourceSnapshots))
	multipleGroups := 0
	aliasCount := 0
	for index, snapshot := range inventory.SourceSnapshots {
		if snapshot.DiscoveryOrdinal != index+1 || snapshot.ID != historySnapshotID(snapshot.EventloopTree) {
			return fmt.Errorf("invalid snapshot identity at ordinal %d", index+1)
		}
		if _, ok := snapshotIDs[snapshot.ID]; ok {
			return fmt.Errorf("duplicate snapshot %q", snapshot.ID)
		}
		group := groups[snapshot.ID]
		if len(group) == 0 {
			return fmt.Errorf("snapshot %q has no occurrences", snapshot.ID)
		}
		if len(group) > 1 {
			multipleGroups++
		}
		aliasCount += len(group) - 1
		snapshotIDs[snapshot.ID] = snapshot
	}
	if multipleGroups != 25 || aliasCount != 67 {
		return fmt.Errorf("history groups/aliases = %d/%d, want 25/67", multipleGroups, aliasCount)
	}
	for _, occurrence := range inventory.SourceOccurrences {
		if _, ok := snapshotIDs[occurrence.SnapshotID]; !ok {
			return fmt.Errorf("occurrence %q has unknown snapshot %q", occurrence.ID, occurrence.SnapshotID)
		}
	}
	wantTransitions := buildHistoryTransitions(inventory.SourceOccurrences)
	if !slices.Equal(inventory.SourceTransitions, wantTransitions) {
		return errors.New("history transitions differ from all direct cross-snapshot parent relations")
	}
	return nil
}

func validateHistoryCommitGraph(occurrences []historyOccurrence, commits map[string]historyOccurrence) error {
	indegree := make(map[string]int, len(occurrences))
	children := make(map[string][]string, len(occurrences))
	for _, occurrence := range occurrences {
		for _, parent := range occurrence.ParentCommits {
			if _, ok := commits[parent]; !ok {
				continue
			}
			indegree[occurrence.Commit]++
			children[parent] = append(children[parent], occurrence.Commit)
		}
	}
	ready := make([]string, 0)
	for _, occurrence := range occurrences {
		if indegree[occurrence.Commit] == 0 {
			ready = append(ready, occurrence.Commit)
		}
	}
	visited := 0
	for len(ready) != 0 {
		commit := ready[len(ready)-1]
		ready = ready[:len(ready)-1]
		visited++
		for _, child := range children[commit] {
			indegree[child]--
			if indegree[child] == 0 {
				ready = append(ready, child)
			}
		}
	}
	if visited != len(occurrences) {
		return errors.New("history commit relation is cyclic")
	}
	return nil
}

func historySnapshotID(tree string) string {
	return "source.eventloop.sha1." + tree
}

func historyAncestryID(boundary, end string) string {
	return "source.ancestry.sha1." + boundary + "." + end
}

func historyOccurrenceID(commit string) string {
	return "source.commit.sha1." + commit
}

func historyTransitionID(predecessorTree, successorTree string) string {
	return "source.transition.eventloop.sha1." + predecessorTree + "." + successorTree
}

func historyCommitArchiveID(commit string) string {
	return "source.commit-archive.sha1." + commit
}

func historyPatchArchiveID(sha256 string) string {
	return "source.tree-patch.sha256." + sha256
}

func validHistoryOID(value string) bool {
	return historyOIDPattern.MatchString(value)
}

func nonemptyLines(value string) []string {
	if value == "" {
		return nil
	}
	lines := strings.Split(value, "\n")
	result := lines[:0]
	for _, line := range lines {
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}
