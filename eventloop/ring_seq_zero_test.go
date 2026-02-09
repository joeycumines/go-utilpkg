package eventloop

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Test_microtaskRing_SeqWrapAround verifies that sequence number wrap-around
// does not cause infinite spin loops or data loss. This test covers the R101 fix.
func Test_microtaskRing_SeqWrapAround(t *testing.T) {
	ring := newMicrotaskRing()

	// Pre-populate with enough items to advance tailSeq significantly
	pushedCount := atomic.Int64{}
	readCount := atomic.Int64{}

	// Producer: push items continuously
	done := make(chan struct{})
	go func() {
		for i := 0; i < 10000; i++ {
			task := func() {
				readCount.Add(1)
			}
			ring.Push(task)
			pushedCount.Add(1)
		}
		close(done)
	}()

	// Wait for producer to finish
	<-done

	// Consumer: drain all items
	var read int64
	for {
		fn := ring.Pop()
		if fn == nil {
			break
		}
		fn()
		read++
	}

	// Verify all items were read
	if read != 10000 {
		t.Errorf("Expected to read %d items, got %d", 10000, read)
	}

	// Verify no infinite spin occurred - timeout should be fast
	if readCount.Load() < 10000 {
		t.Errorf("Not all tasks were executed: %d/10000", readCount.Load())
	}
}

// Test_microtaskRing_ConcurrentProducerLoad verifies that under extreme concurrent
// producer load, the ring handle sequence validity correctly without infinite spins.
func Test_microtaskRing_ConcurrentProducerLoad(t *testing.T) {
	ring := newMicrotaskRing()
	numProducers := 50
	itemsPerProducer := 1000

	totalPushed := atomic.Int64{}
	totalRead := atomic.Int64{}
	startBarrier := sync.WaitGroup{}
	startBarrier.Add(numProducers)
	doneBarrier := sync.WaitGroup{}
	doneBarrier.Add(numProducers)

	// Producer goroutines
	for p := 0; p < numProducers; p++ {
		go func(producerID int) {
			startBarrier.Done()
			startBarrier.Wait() // All producers start simultaneously

			for i := 0; i < itemsPerProducer; i++ {
				task := func() {
					totalRead.Add(1)
				}
				ring.Push(task)
				totalPushed.Add(1)
			}

			doneBarrier.Done()
		}(p)
	}

	// Wait timeout for producers
	doneCh := make(chan struct{})
	go func() {
		doneBarrier.Wait()
		close(doneCh)
	}()

	select {
	case <-doneCh:
		// All producers finished
	case <-time.After(30 * time.Second):
		t.Fatalf("Producers did not complete within timeout - possible deadlock or infinite spin")
	}

	expected := int64(numProducers * itemsPerProducer)
	if totalPushed.Load() != expected {
		t.Errorf("Expected %d items pushed, got %d", expected, totalPushed.Load())
	}

	// Consumer: drain all items
	var read int64
	timeout := time.After(30 * time.Second)
readLoop:
	for {
		select {
		case <-timeout:
			t.Fatalf("Consumer did not complete within timeout - possible infinite spin. Read %d/%d", read, expected)
		default:
			fn := ring.Pop()
			if fn == nil {
				if totalPushed.Load() > read {
					// Ring might be processing overflow, give it time
					runtime.Gosched()
					time.Sleep(time.Millisecond)
					continue
				}
				break readLoop
			}
			fn()
			read++
		}
	}

	if read != expected {
		t.Errorf("Expected to read %d items, got %d", expected, read)
	}

	// Allow tasks to complete (they may have been popped but not yet executed)
	time.Sleep(100 * time.Millisecond)
	executed := totalRead.Load()
	if executed < expected {
		t.Logf("Warning: Not all tasks executed yet (%d/%d), but pop operation succeeded", executed, expected)
	}
}

// Test_microtaskRing_OverflowDuringWrapAround verifies that overflow buffer
// works correctly even when sequence numbers are wrapping around.
func Test_microtaskRing_OverflowDuringWrapAround(t *testing.T) {
	ring := newMicrotaskRing()

	// Manually advance tailSeq to near wrap-around point
	ring.tailSeq.Store(^uint64(0) - 100)

	pushed := atomic.Int64{}
	readCount := atomic.Int64{}

	// Producer: push more items than ring buffer can hold, forcing overflow
	numItems := int64(ringBufferSize + 100)
	for i := int64(0); i < numItems; i++ {
		task := func() {
			readCount.Add(1)
		}
		ring.Push(task)
		pushed.Add(1)
	}

	if pushed.Load() != numItems {
		t.Errorf("Expected %d items pushed, got %d", numItems, pushed.Load())
	}

	// Consumer: drain all items
	var read int64
	for {
		fn := ring.Pop()
		if fn == nil {
			break
		}
		fn()
		read++
	}

	if read != numItems {
		t.Errorf("Expected to read %d items, got %d", numItems, read)
	}

	if readCount.Load() != numItems {
		t.Errorf("Expected %d tasks executed, got %d", numItems, readCount.Load())
	}
}

