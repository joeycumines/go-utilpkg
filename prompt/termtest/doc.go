// Package termtest provides testing utilities for terminal-based applications,
// particularly those built with the go-prompt library. It enables deterministic
// testing of CLI interfaces by simulating user interactions and validating output
// through a pseudo-terminal (PTY) abstraction.
//
// The package offers two primary testing modes:
//
//  1. Console: For testing external processes executed in a PTY environment.
//     Use NewConsole to start a command and interact with it programmatically.
//
//  2. Harness: For in-process testing of go-prompt instances. Use NewHarness
//     to create a test harness that runs a prompt within the same process.
//
// Key features include:
//
// - Synchronous and asynchronous input simulation with key sequence mapping
// - Output validation using flexible condition-based assertions
// - PTY management with platform-specific optimizations (Darwin/Linux)
// - Synchronization primitives for reliable test execution
// - ANSI escape sequence normalization for robust output matching
//
// Basic usage for external process testing:
//
//	console, err := termtest.NewConsole(ctx, termtest.WithCommand("myapp"))
//	if err != nil {
//	    t.Fatal(err)
//	}
//	defer console.Close()
//
//	snap := console.Snapshot()
//	console.SendLine("input")
//	err = console.Expect(ctx, snap, termtest.Contains("expected output"), "output check")
//
// Basic usage for in-process prompt testing:
//
//	harness, err := termtest.NewHarness(ctx)
//	if err != nil {
//	    t.Fatal(err)
//	}
//	defer harness.Close()
//
//	go harness.RunPrompt(func(cmd string) { /* handle command */ })
//	console := harness.Console()
//	// ... interact with console
//
// The package is designed to be thread-safe and provides comprehensive error
// handling for PTY operations, making it suitable for integration and end-to-end
// testing of terminal applications.
package termtest
