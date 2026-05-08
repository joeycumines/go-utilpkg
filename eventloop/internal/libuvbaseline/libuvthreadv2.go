//go:build cgo && libuv

package libuvbaseline

// #cgo pkg-config: libuv
// #include <stdint.h>
// #include <stdlib.h>
// #include <uv.h>
//
// #define BENCH_THREAD_V2_MODE_ASYNC 1
// #define BENCH_THREAD_V2_MODE_TIMER 2
//
// #define BENCH_THREAD_V2_FAULT_NONE 0
// #define BENCH_THREAD_V2_FAULT_ALLOCATION 1
// #define BENCH_THREAD_V2_FAULT_MUTEX 2
// #define BENCH_THREAD_V2_FAULT_CONDITION 3
// #define BENCH_THREAD_V2_FAULT_LOOP 4
// #define BENCH_THREAD_V2_FAULT_ASYNC 5
// #define BENCH_THREAD_V2_FAULT_TIMER 6
// #define BENCH_THREAD_V2_FAULT_PREPARE 7
// #define BENCH_THREAD_V2_FAULT_PREPARE_START 8
//
// #define BENCH_THREAD_V2_INIT_MUTEX (1u << 0)
// #define BENCH_THREAD_V2_INIT_CONDITION (1u << 1)
// #define BENCH_THREAD_V2_INIT_LOOP (1u << 2)
// #define BENCH_THREAD_V2_INIT_ASYNC (1u << 3)
// #define BENCH_THREAD_V2_INIT_TIMER (1u << 4)
// #define BENCH_THREAD_V2_INIT_PREPARE (1u << 5)
// #define BENCH_THREAD_V2_INIT_PREPARE_STARTED (1u << 6)
//
// #define BENCH_THREAD_V2_STATE_READY (1u << 0)
// #define BENCH_THREAD_V2_STATE_STOP_REQUESTED (1u << 1)
// #define BENCH_THREAD_V2_STATE_RUN_RETURNED (1u << 2)
// #define BENCH_THREAD_V2_STATE_LOOP_CLOSED (1u << 3)
//
// typedef struct {
//     uv_loop_t loop;
//     uv_async_t command;
//     uv_timer_t timer;
//     uv_prepare_t prepare;
//     uv_mutex_t mutex;
//     uv_cond_t condition;
//     uint32_t init_flags;
//     uint32_t state_flags;
//     int mode;
//     int terminal_status;
//     int completion_status;
//     uint64_t ready_generation;
//     uint64_t request_generation;
//     uint64_t completion_generation;
//     uint64_t poll_boundary_generation;
//     uint64_t wait_generation;
//     uint64_t timer_generation;
//     int suppress_ready;
//     int exit_before_ready;
//     int suppress_completion;
//     int suppress_boundary;
//     int timer_start_fault;
// } bench_thread_v2_t;
//
// typedef struct {
//     int primary_status;
//     int loop_close_status;
//     uint32_t flags_before;
//     uint32_t flags_after;
// } bench_thread_v2_unwind_report_t;
//
// static void bench_thread_v2_close_handles(bench_thread_v2_t* harness);
//
// static void bench_thread_v2_prepare_callback(uv_prepare_t* prepare) {
//     bench_thread_v2_t* harness = (bench_thread_v2_t*)prepare->data;
//     uv_mutex_lock(&harness->mutex);
//     if (harness->exit_before_ready) {
//         harness->exit_before_ready = 0;
//         harness->state_flags |= BENCH_THREAD_V2_STATE_STOP_REQUESTED;
//         bench_thread_v2_close_handles(harness);
//         uv_cond_broadcast(&harness->condition);
//         uv_mutex_unlock(&harness->mutex);
//         return;
//     }
//     if (!harness->suppress_ready) {
//         harness->ready_generation = 1;
//         harness->state_flags |= BENCH_THREAD_V2_STATE_READY;
//     }
//     if (!harness->suppress_boundary &&
//         harness->poll_boundary_generation < harness->completion_generation) {
//         harness->poll_boundary_generation = harness->completion_generation;
//     }
//     uv_cond_broadcast(&harness->condition);
//     uv_mutex_unlock(&harness->mutex);
// }
//
// static void bench_thread_v2_timer_callback(uv_timer_t* timer) {
//     bench_thread_v2_t* harness = (bench_thread_v2_t*)timer->data;
//     uv_mutex_lock(&harness->mutex);
//     if (!harness->suppress_completion &&
//         harness->completion_generation < harness->timer_generation) {
//         harness->completion_status = 0;
//         harness->completion_generation = harness->timer_generation;
//         uv_cond_broadcast(&harness->condition);
//     }
//     uv_mutex_unlock(&harness->mutex);
// }
//
// static void bench_thread_v2_close_handles(bench_thread_v2_t* harness) {
//     if ((harness->init_flags & BENCH_THREAD_V2_INIT_PREPARE_STARTED) != 0) {
//         uv_prepare_stop(&harness->prepare);
//         harness->init_flags &= ~BENCH_THREAD_V2_INIT_PREPARE_STARTED;
//     }
//     if ((harness->init_flags & BENCH_THREAD_V2_INIT_PREPARE) != 0 &&
//         !uv_is_closing((uv_handle_t*)&harness->prepare)) {
//         uv_close((uv_handle_t*)&harness->prepare, NULL);
//     }
//     if ((harness->init_flags & BENCH_THREAD_V2_INIT_TIMER) != 0 &&
//         !uv_is_closing((uv_handle_t*)&harness->timer)) {
//         uv_timer_stop(&harness->timer);
//         uv_close((uv_handle_t*)&harness->timer, NULL);
//     }
//     if ((harness->init_flags & BENCH_THREAD_V2_INIT_ASYNC) != 0 &&
//         !uv_is_closing((uv_handle_t*)&harness->command)) {
//         uv_close((uv_handle_t*)&harness->command, NULL);
//     }
// }
//
// static void bench_thread_v2_async_callback(uv_async_t* async) {
//     bench_thread_v2_t* harness = (bench_thread_v2_t*)async->data;
//     uv_mutex_lock(&harness->mutex);
//     if ((harness->state_flags & BENCH_THREAD_V2_STATE_STOP_REQUESTED) != 0) {
//         if (harness->completion_generation < harness->request_generation) {
//             harness->completion_status = UV_ECANCELED;
//             harness->completion_generation = harness->request_generation;
//         }
//         uv_cond_broadcast(&harness->condition);
//         uv_mutex_unlock(&harness->mutex);
//         bench_thread_v2_close_handles(harness);
//         return;
//     }
//     uint64_t generation = harness->request_generation;
//     if (generation <= harness->completion_generation) {
//         uv_mutex_unlock(&harness->mutex);
//         return;
//     }
//     if (harness->mode == BENCH_THREAD_V2_MODE_ASYNC) {
//         if (!harness->suppress_completion) {
//             harness->completion_status = 0;
//             harness->completion_generation = generation;
//             uv_cond_broadcast(&harness->condition);
//         }
//         uv_mutex_unlock(&harness->mutex);
//         return;
//     }
//     int inject_timer_failure = harness->timer_start_fault;
//     harness->timer_start_fault = 0;
//     harness->timer_generation = generation;
//     uv_mutex_unlock(&harness->mutex);
//     int status = inject_timer_failure ? UV_EIO : uv_timer_start(
//         &harness->timer,
//         bench_thread_v2_timer_callback,
//         0,
//         0
//     );
//     if (status != 0) {
//         uv_mutex_lock(&harness->mutex);
//         harness->completion_status = status;
//         harness->completion_generation = generation;
//         uv_cond_broadcast(&harness->condition);
//         uv_mutex_unlock(&harness->mutex);
//     }
// }
//
// static int bench_thread_v2_unwind(
//     bench_thread_v2_t* harness,
//     int primary_status,
//     bench_thread_v2_unwind_report_t* report
// ) {
//     report->primary_status = primary_status;
//     report->loop_close_status = 0;
//     report->flags_before = harness == NULL ? 0 : harness->init_flags;
//     if (harness == NULL) {
//         report->flags_after = 0;
//         return primary_status;
//     }
//     if ((harness->init_flags & BENCH_THREAD_V2_INIT_LOOP) != 0) {
//         bench_thread_v2_close_handles(harness);
//         uv_run(&harness->loop, UV_RUN_DEFAULT);
//         report->loop_close_status = uv_loop_close(&harness->loop);
//         if (report->loop_close_status == 0) {
//             harness->init_flags &= ~(
//                 BENCH_THREAD_V2_INIT_LOOP |
//                 BENCH_THREAD_V2_INIT_ASYNC |
//                 BENCH_THREAD_V2_INIT_TIMER |
//                 BENCH_THREAD_V2_INIT_PREPARE
//             );
//         }
//     }
//     if (report->loop_close_status == 0 &&
//         (harness->init_flags & BENCH_THREAD_V2_INIT_CONDITION) != 0) {
//         uv_cond_destroy(&harness->condition);
//         harness->init_flags &= ~BENCH_THREAD_V2_INIT_CONDITION;
//     }
//     if (report->loop_close_status == 0 &&
//         (harness->init_flags & BENCH_THREAD_V2_INIT_MUTEX) != 0) {
//         uv_mutex_destroy(&harness->mutex);
//         harness->init_flags &= ~BENCH_THREAD_V2_INIT_MUTEX;
//     }
//     report->flags_after = harness->init_flags;
//     if (report->loop_close_status == 0) {
//         free(harness);
//     }
//     if (primary_status != 0) {
//         return primary_status;
//     }
//     return report->loop_close_status;
// }
//
// static int bench_thread_v2_new(
//     int mode,
//     int fault_stage,
//     int suppress_ready,
//     int exit_before_ready,
//     bench_thread_v2_t** output,
//     bench_thread_v2_unwind_report_t* report
// ) {
//     *output = NULL;
//     if (mode != BENCH_THREAD_V2_MODE_ASYNC && mode != BENCH_THREAD_V2_MODE_TIMER) {
//         return UV_EINVAL;
//     }
//     if (fault_stage == BENCH_THREAD_V2_FAULT_ALLOCATION) {
//         return bench_thread_v2_unwind(NULL, UV_ENOMEM, report);
//     }
//     bench_thread_v2_t* harness = (bench_thread_v2_t*)calloc(1, sizeof(bench_thread_v2_t));
//     if (harness == NULL) {
//         return bench_thread_v2_unwind(NULL, UV_ENOMEM, report);
//     }
//     harness->mode = mode;
//     harness->suppress_ready = suppress_ready;
//     harness->exit_before_ready = exit_before_ready;
//     if (fault_stage == BENCH_THREAD_V2_FAULT_MUTEX) {
//         return bench_thread_v2_unwind(harness, UV_EIO, report);
//     }
//     int status = uv_mutex_init(&harness->mutex);
//     if (status != 0) {
//         return bench_thread_v2_unwind(harness, status, report);
//     }
//     harness->init_flags |= BENCH_THREAD_V2_INIT_MUTEX;
//     if (fault_stage == BENCH_THREAD_V2_FAULT_CONDITION) {
//         return bench_thread_v2_unwind(harness, UV_EIO, report);
//     }
//     status = uv_cond_init(&harness->condition);
//     if (status != 0) {
//         return bench_thread_v2_unwind(harness, status, report);
//     }
//     harness->init_flags |= BENCH_THREAD_V2_INIT_CONDITION;
//     if (fault_stage == BENCH_THREAD_V2_FAULT_LOOP) {
//         return bench_thread_v2_unwind(harness, UV_EIO, report);
//     }
//     status = uv_loop_init(&harness->loop);
//     if (status != 0) {
//         return bench_thread_v2_unwind(harness, status, report);
//     }
//     harness->init_flags |= BENCH_THREAD_V2_INIT_LOOP;
//     if (fault_stage == BENCH_THREAD_V2_FAULT_ASYNC) {
//         return bench_thread_v2_unwind(harness, UV_EIO, report);
//     }
//     status = uv_async_init(&harness->loop, &harness->command, bench_thread_v2_async_callback);
//     if (status != 0) {
//         return bench_thread_v2_unwind(harness, status, report);
//     }
//     harness->command.data = harness;
//     harness->init_flags |= BENCH_THREAD_V2_INIT_ASYNC;
//     if (mode == BENCH_THREAD_V2_MODE_TIMER) {
//         if (fault_stage == BENCH_THREAD_V2_FAULT_TIMER) {
//             return bench_thread_v2_unwind(harness, UV_EIO, report);
//         }
//         status = uv_timer_init(&harness->loop, &harness->timer);
//         if (status != 0) {
//             return bench_thread_v2_unwind(harness, status, report);
//         }
//         harness->timer.data = harness;
//         harness->init_flags |= BENCH_THREAD_V2_INIT_TIMER;
//     } else if (fault_stage == BENCH_THREAD_V2_FAULT_TIMER) {
//         return bench_thread_v2_unwind(harness, UV_EINVAL, report);
//     }
//     if (fault_stage == BENCH_THREAD_V2_FAULT_PREPARE) {
//         return bench_thread_v2_unwind(harness, UV_EIO, report);
//     }
//     status = uv_prepare_init(&harness->loop, &harness->prepare);
//     if (status != 0) {
//         return bench_thread_v2_unwind(harness, status, report);
//     }
//     harness->prepare.data = harness;
//     harness->init_flags |= BENCH_THREAD_V2_INIT_PREPARE;
//     if (fault_stage == BENCH_THREAD_V2_FAULT_PREPARE_START) {
//         return bench_thread_v2_unwind(harness, UV_EIO, report);
//     }
//     status = uv_prepare_start(&harness->prepare, bench_thread_v2_prepare_callback);
//     if (status != 0) {
//         return bench_thread_v2_unwind(harness, status, report);
//     }
//     harness->init_flags |= BENCH_THREAD_V2_INIT_PREPARE_STARTED;
//     report->primary_status = 0;
//     report->loop_close_status = 0;
//     report->flags_before = harness->init_flags;
//     report->flags_after = harness->init_flags;
//     *output = harness;
//     return 0;
// }
//
// static int bench_thread_v2_wait_ready(bench_thread_v2_t* harness, uint64_t timeout_ns) {
//     uint64_t started = uv_hrtime();
//     uv_mutex_lock(&harness->mutex);
//     while (harness->ready_generation == 0 &&
//            (harness->state_flags & BENCH_THREAD_V2_STATE_RUN_RETURNED) == 0) {
//         uint64_t elapsed = uv_hrtime() - started;
//         if (elapsed >= timeout_ns) {
//             uv_mutex_unlock(&harness->mutex);
//             return UV_ETIMEDOUT;
//         }
//         int status = uv_cond_timedwait(&harness->condition, &harness->mutex, timeout_ns - elapsed);
//         if (status != 0 && status != UV_ETIMEDOUT) {
//             uv_mutex_unlock(&harness->mutex);
//             return status;
//         }
//     }
//     int status = 0;
//     if (harness->ready_generation == 0) {
//         status = harness->terminal_status != 0 ? harness->terminal_status : UV_ECANCELED;
//     }
//     uv_mutex_unlock(&harness->mutex);
//     return status;
// }
//
// static int bench_thread_v2_wait_generation(
//     bench_thread_v2_t* harness,
//     uint64_t generation,
//     uint64_t timeout_ns
// ) {
//     uint64_t started = uv_hrtime();
//     uv_mutex_lock(&harness->mutex);
//     if (harness->wait_generation < generation) {
//         harness->wait_generation = generation;
//     }
//     uv_cond_broadcast(&harness->condition);
//     while ((harness->completion_generation < generation ||
//             harness->poll_boundary_generation < generation) &&
//            (harness->state_flags & BENCH_THREAD_V2_STATE_RUN_RETURNED) == 0) {
//         uint64_t elapsed = uv_hrtime() - started;
//         if (elapsed >= timeout_ns) {
//             uv_mutex_unlock(&harness->mutex);
//             return UV_ETIMEDOUT;
//         }
//         int status = uv_cond_timedwait(&harness->condition, &harness->mutex, timeout_ns - elapsed);
//         if (status != 0 && status != UV_ETIMEDOUT) {
//             uv_mutex_unlock(&harness->mutex);
//             return status;
//         }
//     }
//     int status;
//     if (harness->completion_generation >= generation &&
//         harness->poll_boundary_generation >= generation) {
//         status = harness->completion_status;
//     } else if (harness->terminal_status != 0) {
//         status = harness->terminal_status;
//     } else {
//         status = UV_ECANCELED;
//     }
//     uv_mutex_unlock(&harness->mutex);
//     return status;
// }
//
// static int bench_thread_v2_wait_generation_started(
//     bench_thread_v2_t* harness,
//     uint64_t generation,
//     uint64_t timeout_ns
// ) {
//     uint64_t started = uv_hrtime();
//     uv_mutex_lock(&harness->mutex);
//     while (harness->wait_generation < generation &&
//            (harness->state_flags & BENCH_THREAD_V2_STATE_STOP_REQUESTED) == 0 &&
//            (harness->state_flags & BENCH_THREAD_V2_STATE_RUN_RETURNED) == 0) {
//         uint64_t elapsed = uv_hrtime() - started;
//         if (elapsed >= timeout_ns) {
//             uv_mutex_unlock(&harness->mutex);
//             return UV_ETIMEDOUT;
//         }
//         int status = uv_cond_timedwait(&harness->condition, &harness->mutex, timeout_ns - elapsed);
//         if (status != 0 && status != UV_ETIMEDOUT) {
//             uv_mutex_unlock(&harness->mutex);
//             return status;
//         }
//     }
//     int status;
//     if (harness->wait_generation >= generation) {
//         status = 0;
//     } else if (harness->terminal_status != 0) {
//         status = harness->terminal_status;
//     } else {
//         status = UV_ECANCELED;
//     }
//     uv_mutex_unlock(&harness->mutex);
//     return status;
// }
//
// static int bench_thread_v2_round_trip(
//     bench_thread_v2_t* harness,
//     uint64_t timeout_ns,
//     int send_fault,
//     int timer_start_fault,
//     int suppress_completion,
//     int suppress_boundary
// ) {
//     uv_mutex_lock(&harness->mutex);
//     if ((harness->state_flags & BENCH_THREAD_V2_STATE_STOP_REQUESTED) != 0) {
//         uv_mutex_unlock(&harness->mutex);
//         return UV_ECANCELED;
//     }
//     if (harness->ready_generation == 0) {
//         uv_mutex_unlock(&harness->mutex);
//         return UV_EAGAIN;
//     }
//     if (harness->request_generation != harness->completion_generation ||
//         harness->request_generation != harness->poll_boundary_generation) {
//         uv_mutex_unlock(&harness->mutex);
//         return UV_EBUSY;
//     }
//     if (harness->request_generation == UINT64_MAX) {
//         uv_mutex_unlock(&harness->mutex);
//         return UV_EOVERFLOW;
//     }
//     uint64_t generation = ++harness->request_generation;
//     harness->completion_status = 0;
//     harness->timer_start_fault = timer_start_fault;
//     harness->suppress_completion = suppress_completion;
//     harness->suppress_boundary = suppress_boundary;
//     uv_mutex_unlock(&harness->mutex);
//     int status = send_fault ? UV_EIO : uv_async_send(&harness->command);
//     if (status != 0) {
//         uv_mutex_lock(&harness->mutex);
//         harness->completion_status = status;
//         harness->completion_generation = generation;
//         harness->poll_boundary_generation = generation;
//         harness->timer_start_fault = 0;
//         harness->suppress_completion = 0;
//         harness->suppress_boundary = 0;
//         uv_cond_broadcast(&harness->condition);
//         uv_mutex_unlock(&harness->mutex);
//         return status;
//     }
//     return bench_thread_v2_wait_generation(harness, generation, timeout_ns);
// }
//
// static int bench_thread_v2_recover(bench_thread_v2_t* harness, uint64_t timeout_ns) {
//     uv_mutex_lock(&harness->mutex);
//     if ((harness->state_flags & BENCH_THREAD_V2_STATE_STOP_REQUESTED) != 0) {
//         uv_mutex_unlock(&harness->mutex);
//         return UV_ECANCELED;
//     }
//     uint64_t generation = harness->request_generation;
//     harness->timer_start_fault = 0;
//     harness->suppress_completion = 0;
//     harness->suppress_boundary = 0;
//     uv_mutex_unlock(&harness->mutex);
//     int status = uv_async_send(&harness->command);
//     if (status != 0) {
//         return status;
//     }
//     return bench_thread_v2_wait_generation(harness, generation, timeout_ns);
// }
//
// static int bench_thread_v2_request_stop(bench_thread_v2_t* harness, int send_fault) {
//     uv_mutex_lock(&harness->mutex);
//     if ((harness->state_flags & BENCH_THREAD_V2_STATE_RUN_RETURNED) != 0) {
//         uv_mutex_unlock(&harness->mutex);
//         return 0;
//     }
//     harness->state_flags |= BENCH_THREAD_V2_STATE_STOP_REQUESTED;
//     harness->suppress_ready = 0;
//     harness->suppress_completion = 0;
//     harness->suppress_boundary = 0;
//     uv_mutex_unlock(&harness->mutex);
//     return send_fault ? UV_EIO : uv_async_send(&harness->command);
// }
//
// static int bench_thread_v2_run(bench_thread_v2_t* harness) {
//     int run_status = uv_run(&harness->loop, UV_RUN_DEFAULT);
//     if (run_status != 0) {
//         run_status = UV_EBUSY;
//     }
//     int close_status = uv_loop_close(&harness->loop);
//     int terminal_status = run_status != 0 ? run_status : close_status;
//     uv_mutex_lock(&harness->mutex);
//     harness->terminal_status = terminal_status;
//     harness->state_flags |= BENCH_THREAD_V2_STATE_RUN_RETURNED;
//     if (close_status == 0) {
//         harness->state_flags |= BENCH_THREAD_V2_STATE_LOOP_CLOSED;
//         harness->init_flags &= ~(
//             BENCH_THREAD_V2_INIT_LOOP |
//             BENCH_THREAD_V2_INIT_ASYNC |
//             BENCH_THREAD_V2_INIT_TIMER |
//             BENCH_THREAD_V2_INIT_PREPARE |
//             BENCH_THREAD_V2_INIT_PREPARE_STARTED
//         );
//     }
//     uv_cond_broadcast(&harness->condition);
//     uv_mutex_unlock(&harness->mutex);
//     return terminal_status;
// }
//
// static int bench_thread_v2_destroy(bench_thread_v2_t* harness) {
//     uv_mutex_lock(&harness->mutex);
//     int destroyable =
//         (harness->state_flags & BENCH_THREAD_V2_STATE_RUN_RETURNED) != 0 &&
//         (harness->state_flags & BENCH_THREAD_V2_STATE_LOOP_CLOSED) != 0;
//     uv_mutex_unlock(&harness->mutex);
//     if (!destroyable) {
//         return UV_EBUSY;
//     }
//     uv_cond_destroy(&harness->condition);
//     harness->init_flags &= ~BENCH_THREAD_V2_INIT_CONDITION;
//     uv_mutex_destroy(&harness->mutex);
//     harness->init_flags &= ~BENCH_THREAD_V2_INIT_MUTEX;
//     free(harness);
//     return 0;
// }
//
// static void bench_thread_v2_set_generation_max(bench_thread_v2_t* harness) {
//     uv_mutex_lock(&harness->mutex);
//     harness->request_generation = UINT64_MAX;
//     harness->completion_generation = UINT64_MAX;
//     harness->poll_boundary_generation = UINT64_MAX;
//     uv_mutex_unlock(&harness->mutex);
// }
import "C"

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"time"
)

