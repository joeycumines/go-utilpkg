package eventloop

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestPriorityLane_InternalBypassesBudget verifies that internal priority lane
// tasks execute before external tasks, bypassing the tick budget.
func TestPriorityLane_InternalBypassesBudget(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var executionOrder []string
	var orderMu sync.Mutex
	record := func(name string) {
		orderMu.Lock()
		executionOrder = append(executionOrder, name)
		orderMu.Unlock()
	}

	// Submit 2000 external tasks first
	for i := 0; i < 2000; i++ {
		idx := i
		loop.Submit(Task{Runnable: func() {
			record(fmt.Sprintf("external-%d", idx))
		}})
	}

	// Submit internal priority task
	// NOTE: SubmitInternal is expected to exist as part of priority lane implementation.
	// If this doesn't compile, the priority lane feature is not implemented.
	loop.SubmitInternal(Task{Runnable: func() {
		record("internal-priority")
	}})

	// Run in goroutine since Run() is blocking
	runDone := make(chan struct{})
	errChan := make(chan error, 1)
	go func() {
		if err := loop.Run(ctx); err != nil && err != context.Canceled {
			errChan <- err
			return
		}
		close(runDone)
	}()

	time.Sleep(500 * time.Millisecond)

	// Check for errors
	select {
	case err := <-errChan:
		t.Fatal(err)
	default:
	}

	orderMu.Lock()
	defer orderMu.Unlock()

	internalPos := -1
	for i, name := range executionOrder {
		if name == "internal-priority" {
			internalPos = i
			break
		}
	}

	if internalPos == -1 {
		t.Fatal("Internal task never executed")
	}

	if internalPos > 1024 {
		t.Fatalf("PRIORITY VIOLATION: Internal task at position %d (should be < 1024)", internalPos)
	}
}
