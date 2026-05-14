package microbatch

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestNewBatcher(t *testing.T) {
	for _, tc := range [...]struct {
		name         string
		config       *BatcherConfig
		nilProcessor bool
		wantErr      bool
	}{
		{`valid config`, &BatcherConfig{MaxSize: 10, FlushInterval: 50 * time.Millisecond, MaxConcurrency: 2}, false, false},
		{`nil config`, nil, false, false},
		{`max size disabled`, &BatcherConfig{MaxSize: -1}, false, false},
		{`flush interval disabled`, &BatcherConfig{FlushInterval: -1}, false, false},
		{`all flush options disabled`, &BatcherConfig{MaxSize: -1, FlushInterval: -1}, false, true},
		{`nil processor`, nil, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer checkNumGoroutines(time.Second * 3)(t) // should always clean up
			defer func() {
				if r := recover(); r != nil && !tc.wantErr {
					t.Errorf(`unexpected panic: %v`, r)
				}
			}()
			processor := func(ctx context.Context, jobs []any) error {
				panic(`should not be called`)
			}
			if tc.nilProcessor {
				processor = nil
			}
			batcher := NewBatcher(tc.config, processor)
			if batcher == nil {
				t.Error(`batcher should never be nil`)
			} else {
				defer batcher.Close()
			}
			if tc.wantErr {
				t.Error(`should have errored`)
			}
		})
	}
}

// should be checked first, for consistency of errors
func TestBatcher_Submit_ctxCancelGuarded(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result, err := (*Batcher[any])(nil).Submit(ctx, nil); err != context.Canceled || result != nil {
		t.Fatal(result, err)
	}
}

// like the above, should be checked early, for consistency of errors (no specific error, currently)
func TestBatcher_Submit_batcherClosedGuarded(t *testing.T) {
	batcher := NewBatcher(nil, func(ctx context.Context, jobs []any) error {
		panic(`should not be called`)
	})
	if err := batcher.Close(); err != nil {
		t.Fatal(err)
	}
	if result, err := batcher.Submit(context.Background(), nil); err != context.Canceled || result != nil {
		t.Fatal(result, err)
	}
}

type processorArgsAny struct {
	ctx  context.Context
	jobs []any
}

// sets up two job in a batcher, with channels to control the BatchProcessor
func setupBlockedSubmit(t *testing.T) (_ *Batcher[any], processorInCh <-chan processorArgsAny, processorOutCh chan<- error) {
	t.Helper()
	processorIn := make(chan processorArgsAny) // called BatchProcessor
	processorOut := make(chan error)           // unblock BatchProcessor

	batcher := NewBatcher(
		&BatcherConfig{MaxSize: 1, FlushInterval: 1, MaxConcurrency: 1},
		func(ctx context.Context, jobs []any) error {
			processorIn <- processorArgsAny{ctx, jobs}
			return <-processorOut
		},
	)

	// submit a job so we reach max concurrency
	if result1, err := batcher.Submit(context.Background(), 1); err != nil || result1 == nil {
		t.Fatal(result1, err)
	}

	// ensure it started as expected
	<-processorIn

	// submit another job, which should block the control loop
	if result2, err := batcher.Submit(context.Background(), 2); err != nil || result2 == nil {
		t.Fatal(result2, err)
	}

	// ensure the second job isn't yet running, as expected
	time.Sleep(time.Millisecond * 20)
	select {
	case <-processorIn:
		t.Fatal(`expected no second job to be running`)
	default:
	}

	return batcher, processorIn, processorOut
}

// test cancellation of a job during Submit, before it is added to the batch
func TestBatcher_Submit_ctxCancel(t *testing.T) {
	defer checkNumGoroutines(time.Second * 3)(t) // should always clean up

	batcher, processorIn, processorOut := setupBlockedSubmit(t)

	// submit a third job in the background - this is the thing we are actually testing
	done := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		defer close(done)
		defer cancel()
		if result3, err := batcher.Submit(ctx, 3); err != context.Canceled || result3 != nil {
			t.Error(result3, err)
		}
	}()

	// wait for a bit then check to see if our third job was actually blocked on Submit
	time.Sleep(time.Millisecond * 30)
	select {
	case <-done:
		t.Fatal(`expected third job to be blocked on Submit`)
	default:
	}

	// cancel Submit, should unblock
	cancel()
	<-done
	if t.Failed() {
		t.FailNow()
	}

	// test succeeded (probably), we just need to clean up
	processorOut <- nil
	<-processorIn
	processorOut <- nil
	if err := batcher.Shutdown(context.Background()); err != nil {
		t.Error(err)
	}
}

