//go:build cgo && libuv

package libuvbaseline

// #cgo pkg-config: libuv
// #include <stdint.h>
// #include <stdlib.h>
// #include <uv.h>
//
// #define BENCH_TIMER_V2_FAULT_NONE 0
// #define BENCH_TIMER_V2_FAULT_ALLOCATION 1
// #define BENCH_TIMER_V2_FAULT_TIMER_ARRAY 2
// #define BENCH_TIMER_V2_FAULT_LOOP_INIT 3
// #define BENCH_TIMER_V2_FAULT_TIMER_INIT 4
//
// #define BENCH_TIMER_V2_FLAG_LOOP 1u
// #define BENCH_TIMER_V2_FLAG_TIMER_ARRAY 2u
//
// typedef struct bench_timer_v2_s bench_timer_v2_t;
//
// typedef struct {
//     uv_timer_t timer;
//     bench_timer_v2_t* owner;
//     size_t index;
// } bench_timer_v2_slot_t;
//
// struct bench_timer_v2_s {
//     uv_loop_t loop;
//     bench_timer_v2_slot_t* slots;
//     size_t capacity;
//     size_t initialized;
//     size_t target;
//     size_t fired;
//     uint32_t init_flags;
//     int callback_status;
//     int64_t callback_fault_index;
//     int callback_fault_used;
// };
//
// typedef struct {
//     int primary_status;
//     int loop_close_status;
//     uint32_t flags_before;
//     uint32_t flags_after;
//     size_t initialized_before;
//     size_t initialized_after;
// } bench_timer_v2_unwind_report_t;
//
// static void bench_timer_v2_callback(uv_timer_t* timer) {
//     bench_timer_v2_slot_t* slot = (bench_timer_v2_slot_t*)timer->data;
//     bench_timer_v2_t* harness = slot->owner;
//     if (!harness->callback_fault_used &&
//         harness->callback_fault_index >= 0 &&
//         (uint64_t)harness->callback_fault_index == slot->index) {
//         harness->callback_fault_used = 1;
//         harness->callback_status = UV_EIO;
//     }
//     harness->fired++;
// }
//
// static int bench_timer_v2_unwind(
//     bench_timer_v2_t* harness,
//     int primary_status,
//     bench_timer_v2_unwind_report_t* report
// ) {
//     report->primary_status = primary_status;
//     report->loop_close_status = 0;
//     report->flags_before = harness == NULL ? 0 : harness->init_flags;
//     report->initialized_before = harness == NULL ? 0 : harness->initialized;
//     if (harness == NULL) {
//         report->flags_after = 0;
//         report->initialized_after = 0;
//         return primary_status;
//     }
//     if ((harness->init_flags & BENCH_TIMER_V2_FLAG_LOOP) != 0) {
//         for (size_t index = 0; index < harness->initialized; index++) {
//             uv_handle_t* handle = (uv_handle_t*)&harness->slots[index].timer;
//             if (!uv_is_closing(handle)) {
//                 uv_close(handle, NULL);
//             }
//         }
//         uv_run(&harness->loop, UV_RUN_DEFAULT);
//         report->loop_close_status = uv_loop_close(&harness->loop);
//         if (report->loop_close_status == 0) {
//             harness->init_flags &= ~BENCH_TIMER_V2_FLAG_LOOP;
//             harness->initialized = 0;
//         }
//     }
//     if (report->loop_close_status == 0 &&
//         (harness->init_flags & BENCH_TIMER_V2_FLAG_TIMER_ARRAY) != 0) {
//         free(harness->slots);
//         harness->slots = NULL;
//         harness->init_flags &= ~BENCH_TIMER_V2_FLAG_TIMER_ARRAY;
//     }
//     report->flags_after = harness->init_flags;
//     report->initialized_after = harness->initialized;
//     if (report->loop_close_status == 0) {
//         free(harness);
//     }
//     if (primary_status != 0) {
//         return primary_status;
//     }
//     return report->loop_close_status;
// }
//
// static int bench_timer_v2_new(
//     size_t capacity,
//     int fault_stage,
//     int64_t fault_index,
//     bench_timer_v2_t** output,
//     bench_timer_v2_unwind_report_t* report
// ) {
//     *output = NULL;
//     if (capacity == 0 || capacity > SIZE_MAX / sizeof(bench_timer_v2_slot_t)) {
//         return UV_EINVAL;
//     }
//     if (fault_stage == BENCH_TIMER_V2_FAULT_ALLOCATION) {
//         return bench_timer_v2_unwind(NULL, UV_ENOMEM, report);
//     }
//     bench_timer_v2_t* harness = (bench_timer_v2_t*)calloc(1, sizeof(bench_timer_v2_t));
//     if (harness == NULL) {
//         return bench_timer_v2_unwind(NULL, UV_ENOMEM, report);
//     }
//     harness->capacity = capacity;
//     harness->callback_fault_index = -1;
//     if (fault_stage == BENCH_TIMER_V2_FAULT_TIMER_ARRAY) {
//         return bench_timer_v2_unwind(harness, UV_ENOMEM, report);
//     }
//     harness->slots = (bench_timer_v2_slot_t*)calloc(capacity, sizeof(bench_timer_v2_slot_t));
//     if (harness->slots == NULL) {
//         return bench_timer_v2_unwind(harness, UV_ENOMEM, report);
//     }
//     harness->init_flags |= BENCH_TIMER_V2_FLAG_TIMER_ARRAY;
//     if (fault_stage == BENCH_TIMER_V2_FAULT_LOOP_INIT) {
//         return bench_timer_v2_unwind(harness, UV_EIO, report);
//     }
//     int status = uv_loop_init(&harness->loop);
//     if (status != 0) {
//         return bench_timer_v2_unwind(harness, status, report);
//     }
//     harness->init_flags |= BENCH_TIMER_V2_FLAG_LOOP;
//     for (size_t index = 0; index < capacity; index++) {
//         if (fault_stage == BENCH_TIMER_V2_FAULT_TIMER_INIT &&
//             fault_index >= 0 && (uint64_t)fault_index == index) {
//             return bench_timer_v2_unwind(harness, UV_EIO, report);
//         }
//         status = uv_timer_init(&harness->loop, &harness->slots[index].timer);
//         if (status != 0) {
//             return bench_timer_v2_unwind(harness, status, report);
//         }
//         harness->slots[index].owner = harness;
//         harness->slots[index].index = index;
//         harness->slots[index].timer.data = &harness->slots[index];
//         harness->initialized++;
//     }
//     report->primary_status = 0;
//     report->loop_close_status = 0;
//     report->flags_before = harness->init_flags;
//     report->flags_after = harness->init_flags;
//     report->initialized_before = harness->initialized;
//     report->initialized_after = harness->initialized;
//     *output = harness;
//     return 0;
// }
//
// static int bench_timer_v2_run(
//     bench_timer_v2_t* harness,
//     uint64_t delay_milliseconds,
//     size_t target,
//     int64_t start_fault_index,
//     int64_t callback_fault_index,
//     size_t* fired
// ) {
//     *fired = 0;
//     if (harness == NULL || target == 0 || target > harness->capacity) {
//         return UV_EINVAL;
//     }
//     harness->target = target;
//     harness->fired = 0;
//     harness->callback_status = 0;
//     harness->callback_fault_index = callback_fault_index;
//     harness->callback_fault_used = 0;
//     size_t started = 0;
//     for (size_t index = 0; index < target; index++) {
//         int status;
//         if (start_fault_index >= 0 && (uint64_t)start_fault_index == index) {
//             status = UV_EIO;
//         } else {
//             status = uv_timer_start(
//                 &harness->slots[index].timer,
//                 bench_timer_v2_callback,
//                 delay_milliseconds,
//                 0
//             );
//         }
//         if (status != 0) {
//             for (size_t stop_index = 0; stop_index < started; stop_index++) {
//                 uv_timer_stop(&harness->slots[stop_index].timer);
//             }
//             return status;
//         }
//         started++;
//     }
//     int run_status = uv_run(&harness->loop, UV_RUN_DEFAULT);
//     *fired = harness->fired;
//     if (run_status != 0) {
//         return UV_EBUSY;
//     }
//     if (harness->callback_status != 0) {
//         return harness->callback_status;
//     }
//     if (harness->fired != target) {
//         return UV_EIO;
//     }
//     return 0;
// }
//
// static int bench_timer_v2_close(
//     bench_timer_v2_t* harness,
//     int inject_busy,
//     bench_timer_v2_unwind_report_t* report
// ) {
//     if (inject_busy) {
//         report->primary_status = 0;
//         report->loop_close_status = UV_EBUSY;
//         report->flags_before = harness->init_flags;
//         report->flags_after = harness->init_flags;
//         report->initialized_before = harness->initialized;
//         report->initialized_after = harness->initialized;
//         return UV_EBUSY;
//     }
//     return bench_timer_v2_unwind(harness, 0, report);
// }
import "C"

