package eventloop

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// FORENSIC LATENCY ANALYSIS TEST
// =============================================================================
//
// This file investigates the ~11,000ns ping-pong latency vs expected ~500ns.
// Goal: Find EXACTLY where the 10,500ns of overhead is coming from.
//
// Key instrumentation points:
//   - t0: When Submit() is called from producer goroutine
//   - t1: When task function starts executing on loop goroutine
//   - t2: When task function completes on loop goroutine
//
// Latency breakdown:
//   - Queue latency (t1-t0): Time to push + wake + dispatch
//   - Execution latency (t2-t1): Time for task body to run
//
// Run: go test -v -run TestLatencyAnalysis -count=1 ./eventloop/

// latencyRecord holds timing data for a single task.
type latencyRecord struct {
	t0Submit    time.Time // When Submit() was called
	t1TaskStart time.Time // When task began executing
	t2TaskEnd   time.Time // When task finished executing

	queueLatency time.Duration // t1 - t0
	execLatency  time.Duration // t2 - t1
	totalLatency time.Duration // t2 - t0
}

// TestLatencyAnalysis_EndToEnd measures the complete path from Submit to execution.
func TestLatencyAnalysis_EndToEnd(t *testing.T) {
	const iterations = 100

	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start the loop
	loopDone := make(chan struct{})
	go func() {
		_ = loop.Run(ctx)
		close(loopDone)
	}()

	// Wait for loop to be running
	for i := 0; i < 100; i++ {
		if loop.State() == StateRunning || loop.State() == StateSleeping {
			break
		}
		time.Sleep(time.Millisecond)
	}

	records := make([]latencyRecord, iterations)

	for i := 0; i < iterations; i++ {
		var rec latencyRecord
		taskDone := make(chan struct{})

		// Capture t0 just before Submit
		rec.t0Submit = time.Now()

		err := loop.Submit(Task{Runnable: func() {
			rec.t1TaskStart = time.Now()
			// Tiny work to measure execution overhead
			runtime.Gosched() // Minimal syscall-like operation
			rec.t2TaskEnd = time.Now()
			close(taskDone)
		}})

		if err != nil {
			t.Fatalf("Submit failed at iteration %d: %v", i, err)
		}

		select {
		case <-taskDone:
		case <-time.After(5 * time.Second):
			t.Fatalf("Task %d timed out", i)
		}

		rec.queueLatency = rec.t1TaskStart.Sub(rec.t0Submit)
		rec.execLatency = rec.t2TaskEnd.Sub(rec.t1TaskStart)
		rec.totalLatency = rec.t2TaskEnd.Sub(rec.t0Submit)
		records[i] = rec
	}

	// Shutdown
	cancel()
	<-loopDone

	// Analyze results
	analyzeLatencyRecords(t, "EndToEnd", records)
}

// TestLatencyAnalysis_FastPath measures latency when fast path is enabled.
func TestLatencyAnalysis_FastPath(t *testing.T) {
	const iterations = 100

	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	loop.SetFastPathEnabled(true)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start the loop
	loopDone := make(chan struct{})
	go func() {
		_ = loop.Run(ctx)
		close(loopDone)
	}()

	// Wait for loop to be running
	for i := 0; i < 100; i++ {
		if loop.State() == StateRunning || loop.State() == StateSleeping {
			break
		}
		time.Sleep(time.Millisecond)
	}

	records := make([]latencyRecord, iterations)

	for i := 0; i < iterations; i++ {
		var rec latencyRecord
		taskDone := make(chan struct{})

		// Capture t0 just before Submit
		rec.t0Submit = time.Now()

		err := loop.Submit(Task{Runnable: func() {
			rec.t1TaskStart = time.Now()
			runtime.Gosched()
			rec.t2TaskEnd = time.Now()
			close(taskDone)
		}})

		if err != nil {
			t.Fatalf("Submit failed at iteration %d: %v", i, err)
		}

		select {
		case <-taskDone:
		case <-time.After(5 * time.Second):
			t.Fatalf("Task %d timed out", i)
		}

		rec.queueLatency = rec.t1TaskStart.Sub(rec.t0Submit)
		rec.execLatency = rec.t2TaskEnd.Sub(rec.t1TaskStart)
		rec.totalLatency = rec.t2TaskEnd.Sub(rec.t0Submit)
		records[i] = rec
	}

	// Shutdown
	cancel()
	<-loopDone

	// Analyze results
	analyzeLatencyRecords(t, "FastPathEnabled", records)
}

