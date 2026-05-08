package tournament

import (
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	status := m.Run()
	if outputDir := os.Getenv("EVENTLOOP_TOURNAMENT_RESULTS_DIR"); outputDir != "" {
		results := GetResults()
		results.Finalize()
		if _, err := results.SaveJSON(outputDir); err != nil {
			fmt.Fprintf(os.Stderr, "save tournament correctness results: %v\n", err)
			if status == 0 {
				status = 1
			}
		}
	}
	os.Exit(status)
}
