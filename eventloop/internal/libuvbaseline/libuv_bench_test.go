//go:build cgo && libuv

package libuvbaseline

import (
	"runtime"
	"testing"
)

// BenchmarkLibuv_AsyncSend measures the raw overhead of waking up a libuv loop
// from a different thread using uv_async_send.  This exercises only the wakeup
// path — no timer scheduling or firing is involved.
//
// Fidelity note: this is analogous to a bare Submit() wakeup in eventloop, not
// to timer scheduling.  Compare against BenchmarkLibuv_TimerCrossThread for
// the timer-path analogue that includes a real kevent/epoll wakeup.
func BenchmarkLibuv_AsyncSend(b *testing.B) {
	loop := newLibuvLoop()

	done := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		loop.run()
		close(done)
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		loop.send()
	}
	b.StopTimer()

	loop.stop()
	<-done
}

// BenchmarkLibuv_TimerScheduleAndFire measures the cost of the libuv timer
// state machine for a single zero-delay one-shot timer: scheduling the timer,
// running the loop until the callback fires, then returning.
//
// Important fidelity constraint: because the delay is 0 ms, the timer is
// immediately due and libuv fires it inside uv__run_timers, which is called
// before the main while-loop body in uv_run(UV_RUN_DEFAULT).  The loop exits
// (via uv_stop in the callback) before uv__io_poll is ever reached, so
// kevent/epoll is NOT called.  The ~44 ns/op result reflects CGO call overhead
// plus libuv's timer heap operations, not any OS I/O polling cost.
//
// For a benchmark that does include a real kevent/epoll wakeup on every
// iteration, see BenchmarkLibuv_TimerCrossThread.
//
// Fidelity gaps vs the eventloop benchmark:
//   - No Go-level goroutine-ID check or quiescing overhead.
//   - No timer pool (the libuv timer handle is reused directly).
//   - No ref-counting, nesting-level clamping, or submissionEpoch accounting.
//   - uv__io_poll / kevent is not exercised (see above).
func BenchmarkLibuv_TimerScheduleAndFire(b *testing.B) {
	h := newLibuvOneShotTimer()
	defer h.destroy()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !h.run(0) {
			b.Fatal("libuv one-shot timer did not fire")
		}
	}
}

// BenchmarkLibuv_TimerRepeat measures the per-firing cost of dispatching 100
// simultaneous zero-delay one-shot timers in a single uv_run call.  By
// amortising the loop-entry overhead across 100 callbacks, the per-firing cost
// can be approximated by dividing the reported ns/op by 100.
//
// Important fidelity constraints:
//   - Despite the name, NO repeating timer is used.  Each of the 100 timers is
//     a one-shot timer (the repeat interval argument to uv_timer_start is 0).
//   - Because all timers are zero-delay, they are all due before uv__io_poll
//     is reached, so kevent/epoll is NOT called.
//   - The outer bench_repeat_run precondition requires target <= REPEAT_TIMERS
//     (100); passing a larger value causes uv_run to block indefinitely.
//
// Compare against BenchmarkTimerFire in the eventloop package.
func BenchmarkLibuv_TimerRepeat(b *testing.B) {
	const firingsPerOp = 100
	h := newLibuvRepeatTimer()
	defer h.destroy()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n := h.run(0, firingsPerOp)
		if n != firingsPerOp {
			b.Fatalf("expected %d firings, got %d", firingsPerOp, n)
		}
	}
}

// BenchmarkLibuv_TimerCrossThread measures the full round-trip cost of
// scheduling a zero-delay libuv timer from a separate goroutine and waiting
// for it to fire:
//
//	benchmark goroutine
//	  → uv_async_send (wakes loop thread via kevent/epoll)
//	  → crossthread_async_cb on loop thread schedules zero-delay timer
//	  → crossthread_timer_cb fires, posts semaphore
//	  → benchmark goroutine unblocks
//
// Unlike BenchmarkLibuv_TimerScheduleAndFire, the loop runs continuously on
// a dedicated OS thread.  Because the timer is inactive and no async signal is
// pending after each timer fires, the loop typically enters uv__io_poll
// waiting for the next wakeup; subsequent iterations usually require a real
// kevent/epoll wakeup.  No readiness barrier synchronises the benchmark
// goroutine with the loop thread actually blocking in uv__io_poll, so the
// first few iterations may not require a poll wake.  This makes it the closest
// structural analogue to BenchmarkScheduleTimerWithPool_Immediate in the
// eventloop package.
//
// Fidelity gaps vs the eventloop benchmark:
//   - No Go-level goroutine-ID check or quiescing overhead.
//   - No timer pool; the libuv timer handle is pre-allocated per harness.
//   - No ref-counting, nesting-level clamping, or submissionEpoch accounting.
//   - The async→timer indirection adds one extra async dispatch not present
//     in eventloop's direct ScheduleTimer path.
func BenchmarkLibuv_TimerCrossThread(b *testing.B) {
	h := newLibuvCrossThreadTimer()

	done := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		h.runLoop()
		close(done)
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.send()
	}
	b.StopTimer()

	h.stop()
	<-done
}
