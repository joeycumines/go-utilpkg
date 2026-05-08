// Package oracle implements the bounded Node and Web differential oracle for
// goja-eventloop. It is internal because the protocol is release evidence, not
// part of the adapter's public API.
package oracle

import "encoding/json"

const (
	ManifestSchema = "goja-eventloop.oracle/v3"
	ProtocolSchema = "goja-eventloop.oracle.result/v6"

	NodeVersion      = "26.5.0"
	NodeTag          = "v26.5.0"
	NodeSourceCommit = "bebd1b8d92bf4cc917844d6335ed1ecf9c2a75fb"
	NodeReleaseURL   = "https://nodejs.org/en/download/archive/v26.5.0"

	ExitPass       = 0
	ExitMismatch   = 1
	ExitInvalidRun = 2
)

type ProfileClass string

const (
	ClassNode      ProfileClass = "node"
	ClassWeb       ProfileClass = "web"
	ClassExtension ProfileClass = "extension"
	ClassBoundary  ProfileClass = "boundary"
)

type Comparison string

const (
	CompareNodeExact Comparison = "node-exact"
	CompareExpected  Comparison = "expected"
)

// ComparisonContract partitions an emitted Goja observation by the authority
// used to qualify each subtree. Entries are RFC 6901 JSON pointers; the empty
// pointer identifies the complete observation. Both slices are always emitted
// so machine consumers cannot infer whole-record equality from a fixture's
// manifest policy.
type ComparisonContract struct {
	NodeExact        []string `json:"nodeExact"`
	ExpectedContract []string `json:"expectedContract"`
}

type FixtureRole string

const (
	RoleSurface  FixtureRole = "surface"
	RoleSemantic FixtureRole = "semantic"
)

type Manifest struct {
	Schema      string      `json:"schema"`
	Profile     string      `json:"profile"`
	Node        NodePin     `json:"node"`
	Harness     Asset       `json:"harness"`
	Authorities []Authority `json:"authorities"`
	Fixtures    []Fixture   `json:"fixtures"`
	Surfaces    []Surface   `json:"surfaces"`
}

type NodePin struct {
	Version      string            `json:"version"`
	Tag          string            `json:"tag"`
	SourceCommit string            `json:"sourceCommit"`
	ReleaseURL   string            `json:"releaseURL"`
	Artifacts    []NodeArtifactPin `json:"artifacts"`
}

