package eventloop

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// INTEGRATION-002: Real-World Workload Simulation
// ============================================================================
//
// Creates tests simulating real-world patterns:
// - HTTP server with timeouts (simulated)
// - Scheduled job processing
// - Promise-based data pipelines
// - Worker pool patterns
// - Rate-limited operations
// - Circuit breaker patterns

// TestWorkload_SimulatedHTTPServer simulates an HTTP server handling
// requests with timeouts and concurrent request processing.
func TestWorkload_SimulatedHTTPServer(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	// Simulate request handling with varying response times
	type Request struct {
		ID          int
		ProcessTime time.Duration
	}

	type Response struct {
		ID     int
		Status string
	}

	const numRequests = 100
	const requestTimeout = 50 * time.Millisecond

	var completed atomic.Int32
	var successes atomic.Int32
	var timeouts atomic.Int32

	// Process a simulated HTTP request
	handleRequest := func(req Request) *ChainedPromise {
		promise, resolve, reject := js.NewChainedPromise()

		// Simulate async processing
		go func() {
			time.Sleep(req.ProcessTime)
			if req.ProcessTime < requestTimeout {
				resolve(Response{ID: req.ID, Status: "OK"})
			} else {
				reject(errors.New("request timed out"))
			}
		}()

		return promise
	}

	// Create a timeout promise
	timeoutPromise := func(duration time.Duration) *ChainedPromise {
		p, _, reject := js.NewChainedPromise()
		go func() {
			time.Sleep(duration)
			reject(errors.New("timeout"))
		}()
		return p
	}

	done := make(chan struct{})

	// Simulate incoming requests
	for i := 0; i < numRequests; i++ {
		req := Request{
			ID:          i,
			ProcessTime: time.Duration(rand.Intn(100)) * time.Millisecond,
		}

		// Race between request and timeout
		requestP := handleRequest(req)
		timeoutP := timeoutPromise(requestTimeout)

		js.Race([]*ChainedPromise{requestP, timeoutP}).Then(
			func(r Result) Result {
				successes.Add(1)
				completed.Add(1)
				if int(completed.Load()) == numRequests {
					close(done)
				}
				return nil
			},
			func(r Result) Result {
				timeouts.Add(1)
				completed.Add(1)
				if int(completed.Load()) == numRequests {
					close(done)
				}
				return nil
			},
		)
	}

	select {
	case <-done:
		t.Logf("HTTP Server Simulation: %d requests, %d successes, %d timeouts",
			numRequests, successes.Load(), timeouts.Load())
	case <-time.After(15 * time.Second):
		t.Fatalf("Timeout: completed %d/%d requests", completed.Load(), numRequests)
	}

	cancel()
	<-loopDone
}

