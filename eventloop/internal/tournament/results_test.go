package tournament

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestTournamentResultsFinalizeTie(t *testing.T) {
	results := NewTournamentResults()
	results.RecordTest(TestResult{Implementation: "Zulu", Passed: true})
	results.RecordTest(TestResult{Implementation: "Alpha", Passed: true})
	results.Finalize()

	if !results.Summary.WinnerComparable {
		t.Error("equal contract matrix was marked incomparable")
	}
	if results.Summary.Winner != "" {
		t.Errorf("ambiguous Winner = %q, want empty", results.Summary.Winner)
	}
	if want := []string{"Alpha", "Zulu"}; !slices.Equal(results.Summary.Winners, want) {
		t.Errorf("Winners = %v, want %v", results.Summary.Winners, want)
	}
}

func TestTournamentResultsFinalizeUniqueWinner(t *testing.T) {
	results := NewTournamentResults()
	results.RecordTest(TestResult{Implementation: "Alpha", Passed: true})
	results.RecordTest(TestResult{Implementation: "Alpha", Passed: true})
	results.RecordTest(TestResult{Implementation: "Zulu", Passed: true})
	results.Finalize()

	if !results.Summary.WinnerComparable {
		t.Error("equal contract matrix was marked incomparable")
	}
	if results.Summary.Winner != "Alpha" {
		t.Errorf("Winner = %q, want Alpha", results.Summary.Winner)
	}
	if want := []string{"Alpha"}; !slices.Equal(results.Summary.Winners, want) {
		t.Errorf("Winners = %v, want %v", results.Summary.Winners, want)
	}

	path, err := results.SaveJSON(t.TempDir())
	if err != nil {
		t.Fatalf("SaveJSON: %v", err)
	}
	if filepath.Ext(path) != ".json" {
		t.Errorf("SaveJSON path = %q, want .json extension", path)
	}
}

func TestTournamentResultsFinalizeRefusesAsymmetricMatrix(t *testing.T) {
	results := NewTournamentResults()
	results.RecordTest(TestResult{TestName: "OnlyAlpha", VariantID: "Alpha", Passed: true})
	results.RecordTest(TestResult{TestName: "OnlyZulu", VariantID: "Zulu", Passed: true})
	results.RecordTest(TestResult{
		TestName:  "Diagnostic",
		VariantID: "Alpha",
		Status:    TestStatusDiagnostic,
	})
	results.Finalize()

	if results.Summary.WinnerComparable {
		t.Error("asymmetric contract matrix was marked comparable")
	}
	if results.Summary.Winner != "" || len(results.Summary.Winners) != 0 {
		t.Errorf("incomparable matrix published winner %q / %v", results.Summary.Winner, results.Summary.Winners)
	}
	if got := results.Summary.DiagnosticByImpl["Alpha"]; got != 1 {
		t.Errorf("Alpha diagnostics = %d, want 1", got)
	}
}