type libuvThreadMode int

const (
	libuvThreadAsync libuvThreadMode = C.BENCH_THREAD_V2_MODE_ASYNC
	libuvThreadTimer libuvThreadMode = C.BENCH_THREAD_V2_MODE_TIMER
)

type libuvThreadFaultStage int

const (
	libuvThreadFaultNone         libuvThreadFaultStage = C.BENCH_THREAD_V2_FAULT_NONE
	libuvThreadFaultAllocation   libuvThreadFaultStage = C.BENCH_THREAD_V2_FAULT_ALLOCATION
	libuvThreadFaultMutex        libuvThreadFaultStage = C.BENCH_THREAD_V2_FAULT_MUTEX
	libuvThreadFaultCondition    libuvThreadFaultStage = C.BENCH_THREAD_V2_FAULT_CONDITION
	libuvThreadFaultLoop         libuvThreadFaultStage = C.BENCH_THREAD_V2_FAULT_LOOP
	libuvThreadFaultAsync        libuvThreadFaultStage = C.BENCH_THREAD_V2_FAULT_ASYNC
	libuvThreadFaultTimer        libuvThreadFaultStage = C.BENCH_THREAD_V2_FAULT_TIMER
	libuvThreadFaultPrepare      libuvThreadFaultStage = C.BENCH_THREAD_V2_FAULT_PREPARE
	libuvThreadFaultPrepareStart libuvThreadFaultStage = C.BENCH_THREAD_V2_FAULT_PREPARE_START
)

