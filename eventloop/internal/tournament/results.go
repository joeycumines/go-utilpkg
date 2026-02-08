//go:build linux || darwin

package tournament

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TestResult represents the result of a single tournament test.
type TestResult struct { // betteralign:ignore
	TestName       string                 `json:"test_name"`
	Implementation string                 `json:"implementation"`
	Passed         bool                   `json:"passed"`
	Error          string                 `json:"error,omitempty"`
	Duration       time.Duration          `json:"duration_ns"`
	Metrics        map[string]interface{} `json:"metrics,omitempty"`
	Timestamp      time.Time              `json:"timestamp"`
}

// BenchmarkResult represents the result of a benchmark test.
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
	EndTime       time.Time         `json:"end_time,omitempty"`
	TestResults   []TestResult      `json:"test_results"`
	BenchmarkData []BenchmarkResult `json:"benchmark_results"`
	Summary       TournamentSummary `json:"summary"`
	Incompatibles []Incompatibility `json:"incompatibilities,omitempty"`
}

// TournamentSummary provides a high-level summary of results.
type TournamentSummary struct { // betteralign:ignore
	TotalTests    int               `json:"total_tests"`
	PassedByImpl  map[string]int    `json:"passed_by_implementation"`
	FailedByImpl  map[string]int    `json:"failed_by_implementation"`
	FastestByTest map[string]string `json:"fastest_by_test"`
	Winner        string            `json:"winner"`
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
			PassedByImpl:  make(map[string]int),
			FailedByImpl:  make(map[string]int),
			FastestByTest: make(map[string]string),
		},
	}
}

// RecordTest records a test result.
func (r *TournamentResults) RecordTest(result TestResult) {
	r.mu.Lock()
	defer r.mu.Unlock()

	result.Timestamp = time.Now()
	r.TestResults = append(r.TestResults, result)
	r.Summary.TotalTests++

	if result.Passed {
		r.Summary.PassedByImpl[result.Implementation]++
	} else {
		r.Summary.FailedByImpl[result.Implementation]++
	}
}

// RecordBenchmark records a benchmark result.
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

	// Determine winner by most tests passed
	maxPassed := 0
	for impl, passed := range r.Summary.PassedByImpl {
		if passed > maxPassed {
			maxPassed = passed
			r.Summary.Winner = impl
		}
	}
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
