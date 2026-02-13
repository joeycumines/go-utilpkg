package inprocgrpc_test

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// ============================================================================
// T240: Stress test: 1000 concurrent unary RPCs on inprocgrpc
// ============================================================================

func TestStress_1000ConcurrentUnaryRPCs(t *testing.T) {
	ch := newTestChannel(t)

	const numWorkers = 1000
	var wg sync.WaitGroup
	wg.Add(numWorkers)
	var failures atomic.Int64

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	start := time.Now()
	for i := range numWorkers {
		go func(idx int) {
			defer wg.Done()
			req := &wrapperspb.StringValue{Value: fmt.Sprintf("request-%d", idx)}
			resp := new(wrapperspb.StringValue)
			err := ch.Invoke(ctx, "/test.TestService/Unary", req, resp)
			if err != nil {
				failures.Add(1)
				return
			}
			expected := fmt.Sprintf("echo: request-%d", idx)
			if resp.GetValue() != expected {
				failures.Add(1)
			}
		}(i)
	}
	wg.Wait()
	elapsed := time.Since(start)

	if f := failures.Load(); f > 0 {
		t.Fatalf("%d of %d RPCs failed", f, numWorkers)
	}
	t.Logf("1000 concurrent unary RPCs completed in %v (avg %v/rpc)", elapsed, elapsed/numWorkers)
}

// ============================================================================
// T241: Stress test: 100 concurrent streaming RPCs on inprocgrpc
// ============================================================================

func TestStress_100ConcurrentBidiStreams(t *testing.T) {
	ch := newTestChannel(t)

	const numStreams = 100
	const msgsPerStream = 100
	var wg sync.WaitGroup
	wg.Add(numStreams)
	var failures atomic.Int64

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	desc := &grpc.StreamDesc{
		ClientStreams: true,
		ServerStreams: true,
	}

	start := time.Now()
	for i := range numStreams {
		go func(streamIdx int) {
			defer wg.Done()
			cs, err := ch.NewStream(ctx, desc, "/test.TestService/BidiStream")
			if err != nil {
				failures.Add(1)
				return
			}
			// Send and receive msgsPerStream messages.
			for j := range msgsPerStream {
				msg := &wrapperspb.StringValue{Value: fmt.Sprintf("s%d-m%d", streamIdx, j)}
				if err := cs.SendMsg(msg); err != nil {
					failures.Add(1)
					return
				}
				resp := new(wrapperspb.StringValue)
				if err := cs.RecvMsg(resp); err != nil {
					failures.Add(1)
					return
				}
				expected := fmt.Sprintf("bidi: s%d-m%d", streamIdx, j)
				if resp.GetValue() != expected {
					failures.Add(1)
					return
				}
			}
			if err := cs.CloseSend(); err != nil {
				failures.Add(1)
				return
			}
			// Drain remaining.
			eofMsg := new(wrapperspb.StringValue)
			if err := cs.RecvMsg(eofMsg); err != io.EOF {
				failures.Add(1)
			}
		}(i)
	}
	wg.Wait()
	elapsed := time.Since(start)

	if f := failures.Load(); f > 0 {
		t.Fatalf("%d of %d streams had failures", f, numStreams)
	}
	totalMsgs := numStreams * msgsPerStream
	t.Logf("100 concurrent bidi streams × 100 msgs = %d total messages in %v", totalMsgs, elapsed)
}

// ============================================================================
// T242: Stress test: sustained throughput (5 seconds, not 30, for test speed)
// ============================================================================

func TestStress_SustainedThroughput(t *testing.T) {
	ch := newTestChannel(t)

	const duration = 5 * time.Second
	const numWorkers = 10

	ctx, cancel := context.WithTimeout(context.Background(), duration+10*time.Second)
	defer cancel()

	deadline := time.Now().Add(duration)
	var totalOps atomic.Int64
	var failures atomic.Int64
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	// Capture goroutine count before.
	goroutinesBefore := runtime.NumGoroutine()

	for range numWorkers {
		go func() {
			defer wg.Done()
			for time.Now().Before(deadline) {
				req := &wrapperspb.StringValue{Value: "sustained"}
				resp := new(wrapperspb.StringValue)
				err := ch.Invoke(ctx, "/test.TestService/Unary", req, resp)
				if err != nil {
					failures.Add(1)
					continue
				}
				totalOps.Add(1)
			}
		}()
	}
	wg.Wait()

	// Capture goroutine count after (allow settling).
	time.Sleep(100 * time.Millisecond)
	goroutinesAfter := runtime.NumGoroutine()

	ops := totalOps.Load()
	fails := failures.Load()
	t.Logf("Sustained throughput: %d ops in %v (%d workers), %d failures", ops, duration, numWorkers, fails)
	t.Logf("Goroutines: before=%d after=%d delta=%d", goroutinesBefore, goroutinesAfter, goroutinesAfter-goroutinesBefore)

	if fails > 0 {
		t.Fatalf("%d failures during sustained throughput", fails)
	}
	if ops == 0 {
		t.Fatal("zero operations completed")
	}

	// Goroutine delta should be small (tolerance: 20 for test cleanup goroutines).
	delta := goroutinesAfter - goroutinesBefore
	if delta > 20 {
		t.Errorf("goroutine leak detected: %d more goroutines after test", delta)
	}
}

