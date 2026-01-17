package eventloop

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"
)

// BenchmarkMicroPingPong is a minimal ping-pong benchmark to measure
// the absolute minimum achievable latency.
func BenchmarkMicroPingPong(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}
	loop.SetFastPathEnabled(true)

	ctx := context.Background()
	var runWg sync.WaitGroup
	runWg.Add(1)
	go func() {
		loop.Run(ctx)
		runWg.Done()
	}()

	// Warm up
	done := make(chan struct{})
	loop.Submit(Task{Runnable: func() { close(done) }})
	<-done

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		done := make(chan struct{})
		loop.Submit(Task{Runnable: func() { close(done) }})
		<-done
	}

	b.StopTimer()

	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	loop.Shutdown(stopCtx)
	runWg.Wait()
}

// BenchmarkPureChannelPingPong benchmarks pure channel round-trip without eventloop.
func BenchmarkPureChannelPingPong(b *testing.B) {
	work := make(chan func())

	// Worker goroutine
	go func() {
		for fn := range work {
			fn()
		}
	}()

	// Warm up
	done := make(chan struct{})
	work <- func() { close(done) }
	<-done

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		done := make(chan struct{})
		work <- func() { close(done) }
		<-done
	}

	b.StopTimer()
	close(work)
}

// BenchmarkChannelWithMutexQueue benchmarks channel with mutex-protected queue.
func BenchmarkChannelWithMutexQueue(b *testing.B) {
	type task struct {
		fn func()
	}

	var mu sync.Mutex
	var queue []task
	wakeup := make(chan struct{}, 1)
	stop := make(chan struct{})

	// Worker goroutine
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-wakeup:
				for {
					mu.Lock()
					if len(queue) == 0 {
						mu.Unlock()
						break
					}
					t := queue[0]
					queue = queue[1:]
					mu.Unlock()
					t.fn()
				}
			}
		}
	}()

	// Warm up
	done := make(chan struct{})
	mu.Lock()
	queue = append(queue, task{fn: func() { close(done) }})
	mu.Unlock()
	select {
	case wakeup <- struct{}{}:
	default:
	}
	<-done

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		done := make(chan struct{})
		mu.Lock()
		queue = append(queue, task{fn: func() { close(done) }})
		mu.Unlock()
		select {
		case wakeup <- struct{}{}:
		default:
		}
		<-done
	}

	b.StopTimer()
	close(stop)
}

// BenchmarkGojaStyleSwap benchmarks the EXACT goja pattern:
// - Submit: lock → append → unlock → channel send
// - Drain: lock → swap → unlock → execute all
func BenchmarkGojaStyleSwap(b *testing.B) {
	type task struct {
		fn func()
	}

	var mu sync.Mutex
	var auxJobs []task
	var auxJobsSpare []task
	wakeup := make(chan struct{}, 1)
	stop := make(chan struct{})

	// runAux - exact goja pattern
	runAux := func() {
		mu.Lock()
		jobs := auxJobs
		auxJobs = auxJobsSpare
		mu.Unlock()

		for i, job := range jobs {
			job.fn()
			jobs[i] = task{} // Clear for GC
		}
		auxJobsSpare = jobs[:0]
	}

	// Worker goroutine (goja's run() pattern)
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-wakeup:
				runAux()
			}
		}
	}()

	// Warm up
	done := make(chan struct{})
	mu.Lock()
	auxJobs = append(auxJobs, task{fn: func() { close(done) }})
	mu.Unlock()
	select {
	case wakeup <- struct{}{}:
	default:
	}
	<-done

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		done := make(chan struct{})
		mu.Lock()
		auxJobs = append(auxJobs, task{fn: func() { close(done) }})
		mu.Unlock()
		select {
		case wakeup <- struct{}{}:
		default:
		}
		<-done
	}

	b.StopTimer()
	close(stop)
}