// TestLatencyAnalysis_NoWakeup measures latency when loop is already running.
// This tests if the wakeup path is the bottleneck.
func TestLatencyAnalysis_NoWakeup(t *testing.T) {
	const iterations = 100

	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	loopDone := make(chan struct{})
	go func() {
		_ = loop.Run(ctx)
		close(loopDone)
	}()

	// Wait for loop to be running
	for i := 0; i < 100; i++ {
		if loop.State() == StateRunning || loop.State() == StateSleeping {
			break
		}
		time.Sleep(time.Millisecond)
	}

	records := make([]latencyRecord, iterations)

	// Submit tasks rapidly without waiting between each
	// This keeps the loop busy and should bypass sleeping state
	var wg sync.WaitGroup
	var taskIdx atomic.Int32

	for i := 0; i < iterations; i++ {
		wg.Add(1)
		idx := i
		rec := &records[idx]

		rec.t0Submit = time.Now()

		err := loop.Submit(Task{Runnable: func() {
			rec.t1TaskStart = time.Now()
			runtime.Gosched()
			rec.t2TaskEnd = time.Now()
			taskIdx.Add(1)
			wg.Done()
		}})

		if err != nil {
			t.Fatalf("Submit failed at iteration %d: %v", i, err)
		}
	}

	// Wait for all tasks
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("Tasks timed out: completed %d/%d", taskIdx.Load(), iterations)
	}

	// Compute latencies
	for i := range records {
		records[i].queueLatency = records[i].t1TaskStart.Sub(records[i].t0Submit)
		records[i].execLatency = records[i].t2TaskEnd.Sub(records[i].t1TaskStart)
		records[i].totalLatency = records[i].t2TaskEnd.Sub(records[i].t0Submit)
	}

	// Shutdown
	cancel()
	<-loopDone

	analyzeLatencyRecords(t, "RapidBurst", records)
}

// TestLatencyAnalysis_PollInstrumented tests if poll() is being called unnecessarily.
func TestLatencyAnalysis_PollInstrumented(t *testing.T) {
	const iterations = 100

	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	loopDone := make(chan struct{})
	go func() {
		_ = loop.Run(ctx)
		close(loopDone)
	}()

	// Wait for loop
	for i := 0; i < 100; i++ {
		if loop.State() == StateRunning || loop.State() == StateSleeping {
			break
		}
		time.Sleep(time.Millisecond)
	}

	// Track state transitions
	var stateAtSubmit [iterations]LoopState
	var stateAtExec [iterations]LoopState

	records := make([]latencyRecord, iterations)

	for i := 0; i < iterations; i++ {
		var rec latencyRecord
		taskDone := make(chan struct{})
		idx := i

		// Small delay to let loop settle into sleeping
		time.Sleep(100 * time.Microsecond)

		stateAtSubmit[idx] = loop.State()
		rec.t0Submit = time.Now()

		err := loop.Submit(Task{Runnable: func() {
			stateAtExec[idx] = loop.State()
			rec.t1TaskStart = time.Now()
			runtime.Gosched()
			rec.t2TaskEnd = time.Now()
			close(taskDone)
		}})

		if err != nil {
			t.Fatalf("Submit failed at iteration %d: %v", i, err)
		}

		select {
		case <-taskDone:
		case <-time.After(5 * time.Second):
			t.Fatalf("Task %d timed out", i)
		}

		rec.queueLatency = rec.t1TaskStart.Sub(rec.t0Submit)
		rec.execLatency = rec.t2TaskEnd.Sub(rec.t1TaskStart)
		rec.totalLatency = rec.t2TaskEnd.Sub(rec.t0Submit)
		records[i] = rec
	}

	// Shutdown
	cancel()
	<-loopDone

	// State analysis
	sleepingCount := 0
	runningCount := 0
	for i := 0; i < iterations; i++ {
		if stateAtSubmit[i] == StateSleeping {
			sleepingCount++
		} else if stateAtSubmit[i] == StateRunning {
			runningCount++
		}
	}

	t.Logf("State at Submit: Sleeping=%d, Running=%d, Other=%d",
		sleepingCount, runningCount, iterations-sleepingCount-runningCount)

	analyzeLatencyRecords(t, "PollInstrumented", records)
}