type libuvThreadUnwindReport struct {
	PrimaryStatus   int
	LoopCloseStatus int
	FlagsBefore     uint32
	FlagsAfter      uint32
}

type libuvThreadReadinessError struct {
	Readiness error
	Cleanup   error
}

func (e *libuvThreadReadinessError) Error() string {
	if e.Cleanup == nil {
		return e.Readiness.Error()
	}
	return fmt.Sprintf("%v; cleanup: %v", e.Readiness, e.Cleanup)
}

func (e *libuvThreadReadinessError) Unwrap() []error {
	if e.Cleanup == nil {
		return []error{e.Readiness}
	}
	return []error{e.Readiness, e.Cleanup}
}

type libuvThreadV2 struct {
	requestMu     sync.Mutex
	closeMu       sync.Mutex
	mu            sync.Mutex
	pointer       *C.bench_thread_v2_t
	runDone       chan int
	activeDone    chan struct{}
	active        bool
	closing       bool
	stopRequested bool
	runJoined     bool
	runStatus     int
	stopSendFault bool
}

const (
	libuvThreadCleanupTimeout = 5 * time.Second
	libuvThreadRecoveryLimit  = 5 * time.Second
)

func newLibuvThreadV2(mode libuvThreadMode, timeout time.Duration) (*libuvThreadV2, error) {
	harness, _, err := newLibuvThreadV2Fault(mode, timeout, libuvThreadFaultNone, false)
	return harness, err
}

