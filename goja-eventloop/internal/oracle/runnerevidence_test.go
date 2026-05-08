package oracle

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
)

func TestRunArtifactPreparationFailureEvidence(t *testing.T) {
	manifest := evidenceManifest()
	wantRuntime := evidenceRuntime()
	prepareErr := errors.New("artifact preparation failed")
	type runtimeContextKey struct{}
	contextValue := &struct{}{}
	ctx := context.WithValue(context.Background(), runtimeContextKey{}, contextValue)
	services := runnerServices{
		runtime: func(got context.Context) (RuntimeIdentity, error) {
			if got.Value(runtimeContextKey{}) != contextValue {
				t.Fatal("runner context was not threaded to runtime identity")
			}
			return wantRuntime, nil
		},
		prepare: func(string) (*nodeArtifact, error) { return nil, prepareErr },
	}
	var output, diagnostics bytes.Buffer
	if exit := runEvidence(ctx, manifest, "missing", &output, &diagnostics, services); exit != ExitInvalidRun {
		t.Fatalf("run exit = %d, want %d", exit, ExitInvalidRun)
	}
	records := evidenceRecords(t, output.Bytes(), 2)
	var attempt AttemptRecord
	if err := json.Unmarshal(records[0], &attempt); err != nil {
		t.Fatalf("decode attempt: %v", err)
	}
	if attempt.Type != "attempt" || attempt.Schema != ProtocolSchema || attempt.ManifestSHA256 != manifest.SHA256 || attempt.Runtime != wantRuntime {
		t.Fatalf("attempt = %+v", attempt)
	}
	var summary SummaryRecord
	if err := json.Unmarshal(records[1], &summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if summary.Status != "invalid" || summary.Exit != ExitInvalidRun || !strings.Contains(summary.Error, prepareErr.Error()) {
		t.Fatalf("summary = %+v", summary)
	}
	if got := diagnostics.String(); !strings.Contains(got, "Node artifact") || strings.Contains(got, "Node identity") {
		t.Fatalf("diagnostics = %q", got)
	}
}

func TestRunIdentityFailureEvidence(t *testing.T) {
	manifest := evidenceManifest()
	wantRuntime := evidenceRuntime()
	identityErr := errors.New("authenticated identity failed")
	closes := 0
	services := runnerServices{
		runtime:  func(context.Context) (RuntimeIdentity, error) { return wantRuntime, nil },
		prepare:  func(string) (*nodeArtifact, error) { return &nodeArtifact{}, nil },
		identify: func(context.Context, *nodeArtifact) (NodeIdentity, error) { return NodeIdentity{}, identityErr },
		close: func(*nodeArtifact) error {
			closes++
			return nil
		},
	}
	var output, diagnostics bytes.Buffer
	if exit := runEvidence(context.Background(), manifest, "authenticated", &output, &diagnostics, services); exit != ExitInvalidRun {
		t.Fatalf("run exit = %d, want %d", exit, ExitInvalidRun)
	}
	records := evidenceRecords(t, output.Bytes(), 2)
	var attempt AttemptRecord
	if err := json.Unmarshal(records[0], &attempt); err != nil {
		t.Fatalf("decode attempt: %v", err)
	}
	if attempt.Type != "attempt" || attempt.Runtime != wantRuntime {
		t.Fatalf("attempt = %+v", attempt)
	}
	var summary SummaryRecord
	if err := json.Unmarshal(records[1], &summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if summary.Status != "invalid" || !strings.Contains(summary.Error, identityErr.Error()) || closes != 1 {
		t.Fatalf("summary = %+v, closes = %d", summary, closes)
	}
	if got := diagnostics.String(); !strings.Contains(got, "Node identity") || strings.Contains(got, "Node artifact:") {
		t.Fatalf("diagnostics = %q", got)
	}
}

func TestRunCleanupFailureInvalidatesTerminalSummary(t *testing.T) {
	manifest := evidenceManifest()
	wantRuntime := evidenceRuntime()
	closeErr := errors.New("cleanup failed")
	closes := 0
	services := runnerServices{
		runtime: func(context.Context) (RuntimeIdentity, error) { return wantRuntime, nil },
		prepare: func(string) (*nodeArtifact, error) { return &nodeArtifact{}, nil },
		identify: func(context.Context, *nodeArtifact) (NodeIdentity, error) {
			return NodeIdentity{Version: NodeVersion}, nil
		},
		execute: func(context.Context, *nodeArtifact, *LoadedManifest, Fixture) CaseRecord { panic("unexpected case") },
		close: func(*nodeArtifact) error {
			closes++
			return closeErr
		},
	}
	var output, diagnostics bytes.Buffer
	if exit := runEvidence(context.Background(), manifest, "authenticated", &output, &diagnostics, services); exit != ExitInvalidRun {
		t.Fatalf("run exit = %d, want %d", exit, ExitInvalidRun)
	}
	records := evidenceRecords(t, output.Bytes(), 3)
	var attempt AttemptRecord
	if err := json.Unmarshal(records[0], &attempt); err != nil {
		t.Fatalf("decode attempt: %v", err)
	}
	var header HeaderRecord
	if err := json.Unmarshal(records[1], &header); err != nil {
		t.Fatalf("decode header: %v", err)
	}
	var summary SummaryRecord
	if err := json.Unmarshal(records[2], &summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if attempt.Runtime != wantRuntime || header.Runtime != wantRuntime || header.Type != "header" {
		t.Fatalf("attempt/header runtime = %+v / %+v", attempt.Runtime, header.Runtime)
	}
	if summary.Status != "invalid" || summary.Exit != ExitInvalidRun || !strings.Contains(summary.Error, closeErr.Error()) || closes != 1 {
		t.Fatalf("summary = %+v, closes = %d", summary, closes)
	}
	if !strings.Contains(diagnostics.String(), "close Node artifact") {
		t.Fatalf("diagnostics = %q", diagnostics.String())
	}
}

func TestRuntimeIdentityContentAddressesExecutableAndEventloop(t *testing.T) {
	root := t.TempDir()
	mainRoot := filepath.Join(root, "goja-eventloop")
	source := filepath.Join(mainRoot, "internal", "oracle", "runner.go")
	if err := os.MkdirAll(filepath.Dir(source), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mainRoot, "go.mod"), []byte("module github.com/joeycumines/goja-eventloop\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("package oracle\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	candidateRoot := filepath.Join(root, "eventloop")
	if err := os.MkdirAll(candidateRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(candidateRoot, "go.mod"), []byte("module github.com/joeycumines/go-eventloop\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	candidatePath := filepath.Join(candidateRoot, "candidate.go")
	if err := os.WriteFile(candidatePath, []byte("package eventloop\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	executableBytes := []byte("oracle executable bytes")
	executable := filepath.Join(root, "oracle")
	if err := os.WriteFile(executable, executableBytes, 0o700); err != nil {
		t.Fatal(err)
	}
	info := evidenceBuildInfo()
	identity, err := buildRuntimeIdentity(context.Background(), executable, source, info)
	if err != nil {
		t.Fatalf("buildRuntimeIdentity: %v", err)
	}
	executableSum := sha256.Sum256(executableBytes)
	if got, want := identity.ExecutableSHA256, hex.EncodeToString(executableSum[:]); got != want {
		t.Fatalf("executable SHA-256 = %s, want %s", got, want)
	}
	if identity.IdentityMode != runtimeIdentityVCSWorktree ||
		identity.Package != oracleCommandPackage ||
		identity.ModuleSum != "" {
		t.Fatalf("worktree identity mode = %+v", identity)
	}
	if identity.VCS != "git" || identity.VCSRevision != strings.Repeat("a", 40) || !identity.VCSDirty {
		t.Fatalf("VCS identity = %+v", identity)
	}
	if identity.EventloopVersion != "v0.0.0-test" || identity.EventloopReplacement != "../eventloop" || identity.EventloopCandidateFormat != eventloopCandidateFormat || identity.EventloopCandidateRecords != 2 || len(identity.EventloopCandidateSHA256) != sha256.Size*2 {
		t.Fatalf("eventloop identity = %+v", identity)
	}
	repeated, err := buildRuntimeIdentity(context.Background(), executable, source, info)
	if err != nil || repeated.EventloopCandidateSHA256 != identity.EventloopCandidateSHA256 {
		t.Fatalf("repeated identity = %+v, %v", repeated, err)
	}
	if err := os.WriteFile(candidatePath, []byte("package eventloop\n// changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err := buildRuntimeIdentity(context.Background(), executable, source, info)
	if err != nil {
		t.Fatalf("changed buildRuntimeIdentity: %v", err)
	}
	if changed.EventloopCandidateSHA256 == identity.EventloopCandidateSHA256 {
		t.Fatal("eventloop candidate digest did not change with candidate bytes")
	}
	info.Settings = nil
	if _, err := buildRuntimeIdentity(context.Background(), executable, source, info); err == nil || !strings.Contains(err.Error(), "VCS") {
		t.Fatalf("missing VCS error = %v", err)
	}
}

func TestRuntimeIdentityAuthenticatesModuleArchive(t *testing.T) {
	root := t.TempDir()
	executableBytes := []byte("archive oracle executable")
	executable := filepath.Join(root, "oracle")
	if err := os.WriteFile(executable, executableBytes, 0o700); err != nil {
		t.Fatal(err)
	}
	info := archiveBuildInfo()
	identity, err := buildRuntimeIdentity(
		context.Background(),
		executable,
		filepath.Join(root, "source-does-not-exist.go"),
		info,
	)
	if err != nil {
		t.Fatal(err)
	}
	executableSum := sha256.Sum256(executableBytes)
	if identity.IdentityMode != runtimeIdentityModuleArchive ||
		identity.Package != oracleCommandPackage ||
		identity.Module != oracleModule+"@v0.0.0-candidate.0" ||
		identity.ModuleSum != info.Main.Sum ||
		identity.GojaVersion != "v0.0.0-candidate.0" ||
		identity.GojaSum != info.Deps[0].Sum ||
		identity.EventloopVersion != "v0.0.0-candidate.0" ||
		identity.EventloopSum != info.Deps[1].Sum ||
		identity.ExecutableSHA256 != hex.EncodeToString(executableSum[:]) {
		t.Fatalf("archive identity = %+v", identity)
	}
	if identity.VCS != "" ||
		identity.VCSRevision != "" ||
		identity.VCSDirty ||
		identity.EventloopReplacement != "" ||
		identity.EventloopCandidateSHA256 != "" ||
		identity.EventloopCandidateRecords != 0 {
		t.Fatalf("archive identity retained worktree evidence = %+v", identity)
	}
}

func TestRuntimeIdentityRejectsIncompleteModuleArchive(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "oracle")
	if err := os.WriteFile(executable, []byte("oracle"), 0o700); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*debug.BuildInfo)
	}{
		{name: "wrong command package", mutate: func(info *debug.BuildInfo) { info.Path = oracleModule + "/cmd/other" }},
		{name: "wrong main module", mutate: func(info *debug.BuildInfo) { info.Main.Path = "example.com/other" }},
		{name: "versioned main without sum", mutate: func(info *debug.BuildInfo) { info.Main.Sum = "" }},
		{name: "development main with sum", mutate: func(info *debug.BuildInfo) { info.Main.Version = "(devel)" }},
		{name: "Goja without sum", mutate: func(info *debug.BuildInfo) { info.Deps[0].Sum = "" }},
		{name: "eventloop without sum", mutate: func(info *debug.BuildInfo) { info.Deps[1].Sum = "" }},
		{name: "Goja replacement", mutate: func(info *debug.BuildInfo) {
			info.Deps[0].Replace = &debug.Module{Path: "/tmp/goja", Version: "(devel)"}
		}},
		{name: "eventloop replacement", mutate: func(info *debug.BuildInfo) {
			info.Deps[1].Replace = &debug.Module{Path: "../eventloop", Version: "(devel)"}
		}},
		{name: "missing Goja", mutate: func(info *debug.BuildInfo) { info.Deps = info.Deps[1:] }},
		{name: "duplicate eventloop", mutate: func(info *debug.BuildInfo) {
			copy := *info.Deps[1]
			info.Deps = append(info.Deps, &copy)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			info := archiveBuildInfo()
			test.mutate(info)
			if identity, err := buildRuntimeIdentity(
				context.Background(),
				executable,
				filepath.Join(root, "missing-source.go"),
				info,
			); err == nil {
				t.Fatalf("incomplete archive identity succeeded: %+v", identity)
			}
		})
	}
}

func TestNodeTerminalFrameUsesSynchronousWrite(t *testing.T) {
	if !strings.Contains(nodeFixtureProgram, `fs.writeSync(1, "GOJA_EVENTLOOP_ORACLE_V1:"`) {
		t.Fatal("Node terminal frame is not a synchronous fd 1 write")
	}
	if strings.Contains(nodeFixtureProgram, "process.stdout.write.bind") {
		t.Fatal("Node terminal frame retains asynchronous stdout binding")
	}
}

func TestNodeConsoleCaptureIsScopedAndSynchronous(t *testing.T) {
	for _, forbidden := range []string{"new Writable(", "new Console("} {
		if strings.Contains(nodeFixtureProgram, forbidden) {
			t.Fatalf("Node console capture replaces native console state via %q", forbidden)
		}
	}
	for _, fragment := range []string{
		`process.stdout === process.stderr ? [process.stdout] : [process.stdout, process.stderr]`,
		`Object.getOwnPropertyDescriptor(stream, "write")`,
		`value: captureWrite`,
		`try {
        for (const stream of streams)`,
		`callback();
      } finally {`,
		`Object.defineProperty(stream, "write", descriptor)`,
		`else delete stream.write`,
		`if (overflow) throw new RangeError`,
		`Buffer.concat(chunks).toString("utf8")`,
	} {
		if !strings.Contains(nodeFixtureProgram, fragment) {
			t.Fatalf("Node console capture is missing %q", fragment)
		}
	}
}

func evidenceManifest() *LoadedManifest {
	return &LoadedManifest{
		Manifest: Manifest{
			Schema: ManifestSchema,
			Node:   NodePin{Version: NodeVersion, Tag: NodeTag, SourceCommit: NodeSourceCommit, ReleaseURL: NodeReleaseURL},
		},
		SHA256: "manifest-sha256",
	}
}

func evidenceRuntime() RuntimeIdentity {
	return RuntimeIdentity{
		GoVersion: "go-test", GOOS: "test", GOARCH: "test", ExecutableSHA256: strings.Repeat("1", 64),
		VCS: "git", VCSRevision: strings.Repeat("2", 40), VCSDirty: true,
		IdentityMode: runtimeIdentityVCSWorktree, Package: oracleCommandPackage,
		Module: "github.com/joeycumines/goja-eventloop@(devel)", GojaVersion: "v-test",
		EventloopVersion: "v-test", EventloopReplacement: "../eventloop", EventloopCandidateFormat: eventloopCandidateFormat, EventloopCandidateSHA256: strings.Repeat("3", 64), EventloopCandidateRecords: 1,
	}
}

func evidenceBuildInfo() *debug.BuildInfo {
	return &debug.BuildInfo{
		Path: oracleCommandPackage,
		Main: debug.Module{Path: "github.com/joeycumines/goja-eventloop", Version: "(devel)"},
		Deps: []*debug.Module{
			{Path: "github.com/joeycumines/goja", Version: "v0.0.0-goja", Sum: "h1:goja"},
			{Path: "github.com/joeycumines/go-eventloop", Version: "v0.0.0-test", Replace: &debug.Module{Path: "../eventloop", Version: "(devel)"}},
		},
		Settings: []debug.BuildSetting{
			{Key: "vcs", Value: "git"},
			{Key: "vcs.revision", Value: strings.Repeat("a", 40)},
			{Key: "vcs.modified", Value: "true"},
		},
	}
}

func archiveBuildInfo() *debug.BuildInfo {
	return &debug.BuildInfo{
		Path: oracleCommandPackage,
		Main: debug.Module{
			Path:    oracleModule,
			Version: "v0.0.0-candidate.0",
			Sum:     evidenceModuleSum(1),
		},
		Deps: []*debug.Module{
			{
				Path:    "github.com/joeycumines/goja",
				Version: "v0.0.0-candidate.0",
				Sum:     evidenceModuleSum(2),
			},
			{
				Path:    "github.com/joeycumines/go-eventloop",
				Version: "v0.0.0-candidate.0",
				Sum:     evidenceModuleSum(3),
			},
		},
	}
}

func evidenceModuleSum(value byte) string {
	return "h1:" + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{value}, sha256.Size))
}

func evidenceRecords(t *testing.T, data []byte, want int) [][]byte {
	t.Helper()
	records := bytes.Split(bytes.TrimSpace(data), []byte{'\n'})
	if len(records) != want {
		t.Fatalf("evidence records = %d, want %d: %s", len(records), want, data)
	}
	return records
}
