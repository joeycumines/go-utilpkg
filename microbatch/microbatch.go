// Package microbatch groups tasks into small batches, e.g. to reduce the
// number of round trips.
package microbatch

import (
	"context"
	"errors"
	"sync"
	"time"
)

type (
	// BatcherConfig models optional configuration, for NewBatcher.
	BatcherConfig struct {
		// MaxSize restricts the maximum number of jobs per batch, if positive.
		// **Defaults to 16, if 0, or BatcherConfig is nil.**
		//
		// WARNING: NewBatcher will panic if both MaxSize and FlushInterval are
		// disabled.
		MaxSize int

		// FlushInterval specifies the maximum duration before an "incomplete"
		// batch is passed to the BatchProcessor, if positive.
		// **Defaults to 50ms, if 0, or BatcherConfig is nil.**
		// If MaxSize is specified, time-based flushing can be disabled, by
		// setting this <= 0.
		//
		// WARNING: NewBatcher will panic if both MaxSize and FlushInterval are
		// disabled.
		FlushInterval time.Duration

		// MaxConcurrency specifies the maximum number of concurrent
		// BatchProcessor calls, able to be made by the Batcher, if positive.
		// **Defaults to 1, if 0, or BatcherConfig is nil.**
		MaxConcurrency int
	}

	// BatchProcessor runs jobs, using arbitrary behavior. Individual job
	// results (etc) should be assigned to the jobs themselves. Any returned
	// error will be propagated via JobResult.Wait.
	BatchProcessor[Job any] func(ctx context.Context, jobs []Job) error

	// Batcher accepts jobs, batching them into small groups.
	// Instances must be initialized using the NewBatcher factory.
	Batcher[Job any] struct {
		// betteralign:ignore

		processor      BatchProcessor[Job] // configurable
		maxSize        int                 // configurable
		flushInterval  time.Duration       // configurable
		maxConcurrency int                 // configurable
		ctx            context.Context
		cancel         context.CancelFunc
		done           chan struct{}
		stopped        chan struct{}
		stopOnce       sync.Once
		jobCh          chan Job                // sent on Submit (ping)
		batchCh        chan *batcherState[Job] // received on Submit (pong)
		state          *batcherState[Job]      // pending batch, also used for result
	}

	// batcherState models a pending batch / invocation
	batcherState[Job any] struct {
		err  error
		done chan struct{}
		jobs []Job
	}

	// JobResult models a scheduled job, providing a Wait method that should
	// be called prior to accessing any output/result, which the BatchProcessor
	// may set on the Job.
	//
	// WARNING: The actual value of the Job field will not be modified, meaning
	// any return values from BatchProcessor must be by references available
	// via the Job value.
	JobResult[Job any] struct {
		// Job is the pending job.
		//
		// WARNING: Consider that it may be accessed by the batch processor -
		// consider the implications, e.g. race conditions, if interacting with
		// internal state.
		Job Job

		// only done is allowed to be accessed, until done
		batch *batcherState[Job]
	}
)

// NewBatcher initializes a new Batcher, using the provided BatcherConfig and
// BatchProcessor. The provided config may be nil. A panic will occur if
// processor is nil, or invalid config is provided.
//
// The Batcher.Close method and/or Batcher.Shutdown method should be called
// when the Batcher is no longer needed.
func NewBatcher[Job any](config *BatcherConfig, processor BatchProcessor[Job]) *Batcher[Job] {
	if processor == nil {
		panic(`microbatch: nil processor`)
	}

	batcher := Batcher[Job]{
		processor:      processor,
		maxSize:        16,
		flushInterval:  time.Millisecond * 50,
		maxConcurrency: 1,
		state:          newBatcherState[Job](),
		done:           make(chan struct{}),
		stopped:        make(chan struct{}),
		jobCh:          make(chan Job),
		batchCh:        make(chan *batcherState[Job]),
	}

	if config != nil {
		if config.MaxSize != 0 {
			batcher.maxSize = config.MaxSize
		}
		if config.FlushInterval != 0 {
			batcher.flushInterval = config.FlushInterval
		}
		if config.MaxConcurrency != 0 {
			batcher.maxConcurrency = config.MaxConcurrency
		}
	}

	if batcher.flushInterval <= 0 && batcher.maxSize <= 0 {
		panic(`microbatch: one of MaxSize or FlushInterval must be specified`)
	}

	batcher.ctx, batcher.cancel = context.WithCancel(context.Background())

	go batcher.run()

	return &batcher
}