// TestLatencyAnalysis_ChannelBaseline measures baseline Go channel latency.
// This gives us a floor for what's achievable without I/O.
func TestLatencyAnalysis_ChannelBaseline(t *testing.T) {
	const iterations = 100

	records := make([]latencyRecord, iterations)

	// Create a simple goroutine that processes tasks via channel
	taskCh := make(chan func())
	done := make(chan struct{})

	go func() {
		for task := range taskCh {
			task()
		}
		close(done)
	}()

	for i := 0; i < iterations; i++ {
		var rec latencyRecord
		taskDone := make(chan struct{})

		rec.t0Submit = time.Now()

		taskCh <- func() {
			rec.t1TaskStart = time.Now()
			runtime.Gosched()
			rec.t2TaskEnd = time.Now()
			close(taskDone)
		}

		<-taskDone

		rec.queueLatency = rec.t1TaskStart.Sub(rec.t0Submit)
		rec.execLatency = rec.t2TaskEnd.Sub(rec.t1TaskStart)
		rec.totalLatency = rec.t2TaskEnd.Sub(rec.t0Submit)
		records[i] = rec
	}

	close(taskCh)
	<-done

	analyzeLatencyRecords(t, "ChannelBaseline", records)
}

// TestLatencyAnalysis_FullTick measures whether the full tick() is being run.
func TestLatencyAnalysis_FullTick(t *testing.T) {
	const iterations = 50

	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	loopDone := make(chan struct{})
	go func() {
		_ = loop.Run(ctx)
		close(loopDone)
	}()

	// Wait for loop
	for i := 0; i < 100; i++ {
		if loop.State() == StateRunning || loop.State() == StateSleeping {
			break
		}
		time.Sleep(time.Millisecond)
	}

	records := make([]latencyRecord, iterations)

	for i := 0; i < iterations; i++ {
		var rec latencyRecord
		taskDone := make(chan struct{})

		// Larger delay to ensure loop enters sleeping
		time.Sleep(5 * time.Millisecond)

		rec.t0Submit = time.Now()

		err := loop.Submit(Task{Runnable: func() {
			rec.t1TaskStart = time.Now()
			rec.t2TaskEnd = time.Now()
			close(taskDone)
		}})

		if err != nil {
			t.Fatalf("Submit failed at iteration %d: %v", i, err)
		}

		select {
		case <-taskDone:
		case <-time.After(5 * time.Second):
			t.Fatalf("Task %d timed out", i)
		}

		rec.queueLatency = rec.t1TaskStart.Sub(rec.t0Submit)
		rec.execLatency = rec.t2TaskEnd.Sub(rec.t1TaskStart)
		rec.totalLatency = rec.t2TaskEnd.Sub(rec.t0Submit)
		records[i] = rec
	}

	// Shutdown
	cancel()
	<-loopDone

	analyzeLatencyRecords(t, "FullTickPath", records)
}

