//go:build cgo && libuv

package libuvbaseline

// #cgo pkg-config: libuv
// #include <uv.h>
// #include <stdlib.h>
// #include <string.h>
//
// // ── Async wakeup harness (original) ─────────────────────────────────────────
//
// typedef struct {
//     uv_loop_t loop;
//     uv_async_t async;
//     uv_mutex_t mutex;
//     uv_sem_t ack_sem;
//     int stop_flag;
// } bench_loop_t;
//
// static void async_cb(uv_async_t* handle) {
//     bench_loop_t* l = (bench_loop_t*)handle->data;
//     uv_mutex_lock(&l->mutex);
//     int stop = l->stop_flag;
//     uv_mutex_unlock(&l->mutex);
//
//     if (stop) {
//         uv_close((uv_handle_t*)&l->async, NULL);
//     } else {
//         uv_sem_post(&l->ack_sem);
//     }
// }
//
// static bench_loop_t* new_bench_loop() {
//     bench_loop_t* l = malloc(sizeof(bench_loop_t));
//     uv_loop_init(&l->loop);
//     uv_async_init(&l->loop, &l->async, async_cb);
//     uv_mutex_init(&l->mutex);
//     uv_sem_init(&l->ack_sem, 0);
//     l->async.data = l;
//     l->stop_flag = 0;
//     return l;
// }
//
// static void run_bench_loop(bench_loop_t* l) {
//     uv_run(&l->loop, UV_RUN_DEFAULT);
//     uv_run(&l->loop, UV_RUN_DEFAULT); /* drain close callbacks */
//     uv_loop_close(&l->loop);
//     uv_mutex_destroy(&l->mutex);
//     uv_sem_destroy(&l->ack_sem);
//     free(l);
// }
//
// static void stop_bench_loop(bench_loop_t* l) {
//     uv_mutex_lock(&l->mutex);
//     l->stop_flag = 1;
//     uv_mutex_unlock(&l->mutex);
//     uv_async_send(&l->async);
// }
//
// static void send_bench_loop(bench_loop_t* l) {
//     uv_async_send(&l->async);
//     uv_sem_wait(&l->ack_sem);
// }
//
// static void dummy_async_cb(uv_async_t* handle) {}
//
// // ── Single-shot timer harness ────────────────────────────────────────────────
// //
// // Measures the round-trip cost of scheduling a zero-delay timer, running the
// // loop until it fires, then stopping.  This is the closest libuv analogue to
// // BenchmarkScheduleTimerWithPool_Immediate in the eventloop package.
// //
// // Handle lifecycle:
// //   create → [timer_start + run]* → destroy
// //
// // uv_timer_start() may be called repeatedly on an already-initialised,
// // stopped handle without re-initialising it.  The destroy path must
// // uv_close() the handle and drain the close queue before uv_loop_close()
// // because uv_loop_close() returns UV_EBUSY when any open handles remain.
//
// typedef struct {
//     uv_loop_t   loop;
//     uv_timer_t  timer;
//     uv_async_t  dummy_async;
//     int         fired;
// } bench_oneshot_t;
//
// static void oneshot_timer_cb(uv_timer_t* handle) {
//     bench_oneshot_t* h = (bench_oneshot_t*)handle->data;
//     h->fired = 1;
//     uv_timer_stop(handle);
//     uv_stop(&h->loop);
// }
//
// // bench_oneshot_create allocates and initialises a one-shot timer harness.
// static bench_oneshot_t* bench_oneshot_create() {
//     bench_oneshot_t* h = malloc(sizeof(bench_oneshot_t));
//     memset(h, 0, sizeof(*h));
//     uv_loop_init(&h->loop);
//     uv_async_init(&h->loop, &h->dummy_async, dummy_async_cb);
//     uv_timer_init(&h->loop, &h->timer);
//     h->timer.data = h;
//     return h;
// }
//
// // bench_oneshot_run schedules a timer with the given delay_ms, drives the loop
// // until the timer fires, then returns.  The harness may be reused across calls.
// // Returns 1 if the timer fired, 0 on error.
// static int bench_oneshot_run(bench_oneshot_t* h, uint64_t delay_ms) {
//     h->fired = 0;
//     if (uv_timer_start(&h->timer, oneshot_timer_cb, delay_ms, 0) != 0)
//         return 0;
//     uv_run(&h->loop, UV_RUN_DEFAULT);
//     return h->fired;
// }
//
// // bench_oneshot_destroy closes the timer handle, drains the close queue via
// // a uv_run pass, then closes and frees the loop.
// //
// // uv_close() is asynchronous: the handle's close callback fires on the next
// // loop iteration.  uv_loop_close() returns UV_EBUSY if any handles remain
// // open, so we must drain the close queue before calling it.
// static void bench_oneshot_destroy(bench_oneshot_t* h) {
//     uv_close((uv_handle_t*)&h->timer, NULL);
//     uv_close((uv_handle_t*)&h->dummy_async, NULL);
//     uv_run(&h->loop, UV_RUN_DEFAULT); /* drain close callbacks */
//     uv_loop_close(&h->loop);
//     free(h);
// }
//
// // ── Batched one-shot timer harness ──────────────────────────────────────────
// //
// // Starts up to REPEAT_TIMERS simultaneous zero-delay one-shot timers in a
// // single uv_run call and counts how many fire.  Despite the struct/parameter
// // names containing "repeat", NO repeating timer is used: the fourth argument
// // to uv_timer_start (the repeat interval) is always 0, making every timer
// // a one-shot timer.
// //
// // What this actually measures: the cost of scheduling N simultaneous timers
// // whose timeout has already elapsed, draining all of their callbacks in one
// // uv_run, and stopping the loop.  Like bench_oneshot_t, all timers are due
// // before uv__io_poll is reached, so kevent/epoll is never called.
// //
// // Precondition: target MUST be <= REPEAT_TIMERS.  If target exceeds
// // REPEAT_TIMERS, fewer timers are started than h->target requires, h->count
// // never reaches h->target, uv_stop is never called, and uv_run(DEFAULT)
// // blocks indefinitely (dummy_async keeps the loop alive).
// //
// // Same handle-lifecycle rules apply as for bench_oneshot_t.
//
// #define REPEAT_TIMERS 100
//
// typedef struct {
//     uv_loop_t  loop;
//     uv_timer_t timers[REPEAT_TIMERS];
//     uv_async_t dummy_async;
//     int        target;
//     int        count;
// } bench_repeat_t;
//
// static void repeat_timer_cb(uv_timer_t* handle) {
//     bench_repeat_t* h = (bench_repeat_t*)handle->data;
//     h->count++;
//     uv_timer_stop(handle);
//     if (h->count >= h->target) {
//         uv_stop(&h->loop);
//     }
// }
//
// static bench_repeat_t* bench_repeat_create() {
//     bench_repeat_t* h = malloc(sizeof(bench_repeat_t));
//     memset(h, 0, sizeof(*h));
//     uv_loop_init(&h->loop);
//     uv_async_init(&h->loop, &h->dummy_async, dummy_async_cb);
//     for (int i = 0; i < REPEAT_TIMERS; i++) {
//         uv_timer_init(&h->loop, &h->timers[i]);
//         h->timers[i].data = h;
//     }
//     return h;
// }
//
// // bench_repeat_run starts up to min(target, REPEAT_TIMERS) zero-delay
// // one-shot timers (timeout_ms sets the initial delay; repeat arg is always 0)
// // and drives the loop until all started timers have fired.  Returns the
// // observed firing count; callers should assert it equals target.
// //
// // PRECONDITION: target <= REPEAT_TIMERS.  Violating this causes uv_run to
// // block forever (see struct-level precondition comment above).
// static int bench_repeat_run(bench_repeat_t* h, uint64_t timeout_ms, int target) {
//     h->count  = 0;
//     h->target = target;
//     for (int i = 0; i < target && i < REPEAT_TIMERS; i++) {
//         if (uv_timer_start(&h->timers[i], repeat_timer_cb, timeout_ms, 0) != 0)
//             return -1;
//     }
//     uv_run(&h->loop, UV_RUN_DEFAULT);
//     return h->count;
// }
//
// // bench_repeat_destroy mirrors bench_oneshot_destroy: close the handle, drain
// // the close queue, then close and free the loop.
// static void bench_repeat_destroy(bench_repeat_t* h) {
//     for (int i = 0; i < REPEAT_TIMERS; i++) {
//         uv_close((uv_handle_t*)&h->timers[i], NULL);
//     }
//     uv_close((uv_handle_t*)&h->dummy_async, NULL);
//     uv_run(&h->loop, UV_RUN_DEFAULT); /* drain close callbacks */
//     uv_loop_close(&h->loop);
//     free(h);
// }
//
// // ── Cross-thread timer harness ───────────────────────────────────────────────
// //
// // Measures the full round-trip cost of scheduling a libuv timer from a
// // different thread via uv_async_send: benchmark goroutine → async_cb (loop
// // thread) → zero-delay uv_timer_start → timer_cb → semaphore post →
// // benchmark goroutine.
// //
// // Unlike bench_oneshot_t (which calls uv_run per iteration and fires the
// // zero-delay timer before uv__io_poll is reached), this harness keeps the
// // loop running on a dedicated OS thread.  Because the timer is inactive and
// // no async signal is pending after each timer fires, the loop typically
// // enters uv__io_poll waiting for the next signal — each round trip usually
// // includes a real kevent/epoll wakeup.  No readiness barrier synchronises
// // the benchmark goroutine with the loop thread actually blocking in
// // uv__io_poll, so the first few iterations may not require a poll wake.
// // This makes it the closest structural analogue to
// // BenchmarkScheduleTimerWithPool_Immediate in the eventloop package.
// //
// // Handle lifecycle:
// //   create → [run_loop on OS-locked thread] → [send × N] → stop → run_loop returns
//
// typedef struct {
//     uv_loop_t   loop;
//     uv_async_t  async;      /* benchmark→loop: "schedule a timer" or "stop" */
//     uv_timer_t  timer;
//     uv_sem_t    fired_sem;  /* loop→benchmark: "timer fired" */
//     uv_mutex_t  mutex;
//     int         stop_flag;
// } bench_crossthread_t;
//
// static void crossthread_timer_cb(uv_timer_t* handle) {
//     bench_crossthread_t* h = (bench_crossthread_t*)handle->data;
//     uv_timer_stop(handle);
//     uv_sem_post(&h->fired_sem);
// }
//
// static void crossthread_async_cb(uv_async_t* handle) {
//     bench_crossthread_t* h = (bench_crossthread_t*)handle->data;
//     uv_mutex_lock(&h->mutex);
//     int stop = h->stop_flag;
//     uv_mutex_unlock(&h->mutex);
//     if (stop) {
//         uv_close((uv_handle_t*)&h->timer, NULL);
//         uv_close((uv_handle_t*)&h->async, NULL);
//     } else {
//         uv_timer_start(&h->timer, crossthread_timer_cb, 0, 0);
//     }
// }
//
// static bench_crossthread_t* bench_crossthread_create() {
//     bench_crossthread_t* h = malloc(sizeof(bench_crossthread_t));
//     memset(h, 0, sizeof(*h));
//     uv_loop_init(&h->loop);
//     uv_async_init(&h->loop, &h->async, crossthread_async_cb);
//     uv_timer_init(&h->loop, &h->timer);
//     uv_mutex_init(&h->mutex);
//     uv_sem_init(&h->fired_sem, 0);
//     h->async.data = h;
//     h->timer.data = h;
//     h->stop_flag = 0;
//     return h;
// }
//
// // bench_crossthread_run_loop drives the loop until stop_flag is set and all
// // handles are closed.  Must be called from a dedicated OS-locked goroutine.
// // Frees h before returning; the caller must not use h after this call.
// static void bench_crossthread_run_loop(bench_crossthread_t* h) {
//     uv_run(&h->loop, UV_RUN_DEFAULT);
//     uv_run(&h->loop, UV_RUN_DEFAULT); /* drain close callbacks */
//     uv_loop_close(&h->loop);
//     uv_mutex_destroy(&h->mutex);
//     uv_sem_destroy(&h->fired_sem);
//     free(h);
// }
//
// // bench_crossthread_send signals the loop thread to schedule a zero-delay
// // timer, then blocks until the timer fires.  Called per benchmark iteration.
// static void bench_crossthread_send(bench_crossthread_t* h) {
//     uv_async_send(&h->async);
//     uv_sem_wait(&h->fired_sem);
// }
//
// // bench_crossthread_stop sets stop_flag and wakes the loop so it can close
// // handles and exit.  bench_crossthread_run_loop will return shortly after.
// static void bench_crossthread_stop(bench_crossthread_t* h) {
//     uv_mutex_lock(&h->mutex);
//     h->stop_flag = 1;
//     uv_mutex_unlock(&h->mutex);
//     uv_async_send(&h->async);
// }
import "C"