func newLibuvThreadV2Fault(mode libuvThreadMode, timeout time.Duration, stage libuvThreadFaultStage, suppressReady bool) (*libuvThreadV2, libuvThreadUnwindReport, error) {
	return newLibuvThreadV2FaultState(mode, timeout, stage, suppressReady, false)
}

func newLibuvThreadV2FaultState(mode libuvThreadMode, timeout time.Duration, stage libuvThreadFaultStage, suppressReady, exitBeforeReady bool) (*libuvThreadV2, libuvThreadUnwindReport, error) {
	timeoutNanoseconds, err := libuvTimeoutNanoseconds(timeout)
	if err != nil {
		return nil, libuvThreadUnwindReport{}, err
	}
	var pointer *C.bench_thread_v2_t
	var nativeReport C.bench_thread_v2_unwind_report_t
	status := int(C.bench_thread_v2_new(
		C.int(mode),
		C.int(stage),
		C.int(boolInteger(suppressReady)),
		C.int(boolInteger(exitBeforeReady)),
		&pointer,
		&nativeReport,
	))
	report := libuvThreadReport(nativeReport)
	if status != 0 {
		return nil, report, newLibuvStatusError("create threaded harness", status)
	}
	harness := &libuvThreadV2{pointer: pointer, runDone: make(chan int, 1)}
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		harness.runDone <- int(C.bench_thread_v2_run(pointer))
	}()
	status = int(C.bench_thread_v2_wait_ready(pointer, C.uint64_t(timeoutNanoseconds)))
	if status == 0 {
		return harness, report, nil
	}
	readyErr := newLibuvStatusError("wait threaded harness readiness", status)
	cleanupErr := harness.close(libuvThreadCleanupTimeout)
	resultErr := &libuvThreadReadinessError{Readiness: readyErr, Cleanup: cleanupErr}
	if cleanupErr != nil {
		return harness, report, resultErr
	}
	return nil, report, resultErr
}