// TestLatencyAnalysis_Wakeup isolates the wakeup mechanism overhead.
func TestLatencyAnalysis_Wakeup(t *testing.T) {
	const iterations = 100

	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	loopDone := make(chan struct{})
	go func() {
		_ = loop.Run(ctx)
		close(loopDone)
	}()

	// Wait for loop
	for i := 0; i < 100; i++ {
		if loop.State() == StateRunning || loop.State() == StateSleeping {
			break
		}
		time.Sleep(time.Millisecond)
	}

	// Track wakeup overhead: measure time from Submit return to task start
	// when loop is DEFINITELY sleeping
	records := make([]latencyRecord, iterations)

	for i := 0; i < iterations; i++ {
		var rec latencyRecord
		taskDone := make(chan struct{})

		// Wait for loop to definitely be sleeping
		for j := 0; j < 100; j++ {
			if loop.State() == StateSleeping {
				break
			}
			time.Sleep(100 * time.Microsecond)
		}

		if loop.State() != StateSleeping {
			t.Logf("Warning: iter %d - loop not sleeping (state=%v)", i, loop.State())
		}

		rec.t0Submit = time.Now()

		err := loop.Submit(Task{Runnable: func() {
			rec.t1TaskStart = time.Now()
			rec.t2TaskEnd = time.Now()
			close(taskDone)
		}})

		if err != nil {
			t.Fatalf("Submit failed at iteration %d: %v", i, err)
		}

		select {
		case <-taskDone:
		case <-time.After(5 * time.Second):
			t.Fatalf("Task %d timed out", i)
		}

		rec.queueLatency = rec.t1TaskStart.Sub(rec.t0Submit)
		rec.execLatency = rec.t2TaskEnd.Sub(rec.t1TaskStart)
		rec.totalLatency = rec.t2TaskEnd.Sub(rec.t0Submit)
		records[i] = rec
	}

	// Shutdown
	cancel()
	<-loopDone

	analyzeLatencyRecords(t, "WakeupFromSleep", records)
}

// TestLatencyAnalysis_PipeVsChannel compares pipe wakeup vs channel wakeup.
func TestLatencyAnalysis_PipeVsChannel(t *testing.T) {
	t.Log("=== Pipe vs Channel Wakeup Comparison ===")
	t.Log("")

	// Test 1: Measure pipe write/read round-trip
	t.Run("PipeRoundTrip", func(t *testing.T) {
		const iterations = 100
		var durations []time.Duration

		// Create a pipe pair
		loop, err := New()
		if err != nil {
			t.Fatalf("Failed to create loop: %v", err)
		}
		defer func() {
			_ = loop.Close()
		}()

		for i := 0; i < iterations; i++ {
			start := time.Now()
			err := loop.submitWakeup()
			if err != nil {
				t.Fatalf("submitWakeup failed: %v", err)
			}
			loop.drainWakeUpPipe()
			durations = append(durations, time.Since(start))
		}

		sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
		t.Logf("Pipe write+drain - P50: %v, P95: %v, P99: %v, Mean: %v",
			durations[len(durations)/2],
			durations[len(durations)*95/100],
			durations[len(durations)*99/100],
			mean(durations))
	})

	// Test 2: Measure channel send/recv round-trip
	t.Run("ChannelRoundTrip", func(t *testing.T) {
		const iterations = 100
		var durations []time.Duration

		ch := make(chan struct{}, 1)

		for i := 0; i < iterations; i++ {
			start := time.Now()
			ch <- struct{}{}
			<-ch
			durations = append(durations, time.Since(start))
		}

		sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
		t.Logf("Channel send+recv - P50: %v, P95: %v, P99: %v, Mean: %v",
			durations[len(durations)/2],
			durations[len(durations)*95/100],
			durations[len(durations)*99/100],
			mean(durations))
	})
}