// Shutdown will immediately prevent further jobs via Submit, then wait for
// all already running or scheduled jobs to complete. An error will be returned
// if ctx is canceled prior to this, causing a forced Close.
//
// This method is unsafe to call from within a job or BatchProcessor.
func (x *Batcher[Job]) Shutdown(ctx context.Context) (err error) {
	x.stop()

	select {
	case <-ctx.Done():
		if x.ctx.Err() == nil {
			err = ctx.Err() // indicating we forcibly closed
		}
		x.cancel()
		<-x.done
	case <-x.done:
	}

	return err
}

// Close immediately cancels all jobs, and prevents further jobs via Submit,
// blocking until the Batcher has finished closing.
//
// This method is unsafe to call from within a job or BatchProcessor.
func (x *Batcher[Job]) Close() error {
	x.cancel()
	<-x.done
	return nil
}

// Submit schedules a job for processing, returning an error if ctx is
// canceled, or the Batcher is stopped.
//
// The JobResult.Wait method should be used to wait for the job's completion,
// after which any individual job result(s) may be accessed, on the job itself.
// The job is available via JobResult.Job, for convenience.
func (x *Batcher[Job]) Submit(ctx context.Context, job Job) (*JobResult[Job], error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := x.ctx.Err(); err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()

	case <-x.ctx.Done():
		return nil, x.ctx.Err()

	case <-x.stopped:
		return nil, context.Canceled

	case x.jobCh <- job: // ping
		batch := <-x.batchCh // pong
		return &JobResult[Job]{Job: job, batch: batch}, nil
	}
}

func (x *Batcher[Job]) stop() {
	x.stopOnce.Do(func() {
		close(x.stopped)
	})
}

func (x *Batcher[Job]) run() {
	defer close(x.done)
	defer x.cancel()

	var wg sync.WaitGroup
	wg.Add(1) // decremented on exit

	var runningBatchCh chan struct{} // keeps track of running batches, allows waiting for them
	if x.maxConcurrency > 0 {
		runningBatchCh = make(chan struct{}, x.maxConcurrency)
	}

	// runs the next batch, blocking on max concurrency limiting
	runBatch := func() {
		if len(x.state.jobs) == 0 {
			return
		}

		batch := x.state
		x.state = newBatcherState[Job]()

		wg.Add(1)
		if runningBatchCh != nil {
			runningBatchCh <- struct{}{} // note: relies on the batch processor handling cancel
		}
		go func() {
			defer func() {
				if runningBatchCh != nil {
					<-runningBatchCh
				}
				wg.Done()
			}()
			_ = batch.run(x.ctx, x.processor)
		}()
	}

	// finalizes the last batch, and waits for all batches
	var wait func()
	wait = func() {
		wait = nil
		runBatch()
		wg.Done()
		wg.Wait()
	}

	defer func() {
		// cancel before waiting (unless wait has already been called)
		x.cancel()
		if wait != nil {
			wait()
		}
	}()

	// sent batches once their flush interval expires
	flushCh := make(chan *batcherState[Job])

	for {
		select {
		case <-x.ctx.Done():
			return

		case <-x.stopped:
			// note: there won't be any more jobs coming
			wait()
			return

		case job := <-x.jobCh: // ping
			x.batchCh <- x.state // pong

			x.state.jobs = append(x.state.jobs, job)

			if x.maxSize > 0 && len(x.state.jobs) >= x.maxSize {
				runBatch()
			} else if x.flushInterval > 0 && len(x.state.jobs) == 1 {
				// first job -> start the timer for flush
				batch := x.state
				timer := time.NewTimer(x.flushInterval)
				go func() {
					defer timer.Stop()
					select {
					case <-x.ctx.Done():
					case <-x.stopped:
					case <-batch.done:
					case <-timer.C:
						select {
						case <-x.ctx.Done():
						case <-x.stopped:
						case <-batch.done:
						case flushCh <- batch:
						}
					}
				}()
			}

		case batch := <-flushCh:
			if batch == x.state {
				runBatch()
			}
		}
	}
}

func newBatcherState[Job any]() *batcherState[Job] {
	return &batcherState[Job]{done: make(chan struct{})}
}

func (x *batcherState[Job]) run(ctx context.Context, processor BatchProcessor[Job]) error {
	// nice to make sure the context is cancelled right after processor exists
	// (helps deal with accidental resource leaks in external impl.)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	x.err = errors.New(`microbatch: panic in BatchProcessor`)
	defer close(x.done)

	x.err = processor(ctx, x.jobs)

	return x.err
}

// Wait for the Job to be processed. If the BatchProcessor failed with an
// error, that error will be returned. Handling of any implementation-specific
// behavior is via the JobResult.Job field.
func (x *JobResult[Job]) Wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()

	case <-x.batch.done:
		return x.batch.err
	}
}
