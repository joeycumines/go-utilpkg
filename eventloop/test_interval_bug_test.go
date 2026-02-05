// Copyright 2026 Joseph Cumines
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that this copyright
// notice appears in all copies.

package eventloop

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// TestSetIntervalDoneChannelBug verifies that the done channel is properly initialized
// and tracked across multiple interval executions. This test catches the bug where
// done was closed on first execution, breaking ClearInterval synchronization on
// subsequent executions.
func TestSetIntervalDoneChannelBug(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	errChan := make(chan error, 1)
	go func() {
		if err := loop.Run(ctx); err != nil {
			errChan <- err
		}
	}()

	time.Sleep(10 * time.Millisecond)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	var counter atomic.Int32
	var intervalID atomic.Uint64

	setIntervalClosure := func(n int32) {
		fmt.Printf("Interval fired: %d\n", n)

		// Clear on 5th fire
		if n >= 5 {
			if clearErr := js.ClearInterval(intervalID.Load()); clearErr != nil {
				t.Errorf("ClearInterval failed: %v", clearErr)
			}
			cancel()
		}
	}

	id, err := js.SetInterval(func() {
		n := counter.Add(1)
		setIntervalClosure(n)
	}, 10)

	if err != nil {
		t.Fatalf("SetInterval failed: %v", err)
	}

	intervalID.Store(id)

	// Wait for completion or timeout
	select {
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Test timed out")
	case err := <-errChan:
		if err != context.Canceled {
			t.Fatalf("Loop error: %v", err)
		}
	}

	// Verify we got all 5 fires before clear
	fireCount := counter.Load()
	if fireCount < 5 {
		t.Errorf("Expected at least 5 interval fires, got %d", fireCount)
	}
	t.Logf("Test completed successfully with %d interval fires", fireCount)
}
