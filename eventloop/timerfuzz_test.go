package eventloop

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func FuzzLoopPreRunTimerLifecycle(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0, 1, 2, 3, 4, 5, 6, 7})
	f.Add([]byte("schedule-cancel-unref-ref-batch"))

	f.Fuzz(func(t *testing.T, data []byte) {
		r := newFuzzReader(data)
		loop, err := New(WithAutoExit(true))
		if err != nil {
			panic(err)
		}

		type timerModel struct {
			id       TimerID
			canceled bool
			refed    bool
		}
		var timers []timerModel
		var fired []TimerID
		ops := 1 + min(len(data)*2, 512)

		find := func(id TimerID) int {
			for i, timer := range timers {
				if timer.id == id {
					return i
				}
			}
			return -1
		}
		active := func(id TimerID) bool {
			index := find(id)
			return id != 0 && index >= 0 && !timers[index].canceled
		}
		pickID := func(allowInvalid bool) TimerID {
			if len(timers) == 0 || (allowInvalid && r.byte()%4 == 0) {
				return TimerID(r.uint64()%32 + 1 + uint64(len(timers)))
			}
			return timers[r.intn(len(timers))].id
		}

		for range ops {
			switch r.byte() % 6 {
			case 0, 1:
				var id TimerID
				id, err := loop.ScheduleTimer(0, func() { fired = append(fired, id) })
				if err != nil {
					t.Fatalf("ScheduleTimer before Run: %v", err)
				}
				timers = append(timers, timerModel{id: id, refed: true})

			case 2:
				id := pickID(true)
				err := loop.CancelTimer(id)
				if active(id) {
					if err != nil {
						t.Fatalf("CancelTimer(%d) before Run = %v, want nil", id, err)
					}
					if idx := find(id); idx >= 0 {
						timers[idx].canceled = true
					}
				} else if !errors.Is(err, ErrTimerNotFound) {
					t.Fatalf("CancelTimer(%d) before Run = %v, want ErrTimerNotFound", id, err)
				}

			case 3:
				count := r.intn(min(len(timers)+3, 8) + 1)
				ids := make([]TimerID, count)
				for i := range ids {
					ids[i] = pickID(true)
				}
				errs := loop.CancelTimers(ids...)
				if len(errs) != len(ids) {
					t.Fatalf("CancelTimers returned %d errors for %d ids", len(errs), len(ids))
				}
				for i, id := range ids {
					if active(id) {
						if errs[i] != nil {
							t.Fatalf("CancelTimers active id %d returned %v, want nil", id, errs[i])
						}
						timers[find(id)].canceled = true
					} else if !errors.Is(errs[i], ErrTimerNotFound) {
						t.Fatalf("CancelTimers inactive id %d returned %v at %d, want ErrTimerNotFound", id, errs[i], i)
					}
				}

			case 4:
				id := pickID(true)
				err := loop.UnrefTimer(id)
				if err != nil {
					t.Fatalf("UnrefTimer(%d) before Run = %v, want nil", id, err)
				}
				if idx := find(id); idx >= 0 && !timers[idx].canceled {
					timers[idx].refed = false
				}

			case 5:
				id := pickID(true)
				err := loop.RefTimer(id)
				if err != nil {
					t.Fatalf("RefTimer(%d) before Run = %v, want nil", id, err)
				}
				if idx := find(id); idx >= 0 && !timers[idx].canceled {
					timers[idx].refed = true
				}
			}
		}

		hasRefed := false
		for _, timer := range timers {
			if !timer.canceled && timer.refed {
				hasRefed = true
				break
			}
		}
		var want []TimerID
		if hasRefed {
			for _, timer := range timers {
				if !timer.canceled {
					want = append(want, timer.id)
				}
			}
		}

		if err := runAutoExitLoop(t, loop); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if !reflect.DeepEqual(fired, want) {
			t.Fatalf("fired timers mismatch\ngot  %v\nwant %v\nmodel %+v", fired, want, timers)
		}
	})
}

func FuzzJSPreRunTimerNamespaceInterop(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8})
	f.Add([]byte("clear-timeout-clear-interval-shared-namespace"))

	f.Fuzz(func(t *testing.T, data []byte) {
		r := newFuzzReader(data)
		loop, err := New(WithAutoExit(true))
		if err != nil {
			panic(err)
		}
		js, err := NewJS(loop)
		if err != nil {
			panic(err)
		}

		type jsTimerModel struct {
			id       uint64
			label    string
			cleared  bool
			interval bool
		}
		var timers []jsTimerModel
		var fired []string
		find := func(id uint64) int {
			for i, timer := range timers {
				if timer.id == id {
					return i
				}
			}
			return -1
		}
		pickID := func() uint64 {
			if len(timers) == 0 || r.byte()%4 == 0 {
				return r.uint64()%32 + 1
			}
			return timers[r.intn(len(timers))].id
		}

		ops := 1 + min(len(data)*2, 256)
		for range ops {
			switch r.byte() % 5 {
			case 0, 1:
				label := fmt.Sprintf("timeout:%d", len(timers)+1)
				id, err := js.SetTimeout(func() { fired = append(fired, label) }, 0)
				if err != nil {
					t.Fatalf("SetTimeout: %v", err)
				}
				timers = append(timers, jsTimerModel{id: id, label: label})

			case 2:
				label := fmt.Sprintf("interval:%d", len(timers)+1)
				var id uint64
				id, err := js.SetInterval(func() {
					fired = append(fired, label)
					if err := js.ClearInterval(id); err != nil && !errors.Is(err, ErrTimerNotFound) {
						panic(err)
					}
				}, 0)
				if err != nil {
					t.Fatalf("SetInterval: %v", err)
				}
				timers = append(timers, jsTimerModel{id: id, label: label, interval: true})

			case 3:
				id := pickID()
				err := js.ClearTimeout(id)
				idx := find(id)
				if idx >= 0 && !timers[idx].cleared {
					if err != nil {
						t.Fatalf("ClearTimeout(%d) = %v, want nil", id, err)
					}
					timers[idx].cleared = true
				} else if !errors.Is(err, ErrTimerNotFound) {
					t.Fatalf("ClearTimeout(%d) = %v, want ErrTimerNotFound", id, err)
				}

			case 4:
				id := pickID()
				err := js.ClearInterval(id)
				idx := find(id)
				if idx >= 0 && !timers[idx].cleared {
					if err != nil {
						t.Fatalf("ClearInterval(%d) = %v, want nil", id, err)
					}
					timers[idx].cleared = true
				} else if !errors.Is(err, ErrTimerNotFound) {
					t.Fatalf("ClearInterval(%d) = %v, want ErrTimerNotFound", id, err)
				}
			}
		}

		var want []string
		for _, timer := range timers {
			if !timer.cleared {
				want = append(want, timer.label)
			}
		}
		if err := runAutoExitLoop(t, loop); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if !reflect.DeepEqual(fired, want) {
			t.Fatalf("fired JS timers mismatch\ngot  %v\nwant %v\nmodel %+v", fired, want, timers)
		}
	})
}
