//go:build (aix && ppc64) || (solaris && amd64)

package eventloop

import "testing"

func TestPollBackendSelected(t *testing.T) {
	if !pollBackendSupported || !fdPollingSupported {
		t.Fatal("poll target selected a task-only backend")
	}
}
