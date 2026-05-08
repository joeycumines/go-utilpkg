package tournament

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

var tournamentCandidateScopes = []string{
	"eventloop/docs/tournament",
	"eventloop/eventtargetbenchmark_test.go",
	"eventloop/fd_bench_unix_test.go",
	"eventloop/future_bench_test.go",
	"eventloop/futurevariant_test.go",
	"eventloop/internal/alternateone",
	"eventloop/internal/alternatethree",
	"eventloop/internal/alternatetwo",
	"eventloop/internal/gojabaseline",
	"eventloop/internal/libuvbaseline",
	"eventloop/internal/promisealtfive",
	"eventloop/internal/promisealtfour",
	"eventloop/internal/promisealtone",
	"eventloop/internal/promisealtthree",
	"eventloop/internal/promisealttwo",
	"eventloop/internal/promisetournament",
	"eventloop/internal/tournament",
	"eventloop/internal/tournamentmeta",
	"eventloop/internal/tournamenttest",
	"eventloop/promise_bench_test.go",
	"eventloop/scheduler_priority_bench_test.go",
	"eventloop/scheduler_runtime_bench_test.go",
	"eventloop/scheduler_topology_bench_test.go",
	"eventloop/wakeup_dedup_test.go",
	"go.work",
	"goja-eventloop/adapter_promise_check_test.go",
	"goja-eventloop/benchmark_lifecycle_test.go",
	"goja-eventloop/process_lifecycle_bench_test.go",
	"goja-eventloop/promise_handover_bench_test.go",
	"goja-eventloop/promise_handover_variants_test.go",
	"project.mk",
}