// ── Async wakeup harness ─────────────────────────────────────────────────────

type libuvLoop struct {
	ptr *C.bench_loop_t
}

func newLibuvLoop() *libuvLoop {
	return &libuvLoop{ptr: C.new_bench_loop()}
}

func (l *libuvLoop) run() {
	C.run_bench_loop(l.ptr)
}

func (l *libuvLoop) stop() {
	C.stop_bench_loop(l.ptr)
}

func (l *libuvLoop) send() {
	C.send_bench_loop(l.ptr)
}

// ── One-shot timer harness ───────────────────────────────────────────────────

type libuvOneShotTimer struct {
	ptr *C.bench_oneshot_t
}

func newLibuvOneShotTimer() *libuvOneShotTimer {
	return &libuvOneShotTimer{ptr: C.bench_oneshot_create()}
}

// run schedules a timer with the given delay (in milliseconds), drives the
// loop until the timer fires, and returns whether it fired successfully.
// The harness may be reused across calls.
func (t *libuvOneShotTimer) run(delayMS uint64) bool {
	return C.bench_oneshot_run(t.ptr, C.uint64_t(delayMS)) != 0
}

func (t *libuvOneShotTimer) destroy() {
	C.bench_oneshot_destroy(t.ptr)
}

// ── Batched one-shot timer harness ──────────────────────────────────────────