// consolidated test logic for three variants of stopping (Shutdown, Shutdown canceled, Close)
func testShutdownCloseJobInProgress(t *testing.T, expectCanceled bool, expectedResult error, stopBatcher func(batcher *Batcher[any]) error) {
	t.Helper()
	defer checkNumGoroutines(time.Second * 3)(t) // should always clean up

	batcher, processorIn, processorOut := setupBlockedSubmit(t)

	// submit a third job in the background
	done := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		defer close(done)
		defer cancel()
		if result3, err := batcher.Submit(ctx, 3); err != context.Canceled || result3 != nil {
			t.Error(result3, err)
		}
	}()

	// wait for a bit then check to see if our third job was actually blocked on Submit
	// note: long sleep because the expectCanceled assertion is racey by nature
	time.Sleep(time.Millisecond * 300)
	select {
	case <-done:
		t.Fatal(`expected third job to be blocked on Submit`)
	default:
	}

	// start the shutdown process in the background...
	out := make(chan error)
	go func() {
		out <- stopBatcher(batcher)
	}()

	// should immediately unblock our third job, which hasn't been submitted yet
	<-done

	// finish up with the first job, with an error just because
	processorOut <- errors.New(`some error`)

	// the context the second job receives should match the expected state
	{
		args := <-processorIn
		if (args.ctx.Err() != nil) != expectCanceled {
			t.Errorf(`expected context canceled = %v`, expectCanceled)
		}
		if !reflect.DeepEqual(args.jobs, []any{2}) {
			t.Errorf(`expected jobs to be [2], got %v`, args.jobs)
		}
	}

	// wait a bit and verify we are still waiting for shutdown to finish
	time.Sleep(time.Millisecond * 30)
	select {
	case <-out:
		t.Fatal(`expected shutdown to still be in progress`)
	default:
	}

	// another error, doesn't affect the shutdown process though
	processorOut <- errors.New(`some other error`)

	// we should be done
	if err := <-out; err != expectedResult {
		t.Error(err)
	}
}

// test Shutdown with a job in progress, including a blocked Submit + queued up batch
func TestBatcher_Shutdown_jobInProgress(t *testing.T) {
	testShutdownCloseJobInProgress(t, false, nil, func(batcher *Batcher[any]) error {
		return batcher.Shutdown(context.Background())
	})
}

// test Shutdown with a job in progress, with cancellation, including a blocked Submit + queued up batch
func TestBatcher_Shutdown_jobInProgressCanceled(t *testing.T) {
	defer checkNumGoroutines(time.Second * 3)(t)

	batcher, processorIn, processorOut := setupBlockedSubmit(t)

	done := make(chan struct{})
	ctx, cancelSubmit := context.WithCancel(context.Background())
	go func() {
		defer close(done)
		defer cancelSubmit()
		if result3, err := batcher.Submit(ctx, 3); err != context.Canceled || result3 != nil {
			t.Error(result3, err)
		}
	}()

	time.Sleep(time.Millisecond * 300)
	select {
	case <-done:
		t.Fatal(`expected third job to be blocked on Submit`)
	default:
	}

	shutdownCtx, cancelShutdown := context.WithCancel(context.Background())
	cancelShutdown()
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- batcher.Shutdown(shutdownCtx) }()

	select {
	case err := <-shutdownDone:
		if err != context.Canceled {
			t.Fatalf("Shutdown = %v, want %v", err, context.Canceled)
		}
	case <-time.After(time.Second):
		t.Fatal("Shutdown with an already-canceled context did not return promptly")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Shutdown did not unblock pending Submit")
	}

	processorOut <- errors.New(`some error`)
	select {
	case args := <-processorIn:
		if args.ctx.Err() == nil {
			t.Error("queued job started without canceled batcher context")
		}
		if !reflect.DeepEqual(args.jobs, []any{2}) {
			t.Errorf(`expected jobs to be [2], got %v`, args.jobs)
		}
		processorOut <- errors.New(`some other error`)
	case <-batcher.done:
	}

	select {
	case <-batcher.done:
	case <-time.After(time.Second):
		t.Fatal("batcher did not finish after releasing blocked processor")
	}
}