func TestTournamentCandidateTrackedPathCensus(t *testing.T) {
	repository, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("repository path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repository, ".git")); os.IsNotExist(err) {
		t.Skip("tracked-path census requires the monorepo Git index")
	} else if err != nil {
		t.Fatalf("inspect repository: %v", err)
	}
	arguments := append([]string{"-C", repository, "ls-files", "-z", "--"}, tournamentCandidateScopes...)
	command := exec.Command("git", arguments...)
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
	data, err := command.Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	paths := bytes.Split(bytes.TrimSuffix(data, []byte{0}), []byte{0})
	if len(paths) != 558 {
		t.Fatalf("tracked tournament candidate paths = %d, want 558", len(paths))
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != "9d442421b72ddce3b266caa4b78c3fd729c101b39c4235b44755150a5fb102d4" {
		t.Fatalf("tracked tournament candidate path-set SHA-256 = %s", got)
	}
	for _, required := range [][]byte{
		[]byte("eventloop/docs/tournament/2026-05-08/darwin.log"),
		[]byte("eventloop/docs/tournament/2026-05-08/linux.log"),
		[]byte("eventloop/docs/tournament/2026-05-14/darwin.failed-random-deadlines.log"),
		[]byte("eventloop/docs/tournament/2026-05-14/darwin.pre-source-fingerprint-fix.log"),
		[]byte("eventloop/internal/tournament/go.mod"),
		[]byte("eventloop/internal/tournament/testdata/manifest-v4.json.gz.b64"),
		[]byte("eventloop/internal/gojabaseline/go.mod"),
	} {
		if _, found := slices.BinarySearchFunc(paths, required, bytes.Compare); !found {
			t.Errorf("required tracked tournament path %q is absent", required)
		}
	}
}

func TestTrackedTournamentRawEvidenceExact(t *testing.T) {
	repository, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("repository path: %v", err)
	}
	for path, expected := range map[string]struct {
		bytes  int
		sha256 string
	}{
		"eventloop/docs/tournament/2026-05-08/darwin.log": {
			bytes: 25283, sha256: "b6fdafafbaa02ae16585055cf6065a0f005e829556ce99c3d56912d3313fe864",
		},
		"eventloop/docs/tournament/2026-05-08/linux.log": {
			bytes: 25655, sha256: "bc3f5d625f7130af37550f9d289ed702a192b00a230ce74cc4f7115bd6a26b60",
		},
		"eventloop/docs/tournament/2026-05-14/darwin.failed-random-deadlines.log": {
			bytes: 34490, sha256: "c47d642b642fe405e8e7711b2043db79049634d99955affb8345991bb027d9ff",
		},
		"eventloop/docs/tournament/2026-05-14/darwin.pre-source-fingerprint-fix.log": {
			bytes: 266035, sha256: "bd565d6ec2ea788f839d87d02b8ff20b14af59229c9c27a4d05d39f21351af12",
		},
	} {
		data, err := os.ReadFile(filepath.Join(repository, filepath.FromSlash(path)))
		if err != nil {
			t.Errorf("read %s: %v", path, err)
			continue
		}
		if len(data) != expected.bytes {
			t.Errorf("%s bytes = %d, want %d", path, len(data), expected.bytes)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != expected.sha256 {
			t.Errorf("%s SHA-256 = %s, want %s", path, got, expected.sha256)
		}
	}
}

func TestTournamentCandidateTreeReconstructsTrackedScope(t *testing.T) {
	repository, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("repository path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repository, ".git")); os.IsNotExist(err) {
		t.Skip("candidate-tree reconstruction requires the monorepo Git index")
	} else if err != nil {
		t.Fatalf("inspect repository: %v", err)
	}
	arguments := append([]string{"diff", "--quiet", "--"}, tournamentCandidateScopes...)
	if output, err := tournamentCandidateGit(repository, arguments...); err != nil {
		t.Fatalf("tournament candidate has unstaged tracked drift: %v: %s", err, output)
	}
	for _, ignored := range []bool{false, true} {
		arguments := []string{"ls-files", "--others", "-z"}
		if ignored {
			arguments = append(arguments, "--ignored")
		}
		arguments = append(arguments, "--exclude-standard", "--")
		arguments = append(arguments, tournamentCandidateScopes...)
		output, err := tournamentCandidateGit(repository, arguments...)
		if err != nil {
			t.Fatalf("enumerate untracked tournament paths: %v", err)
		}
		if len(output) != 0 {
			t.Fatalf("tournament candidate has untracked paths: %q", output)
		}
	}
	arguments = append([]string{"ls-files", "-z", "--"}, tournamentCandidateScopes...)
	tracked, err := tournamentCandidateGit(repository, arguments...)
	if err != nil {
		t.Fatalf("enumerate tracked tournament paths: %v", err)
	}
	expected := make(map[string]struct{}, 558)
	for path := range bytes.SplitSeq(bytes.TrimSuffix(tracked, []byte{0}), []byte{0}) {
		expected[string(path)] = struct{}{}
	}
	treeData, err := tournamentCandidateGit(repository, "write-tree")
	if err != nil {
		t.Fatalf("write tournament candidate tree: %v", err)
	}
	tree := string(bytes.TrimSpace(treeData))
	pack, err := tournamentCandidateGitInput(repository, []byte(tree+"\n"), "pack-objects", "--stdout", "--revs")
	if err != nil {
		t.Fatalf("pack tournament candidate tree %s: %v", tree, err)
	}
	isolated := filepath.Join(t.TempDir(), "repository")
	command := exec.Command("git", "init", "-q", "--object-format=sha1", isolated)
	command.Env = tournamentCandidateGitEnvironment()
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("initialize isolated candidate repository: %v: %s", err, output)
	}
	if _, err := tournamentCandidateGitInput(isolated, pack, "index-pack", "--stdin"); err != nil {
		t.Fatalf("import candidate tree pack: %v", err)
	}
	if _, err := tournamentCandidateGit(isolated, "update-ref", "refs/tournament/candidate", tree); err != nil {
		t.Fatalf("anchor isolated candidate tree: %v", err)
	}
	if objectType, err := tournamentCandidateGit(isolated, "cat-file", "-t", tree); err != nil {
		t.Fatalf("resolve isolated candidate tree: %v", err)
	} else if string(bytes.TrimSpace(objectType)) != "tree" {
		t.Fatalf("isolated candidate object %s type = %q", tree, objectType)
	}
	if output, err := tournamentCandidateGit(isolated, "fsck", "--strict", "--no-reflogs"); err != nil {
		t.Fatalf("fsck isolated candidate repository: %v: %s", err, output)
	}
	if _, err := os.Stat(filepath.Join(isolated, ".git", "objects", "info", "alternates")); !os.IsNotExist(err) {
		t.Fatalf("isolated candidate repository has alternates: %v", err)
	}
	arguments = append([]string{"archive", "--format=tar", tree, "--"}, tournamentCandidateScopes...)
	archive, err := tournamentCandidateGit(isolated, arguments...)
	if err != nil {
		t.Fatalf("archive tournament candidate tree %s: %v", tree, err)
	}
	reader := tar.NewReader(bytes.NewReader(archive))
	seen := make(map[string]struct{}, len(expected))
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read tournament candidate archive: %v", err)
		}
		if header.FileInfo().IsDir() {
			continue
		}
		if _, ok := expected[header.Name]; !ok {
			t.Fatalf("candidate archive has unexpected path %q", header.Name)
		}
		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("read candidate archive path %q: %v", header.Name, err)
		}
		worktree, err := os.ReadFile(filepath.Join(repository, filepath.FromSlash(header.Name)))
		if err != nil {
			t.Fatalf("read candidate worktree path %q: %v", header.Name, err)
		}
		if !bytes.Equal(data, worktree) {
			t.Fatalf("candidate archive path %q differs from the worktree", header.Name)
		}
		seen[header.Name] = struct{}{}
	}
	if len(seen) != len(expected) {
		t.Fatalf("candidate archive reconstructed %d/%d tracked paths", len(seen), len(expected))
	}
}

func tournamentCandidateGit(repository string, arguments ...string) ([]byte, error) {
	return tournamentCandidateGitInput(repository, nil, arguments...)
}

func tournamentCandidateGitInput(repository string, input []byte, arguments ...string) ([]byte, error) {
	command := exec.Command("git", append([]string{"-C", repository}, arguments...)...)
	command.Env = tournamentCandidateGitEnvironment()
	command.Stdin = bytes.NewReader(input)
	return command.Output()
}

func tournamentCandidateGitEnvironment() []string {
	return append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0")
}
