// Copyright 2025 Joseph Cumines
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that this copyright
// notice appears in all copies.

package eventloop

import (
	"context"
	"testing"
)

// Test 1.3.5: Test NewJS basic creation
func TestNewJSBasicCreation(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}
	if js == nil {
		t.Fatalf("NewJS() returned nil, expected non-nil *JS")
	}

	// Verify loop reference is correct
	if js.Loop() != loop {
		t.Error("JS.Loop() does not return the original loop")
	}
}

// Test: NewJS with multiple instances
func TestNewJSMultipleInstances(t *testing.T) {
	loop1, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop1.Shutdown(context.Background())

	loop2, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop2.Shutdown(context.Background())

	js1, err := NewJS(loop1)
	if err != nil {
		t.Fatalf("NewJS() failed for js1: %v", err)
	}
	js2, err := NewJS(loop2)
	if err != nil {
		t.Fatalf("NewJS() failed for js2: %v", err)
	}

	if js1.Loop() != loop1 {
		t.Error("js1 loop reference incorrect")
	}
	if js2.Loop() != loop2 {
		t.Error("js2 loop reference incorrect")
	}
	if js1.Loop() == js2.Loop() {
		t.Error("Different JS instances should have different loop references")
	}
}

// Test: NewJS with existing loop options
func TestNewJSWithLoopOptions(t *testing.T) {
	// Create loop with strict microtask ordering
	loop, err := New(WithStrictMicrotaskOrdering(true))
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	if !loop.StrictMicrotaskOrdering {
		t.Error("Loop should have StrictMicrotaskOrdering=true")
	}
	if js.Loop() != loop {
		t.Error("JS loop reference incorrect")
	}
}
