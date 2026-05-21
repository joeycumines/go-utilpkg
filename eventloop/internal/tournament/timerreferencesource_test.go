package tournament

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
