package tournament

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTimerReferenceSourcesAuthenticate(t *testing.T) {
	repository, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, descriptor := range timerReferenceDescriptors() {
		for _, source := range append(append([]componentSourceIdentity(nil), descriptor.Sources...), descriptor.MaterializationSources...) {
			authenticateTimerReferenceSource(t, repository, descriptor.ID, source)
		}
	}
	for _, descriptor := range timerReferenceStrategyDescriptors() {
		for _, source := range append(append([]componentSourceIdentity(nil), descriptor.Sources...), descriptor.MaterializationSources...) {
			authenticateTimerReferenceSource(t, repository, descriptor.ID, source)
		}
	}
	for _, binding := range timerReferenceStorageBindings {
		if binding.Disposition == timerReferenceBindingNormalizedAlias {
			authenticateTimerReferenceSource(t, repository, binding.StorageID, binding.NormalizedSource)
		}
	}
}

func authenticateTimerReferenceSource(t *testing.T, repository, owner string, source componentSourceIdentity) {
	t.Helper()
	name := strings.Join([]string{owner, source.ProvenanceKind, source.OriginBlob, strings.ReplaceAll(source.Path, "/", "_")}, "/")
	t.Run(name, func(t *testing.T) {
		var payload []byte
		var blob string
		switch source.ProvenanceKind {
		case "commit":
			if source.OriginCommit == "" || source.BaseRevision != "" {
				t.Fatalf("commit source has invalid authority fields: %+v", source)
			}
			object := source.OriginCommit + ":eventloop/" + source.Path
			payload = runComponentGit(t, repository, "show", object)
			blob = strings.TrimSpace(string(runComponentGit(t, repository, "rev-parse", object)))
		case "archived-index-candidate", "index-candidate-materialization":
			if source.OriginCommit != "" || source.BaseRevision != timerCandidateArchive().BaseRevision {
				t.Fatalf("candidate source has invalid authority fields: %+v", source)
			}
			object := ":eventloop/" + source.Path
			payload = runComponentGit(t, repository, "show", object)
			blob = strings.TrimSpace(string(runComponentGit(t, repository, "rev-parse", object)))
			worktree, readErr := os.ReadFile(filepath.Join(repository, "eventloop", filepath.FromSlash(source.Path)))
			if readErr != nil {
				t.Fatal(readErr)
			}
			if !bytes.Equal(worktree, payload) {
				t.Fatal("candidate source has unstaged worktree drift")
			}
		default:
			t.Fatalf("unsupported reference provenance kind %q", source.ProvenanceKind)
		}
		if blob != source.OriginBlob {
			t.Errorf("blob = %s, want %s", blob, source.OriginBlob)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(payload)); got != source.SHA256 {
			t.Errorf("SHA-256 = %s, want %s", got, source.SHA256)
		}
	})
}

func TestTimerReferenceMaterializationsKeepTimedBoundary(t *testing.T) {
	for _, descriptor := range timerReferenceDescriptors() {
		if len(descriptor.MaterializationSources) != 1 {
			t.Fatalf("descriptor %q materialization count = %d, want 1", descriptor.ID, len(descriptor.MaterializationSources))
		}
		path := filepath.Join("component", strings.TrimPrefix(descriptor.MaterializationSources[0].Path, "internal/tournament/component/"))
		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		source := string(payload)
		for _, required := range []string{"func (c *Core) Apply(id ID, refed bool)", "c.entries[id]", "value.refed.Swap(refed)", "c.refed.Add(1)", "c.refed.Add(-1)"} {
			if !strings.Contains(source, required) {
				t.Errorf("materialization %q missing source boundary %q", descriptor.ID, required)
			}
		}
		for _, forbidden := range []string{"submissionEpoch", "doWakeup", "reflect.", "interface{", "func (c *Core) Apply[T", "switch refed"} {
			if strings.Contains(source, forbidden) {
				t.Errorf("materialization %q contains forbidden timed-boundary token %q", descriptor.ID, forbidden)
			}
		}
	}
}