// TestLatencyAnalysis_IOFDImpact checks if having user I/O FDs affects latency.
func TestLatencyAnalysis_IOFDImpact(t *testing.T) {
	t.Log("=== I/O FD Impact on Latency ===")
	t.Log("")

	// Test WITHOUT any user FDs (should use fast channel path)
	t.Run("NoUserFDs", func(t *testing.T) {
		const iterations = 100

		loop, err := New()
		if err != nil {
			t.Fatalf("Failed to create loop: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		loopDone := make(chan struct{})
		go func() {
			_ = loop.Run(ctx)
			close(loopDone)
		}()

		for i := 0; i < 100; i++ {
			if loop.State() == StateRunning || loop.State() == StateSleeping {
				break
			}
			time.Sleep(time.Millisecond)
		}

		records := make([]latencyRecord, iterations)

		for i := 0; i < iterations; i++ {
			var rec latencyRecord
			taskDone := make(chan struct{})

			// Wait for sleeping state
			for j := 0; j < 50; j++ {
				if loop.State() == StateSleeping {
					break
				}
				time.Sleep(100 * time.Microsecond)
			}

			rec.t0Submit = time.Now()

			err := loop.Submit(Task{Runnable: func() {
				rec.t1TaskStart = time.Now()
				rec.t2TaskEnd = time.Now()
				close(taskDone)
			}})

			if err != nil {
				t.Fatalf("Submit failed: %v", err)
			}

			<-taskDone

			rec.queueLatency = rec.t1TaskStart.Sub(rec.t0Submit)
			rec.execLatency = rec.t2TaskEnd.Sub(rec.t1TaskStart)
			rec.totalLatency = rec.t2TaskEnd.Sub(rec.t0Submit)
			records[i] = rec
		}

		cancel()
		<-loopDone

		analyzeLatencyRecords(t, "NoUserFDs", records)
	})
}

// analyzeLatencyRecords computes and logs statistics for latency records.
func analyzeLatencyRecords(t *testing.T, testName string, records []latencyRecord) {
	if len(records) == 0 {
		t.Logf("[%s] No records to analyze", testName)
		return
	}

	// Extract latencies
	queueLats := make([]time.Duration, len(records))
	execLats := make([]time.Duration, len(records))
	totalLats := make([]time.Duration, len(records))

	for i, r := range records {
		queueLats[i] = r.queueLatency
		execLats[i] = r.execLatency
		totalLats[i] = r.totalLatency
	}

	// Sort for percentiles
	sort.Slice(queueLats, func(i, j int) bool { return queueLats[i] < queueLats[j] })
	sort.Slice(execLats, func(i, j int) bool { return execLats[i] < execLats[j] })
	sort.Slice(totalLats, func(i, j int) bool { return totalLats[i] < totalLats[j] })

	t.Log("==========================================================")
	t.Logf("[%s] LATENCY ANALYSIS (n=%d)", testName, len(records))
	t.Log("==========================================================")
	t.Log("")
	t.Log("QUEUE LATENCY (t1 - t0): Time from Submit() to task start")
	t.Logf("  Min:    %12v", queueLats[0])
	t.Logf("  P50:    %12v", queueLats[len(queueLats)/2])
	t.Logf("  P95:    %12v", queueLats[len(queueLats)*95/100])
	t.Logf("  P99:    %12v", queueLats[len(queueLats)*99/100])
	t.Logf("  Max:    %12v", queueLats[len(queueLats)-1])
	t.Logf("  Mean:   %12v", mean(queueLats))
	t.Log("")
	t.Log("EXEC LATENCY (t2 - t1): Time for task body to execute")
	t.Logf("  Min:    %12v", execLats[0])
	t.Logf("  P50:    %12v", execLats[len(execLats)/2])
	t.Logf("  P95:    %12v", execLats[len(execLats)*95/100])
	t.Logf("  P99:    %12v", execLats[len(execLats)*99/100])
	t.Logf("  Max:    %12v", execLats[len(execLats)-1])
	t.Logf("  Mean:   %12v", mean(execLats))
	t.Log("")
	t.Log("TOTAL LATENCY (t2 - t0): End-to-end time")
	t.Logf("  Min:    %12v", totalLats[0])
	t.Logf("  P50:    %12v", totalLats[len(totalLats)/2])
	t.Logf("  P95:    %12v", totalLats[len(totalLats)*95/100])
	t.Logf("  P99:    %12v", totalLats[len(totalLats)*99/100])
	t.Logf("  Max:    %12v", totalLats[len(totalLats)-1])
	t.Logf("  Mean:   %12v", mean(totalLats))
	t.Log("")

	// Calculate breakdown
	meanQueue := mean(queueLats)
	meanExec := mean(execLats)
	meanTotal := mean(totalLats)

	queuePct := float64(meanQueue) / float64(meanTotal) * 100
	execPct := float64(meanExec) / float64(meanTotal) * 100

	t.Log("BREAKDOWN:")
	t.Logf("  Queue overhead: %.1f%% of total latency", queuePct)
	t.Logf("  Exec overhead:  %.1f%% of total latency", execPct)
	t.Log("")

	// Flag concerning patterns
	if meanQueue > 5*time.Millisecond {
		t.Log("⚠️  WARNING: Queue latency >5ms - possible kqueue/epoll blocking")
	}
	if meanQueue > 100*time.Microsecond && meanQueue < 1*time.Millisecond {
		t.Log("⚠️  WARNING: Queue latency in 100µs-1ms range - likely syscall overhead")
	}
	if queuePct > 90 {
		t.Log("⚠️  WARNING: Queue latency dominates - wakeup mechanism is bottleneck")
	}
	t.Log("==========================================================")
}

// mean computes the arithmetic mean of durations.
func mean(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	var sum time.Duration
	for _, d := range durations {
		sum += d
	}
	return sum / time.Duration(len(durations))
}

// TestLatencyAnalysis_Summary runs all tests and prints a summary.
func TestLatencyAnalysis_Summary(t *testing.T) {
	t.Log("")
	t.Log("╔══════════════════════════════════════════════════════════════════╗")
	t.Log("║               EVENTLOOP LATENCY FORENSIC ANALYSIS                ║")
	t.Log("╠══════════════════════════════════════════════════════════════════╣")
	t.Log("║ This test suite investigates why ping-pong latency is ~11,000ns  ║")
	t.Log("║ when individual operations are measured at 10-50ns each.         ║")
	t.Log("║                                                                  ║")
	t.Log("║ Key hypothesis to test:                                          ║")
	t.Log("║ 1. Wakeup syscall overhead (pipe write/read)                     ║")
	t.Log("║ 2. kqueue/epoll blocking even with pending tasks                 ║")
	t.Log("║ 3. State machine transition overhead                             ║")
	t.Log("║ 4. Full tick() machinery running unnecessarily                   ║")
	t.Log("╚══════════════════════════════════════════════════════════════════╝")
	t.Log("")
	t.Log("Run individual tests with:")
	t.Log("  go test -v -run TestLatencyAnalysis -count=1 ./eventloop/")
	t.Log("")
}

// BenchmarkLatencyAnalysis_EndToEnd provides a proper benchmark version.
func BenchmarkLatencyAnalysis_EndToEnd(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	loopDone := make(chan struct{})
	go func() {
		_ = loop.Run(ctx)
		close(loopDone)
	}()

	// Wait for loop
	for i := 0; i < 100; i++ {
		if loop.State() == StateRunning || loop.State() == StateSleeping {
			break
		}
		time.Sleep(time.Millisecond)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		taskDone := make(chan struct{})
		start := time.Now()

		err := loop.Submit(Task{Runnable: func() {
			close(taskDone)
		}})

		if err != nil {
			b.Fatalf("Submit failed: %v", err)
		}

		<-taskDone
		_ = time.Since(start) // Latency per task
	}

	b.StopTimer()
	cancel()
	<-loopDone
}

// BenchmarkLatencyAnalysis_PingPong measures round-trip latency.
func BenchmarkLatencyAnalysis_PingPong(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	loopDone := make(chan struct{})
	go func() {
		_ = loop.Run(ctx)
		close(loopDone)
	}()

	for i := 0; i < 100; i++ {
		if loop.State() == StateRunning || loop.State() == StateSleeping {
			break
		}
		time.Sleep(time.Millisecond)
	}

	// Ping-pong channels
	ping := make(chan struct{})
	pong := make(chan struct{})

	// Start pong responder
	go func() {
		for range ping {
			_ = loop.Submit(Task{Runnable: func() {
				pong <- struct{}{}
			}})
		}
	}()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ping <- struct{}{}
		<-pong
	}

	b.StopTimer()
	close(ping)
	cancel()
	<-loopDone
}

// TestLatencyAnalysis_DetailedBreakdown provides per-operation timing.
func TestLatencyAnalysis_DetailedBreakdown(t *testing.T) {
	t.Log("")
	t.Log("=== DETAILED OPERATION BREAKDOWN ===")
	t.Log("")

	// 1. Measure mutex lock/unlock
	var mu sync.Mutex
	var sink int //nolint:staticcheck
	const ops = 10000
	start := time.Now()
	for i := 0; i < ops; i++ {
		mu.Lock()
		sink = i // Prevent empty critical section warning
		mu.Unlock()
	}
	_ = sink
	mutexTime := time.Since(start)
	t.Logf("Mutex lock/unlock:   %v/op", mutexTime/ops)

	// 2. Measure ChunkedIngress push
	q := NewChunkedIngress()
	task := Task{Runnable: func() {}}
	start = time.Now()
	for i := 0; i < ops; i++ {
		q.PushTask(task)
	}
	pushTime := time.Since(start)
	t.Logf("ChunkedIngress push: %v/op", pushTime/ops)

	// 3. Measure state load
	state := NewFastState()
	state.Store(StateRunning)
	start = time.Now()
	for i := 0; i < ops; i++ {
		_ = state.Load()
	}
	loadTime := time.Since(start)
	t.Logf("FastState load:      %v/op", loadTime/ops)

	// 4. Measure CAS
	start = time.Now()
	for i := 0; i < ops; i++ {
		if i%2 == 0 {
			state.TryTransition(StateRunning, StateSleeping)
		} else {
			state.TryTransition(StateSleeping, StateRunning)
		}
	}
	casTime := time.Since(start)
	t.Logf("FastState CAS:       %v/op", casTime/ops)

	// 5. Measure wakeUpSignalPending CAS
	var pending atomic.Uint32
	start = time.Now()
	for i := 0; i < ops; i++ {
		pending.CompareAndSwap(0, 1)
		pending.Store(0)
	}
	wakeupCasTime := time.Since(start)
	t.Logf("Wakeup pending CAS:  %v/op", wakeupCasTime/ops)

	// 6. Measure channel send (buffered)
	ch := make(chan struct{}, 1)
	start = time.Now()
	for i := 0; i < ops; i++ {
		select {
		case ch <- struct{}{}:
		default:
		}
		select {
		case <-ch:
		default:
		}
	}
	chanTime := time.Since(start)
	t.Logf("Channel send+recv:   %v/op", chanTime/ops)

	// Sum up the expected fast-path overhead
	expectedPerOp := (mutexTime + pushTime + loadTime + casTime + wakeupCasTime + chanTime) / ops
	t.Log("")
	t.Logf("Expected Submit() overhead (sum): ~%v/op", expectedPerOp)
	t.Logf("Observed ping-pong latency:       ~11,000ns/op")
	t.Logf("Unexplained gap:                  ~%v", 11*time.Microsecond-expectedPerOp)
	t.Log("")
	t.Log("The gap is likely due to:")
	t.Log("  1. Cross-goroutine scheduling latency (~1-10µs)")
	t.Log("  2. kqueue/epoll poll() syscall if loop is sleeping")
	t.Log("  3. Pipe write syscall for wakeup")
	t.Log("")
}

// TestLatencyAnalysis_FastWakeupChannel specifically tests the fast wakeup path.
func TestLatencyAnalysis_FastWakeupChannel(t *testing.T) {
	const iterations = 100

	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	// Verify no user I/O FDs
	if loop.userIOFDCount.Load() != 0 {
		t.Fatalf("Expected 0 user I/O FDs, got %d", loop.userIOFDCount.Load())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	loopDone := make(chan struct{})
	go func() {
		_ = loop.Run(ctx)
		close(loopDone)
	}()

	for i := 0; i < 100; i++ {
		if loop.State() == StateRunning || loop.State() == StateSleeping {
			break
		}
		time.Sleep(time.Millisecond)
	}

	t.Log("=== Fast Wakeup Channel Path (No User FDs) ===")
	t.Logf("userIOFDCount: %d", loop.userIOFDCount.Load())
	t.Log("")

	records := make([]latencyRecord, iterations)

	for i := 0; i < iterations; i++ {
		var rec latencyRecord
		taskDone := make(chan struct{})

		// Wait for sleeping
		for j := 0; j < 100; j++ {
			if loop.State() == StateSleeping {
				break
			}
			time.Sleep(50 * time.Microsecond)
		}

		rec.t0Submit = time.Now()

		err := loop.Submit(Task{Runnable: func() {
			rec.t1TaskStart = time.Now()
			rec.t2TaskEnd = time.Now()
			close(taskDone)
		}})

		if err != nil {
			t.Fatalf("Submit failed: %v", err)
		}

		<-taskDone

		rec.queueLatency = rec.t1TaskStart.Sub(rec.t0Submit)
		rec.execLatency = rec.t2TaskEnd.Sub(rec.t1TaskStart)
		rec.totalLatency = rec.t2TaskEnd.Sub(rec.t0Submit)
		records[i] = rec
	}

	cancel()
	<-loopDone

	analyzeLatencyRecords(t, "FastWakeupChannel", records)
}

// TestLatencyAnalysis_GoroutineScheduling measures pure scheduler overhead.
func TestLatencyAnalysis_GoroutineScheduling(t *testing.T) {
	const iterations = 100

	t.Log("=== Goroutine Scheduling Latency ===")
	t.Log("")

	var latencies []time.Duration

	for i := 0; i < iterations; i++ {
		done := make(chan struct{})
		start := time.Now()

		go func() {
			close(done)
		}()

		<-done
		latencies = append(latencies, time.Since(start))
	}

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

	t.Logf("Goroutine spawn + signal - P50: %v, P95: %v, P99: %v, Mean: %v",
		latencies[len(latencies)/2],
		latencies[len(latencies)*95/100],
		latencies[len(latencies)*99/100],
		mean(latencies))

	// Test with GOMAXPROCS=1 simulation
	t.Log("")
	t.Log("Note: Actual cross-goroutine scheduling may be higher under contention.")
}

// TestLatencyAnalysis_RawPollCost measures the cost of a non-blocking poll.
func TestLatencyAnalysis_RawPollCost(t *testing.T) {
	t.Log("=== Raw Poller Cost (Non-blocking) ===")
	t.Log("")

	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer func() {
		_ = loop.Close()
	}()

	const iterations = 1000
	var durations []time.Duration

	for i := 0; i < iterations; i++ {
		start := time.Now()
		_, _ = loop.poller.PollIO(0) // Non-blocking poll
		durations = append(durations, time.Since(start))
	}

	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })

	t.Logf("PollIO(0) - P50: %v, P95: %v, P99: %v, Mean: %v",
		durations[len(durations)/2],
		durations[len(durations)*95/100],
		durations[len(durations)*99/100],
		mean(durations))

	if mean(durations) > time.Microsecond {
		t.Log("⚠️  Non-blocking poll cost >1µs - this adds to every tick()")
	}
}