// TestWorkload_ScheduledJobProcessing simulates a job scheduler
// that processes jobs at regular intervals.
func TestWorkload_ScheduledJobProcessing(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	type Job struct {
		ID       int
		Priority int
		Data     string
	}

	var processedJobs []Job
	var jobsMu sync.Mutex
	var jobCounter atomic.Int32

	const targetJobs = 20
	done := make(chan struct{})

	// Job processor function
	processJob := func(job Job) *ChainedPromise {
		p, resolve, _ := js.NewChainedPromise()
		// Simulate async processing
		go func() {
			time.Sleep(time.Duration(5+rand.Intn(10)) * time.Millisecond)
			resolve(job)
		}()
		return p
	}

	// Schedule job batches
	var scheduleID uint64
	var scheduleNextBatch func()

	scheduleNextBatch = func() {
		batchSize := 5
		batch := make([]*ChainedPromise, batchSize)

		for i := 0; i < batchSize; i++ {
			job := Job{
				ID:       int(jobCounter.Add(1)),
				Priority: rand.Intn(10),
				Data:     fmt.Sprintf("job-data-%d", i),
			}
			batch[i] = processJob(job)
		}

		js.All(batch).Then(func(r Result) Result {
			results := r.([]Result)
			jobsMu.Lock()
			for _, result := range results {
				if job, ok := result.(Job); ok {
					processedJobs = append(processedJobs, job)
				}
			}
			if len(processedJobs) >= targetJobs {
				close(done)
				jobsMu.Unlock()
				return nil
			}
			jobsMu.Unlock()

			// Schedule next batch
			var err error
			scheduleID, err = js.SetTimeout(scheduleNextBatch, 20)
			if err != nil && !errors.Is(err, ErrLoopTerminated) {
				t.Errorf("Failed to schedule next batch: %v", err)
			}
			return nil
		}, nil)
	}

	// Start scheduling
	scheduleNextBatch()

	select {
	case <-done:
		jobsMu.Lock()
		t.Logf("Scheduled jobs processed: %d", len(processedJobs))
		jobsMu.Unlock()
		// Cancel the next scheduled batch
		if scheduleID != 0 {
			_ = js.ClearTimeout(scheduleID)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Timeout waiting for job processing")
	}

	cancel()
	<-loopDone
}

// TestWorkload_PromiseDataPipeline simulates a data processing pipeline
// with multiple transformation stages.
func TestWorkload_PromiseDataPipeline(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	// Pipeline: fetch -> parse -> transform -> validate -> store
	type DataRecord struct {
		ID        int
		Value     int
		Stage     string
		Processed bool
	}

	// Stage 1: Fetch (simulate network delay)
	fetch := func(id int) *ChainedPromise {
		p, resolve, _ := js.NewChainedPromise()
		go func() {
			time.Sleep(5 * time.Millisecond)
			resolve(DataRecord{ID: id, Value: id * 10, Stage: "fetched"})
		}()
		return p
	}

	// Stage 2: Parse
	parse := func(data Result) Result {
		record := data.(DataRecord)
		record.Stage = "parsed"
		record.Value = record.Value + 1
		return record
	}

	// Stage 3: Transform
	transform := func(data Result) Result {
		record := data.(DataRecord)
		record.Stage = "transformed"
		record.Value = record.Value * 2
		return record
	}

	// Stage 4: Validate
	validate := func(data Result) Result {
		record := data.(DataRecord)
		if record.Value < 0 {
			panic("Invalid value")
		}
		record.Stage = "validated"
		return record
	}

	// Stage 5: Store
	store := func(data Result) Result {
		record := data.(DataRecord)
		record.Stage = "stored"
		record.Processed = true
		return record
	}

	const numRecords = 50
	var results []DataRecord
	var resultsMu sync.Mutex
	var completed atomic.Int32
	done := make(chan struct{})

	for i := 0; i < numRecords; i++ {
		fetch(i).
			Then(parse, nil).
			Then(transform, nil).
			Then(validate, nil).
			Then(store, nil).
			Then(func(r Result) Result {
				record := r.(DataRecord)
				resultsMu.Lock()
				results = append(results, record)
				resultsMu.Unlock()

				if int(completed.Add(1)) == numRecords {
					close(done)
				}
				return nil
			}, func(r Result) Result {
				if int(completed.Add(1)) == numRecords {
					close(done)
				}
				return nil
			})
	}

	select {
	case <-done:
		resultsMu.Lock()
		processedCount := 0
		for _, r := range results {
			if r.Processed && r.Stage == "stored" {
				processedCount++
			}
		}
		t.Logf("Pipeline: %d/%d records fully processed", processedCount, numRecords)
		resultsMu.Unlock()
	case <-time.After(10 * time.Second):
		t.Fatal("Timeout waiting for pipeline")
	}

	cancel()
	<-loopDone
}

// TestWorkload_WorkerPool simulates a worker pool processing tasks.
func TestWorkload_WorkerPool(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	const numWorkers = 5
	const numTasks = 100

	// Worker function
	worker := func(workerId int, taskId int) *ChainedPromise {
		p, resolve, _ := js.NewChainedPromise()
		go func() {
			// Simulate work
			time.Sleep(time.Duration(1+rand.Intn(5)) * time.Millisecond)
			resolve(map[string]int{"worker": workerId, "task": taskId})
		}()
		return p
	}

	// Task queue
	tasks := make(chan int, numTasks)
	for i := 0; i < numTasks; i++ {
		tasks <- i
	}
	close(tasks)

	var completed atomic.Int32
	var workerCounts [numWorkers]atomic.Int32
	done := make(chan struct{})

	// Start workers
	for w := 0; w < numWorkers; w++ {
		workerId := w
		var processNext func()
		processNext = func() {
			select {
			case taskId, ok := <-tasks:
				if !ok {
					return // No more tasks
				}
				worker(workerId, taskId).Then(func(r Result) Result {
					workerCounts[workerId].Add(1)
					if int(completed.Add(1)) == numTasks {
						close(done)
					} else {
						// Process next task
						processNext()
					}
					return nil
				}, nil)
			default:
				return
			}
		}
		processNext()
	}

	select {
	case <-done:
		t.Logf("Worker pool completed %d tasks", completed.Load())
		for i := 0; i < numWorkers; i++ {
			t.Logf("  Worker %d: %d tasks", i, workerCounts[i].Load())
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("Timeout: completed %d/%d tasks", completed.Load(), numTasks)
	}

	cancel()
	<-loopDone
}

// TestWorkload_RateLimitedOperations simulates rate-limited API calls.
func TestWorkload_RateLimitedOperations(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	// Rate limiter: max 10 requests per 100ms
	const maxRequests = 10
	const windowMs = 100

	var requestCount atomic.Int32
	var lastReset atomic.Int64
	lastReset.Store(time.Now().UnixMilli())

	// Check rate limit
	checkLimit := func() bool {
		now := time.Now().UnixMilli()
		last := lastReset.Load()
		if now-last >= windowMs {
			lastReset.Store(now)
			requestCount.Store(0)
		}
		if requestCount.Load() >= maxRequests {
			return false
		}
		requestCount.Add(1)
		return true
	}

	// Rate-limited API call
	apiCall := func(id int) *ChainedPromise {
		p, resolve, reject := js.NewChainedPromise()

		if !checkLimit() {
			reject(errors.New("rate limit exceeded"))
			return p
		}

		go func() {
			time.Sleep(5 * time.Millisecond)
			resolve(fmt.Sprintf("response-%d", id))
		}()
		return p
	}

	const numCalls = 50
	var successes atomic.Int32
	var rateLimited atomic.Int32
	var completed atomic.Int32
	done := make(chan struct{})

	for i := 0; i < numCalls; i++ {
		callId := i
		apiCall(callId).Then(
			func(r Result) Result {
				successes.Add(1)
				if int(completed.Add(1)) == numCalls {
					close(done)
				}
				return nil
			},
			func(r Result) Result {
				rateLimited.Add(1)
				if int(completed.Add(1)) == numCalls {
					close(done)
				}
				return nil
			},
		)
		// Small delay between calls to spread them out
		time.Sleep(2 * time.Millisecond)
	}

	select {
	case <-done:
		t.Logf("Rate limiting: %d calls, %d successes, %d rate-limited",
			numCalls, successes.Load(), rateLimited.Load())
	case <-time.After(10 * time.Second):
		t.Fatalf("Timeout: completed %d/%d calls", completed.Load(), numCalls)
	}

	cancel()
	<-loopDone
}

// TestWorkload_CircuitBreaker simulates a circuit breaker pattern.
func TestWorkload_CircuitBreaker(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	// Circuit breaker state
	type CircuitState int
	const (
		Closed CircuitState = iota
		Open
		HalfOpen
	)

	var state atomic.Int32
	var failureCount atomic.Int32
	var lastFailure atomic.Int64
	const failureThreshold = 3
	const recoveryTimeMs = 100

	getState := func() CircuitState {
		return CircuitState(state.Load())
	}

	// Unreliable service (50% failure rate)
	unreliableService := func(id int) *ChainedPromise {
		p, resolve, reject := js.NewChainedPromise()

		currentState := getState()
		if currentState == Open {
			if time.Now().UnixMilli()-lastFailure.Load() > recoveryTimeMs {
				state.Store(int32(HalfOpen))
				currentState = HalfOpen
			} else {
				reject(errors.New("circuit open"))
				return p
			}
		}

		go func() {
			time.Sleep(5 * time.Millisecond)
			if rand.Float32() < 0.5 {
				// Success
				if currentState == HalfOpen {
					state.Store(int32(Closed))
					failureCount.Store(0)
				}
				resolve(fmt.Sprintf("success-%d", id))
			} else {
				// Failure
				failures := failureCount.Add(1)
				lastFailure.Store(time.Now().UnixMilli())
				if failures >= failureThreshold {
					state.Store(int32(Open))
				}
				reject(errors.New("service failure"))
			}
		}()
		return p
	}

	const numCalls = 50
	var successes atomic.Int32
	var failures atomic.Int32
	var circuitOpen atomic.Int32
	var completed atomic.Int32
	done := make(chan struct{})

	for i := 0; i < numCalls; i++ {
		callId := i
		unreliableService(callId).Then(
			func(r Result) Result {
				successes.Add(1)
				if int(completed.Add(1)) == numCalls {
					close(done)
				}
				return nil
			},
			func(r Result) Result {
				if err, ok := r.(error); ok && err.Error() == "circuit open" {
					circuitOpen.Add(1)
				} else {
					failures.Add(1)
				}
				if int(completed.Add(1)) == numCalls {
					close(done)
				}
				return nil
			},
		)
		// Small delay between calls
		time.Sleep(5 * time.Millisecond)
	}

	select {
	case <-done:
		t.Logf("Circuit breaker: %d calls, %d successes, %d failures, %d circuit-open rejections",
			numCalls, successes.Load(), failures.Load(), circuitOpen.Load())
	case <-time.After(10 * time.Second):
		t.Fatalf("Timeout: completed %d/%d calls", completed.Load(), numCalls)
	}

	cancel()
	<-loopDone
}

// TestWorkload_RetryWithBackoff simulates retry logic with exponential backoff.
func TestWorkload_RetryWithBackoff(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	// Flaky operation that succeeds after N attempts
	var attemptCounter sync.Map

	flakyOperation := func(id int) *ChainedPromise {
		p, resolve, reject := js.NewChainedPromise()

		count := 0
		if val, ok := attemptCounter.Load(id); ok {
			count = val.(int)
		}
		count++
		attemptCounter.Store(id, count)

		go func() {
			time.Sleep(2 * time.Millisecond)
			successThreshold := 3 + (id % 3) // Succeed after 3-5 attempts
			if count >= successThreshold {
				resolve(fmt.Sprintf("success-%d-after-%d-attempts", id, count))
			} else {
				reject(fmt.Errorf("attempt %d failed", count))
			}
		}()
		return p
	}

	// Retry with backoff
	retryWithBackoff := func(id int, maxRetries int, baseDelayMs int) *ChainedPromise {
		resultP, resolve, reject := js.NewChainedPromise()

		var attempt func(retry int)
		attempt = func(retry int) {
			flakyOperation(id).Then(
				func(r Result) Result {
					resolve(r)
					return nil
				},
				func(r Result) Result {
					if retry >= maxRetries {
						reject(fmt.Errorf("max retries exceeded: %v", r))
						return nil
					}

					// Exponential backoff
					delayMs := baseDelayMs * (1 << retry)
					if delayMs > 100 {
						delayMs = 100 // Cap delay
					}

					_, err := js.SetTimeout(func() {
						attempt(retry + 1)
					}, delayMs)
					if err != nil && !errors.Is(err, ErrLoopTerminated) {
						reject(err)
					}
					return nil
				},
			)
		}

		attempt(0)
		return resultP
	}

	const numOperations = 20
	var successes atomic.Int32
	var failures atomic.Int32
	var completed atomic.Int32
	done := make(chan struct{})

	for i := 0; i < numOperations; i++ {
		opId := i
		retryWithBackoff(opId, 10, 5).Then(
			func(r Result) Result {
				successes.Add(1)
				if int(completed.Add(1)) == numOperations {
					close(done)
				}
				return nil
			},
			func(r Result) Result {
				failures.Add(1)
				if int(completed.Add(1)) == numOperations {
					close(done)
				}
				return nil
			},
		)
	}

	select {
	case <-done:
		t.Logf("Retry with backoff: %d operations, %d successes, %d failures",
			numOperations, successes.Load(), failures.Load())
	case <-time.After(15 * time.Second):
		t.Fatalf("Timeout: completed %d/%d operations", completed.Load(), numOperations)
	}

	cancel()
	<-loopDone
}

// TestWorkload_BatchProcessor simulates batch processing with accumulation.
func TestWorkload_BatchProcessor(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	// Batch accumulator
	type Batch struct {
		Items []int
		mu    sync.Mutex
	}
	batch := &Batch{Items: make([]int, 0)}

	const batchSize = 10
	const flushIntervalMs = 50

	var batchesProcessed atomic.Int32
	var itemsProcessed atomic.Int32
	var flushTimerID uint64

	// Process a batch
	processBatch := func(items []int) *ChainedPromise {
		p, resolve, _ := js.NewChainedPromise()
		go func() {
			time.Sleep(10 * time.Millisecond) // Simulate batch processing
			resolve(len(items))
		}()
		return p
	}

	// Flush current batch
	flush := func() {
		batch.mu.Lock()
		if len(batch.Items) == 0 {
			batch.mu.Unlock()
			return
		}
		items := batch.Items
		batch.Items = make([]int, 0, batchSize)
		batch.mu.Unlock()

		processBatch(items).Then(func(r Result) Result {
			count := r.(int)
			itemsProcessed.Add(int32(count))
			batchesProcessed.Add(1)
			return nil
		}, nil)
	}

	// Schedule periodic flush
	var scheduleFlush func()
	scheduleFlush = func() {
		var err error
		flushTimerID, err = js.SetTimeout(func() {
			flush()
			scheduleFlush() // Reschedule
		}, flushIntervalMs)
		if err != nil && !errors.Is(err, ErrLoopTerminated) {
			t.Errorf("Failed to schedule flush: %v", err)
		}
	}
	scheduleFlush()

	// Add items
	addItem := func(item int) {
		batch.mu.Lock()
		batch.Items = append(batch.Items, item)
		shouldFlush := len(batch.Items) >= batchSize
		batch.mu.Unlock()

		if shouldFlush {
			flush()
		}
	}

	const numItems = 100
	for i := 0; i < numItems; i++ {
		addItem(i)
		time.Sleep(5 * time.Millisecond)
	}

	// Wait for processing to complete
	time.Sleep(200 * time.Millisecond)

	// Final flush
	flush()
	time.Sleep(50 * time.Millisecond)

	// Cancel flush timer
	if flushTimerID != 0 {
		_ = js.ClearTimeout(flushTimerID)
	}

	t.Logf("Batch processor: %d items in %d batches processed",
		itemsProcessed.Load(), batchesProcessed.Load())

	if itemsProcessed.Load() < int32(numItems)-batchSize {
		t.Errorf("Too few items processed: %d/%d", itemsProcessed.Load(), numItems)
	}

	cancel()
	<-loopDone
}

// TestWorkload_PubSubPattern simulates a publish-subscribe pattern.
func TestWorkload_PubSubPattern(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	// Simple pub-sub
	type Event struct {
		Type string
		Data interface{}
	}

	subscribers := make(map[string][]func(Event))
	var subMu sync.RWMutex

	subscribe := func(eventType string, handler func(Event)) {
		subMu.Lock()
		subscribers[eventType] = append(subscribers[eventType], handler)
		subMu.Unlock()
	}

	publish := func(event Event) *ChainedPromise {
		subMu.RLock()
		handlers := subscribers[event.Type]
		subMu.RUnlock()

		if len(handlers) == 0 {
			return js.Resolve(0)
		}

		promises := make([]*ChainedPromise, len(handlers))
		for i, handler := range handlers {
			h := handler
			e := event
			p, resolve, _ := js.NewChainedPromise()
			promises[i] = p

			// Execute handler asynchronously
			go func() {
				h(e)
				resolve(nil)
			}()
		}

		return js.All(promises).Then(func(r Result) Result {
			return len(handlers)
		}, nil)
	}

	// Set up subscribers
	var userEvents atomic.Int32
	var orderEvents atomic.Int32
	var systemEvents atomic.Int32

	subscribe("user", func(e Event) {
		userEvents.Add(1)
	})
	subscribe("user", func(e Event) {
		// Second subscriber for user events
		userEvents.Add(1)
	})
	subscribe("order", func(e Event) {
		orderEvents.Add(1)
	})
	subscribe("system", func(e Event) {
		systemEvents.Add(1)
	})

	// Publish events
	const numEvents = 30
	var completed atomic.Int32
	done := make(chan struct{})

	eventTypes := []string{"user", "order", "system"}
	for i := 0; i < numEvents; i++ {
		eventID := i
		eventType := eventTypes[i%len(eventTypes)]
		publish(Event{Type: eventType, Data: eventID}).Then(func(r Result) Result {
			if int(completed.Add(1)) == numEvents {
				close(done)
			}
			return nil
		}, nil)
	}

	select {
	case <-done:
		t.Logf("PubSub: user=%d, order=%d, system=%d",
			userEvents.Load(), orderEvents.Load(), systemEvents.Load())
	case <-time.After(10 * time.Second):
		t.Fatal("Timeout waiting for pub-sub")
	}

	cancel()
	<-loopDone
}

// TestWorkload_StreamProcessing simulates stream processing with backpressure.
func TestWorkload_StreamProcessing(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	// Stream processing with buffer
	const bufferSize = 10
	buffer := make(chan int, bufferSize)

	var produced atomic.Int32
	var consumed atomic.Int32
	var dropped atomic.Int32

	const numItems = 100
	done := make(chan struct{})

	// Producer
	go func() {
		for i := 0; i < numItems; i++ {
			select {
			case buffer <- i:
				produced.Add(1)
			default:
				dropped.Add(1)
			}
			time.Sleep(2 * time.Millisecond)
		}
		close(buffer)
	}()

	// Consumer (using event loop)
	var consumeNext func()
	consumeNext = func() {
		select {
		case item, ok := <-buffer:
			if !ok {
				close(done)
				return
			}

			// Process item with promise
			p, resolve, _ := js.NewChainedPromise()
			go func() {
				time.Sleep(5 * time.Millisecond) // Slower than producer
				resolve(item)
			}()

			p.Then(func(r Result) Result {
				consumed.Add(1)
				// Process next after current completes
				consumeNext()
				return nil
			}, nil)
		default:
			// Buffer empty, wait and try again
			_, err := js.SetTimeout(consumeNext, 10)
			if err != nil && !errors.Is(err, ErrLoopTerminated) {
				close(done)
			}
		}
	}

	consumeNext()

	select {
	case <-done:
		t.Logf("Stream: produced=%d, consumed=%d, dropped=%d",
			produced.Load(), consumed.Load(), dropped.Load())
	case <-time.After(10 * time.Second):
		t.Fatalf("Timeout: produced=%d, consumed=%d, dropped=%d",
			produced.Load(), consumed.Load(), dropped.Load())
	}

	cancel()
	<-loopDone
}

// TestWorkload_MixedAsyncPatterns tests a combination of async patterns.
func TestWorkload_MixedAsyncPatterns(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("Failed to create JS: %v", err)
	}

	const numIterations = 10
	var completed atomic.Int32
	done := make(chan struct{})

	for iter := 0; iter < numIterations; iter++ {
		iteration := iter

		// 1. Create multiple promises with different characteristics
		promises := make([]*ChainedPromise, 5)

		// Fast promise
		p1, r1, _ := js.NewChainedPromise()
		promises[0] = p1
		go func() {
			time.Sleep(1 * time.Millisecond)
			r1(fmt.Sprintf("fast-%d", iteration))
		}()

		// Slow promise
		p2, r2, _ := js.NewChainedPromise()
		promises[1] = p2
		go func() {
			time.Sleep(50 * time.Millisecond)
			r2(fmt.Sprintf("slow-%d", iteration))
		}()

		// Promise chain
		promises[2] = js.Resolve(iteration).
			Then(func(r Result) Result { return r.(int) * 2 }, nil).
			Then(func(r Result) Result { return r.(int) + 1 }, nil)

		// Promise that rejects
		p4, _, rej4 := js.NewChainedPromise()
		promises[3] = p4.Catch(func(r Result) Result {
			return fmt.Sprintf("recovered-%d", iteration)
		})
		go func() {
			time.Sleep(10 * time.Millisecond)
			rej4(errors.New("intentional rejection"))
		}()

		// Timer-based promise
		p5, r5, _ := js.NewChainedPromise()
		promises[4] = p5
		_, err := js.SetTimeout(func() {
			r5(fmt.Sprintf("timer-%d", iteration))
		}, 20)
		if err != nil && !errors.Is(err, ErrLoopTerminated) {
			t.Errorf("Failed to set timeout: %v", err)
		}

		// 2. Wait for all with AllSettled
		js.AllSettled(promises).Then(func(r Result) Result {
			results := r.([]Result)
			for _, res := range results {
				m := res.(map[string]interface{})
				if m["status"] != "fulfilled" && m["status"] != "rejected" {
					t.Errorf("Unexpected status: %v", m["status"])
				}
			}

			if int(completed.Add(1)) == numIterations {
				close(done)
			}
			return nil
		}, nil)
	}

	select {
	case <-done:
		t.Logf("Mixed patterns: %d iterations completed", completed.Load())
	case <-time.After(20 * time.Second):
		t.Fatalf("Timeout: completed %d/%d iterations", completed.Load(), numIterations)
	}

	cancel()
	<-loopDone
}
