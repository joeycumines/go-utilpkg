package main

type historyInventory struct { // betteralign:ignore canonical JSON field order
	SchemaVersion     int                    `json:"schema_version"`
	ObjectFormat      string                 `json:"object_format"`
	AncestrySegments  []historyAncestry      `json:"ancestry_segments"`
	SourceSnapshots   []historySnapshot      `json:"source_snapshots"`
	SourceOccurrences []historyOccurrence    `json:"source_occurrences"`
	SourceTransitions []historyTransition    `json:"source_transitions"`
	CommitArchives    []historyCommitArchive `json:"commit_archives"`
	TreePatchArchives []historyPatchArchive  `json:"tree_patch_archives"`
	DynamicCurrent    historyCurrent         `json:"dynamic_current"`
}

type historyAncestry struct { // betteralign:ignore canonical JSON field order
	ID               string `json:"id"`
	DiscoveryOrdinal int    `json:"discovery_ordinal"`
	BoundaryCommit   string `json:"boundary_commit"`
	AncestryStart    string `json:"ancestry_start"`
	AncestryEnd      string `json:"ancestry_end"`
	OccurrenceCount  int    `json:"occurrence_count"`
}

type historySnapshot struct { // betteralign:ignore canonical JSON field order
	ID               string `json:"id"`
	DiscoveryOrdinal int    `json:"discovery_ordinal"`
	EventloopTree    string `json:"eventloop_tree"`
}

type historyOccurrence struct { // betteralign:ignore canonical JSON field order
	ID                      string   `json:"id"`
	DiscoveryOrdinal        int      `json:"discovery_ordinal"`
	SnapshotID              string   `json:"snapshot_id"`
	Commit                  string   `json:"commit"`
	RootTree                string   `json:"root_tree"`
	EventloopTree           string   `json:"eventloop_tree"`
	GojaEventloopTree       string   `json:"goja_eventloop_tree"`
	ParentCommits           []string `json:"parent_commits"`
	AuthorEpoch             int64    `json:"author_epoch"`
	CommitterEpoch          int64    `json:"committer_epoch"`
	SubjectBase64           string   `json:"subject_base64"`
	AuthorityKind           string   `json:"authority_kind"`
	AuthorityID             string   `json:"authority_id"`
	TreeMaterializationKind string   `json:"tree_materialization_kind"`
	TreeMaterializationID   string   `json:"tree_materialization_id"`
}

type historyTransition struct { // betteralign:ignore canonical JSON field order
	ID                        string `json:"id"`
	DiscoveryOrdinal          int    `json:"discovery_ordinal"`
	PredecessorSnapshotID     string `json:"predecessor_snapshot_id"`
	SuccessorSnapshotID       string `json:"successor_snapshot_id"`
	WitnessParentOccurrenceID string `json:"witness_parent_occurrence_id"`
	WitnessChildOccurrenceID  string `json:"witness_child_occurrence_id"`
	WitnessParentIndex        int    `json:"witness_parent_index"`
}

type historyCommitArchive struct { // betteralign:ignore canonical JSON field order
	ID               string `json:"id"`
	DiscoveryOrdinal int    `json:"discovery_ordinal"`
	OccurrenceID     string `json:"occurrence_id"`
	Path             string `json:"path"`
	Encoding         string `json:"encoding"`
	EncodedBytes     int    `json:"encoded_bytes"`
	EncodedSHA256    string `json:"encoded_sha256"`
	PayloadBytes     int    `json:"payload_bytes"`
	PayloadSHA256    string `json:"payload_sha256"`
	GitObjectSHA1    string `json:"git_object_sha1"`
}

type historyPatchArchive struct { // betteralign:ignore canonical JSON field order
	ID                string `json:"id"`
	DiscoveryOrdinal  int    `json:"discovery_ordinal"`
	OccurrenceID      string `json:"occurrence_id"`
	Path              string `json:"path"`
	Format            string `json:"format"`
	PatchBytes        int    `json:"patch_bytes"`
	PatchSHA256       string `json:"patch_sha256"`
	BaseCommit        string `json:"base_commit"`
	RootTree          string `json:"root_tree"`
	EventloopTree     string `json:"eventloop_tree"`
	GojaEventloopTree string `json:"goja_eventloop_tree"`
}

type historyCurrent struct { // betteralign:ignore canonical JSON field order
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	BaseCommit     string `json:"base_commit"`
	IdentityPolicy string `json:"identity_policy"`
}

type historyCommit struct {
	oid           string
	rootTree      string
	eventloopTree string
	gojaTree      string
	parents       []string
	subject       []byte
	authorEpoch   int64
	commitEpoch   int64
	lateDiscovery int
	anchored      bool
}

type historyGit struct {
	executable  string
	repository  string
	environment []string
}
