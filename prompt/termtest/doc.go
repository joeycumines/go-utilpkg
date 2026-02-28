// Package termtest provides testing utilities for terminal-based applications,
// particularly those built with the go-prompt library.
//
// Two testing modes are offered:
//
//  1. [Console]: tests external processes in a PTY environment.
//  2. [Harness]: tests go-prompt instances in-process.
//
// External process testing:
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
// In-process prompt testing:
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
package termtest
