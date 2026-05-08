package inprocgrpc

import (
	"runtime"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRPCStatsRunnerQuarantinesPanic(t *testing.T) {
	runner := newRPCStatsRunner()
	err := runner.execute("stats panic", func() { panic("boom") })
	if status.Code(err) != codes.Internal {
		t.Fatalf("panic error = %v", err)
	}
	called := false
	if err := runner.execute("stats after panic", func() {
		called = true
	}); err != nil {
		t.Fatalf("quarantined execute = %v", err)
	}
	if called {
		t.Fatal("quarantined stats callback executed")
	}
	ended := false
	end, ok := runner.prepareFinal("stats End", func() { ended = true })
	if !ok {
		t.Fatal("End was not prepared after quarantine")
	}
	if err := end.execute(); err != nil {
		t.Fatalf("End after quarantine = %v", err)
	}
	if !ended {
		t.Fatal("End after quarantine did not execute")
	}
}

func TestRPCStatsRunnerContainsGoexit(t *testing.T) {
	runner := newRPCStatsRunner()
	err := runner.execute("stats Goexit", runtime.Goexit)
	if status.Code(err) != codes.Internal {
		t.Fatalf("Goexit error = %v", err)
	}
}

func TestRPCStatsRunnerQuarantineStopsAfterReservedFollower(t *testing.T) {
	runner := newRPCStatsRunner()
	failure, ok := runner.prepare("stats panic", func() { panic("boom") })
	if !ok {
		t.Fatal("failure was not prepared")
	}
	called := false
	follower, ok := runner.prepare("stats follower", func() { called = true })
	if !ok {
		t.Fatal("follower was not prepared")
	}
	if err := failure.execute(); status.Code(err) != codes.Internal {
		t.Fatalf("failure = %v", err)
	}
	if err := follower.execute(); err != nil {
		t.Fatalf("follower = %v", err)
	}
	if called {
		t.Fatal("quarantined follower executed")
	}
	ended, ok := runner.prepareFinal("stats End", func() {})
	if !ok {
		t.Fatal("End was not prepared after quarantine")
	}
	if err := ended.execute(); err != nil {
		t.Fatalf("End = %v", err)
	}
	<-runner.stopped
}

func TestRPCStatsRunnerAllowsIndependentCallbacks(t *testing.T) {
	runner := newRPCStatsRunner()
	entered := make(chan struct{})
	release := make(chan struct{})
	first, ok := runner.prepare("first", func() {
		close(entered)
		<-release
	})
	if !ok {
		t.Fatal("first stats call was not prepared")
	}
	firstDone := make(chan error, 1)
	go func() { firstDone <- first.execute() }()
	<-entered
	second, ok := runner.prepare("second", func() {})
	if !ok {
		t.Fatal("second stats call was not prepared")
	}
	if err := second.execute(); err != nil {
		t.Fatalf("second execute = %v", err)
	}
	select {
	case err := <-firstDone:
		t.Fatalf("first returned before release: %v", err)
	default:
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first execute = %v", err)
	}
}

func TestRPCStatsRunnerMandatoryBeginRunsAfterQuarantine(t *testing.T) {
	runner := newRPCStatsRunner()
	if err := runner.execute(
		"stats InHeader",
		func() { panic("boom") },
	); status.Code(err) != codes.Internal {
		t.Fatalf("InHeader error = %v, want Internal", err)
	}
	beginCalls := 0
	if err := runner.executeMandatory("stats Begin", func() {
		beginCalls++
	}); err != nil {
		t.Fatalf("Begin = %v", err)
	}
	if beginCalls != 1 {
		t.Fatalf("Begin calls = %d, want 1", beginCalls)
	}
	ordinaryCalls := 0
	if err := runner.execute("stats InPayload", func() {
		ordinaryCalls++
	}); err != nil {
		t.Fatalf("InPayload = %v", err)
	}
	if ordinaryCalls != 0 {
		t.Fatalf("InPayload calls = %d, want 0", ordinaryCalls)
	}
	endCalls := 0
	end, ok := runner.prepareFinal("stats End", func() {
		endCalls++
	})
	if !ok {
		t.Fatal("End was not prepared")
	}
	if err := end.execute(); err != nil {
		t.Fatalf("End = %v", err)
	}
	if endCalls != 1 {
		t.Fatalf("End calls = %d, want 1", endCalls)
	}
}
