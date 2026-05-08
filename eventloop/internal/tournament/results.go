package tournament

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"sync"
	"time"
)

// TestResult represents the result of a single tournament test.
type TestResult struct { // betteralign:ignore
	TestName       string         `json:"test_name"`
	VariantID      string         `json:"variant_id"`
	Implementation string         `json:"implementation"`
	Status         TestStatus     `json:"status"`
	Passed         bool           `json:"passed"`
	Error          string         `json:"error,omitzero"`
	Duration       time.Duration  `json:"duration_ns"`
	Metrics        map[string]any `json:"metrics,omitzero"`
	Timestamp      time.Time      `json:"timestamp"`
}

type TestStatus string

const (
	TestStatusPassed      TestStatus = "passed"
	TestStatusFailed      TestStatus = "failed"
	TestStatusUnsupported TestStatus = "unsupported"
	TestStatusDiagnostic  TestStatus = "diagnostic"
)

// BenchmarkResult represents a legacy in-process benchmark observation.
//
// New tournament benchmarks must use the raw records emitted by testing.B.
// Calls made inside a benchmark include calibration invocations and cannot be
// identified as the final sample, so this type is retained only for decoding
// and inspecting historical tournament JSON.
type BenchmarkResult struct { // betteralign:ignore
	BenchmarkName  string        `json:"benchmark_name"`
	Implementation string        `json:"implementation"`
	NsPerOp        float64       `json:"ns_per_op"`
	AllocsPerOp    int64         `json:"allocs_per_op"`
	BytesPerOp     int64         `json:"bytes_per_op"`
	Iterations     int           `json:"iterations"`
	Duration       time.Duration `json:"duration_ns"`
	Timestamp      time.Time     `json:"timestamp"`
}

// TournamentResults aggregates all test results.
type TournamentResults struct { // betteralign:ignore
	mu sync.Mutex

	RunID         string            `json:"run_id"`
	StartTime     time.Time         `json:"start_time"`
	EndTime       time.Time         `json:"end_time"`
	TestResults   []TestResult      `json:"test_results"`
	BenchmarkData []BenchmarkResult `json:"benchmark_results"`
	Summary       TournamentSummary `json:"summary"`
	Incompatibles []Incompatibility `json:"incompatibilities,omitzero"`
}

// TournamentSummary provides a high-level summary of results.
type TournamentSummary struct { // betteralign:ignore
	TotalTests        int               `json:"total_tests"`
	PassedByImpl      map[string]int    `json:"passed_by_implementation"`
	FailedByImpl      map[string]int    `json:"failed_by_implementation"`
	UnsupportedByImpl map[string]int    `json:"unsupported_by_implementation"`
	DiagnosticByImpl  map[string]int    `json:"diagnostic_by_implementation"`
	FastestByTest     map[string]string `json:"fastest_by_test"`
	Winner            string            `json:"winner"`
	Winners           []string          `json:"winners,omitzero"`
	WinnerComparable  bool              `json:"winner_comparable"`
}

// Incompatibility records an API incompatibility discovered during testing.
type Incompatibility struct {
	Implementation string `json:"implementation"`
	Feature        string `json:"feature"`
	Description    string `json:"description"`
}

// NewTournamentResults creates a new results container.
func NewTournamentResults() *TournamentResults {
	return &TournamentResults{
		RunID:     fmt.Sprintf("run-%d", time.Now().UnixNano()),
		StartTime: time.Now(),
		Summary: TournamentSummary{
			PassedByImpl:      make(map[string]int),
			FailedByImpl:      make(map[string]int),
			UnsupportedByImpl: make(map[string]int),
			DiagnosticByImpl:  make(map[string]int),
			FastestByTest:     make(map[string]string),
		},
	}
}