import (
	"errors"
	"fmt"
	"sync"
)

const libuvTimerV2CapacityMaximum = 100

type libuvTimerFaultStage int

const (
	libuvTimerFaultNone       libuvTimerFaultStage = C.BENCH_TIMER_V2_FAULT_NONE
	libuvTimerFaultAllocation libuvTimerFaultStage = C.BENCH_TIMER_V2_FAULT_ALLOCATION
	libuvTimerFaultArray      libuvTimerFaultStage = C.BENCH_TIMER_V2_FAULT_TIMER_ARRAY
	libuvTimerFaultLoop       libuvTimerFaultStage = C.BENCH_TIMER_V2_FAULT_LOOP_INIT
	libuvTimerFaultInit       libuvTimerFaultStage = C.BENCH_TIMER_V2_FAULT_TIMER_INIT
)

type libuvTimerUnwindReport struct {
	PrimaryStatus     int
	LoopCloseStatus   int
	FlagsBefore       uint32
	FlagsAfter        uint32
	InitializedBefore uint64
	InitializedAfter  uint64
}

type libuvTimerV2 struct {
	mu      sync.Mutex
	pointer *C.bench_timer_v2_t
}

func newLibuvTimerV2(capacity int) (*libuvTimerV2, error) {
	harness, _, err := newLibuvTimerV2Fault(capacity, libuvTimerFaultNone, -1)
	return harness, err
}

