package tournament

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestHarnessIndexCandidateReconstructsExactAuthority(t *testing.T) {
	const (
		patchPath                  = "revisions/candidates/0002-index-on-469fd952-benchmark-harnesses.patch"
		patchSHA256                = "886935318857f1ab3c46ccd999396bf28c2cf8e7f6830bf6ff3309fcf776cfb5"
		patchBytes                 = 24970
		baseRevision               = "469fd952ed251edc7ea1d2bb0faf4e04fc94dd88"
		baseTree                   = "def5cb294735fd57b36e7075084ba88991916421"
		baseEventloopTree          = "227ee042f259cdb35c3e33a6dbecb2b8ec746a21"
		reconstructedTree          = "0b6c304c6e82e3ad10f2bef51f916ba24f4c87f1"
		reconstructedEventloopTree = "171ef89dd82213cd61697d37d33887088014e5c5"
		unchangedGojaTree          = "69d8cf81666942396704d3d4bdb75208a0e523c6"
	)
	targets := []struct {
		path           string
		blob           string
		bytes          int64
		sha256         string
		manifestSHA256 string
	}{
		{"eventloop/eventtargetbenchmark_test.go", "559a868ec9a726e7f4e0fb144053a0d92180f64a", 3378, "088188895a99d52a98b32d714fce2f3a084b0bad351e0dbb33fd131b1b853663", "7d4bc566aaa5ac20ec457e5f5a1efc0bfe6299bcbff9e7d38d0f2d9c29aec35a"},
		{"eventloop/fd_bench_unix_test.go", "198b0104ea9600cd8d3f4cb9d89c60b99bcf31c0", 7404, "13991fb30c426a769d9615826e32ea6f5692476ee843221d007f37a44e610d22", "598c98f3505336a631ea2358ec8dc79edde835152bb57f5cabf4c545d0a490c4"},
		{"eventloop/promise_bench_test.go", "6d4f5f2a124925740498057c05b0d3f247cf13ee", 7712, "4645472b48c6e5c93dcd1e7256438af4f9094278bd461c2004d1884c61044d7f", "a124c302780e234df9be234c4d9b2dd7def2c10051263caff69b63b1b549caf4"},
		{"eventloop/scheduler_priority_bench_test.go", "a93e21366055ae02d605a29e64e9f6b9bb7ac953", 3967, "cf0115edb56e0b021d30f6a6437b9a7de7b7f02f6fa24a77734623bb965e12f8", "e724e69f6a94a3e88dcadb7db384dd03aeb2e4b41ceea4d9de0fdfddda7d6928"},
		{"eventloop/wakeup_dedup_test.go", "e487b452863620e24774126c27c85202a4f994d5", 3883, "b1bbf2738e6330098a969d30394c80c2fd99e512f433e3fe8338c1dfb8f1bff3", "26a626161e738b2b61fb409aba8c2e1b8cf2a24684b961dd3446655ea96c75b1"},
	}

	repository, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repository, ".git")); os.IsNotExist(err) {
		t.Skip("exact reconstruction requires the monorepo Git object store")
	} else if err != nil {
		t.Fatal(err)
	}
	archivePath, err := filepath.Abs(patchPath)
	if err != nil {
		t.Fatal(err)
	}
	patch, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(patch) != patchBytes {
		t.Fatalf("harness archive bytes = %d, want %d", len(patch), patchBytes)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(patch)); got != patchSHA256 {
		t.Fatalf("harness archive SHA-256 = %s, want %s", got, patchSHA256)
	}

	temporary := t.TempDir()
	configPath := filepath.Join(temporary, "global.gitconfig")
	if err := os.WriteFile(configPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	environment := revisionGitEnvironment([]string{
		"GIT_CONFIG_GLOBAL=" + configPath,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_NO_REPLACE_OBJECTS=1",
		"HOME=" + temporary,
		"XDG_CONFIG_HOME=" + temporary,
	})
	if got := revisionGitOutput(t, repository, environment, "rev-parse", baseRevision+"^{commit}"); got != baseRevision {
		t.Fatalf("harness base revision = %s, want %s", got, baseRevision)
	}
	if got := revisionGitOutput(t, repository, environment, "rev-parse", baseRevision+"^{tree}"); got != baseTree {
		t.Fatalf("harness base tree = %s, want %s", got, baseTree)
	}
	if got := revisionGitOutput(t, repository, environment, "rev-parse", baseTree+":eventloop"); got != baseEventloopTree {
		t.Fatalf("harness base eventloop tree = %s, want %s", got, baseEventloopTree)
	}

	isolated := filepath.Join(temporary, "isolated.git")
	runIndexArchiveGit(t, environment, "init", "--bare", "--quiet", isolated)
	seedIndexArchiveBase(t, repository, isolated, environment, baseTree)
	if _, err := os.Stat(filepath.Join(isolated, "objects", "info", "alternates")); !os.IsNotExist(err) {
		t.Fatalf("isolated harness repository has alternates: %v", err)
	}
	for _, target := range targets {
		command := exec.Command("git", "-C", isolated, "cat-file", "-e", target.blob)
		command.Env = environment
		if err := command.Run(); err == nil {
			t.Fatalf("target blob %s existed before applying the archive", target.blob)
		} else if _, ok := err.(*exec.ExitError); !ok {
			t.Fatalf("probe target blob %s: %v", target.blob, err)
		}
	}

	indexEnvironment := append(environment, "GIT_INDEX_FILE="+filepath.Join(temporary, "index"))
	runRevisionGit(t, isolated, indexEnvironment, "read-tree", baseTree)
	runRevisionGit(t, isolated, indexEnvironment, "apply", "--cached", "--binary", "--whitespace=error-all", archivePath)
	if got := revisionGitOutput(t, isolated, indexEnvironment, "write-tree"); got != reconstructedTree {
		t.Fatalf("harness reconstructed tree = %s, want %s", got, reconstructedTree)
	}
	if got := revisionGitOutput(t, isolated, indexEnvironment, "rev-parse", reconstructedTree+":eventloop"); got != reconstructedEventloopTree {
		t.Fatalf("harness reconstructed eventloop tree = %s, want %s", got, reconstructedEventloopTree)
	}
	if got := revisionGitOutput(t, isolated, indexEnvironment, "rev-parse", reconstructedTree+":goja-eventloop"); got != unchangedGojaTree {
		t.Fatalf("harness reconstructed goja tree = %s, want %s", got, unchangedGojaTree)
	}
	wantStatus := strings.Join([]string{
		"A\teventloop/eventtargetbenchmark_test.go",
		"M\teventloop/fd_bench_unix_test.go",
		"A\teventloop/promise_bench_test.go",
		"A\teventloop/scheduler_priority_bench_test.go",
		"M\teventloop/wakeup_dedup_test.go",
	}, "\n")
	if got := revisionGitOutput(t, isolated, indexEnvironment, "diff-tree", "--no-commit-id", "--name-status", "-r", baseTree, reconstructedTree); got != wantStatus {
		t.Fatalf("harness reconstructed paths:\n%s\nwant:\n%s", got, wantStatus)
	}

	for _, target := range targets {
		blob := revisionGitOutput(t, isolated, indexEnvironment, "rev-parse", reconstructedTree+":"+target.path)
		if blob != target.blob {
			t.Errorf("harness %s blob = %s, want %s", target.path, blob, target.blob)
		}
		entry := strings.Fields(revisionGitOutput(t, isolated, indexEnvironment, "ls-tree", reconstructedTree, "--", target.path))
		if len(entry) < 3 || entry[0] != "100644" || entry[1] != "blob" || entry[2] != target.blob {
			t.Errorf("harness %s tree entry = %v", target.path, entry)
		}
		bytesValue, err := strconv.ParseInt(revisionGitOutput(t, isolated, indexEnvironment, "cat-file", "-s", blob), 10, 64)
		if err != nil || bytesValue != target.bytes {
			t.Errorf("harness %s bytes = (%d, %v), want %d", target.path, bytesValue, err, target.bytes)
		}
		payload := runIndexArchiveGitOutput(t, isolated, indexEnvironment, "cat-file", "blob", blob)
		if got := fmt.Sprintf("%x", sha256.Sum256(payload)); got != target.sha256 {
			t.Errorf("harness %s SHA-256 = %s, want %s", target.path, got, target.sha256)
		}
		digest := sha256.New()
		writeFramedDigest(digest, []byte(strings.TrimPrefix(target.path, "eventloop/")))
		writeFramedDigest(digest, payload)
		if got := fmt.Sprintf("%x", digest.Sum(nil)); got != target.manifestSHA256 {
			t.Errorf("harness %s manifest SHA-256 = %s, want %s", target.path, got, target.manifestSHA256)
		}
	}
	runRevisionGit(t, isolated, indexEnvironment, "fsck", "--strict", "--no-reflogs")
}