// BenchmarkMinimalLoop benchmarks the minimal eventloop pattern
// that exactly matches our runFastPath + runAux
func BenchmarkMinimalLoop(b *testing.B) {
	type task struct {
		fn func()
	}

	// Match Loop struct layout (relevant parts)
	var externalMu sync.Mutex
	var auxJobs []task
	var auxJobsSpare []task
	fastWakeupCh := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)

	// runAux - matching our implementation exactly
	runAux := func() {
		externalMu.Lock()
		jobs := auxJobs
		auxJobs = auxJobsSpare
		externalMu.Unlock()

		for i, job := range jobs {
			job.fn()
			jobs[i] = task{} // Clear for GC
		}
		auxJobsSpare = jobs[:0]
	}

	// runFastPath - matching our implementation exactly
	go func() {
		defer wg.Done()
		// Initial drain
		runAux()

		// GOJA-STYLE LOOP
		for {
			select {
			case <-ctx.Done():
				return
			case <-fastWakeupCh:
				runAux()
			}
		}
	}()

	// Warm up
	done := make(chan struct{})
	externalMu.Lock()
	auxJobs = append(auxJobs, task{fn: func() { close(done) }})
	externalMu.Unlock()
	select {
	case fastWakeupCh <- struct{}{}:
	default:
	}
	<-done

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		done := make(chan struct{})
		externalMu.Lock()
		auxJobs = append(auxJobs, task{fn: func() { close(done) }})
		externalMu.Unlock()
		select {
		case fastWakeupCh <- struct{}{}:
		default:
		}
		<-done
	}

	b.StopTimer()
	cancel()
	wg.Wait()
}

// BenchmarkLoopDirect benchmarks the Loop with direct runAux call
func BenchmarkLoopDirect(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}
	loop.SetFastPathEnabled(true)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)

	// Start the loop in a goroutine
	go func() {
		defer wg.Done()
		// NOTE: We can't call Run() because it has overhead
		// Instead, let's directly exercise runAux

		// Initial drain
		loop.runAux()

		// GOJA-STYLE LOOP
		for {
			select {
			case <-ctx.Done():
				return
			case <-loop.fastWakeupCh:
				loop.runAux()
			}
		}
	}()

	// Warm up
	done := make(chan struct{})
	loop.externalMu.Lock()
	loop.auxJobs = append(loop.auxJobs, Task{Runnable: func() { close(done) }})
	loop.externalMu.Unlock()
	select {
	case loop.fastWakeupCh <- struct{}{}:
	default:
	}
	<-done

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		done := make(chan struct{})
		loop.externalMu.Lock()
		loop.auxJobs = append(loop.auxJobs, Task{Runnable: func() { close(done) }})
		loop.externalMu.Unlock()
		select {
		case loop.fastWakeupCh <- struct{}{}:
		default:
		}
		<-done
	}

	b.StopTimer()
	cancel()
	wg.Wait()
	loop.Close()
}

// BenchmarkLoopDirectWithSubmit benchmarks Loop with direct runAux + real Submit
func BenchmarkLoopDirectWithSubmit(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}
	loop.SetFastPathEnabled(true)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)

	// Start the loop in a goroutine (bypassing Run() but with LockOSThread)
	go func() {
		defer wg.Done()
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		loop.runAux()

		for {
			select {
			case <-ctx.Done():
				return
			case <-loop.fastWakeupCh:
				loop.runAux()
			}
		}
	}()

	// Warm up
	done := make(chan struct{})
	loop.Submit(Task{Runnable: func() { close(done) }})
	<-done

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		done := make(chan struct{})
		loop.Submit(Task{Runnable: func() { close(done) }})
		<-done
	}

	b.StopTimer()
	cancel()
	wg.Wait()
	loop.Close()
}

// BenchmarkMicroPingPongWithCount benchmarks and counts fast path entries
func BenchmarkMicroPingPongWithCount(b *testing.B) {
	loop, err := New()
	if err != nil {
		b.Fatal(err)
	}
	loop.SetFastPathEnabled(true)

	ctx := context.Background()
	var runWg sync.WaitGroup
	runWg.Add(1)
	go func() {
		loop.Run(ctx)
		runWg.Done()
	}()

	// Warm up
	done := make(chan struct{})
	loop.Submit(Task{Runnable: func() { close(done) }})
	<-done

	startEntries := loop.fastPathEntries.Load()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		done := make(chan struct{})
		loop.Submit(Task{Runnable: func() { close(done) }})
		<-done
	}

	b.StopTimer()

	endEntries := loop.fastPathEntries.Load()
	endSubmits := loop.fastPathSubmits.Load()
	b.Logf("Fast path entries: start=%d, end=%d, delta=%d, N=%d",
		startEntries, endEntries, endEntries-startEntries, b.N)
	b.Logf("Fast path submits: %d (expected=%d)", endSubmits, b.N+1)

	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	loop.Shutdown(stopCtx)
	runWg.Wait()
}