func (h *libuvThreadV2) roundTrip(timeout time.Duration) error {
	return h.roundTripFault(timeout, false, false, false, false)
}

func (h *libuvThreadV2) roundTripFault(timeout time.Duration, sendFault, timerFault, suppressCompletion, suppressBoundary bool) error {
	return h.roundTripFaultControl(timeout, sendFault, timerFault, suppressCompletion, suppressBoundary, nil, nil)
}

func (h *libuvThreadV2) roundTripFaultControl(timeout time.Duration, sendFault, timerFault, suppressCompletion, suppressBoundary bool, activeSignal chan<- struct{}, requestRelease <-chan struct{}) error {
	timeoutNanoseconds, err := libuvTimeoutNanoseconds(timeout)
	if err != nil {
		return err
	}
	h.requestMu.Lock()
	defer h.requestMu.Unlock()
	pointer, finish, err := h.beginThreadRequest()
	if err != nil {
		return err
	}
	defer finish()
	if activeSignal != nil {
		close(activeSignal)
	}
	if requestRelease != nil {
		<-requestRelease
	}
	status := int(C.bench_thread_v2_round_trip(
		pointer,
		C.uint64_t(timeoutNanoseconds),
		C.int(boolInteger(sendFault)),
		C.int(boolInteger(timerFault)),
		C.int(boolInteger(suppressCompletion)),
		C.int(boolInteger(suppressBoundary)),
	))
	if status == int(C.UV_ETIMEDOUT) {
		recoveryTimeout := libuvThreadRecoveryTimeout(timeout)
		recoveryStatus := int(C.bench_thread_v2_recover(pointer, C.uint64_t(recoveryTimeout)))
		return errors.Join(
			newLibuvStatusError("threaded round trip", status),
			newLibuvStatusError("recover threaded round trip", recoveryStatus),
		)
	}
	return newLibuvStatusError("threaded round trip", status)
}