// RecordTest records a test result.
func (r *TournamentResults) RecordTest(result TestResult) {
	r.mu.Lock()
	defer r.mu.Unlock()

	result.Timestamp = time.Now()
	if result.VariantID == "" {
		result.VariantID = result.Implementation
	}
	if result.Status == "" {
		if result.Passed {
			result.Status = TestStatusPassed
		} else {
			result.Status = TestStatusFailed
		}
	}
	result.Passed = result.Status == TestStatusPassed
	r.TestResults = append(r.TestResults, result)
	r.Summary.TotalTests++

	switch result.Status {
	case TestStatusPassed:
		r.Summary.PassedByImpl[result.VariantID]++
	case TestStatusFailed:
		r.Summary.FailedByImpl[result.VariantID]++
	case TestStatusUnsupported:
		r.Summary.UnsupportedByImpl[result.VariantID]++
	case TestStatusDiagnostic:
		r.Summary.DiagnosticByImpl[result.VariantID]++
	default:
		panic(fmt.Sprintf("invalid tournament test status %q", result.Status))
	}
}

// RecordBenchmark records a legacy diagnostic benchmark result.
//
// Tournament benchmarks do not call this method. Performance authority is the
// raw Go benchmark output consumed by benchstat and parse_benchmarks.py.
func (r *TournamentResults) RecordBenchmark(result BenchmarkResult) {
	r.mu.Lock()
	defer r.mu.Unlock()

	result.Timestamp = time.Now()
	r.BenchmarkData = append(r.BenchmarkData, result)

	// Track fastest implementation per benchmark
	key := result.BenchmarkName
	if current, exists := r.Summary.FastestByTest[key]; !exists || result.NsPerOp < r.findBenchmarkNsPerOp(key, current) {
		r.Summary.FastestByTest[key] = result.Implementation
	}
}

// findBenchmarkNsPerOp finds the ns/op for a given benchmark and implementation.
func (r *TournamentResults) findBenchmarkNsPerOp(benchName, implName string) float64 {
	for _, b := range r.BenchmarkData {
		if b.BenchmarkName == benchName && b.Implementation == implName {
			return b.NsPerOp
		}
	}
	return float64(^uint64(0) >> 1) // Max float64
}

// RecordIncompatibility records an API incompatibility.
func (r *TournamentResults) RecordIncompatibility(incomp Incompatibility) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.Incompatibles = append(r.Incompatibles, incomp)
}

// Finalize completes the results and determines the winner.
func (r *TournamentResults) Finalize() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.EndTime = time.Now()

	// A winner is meaningful only when every variant was evaluated against the
	// same contract names. Unsupported cells make the matrix non-comparable.
	evaluated := make(map[string]map[string]struct{})
	for _, result := range r.TestResults {
		if evaluated[result.VariantID] == nil {
			evaluated[result.VariantID] = make(map[string]struct{})
		}
		if result.Status == TestStatusUnsupported || result.Status == TestStatusDiagnostic {
			continue
		}
		evaluated[result.VariantID][result.TestName] = struct{}{}
	}
	r.Summary.WinnerComparable = comparableTestMatrix(evaluated)
	maxPassed := 0
	r.Summary.Winner = ""
	r.Summary.Winners = r.Summary.Winners[:0]
	if !r.Summary.WinnerComparable {
		return
	}
	for impl, passed := range r.Summary.PassedByImpl {
		if passed > maxPassed {
			maxPassed = passed
			r.Summary.Winners = append(r.Summary.Winners[:0], impl)
		} else if passed == maxPassed {
			r.Summary.Winners = append(r.Summary.Winners, impl)
		}
	}
	sort.Strings(r.Summary.Winners)
	if len(r.Summary.Winners) == 1 {
		r.Summary.Winner = r.Summary.Winners[0]
	}
}

func comparableTestMatrix(evaluated map[string]map[string]struct{}) bool {
	var reference []string
	first := true
	for _, tests := range evaluated {
		current := sortedContractSet(tests)
		if first {
			reference = current
			first = false
			continue
		}
		if !slices.Equal(reference, current) {
			return false
		}
	}
	return !first
}

func sortedContractSet(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

// SaveJSON saves the results to a JSON file.
func (r *TournamentResults) SaveJSON(dir string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	filename := filepath.Join(dir, fmt.Sprintf("tournament_%s.json", r.RunID))
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return "", err
	}

	return filename, nil
}

// Global results instance for the tournament run.
var globalResults = NewTournamentResults()

// GetResults returns the global results instance.
func GetResults() *TournamentResults {
	return globalResults
}