type libuvRepeatTimer struct {
	ptr *C.bench_repeat_t
}

func newLibuvRepeatTimer() *libuvRepeatTimer {
	return &libuvRepeatTimer{ptr: C.bench_repeat_create()}
}

// run starts min(target, REPEAT_TIMERS) simultaneous zero-delay one-shot
// timers (timeoutMS is the initial delay; repeat interval is always 0) and
// drives the loop until all started timers have fired.  Returns the observed
// firing count; callers should assert it equals target.
//
// PRECONDITION: target must be <= REPEAT_TIMERS (100).  Exceeding this causes
// bench_repeat_run to start fewer timers than target requires, so uv_stop is
// never called and uv_run blocks indefinitely.
func (t *libuvRepeatTimer) run(timeoutMS uint64, target int) int {
	return int(C.bench_repeat_run(t.ptr, C.uint64_t(timeoutMS), C.int(target)))
}

func (t *libuvRepeatTimer) destroy() {
	C.bench_repeat_destroy(t.ptr)
}

// ── Cross-thread timer harness ───────────────────────────────────────────────

type libuvCrossThreadTimer struct {
	ptr *C.bench_crossthread_t
}

func newLibuvCrossThreadTimer() *libuvCrossThreadTimer {
	return &libuvCrossThreadTimer{ptr: C.bench_crossthread_create()}
}

// send signals the loop thread to schedule a zero-delay timer, then blocks
// until the timer fires.  The loop must already be running (via runLoop)
// before send is called.
func (t *libuvCrossThreadTimer) send() {
	C.bench_crossthread_send(t.ptr)
}

// runLoop drives the loop until stop is called.  Must be called from a
// goroutine that has called runtime.LockOSThread.  Blocks until the loop
// exits; the underlying memory is freed before runLoop returns so the
// receiver must not be used afterwards.
func (t *libuvCrossThreadTimer) runLoop() {
	C.bench_crossthread_run_loop(t.ptr)
}

// stop sets the stop flag and wakes the loop so it closes handles and exits.
// runLoop will return shortly after stop is called.
func (t *libuvCrossThreadTimer) stop() {
	C.bench_crossthread_stop(t.ptr)
}