func TestBatcher_Shutdown_ContextCanceledWhileProcessorStuckReturnsPromptly(t *testing.T) {
	defer checkNumGoroutines(time.Second * 3)(t)

	processorStarted := make(chan struct{})
	releaseProcessor := make(chan struct{})
	batcher := NewBatcher(&BatcherConfig{MaxSize: 1, FlushInterval: -1}, func(ctx context.Context, jobs []any) error {
		close(processorStarted)
		<-releaseProcessor
		return ctx.Err()
	})

	if result, err := batcher.Submit(context.Background(), 1); err != nil || result == nil {
		t.Fatalf("Submit: result=%v err=%v", result, err)
	}
	select {
	case <-processorStarted:
	case <-time.After(time.Second):
		t.Fatal("processor did not start")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- batcher.Shutdown(ctx) }()
	select {
	case err := <-shutdownDone:
		if err != context.Canceled {
			t.Fatalf("Shutdown = %v, want %v", err, context.Canceled)
		}
	case <-time.After(time.Second):
		close(releaseProcessor)
		t.Fatal("Shutdown waited for a stuck processor despite canceled context")
	}

	close(releaseProcessor)
	select {
	case <-batcher.done:
	case <-time.After(time.Second):
		t.Fatal("batcher did not finish after processor release")
	}
}

// this is effectively identical to calling Shutdown with a canceled context
func TestBatcher_Close_jobInProgress(t *testing.T) {
	testShutdownCloseJobInProgress(t, true, nil, func(batcher *Batcher[any]) error {
		return batcher.Close()
	})
}

func TestJobResult_Wait_contextCancel(t *testing.T) {
	result := JobResult[any]{batch: &batcherState[any]{}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := result.Wait(ctx); err != context.Canceled {
		t.Errorf(`expected context canceled, got %v`, err)
	}
}

// basic test to ensure it flushes after the interval as expected (testing timing is painful)
func TestBatcher_flushInterval(t *testing.T) {
	defer checkNumGoroutines(time.Second * 3)(t) // should always clean up

	processorIn := make(chan processorArgsAny)
	processorOut := make(chan error)

	const flushInterval = 200 * time.Millisecond

	batcher := NewBatcher(
		&BatcherConfig{MaxSize: -1, FlushInterval: flushInterval, MaxConcurrency: -1},
		func(ctx context.Context, jobs []any) error {
			processorIn <- processorArgsAny{ctx, jobs}
			return <-processorOut
		},
	)

	// submit 5 jobs, then wait for it to be flushed
	firstSubmitTime := time.Now()

	var jobs []*JobResult[any]
	for i := range 5 {
		result, err := batcher.Submit(context.Background(), i)
		if err != nil || result == nil || result.Job != i {
			t.Fatal(result, err)
		}
		jobs = append(jobs, result)
	}

	err := errors.New(`expected error`)

	// Wait for all submitted jobs to reach the processor. If an overloaded CI
	// host ever splits the work into more than one batch, keep draining and
	// releasing processors so JobResult.Wait cannot hang behind an unobserved
	// processor call.
	var batches []processorArgsAny
	for total := 0; total < len(jobs); {
		select {
		case args := <-processorIn:
			batches = append(batches, args)
			total += len(args.jobs)
			processorOut <- err
		case <-time.After(5 * time.Second):
			t.Fatalf(`timed out waiting for flushed jobs; got %d/%d`, total, len(jobs))
		}
	}
	if len(batches) != 1 || len(batches[0].jobs) != len(jobs) {
		gotLens := make([]int, len(batches))
		for i, batch := range batches {
			gotLens[i] = len(batch.jobs)
		}
		t.Errorf(`expected one batch with %d jobs, got batch sizes %v`, len(jobs), gotLens)
	}

	// ensure it took at least the expected time, but not too much longer
	if elapsed := time.Since(firstSubmitTime); elapsed < flushInterval*9/10 || elapsed > 5*time.Second {
		t.Errorf(`expected flush interval around %s, got %s`, flushInterval, elapsed)
	} else {
		t.Logf(`interval delta: %s`, elapsed-flushInterval)
	}

	// ensure all jobs are done, and have our expected error
	for _, job := range jobs {
		if e := job.Wait(context.Background()); e != err {
			t.Fatal(e)
		}
	}

	// close the batcher, for cleanup purposes
	if err := batcher.Close(); err != nil {
		t.Error(err)
	}
}