// --- Benchmark comparing different submission modes ---

func BenchmarkLatencyAnalysis_SubmitWhileRunning(b *testing.B) {
	// Measure Submit() when loop is definitely in Running state
	loop, err := New()
	if err != nil {
		b.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	loopDone := make(chan struct{})
	go func() {
		_ = loop.Run(ctx)
		close(loopDone)
	}()

	for i := 0; i < 100; i++ {
		if loop.State() == StateRunning || loop.State() == StateSleeping {
			break
		}
		time.Sleep(time.Millisecond)
	}

	// Pre-warm and keep loop busy
	taskDone := make(chan struct{}, b.N)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = loop.Submit(Task{Runnable: func() {
			taskDone <- struct{}{}
		}})
		<-taskDone
	}

	b.StopTimer()
	cancel()
	<-loopDone

	b.ReportAllocs()
}

// --- Final Summary Test ---

func TestLatencyAnalysis_Conclusion(t *testing.T) {
	t.Log("")
	t.Log("╔══════════════════════════════════════════════════════════════════╗")
	t.Log("║                    ANALYSIS CONCLUSION                           ║")
	t.Log("╠══════════════════════════════════════════════════════════════════╣")
	t.Log("║ Run the full test suite:                                         ║")
	t.Log("║   go test -v -run TestLatencyAnalysis -count=1 ./eventloop/      ║")
	t.Log("║                                                                  ║")
	t.Log("║ Key findings to look for:                                        ║")
	t.Log("║ - If queue latency is ~100ns-500ns: Wakeup mechanism is fine     ║")
	t.Log("║ - If queue latency is ~1-10µs: Goroutine scheduling overhead     ║")
	t.Log("║ - If queue latency is ~10-100µs: Syscall overhead (poll/pipe)    ║")
	t.Log("║ - If queue latency is >100µs: Bug in poll() logic or blocking    ║")
	t.Log("╚══════════════════════════════════════════════════════════════════╝")
	fmt.Println() // Force output flush
}
