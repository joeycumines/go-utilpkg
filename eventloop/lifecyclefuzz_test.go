package eventloop

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func FuzzLoopConcurrentLifecycleNoDeadlock(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
	f.Add([]byte("submit-microtask-timer-shutdown-close-wake"))

	f.Fuzz(func(t *testing.T, data []byte) {
		r := newFuzzReader(data)
		loop := New()
		registerLoopCleanupT(t, loop)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		runDone := make(chan error, 1)
		go func() { runDone <- loop.Run(ctx) }()
		waitLoopOwnerTurnT(t, loop)

		var callbackErrs fuzzErrs
		var timerMu sync.Mutex
		var timerIDs []TimerID
		recordErr := func(op string, err error) {
			if err == nil || errors.Is(err, ErrLoopTerminated) || errors.Is(err, ErrTimerNotFound) {
				return
			}
			callbackErrs.add("%s returned unexpected error: %v", op, err)
		}
		pickTimer := func(param uint64) TimerID {
			timerMu.Lock()
			defer timerMu.Unlock()
			if len(timerIDs) == 0 || param%4 == 0 {
				return TimerID(param%64 + 1)
			}
			return timerIDs[int(param%uint64(len(timerIDs)))]
		}
		rememberTimer := func(id TimerID) {
			if id == 0 {
				return
			}
			timerMu.Lock()
			timerIDs = append(timerIDs, id)
			timerMu.Unlock()
		}

		type lifecycleOp struct {
			op    byte
			param uint64
		}
		ops := make([]lifecycleOp, 1+min(len(data), 32))
		for i := range ops {
			ops[i] = lifecycleOp{op: r.byte() % 10, param: r.uint64()}
		}
		var wg sync.WaitGroup
		for i, planned := range ops {
			wg.Add(1)
			go func(i int, planned lifecycleOp) {
				defer wg.Done()
				fn := func() {}
				switch planned.op {
				case 0:
					recordErr("Submit", loop.Submit(fn))
				case 1:
					recordErr("SubmitInternal", loop.SubmitInternal(fn))
				case 2:
					recordErr("ScheduleMicrotask", loop.ScheduleMicrotask(fn))
				case 3:
					recordErr("ScheduleNextTick", loop.ScheduleNextTick(fn))
				case 4:
					recordErr("ScheduleImmediate", loop.ScheduleImmediate(fn))
				case 5:
					recordErr("ScheduleCloseCallback", loop.ScheduleCloseCallback(fn))
				case 6:
					id, err := loop.ScheduleTimer(time.Duration(planned.param%3)*time.Millisecond, fn)
					recordErr("ScheduleTimer", err)
					rememberTimer(id)
				case 7:
					recordErr("CancelTimer", loop.CancelTimer(pickTimer(planned.param)))
				case 8:
					if planned.param&1 == 0 {
						recordErr("RefTimer", loop.RefTimer(pickTimer(planned.param)))
					} else {
						recordErr("UnrefTimer", loop.UnrefTimer(pickTimer(planned.param)))
					}
				case 9:
					recordErr("Wake", loop.Wake())
				}
			}(i, planned)
		}
		operationsDone := make(chan struct{})
		go func() {
			wg.Wait()
			close(operationsDone)
		}()
		waitContractSignal(t, operationsDone, "concurrent lifecycle operations")

		terminalOperationCount := 2 + int(r.byte()%3)
		var terminalWG sync.WaitGroup
		for range terminalOperationCount {
			operation := r.byte() % 3
			terminalWG.Go(func() {
				switch operation {
				case 0:
					shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
					defer shutdownCancel()
					err := loop.Shutdown(shutdownCtx)
					if err != nil && !errors.Is(err, ErrLoopTerminated) && !errors.Is(err, context.DeadlineExceeded) {
						callbackErrs.add("Shutdown returned unexpected error: %v", err)
					}
				case 1:
					if err := loop.Close(); err != nil && !errors.Is(err, ErrLoopTerminated) {
						callbackErrs.add("Close returned unexpected error: %v", err)
					}
				case 2:
					cancel()
				}
			})
		}
		terminalOperationsDone := make(chan struct{})
		go func() {
			terminalWG.Wait()
			close(terminalOperationsDone)
		}()
		waitContractSignal(t, terminalOperationsDone, "concurrent terminal operations")

		if err := waitContractValue(t, runDone, "fuzz Run completion"); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, ErrLoopTerminated) {
			t.Fatalf("Run returned unexpected error: %v", err)
		}
		callbackErrs.failNow(t)
	})
}