func newLibuvTimerV2Fault(capacity int, stage libuvTimerFaultStage, index int) (*libuvTimerV2, libuvTimerUnwindReport, error) {
	if capacity < 1 || capacity > libuvTimerV2CapacityMaximum {
		return nil, libuvTimerUnwindReport{}, fmt.Errorf("libuv timer capacity %d outside 1..%d", capacity, libuvTimerV2CapacityMaximum)
	}
	var pointer *C.bench_timer_v2_t
	var nativeReport C.bench_timer_v2_unwind_report_t
	status := int(C.bench_timer_v2_new(
		C.size_t(capacity),
		C.int(stage),
		C.int64_t(index),
		&pointer,
		&nativeReport,
	))
	report := libuvTimerReport(nativeReport)
	if status != 0 {
		return nil, report, newLibuvStatusError("create timer harness", status)
	}
	return &libuvTimerV2{pointer: pointer}, report, nil
}

func (h *libuvTimerV2) run(delayMilliseconds uint64, target int) (int, error) {
	return h.runFault(delayMilliseconds, target, -1, -1)
}

func (h *libuvTimerV2) runFault(delayMilliseconds uint64, target, startFaultIndex, callbackFaultIndex int) (int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.pointer == nil {
		return 0, errors.New("libuv timer harness is closed")
	}
	if delayMilliseconds != 0 {
		return 0, errors.New("libuv timer benchmark harness accepts only zero-delay timers")
	}
	if target < 1 || target > libuvTimerV2CapacityMaximum {
		return 0, newLibuvStatusError("run timer harness", int(C.UV_EINVAL))
	}
	var fired C.size_t
	status := int(C.bench_timer_v2_run(
		h.pointer,
		C.uint64_t(delayMilliseconds),
		C.size_t(target),
		C.int64_t(startFaultIndex),
		C.int64_t(callbackFaultIndex),
		&fired,
	))
	if status != 0 {
		return int(fired), newLibuvStatusError("run timer harness", status)
	}
	return int(fired), nil
}

func (h *libuvTimerV2) close() error {
	_, err := h.closeFault(false)
	return err
}

func (h *libuvTimerV2) closeFault(injectBusy bool) (libuvTimerUnwindReport, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.pointer == nil {
		return libuvTimerUnwindReport{}, nil
	}
	var nativeReport C.bench_timer_v2_unwind_report_t
	status := int(C.bench_timer_v2_close(h.pointer, C.int(boolInteger(injectBusy)), &nativeReport))
	report := libuvTimerReport(nativeReport)
	if status != 0 {
		return report, errors.Join(
			newLibuvStatusError("close timer harness", status),
			fmt.Errorf("libuv timer unwind: %+v", report),
		)
	}
	h.pointer = nil
	return report, nil
}

func libuvTimerReport(report C.bench_timer_v2_unwind_report_t) libuvTimerUnwindReport {
	return libuvTimerUnwindReport{
		PrimaryStatus:     int(report.primary_status),
		LoopCloseStatus:   int(report.loop_close_status),
		FlagsBefore:       uint32(report.flags_before),
		FlagsAfter:        uint32(report.flags_after),
		InitializedBefore: uint64(report.initialized_before),
		InitializedAfter:  uint64(report.initialized_after),
	}
}