func TestRestoredHarnessArchiveReconstructsExactAuthority(t *testing.T) {
	const (
		patchPath         = "revisions/candidates/0003-current-restored-benchmark-harnesses.patch"
		patchSHA256       = "53b480b1c23d888ce4e4159811f3d23f3d463c77593258f055ba5be7c1ead021"
		patchBytes        = 28863
		emptyTree         = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"
		reconstructedTree = "1da3de0d68ef0a6e5d5e921f84a94bd20ab6ab50"
	)
	targets := []struct {
		path   string
		blob   string
		bytes  int64
		sha256 string
	}{
		{"eventloop/internal/promisetournament/promise_tournament_test.go", "2bb107c956267e498da9e3bf6bdb10b6ad991fef", 7866, "c377367e97a9e568c137ecc585bc7ca17267a9815e15afc4c3eb1cb9041a39f8"},
		{"eventloop/internal/tournament/bench_multiproducer_test.go", "df6f1545cd6f67b8ff2e760df52f6a8dd895e9d6", 5568, "3c217b7fab1c74a05527e1a943a73b4f5382837e4f7799b430a24462642e3f7a"},
		{"eventloop/internal/tournament/micro_batch_test.go", "d7b3ce2317828e128ceacb0bb1af56916d6c8dd1", 4924, "f5f687e5214f300eca337ed0d7c0e17ce0bafb9d4b1726c2054af9f1b8c09ab2"},
		{"eventloop/internal/tournament/micro_cas_test.go", "d3be732b79ec2b64c64eb1c52f122864a081c2fd", 3164, "e74401606cde66633566d47a7d3c8c832d6b0e090d3cadf15a2287adc78ec70e"},
		{"eventloop/internal/tournament/promise_corrected_bench_test.go", "cc514b35e00ceb5e1270fd661022d4657b46bc77", 4710, "f38711ef1bf1407d5d6816c037527aa69da03d198d30033c706d29d7cf893907"},
	}

	archivePath, err := filepath.Abs(patchPath)
	if err != nil {
		t.Fatal(err)
	}
	patch, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(patch) != patchBytes {
		t.Fatalf("restored harness archive bytes = %d, want %d", len(patch), patchBytes)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(patch)); got != patchSHA256 {
		t.Fatalf("restored harness archive SHA-256 = %s, want %s", got, patchSHA256)
	}

	temporary := t.TempDir()
	configPath := filepath.Join(temporary, "global.gitconfig")
	if err := os.WriteFile(configPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	environment := revisionGitEnvironment([]string{
		"GIT_CONFIG_GLOBAL=" + configPath,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_NO_REPLACE_OBJECTS=1",
		"HOME=" + temporary,
		"XDG_CONFIG_HOME=" + temporary,
	})
	isolated := filepath.Join(temporary, "isolated.git")
	runIndexArchiveGit(t, environment, "init", "--bare", "--quiet", isolated)
	indexEnvironment := append(environment, "GIT_INDEX_FILE="+filepath.Join(temporary, "index"))
	runRevisionGit(t, isolated, indexEnvironment, "read-tree", "--empty")
	if got := revisionGitOutput(t, isolated, indexEnvironment, "write-tree"); got != emptyTree {
		t.Fatalf("empty archive tree = %s, want %s", got, emptyTree)
	}
	runRevisionGit(t, isolated, indexEnvironment, "apply", "--cached", "--binary", "--whitespace=error-all", archivePath)
	if got := revisionGitOutput(t, isolated, indexEnvironment, "write-tree"); got != reconstructedTree {
		t.Fatalf("restored harness tree = %s, want %s", got, reconstructedTree)
	}
	wantStatus := strings.Join([]string{
		"A\teventloop/internal/promisetournament/promise_tournament_test.go",
		"A\teventloop/internal/tournament/bench_multiproducer_test.go",
		"A\teventloop/internal/tournament/micro_batch_test.go",
		"A\teventloop/internal/tournament/micro_cas_test.go",
		"A\teventloop/internal/tournament/promise_corrected_bench_test.go",
	}, "\n")
	if got := revisionGitOutput(t, isolated, indexEnvironment, "diff-tree", "--no-commit-id", "--name-status", "-r", emptyTree, reconstructedTree); got != wantStatus {
		t.Fatalf("restored harness paths:\n%s\nwant:\n%s", got, wantStatus)
	}
	for _, target := range targets {
		blob := revisionGitOutput(t, isolated, indexEnvironment, "rev-parse", reconstructedTree+":"+target.path)
		if blob != target.blob {
			t.Errorf("restored harness %s blob = %s, want %s", target.path, blob, target.blob)
		}
		bytesValue, err := strconv.ParseInt(revisionGitOutput(t, isolated, indexEnvironment, "cat-file", "-s", blob), 10, 64)
		if err != nil || bytesValue != target.bytes {
			t.Errorf("restored harness %s bytes = (%d, %v), want %d", target.path, bytesValue, err, target.bytes)
		}
		payload := runIndexArchiveGitOutput(t, isolated, indexEnvironment, "cat-file", "blob", blob)
		if got := fmt.Sprintf("%x", sha256.Sum256(payload)); got != target.sha256 {
			t.Errorf("restored harness %s SHA-256 = %s, want %s", target.path, got, target.sha256)
		}
	}
	runRevisionGit(t, isolated, indexEnvironment, "fsck", "--strict", "--no-reflogs")
}
