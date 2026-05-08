package tournament

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestTimerIndexCandidateReconstructsExactAuthority(t *testing.T) {
	descriptor := timerDescriptor(timerNativeBucketCurrent)
	wantArchive := timerSourceArchive{
		PatchPath:                  "revisions/candidates/0001-index-on-469fd952-timer-bucket-phase-v2.patch",
		PatchSHA256:                "2d6ae645435d945de260e4cfa4bf0dc74312aee033e781f227514924b1b50c2b",
		PatchBytes:                 233206,
		BaseRevision:               "469fd952ed251edc7ea1d2bb0faf4e04fc94dd88",
		BaseTree:                   "def5cb294735fd57b36e7075084ba88991916421",
		BaseEventloopTree:          "227ee042f259cdb35c3e33a6dbecb2b8ec746a21",
		ReconstructedTree:          "16b35ccc1445cb8784f95cbb8be48eed54a38e04",
		ReconstructedEventloopTree: "2d320d39718705c5c63ac940b567863ba35d2ba3",
		UnchangedGojaTree:          "69d8cf81666942396704d3d4bdb75208a0e523c6",
	}
	if !reflect.DeepEqual(descriptor.SourceArchive, wantArchive) {
		t.Fatalf("T9 source archive = %+v, want %+v", descriptor.SourceArchive, wantArchive)
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
	patchPath, err := filepath.Abs(wantArchive.PatchPath)
	if err != nil {
		t.Fatal(err)
	}
	patch, err := os.ReadFile(patchPath)
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(patch)) != wantArchive.PatchBytes {
		t.Fatalf("T9 archive bytes = %d, want %d", len(patch), wantArchive.PatchBytes)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(patch)); got != wantArchive.PatchSHA256 {
		t.Fatalf("T9 archive SHA-256 = %s, want %s", got, wantArchive.PatchSHA256)
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
	if got := revisionGitOutput(t, repository, environment, "rev-parse", wantArchive.BaseRevision+"^{commit}"); got != wantArchive.BaseRevision {
		t.Fatalf("T9 base revision = %s, want %s", got, wantArchive.BaseRevision)
	}
	if got := revisionGitOutput(t, repository, environment, "rev-parse", wantArchive.BaseRevision+"^{tree}"); got != wantArchive.BaseTree {
		t.Fatalf("T9 base tree = %s, want %s", got, wantArchive.BaseTree)
	}
	if got := revisionGitOutput(t, repository, environment, "rev-parse", wantArchive.BaseTree+":eventloop"); got != wantArchive.BaseEventloopTree {
		t.Fatalf("T9 base eventloop tree = %s, want %s", got, wantArchive.BaseEventloopTree)
	}
	if got := revisionGitOutput(t, repository, environment, "rev-parse", wantArchive.BaseTree+":goja-eventloop"); got != wantArchive.UnchangedGojaTree {
		t.Fatalf("T9 base goja tree = %s, want %s", got, wantArchive.UnchangedGojaTree)
	}

	isolated := filepath.Join(temporary, "isolated.git")
	runIndexArchiveGit(t, environment, "init", "--bare", "--quiet", isolated)
	seedIndexArchiveBase(t, repository, isolated, environment, wantArchive.BaseTree)
	if _, err := os.Stat(filepath.Join(isolated, "objects", "info", "alternates")); !os.IsNotExist(err) {
		t.Fatalf("isolated T9 repository has alternates: %v", err)
	}
	for _, source := range descriptor.Sources {
		command := exec.Command("git", "-C", isolated, "cat-file", "-e", source.OriginBlob)
		command.Env = environment
		if err := command.Run(); err == nil {
			t.Fatalf("target blob %s existed before applying the archive", source.OriginBlob)
		} else if _, ok := err.(*exec.ExitError); !ok {
			t.Fatalf("probe target blob %s: %v", source.OriginBlob, err)
		}
	}

	indexEnvironment := append(environment, "GIT_INDEX_FILE="+filepath.Join(temporary, "index"))
	runRevisionGit(t, isolated, indexEnvironment, "read-tree", wantArchive.BaseTree)
	runRevisionGit(t, isolated, indexEnvironment, "apply", "--cached", "--binary", "--whitespace=error-all", patchPath)
	if got := revisionGitOutput(t, isolated, indexEnvironment, "write-tree"); got != wantArchive.ReconstructedTree {
		t.Fatalf("T9 reconstructed tree = %s, want %s", got, wantArchive.ReconstructedTree)
	}
	if got := revisionGitOutput(t, isolated, indexEnvironment, "rev-parse", wantArchive.ReconstructedTree+":eventloop"); got != wantArchive.ReconstructedEventloopTree {
		t.Fatalf("T9 reconstructed eventloop tree = %s, want %s", got, wantArchive.ReconstructedEventloopTree)
	}
	if got := revisionGitOutput(t, isolated, indexEnvironment, "rev-parse", wantArchive.ReconstructedTree+":goja-eventloop"); got != wantArchive.UnchangedGojaTree {
		t.Fatalf("T9 reconstructed goja tree = %s, want %s", got, wantArchive.UnchangedGojaTree)
	}
	wantStatus := strings.Join([]string{
		"M\teventloop/loop.go",
		"A\teventloop/scheduler.go",
		"A\teventloop/timer.go",
		"A\teventloop/timercancel.go",
		"A\teventloop/timerid.go",
	}, "\n")
	if got := revisionGitOutput(t, isolated, indexEnvironment, "diff-tree", "--no-commit-id", "--name-status", "-r", wantArchive.BaseTree, wantArchive.ReconstructedTree); got != wantStatus {
		t.Fatalf("T9 reconstructed paths:\n%s\nwant:\n%s", got, wantStatus)
	}

	wantBytes := map[string]int64{
		"loop.go": 14616, "scheduler.go": 18309, "timer.go": 12310,
		"timercancel.go": 13715, "timerid.go": 435,
	}
	for _, source := range descriptor.Sources {
		object := wantArchive.ReconstructedTree + ":eventloop/" + source.Path
		blob := revisionGitOutput(t, isolated, indexEnvironment, "rev-parse", object)
		if blob != source.OriginBlob {
			t.Errorf("T9 %s blob = %s, want %s", source.Path, blob, source.OriginBlob)
		}
		entry := strings.Fields(revisionGitOutput(t, isolated, indexEnvironment, "ls-tree", wantArchive.ReconstructedTree, "--", "eventloop/"+source.Path))
		if len(entry) < 3 || entry[0] != "100644" || entry[1] != "blob" || entry[2] != source.OriginBlob {
			t.Errorf("T9 %s tree entry = %v", source.Path, entry)
		}
		bytesValue, err := strconv.ParseInt(revisionGitOutput(t, isolated, indexEnvironment, "cat-file", "-s", blob), 10, 64)
		if err != nil || bytesValue != wantBytes[source.Path] {
			t.Errorf("T9 %s bytes = (%d, %v), want %d", source.Path, bytesValue, err, wantBytes[source.Path])
		}
		payload := runIndexArchiveGitOutput(t, isolated, indexEnvironment, "cat-file", "blob", blob)
		if got := fmt.Sprintf("%x", sha256.Sum256(payload)); got != source.SHA256 {
			t.Errorf("T9 %s SHA-256 = %s, want %s", source.Path, got, source.SHA256)
		}
	}
	runRevisionGit(t, isolated, indexEnvironment, "fsck", "--strict", "--no-reflogs")
}

func seedIndexArchiveBase(t *testing.T, source, destination string, environment []string, tree string) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	producer := exec.Command("git", "-C", source, "pack-objects", "--revs", "--stdout")
	producer.Env = environment
	producer.Stdin = strings.NewReader(tree + "\n")
	producer.Stdout = writer
	consumer := exec.Command("git", "-C", destination, "index-pack", "--stdin")
	consumer.Env = environment
	consumer.Stdin = reader
	var producerError, consumerError bytes.Buffer
	producer.Stderr = &producerError
	consumer.Stderr = &consumerError
	if err := consumer.Start(); err != nil {
		reader.Close()
		writer.Close()
		t.Fatal(err)
	}
	if err := producer.Start(); err != nil {
		reader.Close()
		writer.Close()
		consumer.Process.Kill()
		consumer.Wait()
		t.Fatal(err)
	}
	reader.Close()
	writer.Close()
	producerWait := producer.Wait()
	consumerWait := consumer.Wait()
	if producerWait != nil || consumerWait != nil {
		t.Fatalf("seed isolated archive objects = (%v, %v)\nproducer: %s\nconsumer: %s", producerWait, consumerWait, producerError.String(), consumerError.String())
	}
}

func runIndexArchiveGit(t *testing.T, environment []string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Env = environment
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", arguments, err, output)
	}
}

func runIndexArchiveGitOutput(t *testing.T, repository string, environment []string, arguments ...string) []byte {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", repository}, arguments...)...)
	command.Env = environment
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git %v: %v", arguments, err)
	}
	return output
}