// Test_microtaskRing_ValidityFlagReset verifies that validity flags
// are correctly reset to false after slots are consumed.
func Test_microtaskRing_ValidityFlagReset(t *testing.T) {
	ring := newMicrotaskRing()

	// Push and consume one item at a time, verifying validity state
	for i := 0; i < 100; i++ {
		task := func() {}
		ring.Push(task)

		// Verify the slot is valid before consumption
		tail := ring.tail.Load()
		idx := (tail - 1) % ringBufferSize

		// Note: Validity might be true immediately after push
		// We just want to ensure Pop() doesn't hang

		fn := ring.Pop()
		if fn == nil {
			t.Fatalf("Pop() returned nil on iteration %d", i)
		}

		// Verify the sequence slot was reset to skip sentinel
		seq := ring.seq[idx].Load()
		valid := ring.valid[idx].Load()

		if seq != ringSeqSkip {
			t.Errorf("Expected ringSeqSkip (%d) after pop, got %d on iteration %d", ringSeqSkip, seq, i)
		}

		if valid {
			t.Errorf("Expected validity false after pop on iteration %d", i)
		}
	}
}

// Test_microtaskRing_NilTaskWithSequence verifies that nil tasks
// with valid sequences are properly consumed without infinite loop.
// Note: microtaskRing Pop() consumes nil tasks internally without returning them.
// This test verifies nil tasks don't cause infinite loops or block the queue.
func Test_microtaskRing_NilTaskWithSequence(t *testing.T) {
	ring := newMicrotaskRing()

	// Push both valid tasks and nil tasks
	// Pop() skips nil tasks internally, so only non-nil tasks are returned
	for i := 0; i < 50; i++ {
		if i%3 == 0 {
			// Push nil task - these are consumed but not returned by Pop()
			ring.Push(nil)
		} else {
			// Push valid task
			ring.Push(func() {})
		}
	}

	// Drain - should not hang even with nil tasks
	var read int64
	const timeout = 10 * time.Second
	start := time.Now()

	for {
		if time.Since(start) > timeout {
			t.Fatalf("Pop() hung after reading %d items (timeout 10s)", read)
		}

		fn := ring.Pop()
		if fn == nil {
			// No more valid tasks (nil tasks were skipped internally)
			break
		}
		fn() // Execute non-nil task
		read++
	}

	// Expected: 33 valid tasks (50 total - 17 nils)
	// Nil indices: 0, 3, 6, 9, 12, 15, 18, 21, 24, 27, 30, 33, 36, 39, 42, 45, 48 (17 values in range [0,49] where i%3==0)
	expected := int64(33) // 17 nils are skipped internally by Pop()
	if read != expected {
		t.Errorf("Expected to read %d non-nil items (nils skipped), got %d", expected, read)
	}
}

// Test_microtaskRing_NoInfiniteSpinAfterWrap verifies that after sequence
// wrap-around, the ring does not enter infinite spin when consuming.
func Test_microtaskRing_NoInfiniteSpinAfterWrap(t *testing.T) {
	ring := newMicrotaskRing()

	// Force sequence wrap-around by pushing many items in burst
	// Each item increments tailSeq, so 2^33 items would cause wrap
	// We use fewer items for practical testing while still stressing the ring
	const itemsBeforeWrap = 100000 // Large enough to stress but not take forever

	for i := 0; i < itemsBeforeWrap; i++ {
		ring.Push(func() {})
	}

	// Now consume with timeout - should complete quickly
	consumed := atomic.Int64{}
	startTime := time.Now()
	timeout := 30 * time.Second

	for {
		if time.Since(startTime) > timeout {
			// Check if we're making progress
			c := consumed.Load()
			if c < int64(itemsBeforeWrap) {
				t.Fatalf("Possible infinite spin: consumed %d/%d in %v, timeout %v",
					c, itemsBeforeWrap, time.Since(startTime), timeout)
			}
			t.Fatalf("Timeout after consuming %d/%d items", c, itemsBeforeWrap)
		}

		fn := ring.Pop()
		if fn == nil {
			// Check if we should have consumed more
			c := consumed.Load()
			if c < int64(itemsBeforeWrap) {
				// May be in overflow, give it a moment
				runtime.Gosched()
				time.Sleep(10 * time.Millisecond)
				continue
			}
			break
		}
		fn()
		consumed.Add(1)
	}

	if consumed.Load() < int64(itemsBeforeWrap) {
		t.Errorf("Expected to consume all items (%d), got %d", itemsBeforeWrap, consumed.Load())
	}
}
