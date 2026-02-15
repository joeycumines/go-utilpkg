package microbatch_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/joeycumines/go-microbatch"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// Demonstrates how Batcher can be used to model logically-independent
// operations, that are batched together for efficiency.
func ExampleBatcher_independentOperations() {
	// this can be any type you wish, though references are necessary to pass on results
	type Job struct {
		Callback func(ctx context.Context) (int, error)
		Result   int
		Err      error
	}

	// note: see the example output for the final value (implementation irrelevant to the example)
	mu, maxRunningBatchProcessorCount, incDecBatchProcessorCount := newMaxRunningTracker()

	// the default batching behavior has "sensible" defaults, but may not be suitable for all use cases
	// (e.g. you may want to allow more than one concurrent batch, as in this example, which allows 10)
	batcher := microbatch.NewBatcher(&microbatch.BatcherConfig{MaxConcurrency: 10}, func(ctx context.Context, jobs []*Job) error {
		defer incDecBatchProcessorCount()() // not relevant - you can ignore this

		// this guard is optional, but a good idea to ensure Batcher.Close exits promptly
		if err := ctx.Err(); err != nil {
			return err
		}

		// in practice, this might involve calling out to a remote service
		for _, job := range jobs {
			// any potentially long-running operation should accept and respect the context
			job.Result, job.Err = job.Callback(ctx)
		}

		time.Sleep(time.Millisecond * 20) // simulating load

		return nil
	})
	defer batcher.Close() // always remember to close the Batcher

	// for the sake of this example, we just run a bunch of operations, concurrently

	const (
		numOpsPerWorker = 1000
		numWorkers      = 5
		numOps          = numOpsPerWorker * numWorkers
	)

	var submitWg sync.WaitGroup
	submitWg.Add(numOps)
	var resultWg sync.WaitGroup
	resultWg.Add(numOps)

	var callbackCount int64
	submit := func() {
		defer submitWg.Done() // note: (potentially) prior to the result being available
		succeed := rand.Intn(2) == 0
		expectedResult := rand.Int()
		var expectedErr error
		if !succeed {
			expectedErr = errors.New("expected error")
		}
		result, err := batcher.Submit(context.Background(), &Job{
			Callback: func(ctx context.Context) (int, error) {
				atomic.AddInt64(&callbackCount, 1)
				return expectedResult, expectedErr
			},
		})
		if err != nil {
			panic(err)
		}
		go func() {
			defer resultWg.Done()
			if err := result.Wait(context.Background()); err != nil {
				panic(err)
			}
			if result.Job.Result != expectedResult {
				panic(fmt.Sprintf("expected %d, got %d", expectedResult, result.Job.Result))
			}
			if result.Job.Err != expectedErr {
				panic(fmt.Sprintf("expected %v, got %v", expectedErr, result.Job.Err))
			}
		}()
	}

	for range numWorkers {
		go func() {
			for range numOpsPerWorker {
				submit()
			}
		}()
	}

	submitWg.Wait() // waits for all jobs to be submitted

	// note shutdown guarantees close (by the time it exits) but doesn't cancel
	// the batcher's context until/unless its context is canceled
	if err := batcher.Shutdown(context.Background()); err != nil {
		panic(err)
	}

	resultWg.Wait() // wait for all results to be available (and checked)

	mu.Lock()
	defer mu.Unlock()

	fmt.Println(`total number of callback calls:`, atomic.LoadInt64(&callbackCount))
	fmt.Println(`max number of concurrent batch processors:`, *maxRunningBatchProcessorCount)

	//output:
	//total number of callback calls: 5000
	//max number of concurrent batch processors: 10
}

// Demonstrates the basic pattern, in a scenario where individual job results
// are unnecessary.
func ExampleBatcher_bulkInsert() {
	const batchSize = 3
	batcher := microbatch.NewBatcher(&microbatch.BatcherConfig{
		MaxSize:       batchSize, // small batch size, for demonstrative purposes
		FlushInterval: -1,        // (DON'T DO THIS FOR ACTUAL USE) disable flush interval, to make the output stable
	}, func(ctx context.Context, rows [][]any) error {
		if ctx.Err() != nil {
			panic(`expected ctx not to be canceled`)
		}
		// in practice, this would be interacting with your chosen database
		fmt.Printf("Inserted %d rows:\n", len(rows))
		for _, row := range rows {
			b, _ := json.Marshal(row)
			fmt.Printf("%s\n", b)
			// you wouldn't normally do this - this is for the benefit of the test suite being useful for race detection
			row[0] = nil
		}
		return nil
	})
	defer batcher.Close()

	// the following is very contrived - in practice you would simply call
	// batcher.Submit to insert the row, then wait for the result

	rows := make([][]any, batchSize*5+2)

	var wg sync.WaitGroup
	wg.Add(len(rows))

	for i := 0; i < len(rows); i += batchSize {
		for j := 0; j < batchSize && i+j < len(rows); j++ {
			row := [1]any{fmt.Sprintf("row %d", i+j)}
			rows[i+j] = row[:]

			result, err := batcher.Submit(context.Background(), rows[i+j])
			if err != nil {
				panic(err)
			}
			if (*[1]any)(result.Job) != &row {
				panic(result.Job)
			}

			go func() {
				if err := result.Wait(context.Background()); err != nil {
					panic(err)
				}
				if row[0] != nil {
					panic(`expected the value to be cleared`)
				}
				wg.Done()
			}()
		}
	}

	// will send the last, partial batch
	if err := batcher.Shutdown(context.Background()); err != nil {
		panic(err)
	}

	wg.Wait()

	//output:
	//Inserted 3 rows:
	//["row 0"]
	//["row 1"]
	//["row 2"]
	//Inserted 3 rows:
	//["row 3"]
	//["row 4"]
	//["row 5"]
	//Inserted 3 rows:
	//["row 6"]
	//["row 7"]
	//["row 8"]
	//Inserted 3 rows:
	//["row 9"]
	//["row 10"]
	//["row 11"]
	//Inserted 3 rows:
	//["row 12"]
	//["row 13"]
	//["row 14"]
	//Inserted 2 rows:
	//["row 15"]
	//["row 16"]
}

func newMaxRunningTracker() (*sync.Mutex, *int, func() func()) {
	var (
		mu                            sync.Mutex
		runningBatchProcessorCount    int
		maxRunningBatchProcessorCount int
		incDecBatchProcessorCount     = func() func() {
			mu.Lock()
			runningBatchProcessorCount++
			if runningBatchProcessorCount > maxRunningBatchProcessorCount {
				maxRunningBatchProcessorCount = runningBatchProcessorCount
			}
			mu.Unlock()
			return func() {
				mu.Lock()
				runningBatchProcessorCount--
				mu.Unlock()
			}
		}
	)
	return &mu, &maxRunningBatchProcessorCount, incDecBatchProcessorCount
}