// ============================================================================
// T245: Memory leak detection: goroutine monitoring
// ============================================================================

func TestStress_GoroutineLeakCheck(t *testing.T) {
	// Run a batch of stress, then measure goroutine residue.
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	baselineGoroutines := runtime.NumGoroutine()

	// Create and destroy 10 channels, each processing 100 RPCs.
	for batch := range 10 {
		ch := newTestChannel(t)
		ctx := context.Background()
		var wg sync.WaitGroup
		wg.Add(100)
		for i := range 100 {
			go func() {
				defer wg.Done()
				req := &wrapperspb.StringValue{Value: fmt.Sprintf("batch%d-rpc%d", batch, i)}
				resp := new(wrapperspb.StringValue)
				_ = ch.Invoke(ctx, "/test.TestService/Unary", req, resp)
			}()
		}
		wg.Wait()
	}

	// Allow goroutines to settle.
	runtime.GC()
	time.Sleep(200 * time.Millisecond)
	finalGoroutines := runtime.NumGoroutine()

	delta := finalGoroutines - baselineGoroutines
	t.Logf("Goroutine leak check: baseline=%d final=%d delta=%d", baselineGoroutines, finalGoroutines, delta)

	// Allow generous tolerance for test framework goroutines.
	if delta > 30 {
		t.Errorf("potential goroutine leak: %d goroutines above baseline", delta)
	}
}

// ============================================================================
// T246: Memory leak detection: heap profiling (allocation check)
// ============================================================================

func TestStress_HeapAllocationCheck(t *testing.T) {
	ch := newTestChannel(t)
	ctx := context.Background()

	// Warm up to ensure JIT and caches are populated.
	for range 100 {
		req := &wrapperspb.StringValue{Value: "warmup"}
		resp := new(wrapperspb.StringValue)
		_ = ch.Invoke(ctx, "/test.TestService/Unary", req, resp)
	}

	// Measure heap before.
	runtime.GC()
	runtime.GC()
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	// Run 10,000 RPCs.
	const numRPCs = 10000
	for range numRPCs {
		req := &wrapperspb.StringValue{Value: "heap-check"}
		resp := new(wrapperspb.StringValue)
		if err := ch.Invoke(ctx, "/test.TestService/Unary", req, resp); err != nil {
			t.Fatalf("RPC failed: %v", err)
		}
	}

	// Measure heap after.
	runtime.GC()
	runtime.GC()
	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	// Calculate per-RPC allocation.
	totalAlloc := memAfter.TotalAlloc - memBefore.TotalAlloc
	allocsPerRPC := totalAlloc / numRPCs
	mallocsPerRPC := (memAfter.Mallocs - memBefore.Mallocs) / numRPCs

	t.Logf("Heap profiling: %d RPCs, total allocated=%d bytes, per-RPC=%d bytes, mallocs/rpc=%d",
		numRPCs, totalAlloc, allocsPerRPC, mallocsPerRPC)

	// HeapInuse delta should be modest — no retained leak.
	heapDelta := int64(memAfter.HeapInuse) - int64(memBefore.HeapInuse)
	t.Logf("HeapInuse: before=%d after=%d delta=%d bytes",
		memBefore.HeapInuse, memAfter.HeapInuse, heapDelta)

	// If heap grows by more than 10MB after 10k RPCs, something is leaking.
	if heapDelta > 10*1024*1024 {
		t.Errorf("potential heap leak: HeapInuse grew by %d bytes after %d RPCs", heapDelta, numRPCs)
	}
}