type NodeArtifactPin struct {
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
	File   string `json:"file"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Entry  string `json:"entry"`
}

type Asset struct {
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
}

type Authority struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Revision string `json:"revision"`
	Locator  string `json:"locator"`
}

type Fixture struct {
	ID            string          `json:"id"`
	File          string          `json:"file"`
	SHA256        string          `json:"sha256"`
	Class         ProfileClass    `json:"class"`
	Comparison    Comparison      `json:"comparison"`
	Role          FixtureRole     `json:"role"`
	Setup         Setup           `json:"setup"`
	Authorities   []string        `json:"authorities"`
	Expected      json.RawMessage `json:"expected,omitempty"`
	TimeoutMillis int             `json:"timeoutMillis"`
}

type Setup struct {
	Globals    []string         `json:"globals,omitempty"`
	Members    []SetupMember    `json:"members,omitempty"`
	BrandPairs []SetupBrandPair `json:"brandPairs,omitempty"`
}

type SetupMember struct {
	Object   string `json:"object"`
	Property string `json:"property"`
	Value    string `json:"value"`
}

// SetupBrandPair installs coherent foreign constructor/prototype/singleton
// state. It exercises Bind's preservation contract without pretending that a
// plain object is a valid implementation of a standard-branded singleton.
type SetupBrandPair struct {
	Constructor string   `json:"constructor"`
	Singleton   string   `json:"singleton"`
	Sentinel    string   `json:"sentinel"`
	Methods     []string `json:"methods,omitempty"`
	Accessors   []string `json:"accessors,omitempty"`
}

type Surface struct {
	ID          string          `json:"id"`
	Path        string          `json:"path"`
	Root        string          `json:"root"`
	Class       ProfileClass    `json:"class"`
	Mode        string          `json:"mode"`
	ValueMode   string          `json:"valueMode,omitempty"`
	Segments    []string        `json:"segments"`
	Authorities []string        `json:"authorities"`
	Fixtures    []string        `json:"fixtures"`
	Expected    SurfaceExpected `json:"expected"`
}

type SurfaceExpected struct {
	Descriptor *DescriptorExpectation `json:"descriptor,omitempty"`
	Function   *FunctionExpectation   `json:"function,omitempty"`
	Kind       string                 `json:"kind,omitempty"`
	Prototype  string                 `json:"prototype,omitempty"`
	Sentinel   string                 `json:"sentinel,omitempty"`
	Value      json.RawMessage        `json:"value,omitempty"`
	Exists     bool                   `json:"exists"`
}

type DescriptorExpectation struct {
	Writable     *bool                `json:"writable,omitempty"`
	Getter       *FunctionExpectation `json:"getter,omitempty"`
	Setter       *FunctionExpectation `json:"setter,omitempty"`
	Depth        int                  `json:"depth"`
	Configurable bool                 `json:"configurable"`
	Enumerable   bool                 `json:"enumerable"`
}

type FunctionExpectation struct {
	OwnPrototype     *FunctionPrototypeExpectation `json:"ownPrototype"`
	Constructable    *bool                         `json:"constructable"`
	Name             string                        `json:"name"`
	OwnPropertyNames []string                      `json:"ownPropertyNames"`
	Length           int                           `json:"length"`
}

type FunctionPrototypeExpectation struct {
	Configurable *bool `json:"configurable,omitempty"`
	Enumerable   *bool `json:"enumerable,omitempty"`
	Writable     *bool `json:"writable,omitempty"`
	Exists       bool  `json:"exists"`
}

type LoadedManifest struct {
	Fixtures map[string][]byte
	Path     string
	Root     string
	SHA256   string
	Manifest Manifest
	Harness  []byte
}

type NodeIdentity struct {
	Version          string            `json:"version"`
	V8               string            `json:"v8"`
	Platform         string            `json:"platform"`
	Arch             string            `json:"arch"`
	Executable       string            `json:"executable"`
	ExecutableSHA256 string            `json:"executableSHA256"`
	Release          map[string]string `json:"release"`
	Artifact         NodeArtifact      `json:"artifact"`
}

type NodeArtifact struct {
	GOOS                  string `json:"goos"`
	GOARCH                string `json:"goarch"`
	URL                   string `json:"url"`
	File                  string `json:"file"`
	Entry                 string `json:"entry"`
	ExpectedArchiveSHA256 string `json:"expectedArchiveSHA256"`
	ArchiveSHA256         string `json:"archiveSHA256"`
	ExecutableSHA256      string `json:"executableSHA256"`
	LaunchMode            string `json:"launchMode"`
}

type RuntimeIdentity struct {
	GoVersion                 string `json:"goVersion"`
	GOOS                      string `json:"goos"`
	GOARCH                    string `json:"goarch"`
	IdentityMode              string `json:"identityMode"`
	Package                   string `json:"package"`
	ExecutableSHA256          string `json:"executableSHA256"`
	VCS                       string `json:"vcs"`
	VCSRevision               string `json:"vcsRevision"`
	Module                    string `json:"module"`
	ModuleSum                 string `json:"moduleSum,omitempty"`
	GojaVersion               string `json:"gojaVersion"`
	GojaSum                   string `json:"gojaSum,omitempty"`
	EventloopVersion          string `json:"eventloopVersion"`
	EventloopSum              string `json:"eventloopSum,omitempty"`
	EventloopReplacement      string `json:"eventloopReplacement,omitempty"`
	EventloopCandidateFormat  string `json:"eventloopCandidateFormat,omitempty"`
	EventloopCandidateSHA256  string `json:"eventloopCandidateSHA256,omitempty"`
	EventloopCandidateRecords int    `json:"eventloopCandidateRecords,omitempty"`
	VCSDirty                  bool   `json:"vcsDirty"`
}

type AttemptRecord struct {
	Type           string          `json:"type"`
	Schema         string          `json:"schema"`
	ManifestSchema string          `json:"manifestSchema"`
	ManifestSHA256 string          `json:"manifestSHA256"`
	NodePin        NodePin         `json:"nodePin"`
	Runtime        RuntimeIdentity `json:"runtime"`
	Cases          int             `json:"cases"`
	Surfaces       int             `json:"surfaces"`
}

type HeaderRecord struct {
	Type           string          `json:"type"`
	Schema         string          `json:"schema"`
	ManifestSchema string          `json:"manifestSchema"`
	ManifestSHA256 string          `json:"manifestSHA256"`
	NodePin        NodePin         `json:"nodePin"`
	Node           NodeIdentity    `json:"node"`
	Runtime        RuntimeIdentity `json:"runtime"`
	Cases          int             `json:"cases"`
	Surfaces       int             `json:"surfaces"`
}

type Observation struct {
	Value json.RawMessage
}

type Difference struct {
	Path string          `json:"path"`
	Want json.RawMessage `json:"want,omitempty"`
	Got  json.RawMessage `json:"got,omitempty"`
}

type CaseRecord struct {
	Type        string             `json:"type"`
	ID          string             `json:"id"`
	Class       ProfileClass       `json:"class"`
	Comparison  ComparisonContract `json:"comparison"`
	Status      string             `json:"status"`
	Error       string             `json:"error,omitempty"`
	Authorities []string           `json:"authorities"`
	Node        json.RawMessage    `json:"node,omitempty"`
	Goja        json.RawMessage    `json:"goja,omitempty"`
	Expected    json.RawMessage    `json:"expected,omitempty"`
	Differences []Difference       `json:"differences,omitempty"`
}

type ClassSummary struct {
	Total    int `json:"total"`
	Passed   int `json:"passed"`
	Mismatch int `json:"mismatch"`
	Invalid  int `json:"invalid"`
}

type SummaryRecord struct {
	Classes     map[string]ClassSummary `json:"classes"`
	Type        string                  `json:"type"`
	Status      string                  `json:"status"`
	Error       string                  `json:"error,omitempty"`
	Conformance ClassSummary            `json:"conformance"`
	Exit        int                     `json:"exit"`
	Cases       int                     `json:"cases"`
	Passed      int                     `json:"passed"`
	Mismatch    int                     `json:"mismatch"`
	Invalid     int                     `json:"invalid"`
	Surfaces    int                     `json:"surfaces"`
}