func (h *libuvThreadV2) beginThreadRequest() (*C.bench_thread_v2_t, func(), error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.pointer == nil {
		return nil, nil, errors.New("libuv threaded harness is closed")
	}
	if h.closing {
		return nil, nil, errors.New("libuv threaded harness is closing")
	}
	if h.active {
		return nil, nil, errors.New("libuv threaded harness already has an active request")
	}
	h.active = true
	h.activeDone = make(chan struct{})
	return h.pointer, h.finishThreadRequest, nil
}

func (h *libuvThreadV2) finishThreadRequest() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.active {
		panic("libuv threaded harness request accounting underflow")
	}
	h.active = false
	close(h.activeDone)
}

func (h *libuvThreadV2) waitGenerationStart(generation uint64, timeout time.Duration) error {
	timeoutNanoseconds, err := libuvTimeoutNanoseconds(timeout)
	if err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.pointer == nil {
		return errors.New("libuv threaded harness is closed")
	}
	if h.closing {
		return errors.New("libuv threaded harness is closing")
	}
	if !h.active {
		return errors.New("libuv threaded harness has no active request")
	}
	status := int(C.bench_thread_v2_wait_generation_started(
		h.pointer,
		C.uint64_t(generation),
		C.uint64_t(timeoutNanoseconds),
	))
	return newLibuvStatusError("wait threaded request generation start", status)
}

