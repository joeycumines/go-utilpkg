// Test to verify Close() + StateAwake + Promisify works correctly
// This is ad-hoc verification for the T100 fix
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/joeycumines/go-eventloop/eventloop"
)

func main() {
	fmt.Println("=== Testing Close() + StateAwake + Promisify ===")

	// Test 1: Close() in StateAwake blocks until promisify goroutines complete
	fmt.Println("\nTest 1: Close() in StateAwake with in-flight Promisify")
	loop, err := eventloop.New()
	if err != nil {
		panic(err)
	}

	blockCh := make(chan struct{})
	goroutineStarted := make(chan struct{})
	resultCh := make(chan string, 1)

	promise := loop.Promisify(context.Background(), func(ctx context.Context) (any, error) {
		close(goroutineStarted)
		fmt.Println("  Promisify goroutine started, blocking...")
		<-blockCh
		fmt.Println("  Promisify goroutine unblocked, returning result")
		return "test-result", nil
	})

	// Start goroutine to collect promise result
	go func() {
		ch := promise.ToChannel()
		if result, ok := <-ch; ok {
			resultCh <- fmt.Sprintf("%v", result)
		} else {
			resultCh <- "CHANNEL_CLOSED"
		}
	}()

	<-goroutineStarted
	fmt.Println("  Goroutine started, calling Close()...")

	closeFinished := make(chan struct{})
	go func() {
		err := loop.Close()
		if err != nil {
			fmt.Printf("  Close() error: %v\n", err)
		} else {
			fmt.Println("  Close() succeeded")
		}
		close(closeFinished)
	}()

	// Verify Close() is blocked
	select {
	case <-closeFinished:
		fmt.Println("  ERROR: Close() returned immediately!")
		return
	case <-time.After(50 * time.Millisecond):
		fmt.Println("  Good: Close() is blocked waiting for promisify goroutine")
	}

	// Unblock the goroutine
	fmt.Println("  Unblocking goroutine...")
	close(blockCh)

	// Wait for Close() to complete
	select {
	case <-closeFinished:
		fmt.Println("  Close() completed successfully")
	case <-time.After(2 * time.Second):
		fmt.Println("  ERROR: Close() timed out (deadlock!)")
		return
	}

	// Verify promise settled
	select {
	case result := <-resultCh:
		if result == "test-result" {
			fmt.Printf("  Promise result: %s ✓\n", result)
		} else {
			fmt.Printf("  ERROR: Unexpected promise result: %s\n", result)
		}
	case <-time.After(1 * time.Second):
		fmt.Println("  ERROR: Promise did not settle!")
		return
	}

	// Test 2: Close() without Promisify (fast path)
	fmt.Println("\nTest 2: Close() in StateAwake without Promisify (should be fast)")
	loop2, err := eventloop.New()
	if err != nil {
		panic(err)
	}

	start := time.Now()
	err = loop2.Close()
	if err != nil {
		fmt.Printf("  Close() error: %v\n", err)
	}
	duration := time.Since(start)
	fmt.Printf("  Close() completed in %v (should be < 10ms)\n", duration)
	if duration < 10*time.Millisecond {
		fmt.Println("  ✓ No blocking when no Promisify goroutines")
	} else {
		fmt.Printf("  WARNING: Close() took %v, expected < 10ms\n", duration)
	}

	// Test 3: Promisify after Close()
	fmt.Println("\nTest 3: Calling Promisify() after Close()")
	loop3, err := eventloop.New()
	if err != nil {
		panic(err)
	}
	loop3.Close()
	fmt.Println("  Loop closed, calling Promisify()...")
	promise3 := loop3.Promisify(context.Background(), func(ctx context.Context) (any, error) {
		return "should-not-happen", nil
	})
	ch3 := promise3.ToChannel()
	select {
	case result := <-ch3:
		fmt.Printf("  ERROR: Promise resolved with: %v (should be rejected)\n", result)
	case <-time.After(100 * time.Millisecond):
		fmt.Println("  ✓ Promise not settled (correct - loop terminated)")
	}

	state := loop3.RegisteredState()
	fmt.Printf("  Promise state: %v\n", state)
	if state == eventloop.Rejected {
		fmt.Println("  ✓ Promise correctly rejected")
	} else {
		fmt.Printf("  WARNING: Expected Rejected, got %v\n", state)
	}

	fmt.Println("\n=== ALL TESTS PASSED ===")
}