func (h *libuvThreadV2) exhaustGeneration() {
	_ = h.exhaustGenerationSignal(nil)
}

func (h *libuvThreadV2) exhaustGenerationSignal(attempted chan<- struct{}) error {
	h.requestMu.Lock()
	defer h.requestMu.Unlock()
	if attempted != nil {
		close(attempted)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.pointer == nil {
		return errors.New("libuv threaded harness is closed")
	}
	if h.closing {
		return errors.New("libuv threaded harness is closing")
	}
	C.bench_thread_v2_set_generation_max(h.pointer)
	return nil
}

func (h *libuvThreadV2) close(timeout time.Duration) error {
	return h.closeFaultControl(timeout, nil, nil)
}

func (h *libuvThreadV2) closeFaultControl(timeout time.Duration, destroySignal chan<- struct{}, destroyRelease <-chan struct{}) error {
	if _, err := libuvTimeoutNanoseconds(timeout); err != nil {
		return err
	}
	h.closeMu.Lock()
	defer h.closeMu.Unlock()
	deadline := time.Now().Add(timeout)
	h.mu.Lock()
	if h.pointer == nil {
		h.mu.Unlock()
		return nil
	}
	h.closing = true
	pointer := h.pointer
	if !h.stopRequested {
		stopSendFault := h.stopSendFault
		h.stopSendFault = false
		status := int(C.bench_thread_v2_request_stop(pointer, C.int(boolInteger(stopSendFault))))
		if status != 0 {
			h.mu.Unlock()
			return newLibuvStatusError("stop threaded harness", status)
		}
		h.stopRequested = true
	}
	var activeDone <-chan struct{}
	if h.active {
		activeDone = h.activeDone
	}
	h.mu.Unlock()
	if activeDone != nil {
		if err := waitLibuvThreadDeadline(activeDone, deadline, "join active threaded request"); err != nil {
			return err
		}
	}
	h.mu.Lock()
	if !h.runJoined {
		h.mu.Unlock()
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("join threaded harness: %w", context.DeadlineExceeded)
		}
		timer := time.NewTimer(remaining)
		select {
		case runStatus := <-h.runDone:
			timer.Stop()
			h.mu.Lock()
			h.runStatus = runStatus
			h.runJoined = true
		case <-timer.C:
			return fmt.Errorf("join threaded harness: %w", context.DeadlineExceeded)
		}
	}
	runStatus := h.runStatus
	pointer = h.pointer
	if runStatus != 0 {
		h.mu.Unlock()
		return newLibuvStatusError("run threaded harness", runStatus)
	}
	if destroySignal != nil {
		close(destroySignal)
	}
	if destroyRelease != nil {
		<-destroyRelease
	}
	status := int(C.bench_thread_v2_destroy(pointer))
	if status != 0 {
		h.mu.Unlock()
		return newLibuvStatusError("destroy threaded harness", status)
	}
	h.pointer = nil
	h.mu.Unlock()
	return nil
}

func (h *libuvThreadV2) injectStopSendFault() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.stopSendFault = true
}

func libuvThreadReport(report C.bench_thread_v2_unwind_report_t) libuvThreadUnwindReport {
	return libuvThreadUnwindReport{
		PrimaryStatus:   int(report.primary_status),
		LoopCloseStatus: int(report.loop_close_status),
		FlagsBefore:     uint32(report.flags_before),
		FlagsAfter:      uint32(report.flags_after),
	}
}

func libuvTimeoutNanoseconds(timeout time.Duration) (uint64, error) {
	if timeout <= 0 {
		return 0, fmt.Errorf("libuv timeout must be positive: %s", timeout)
	}
	return uint64(timeout), nil
}

func boolInteger(value bool) int {
	if value {
		return 1
	}
	return 0
}
