package eventloop

import (
	"reflect"
	"testing"
)

type microtaskChild struct {
	foreign bool
	kind    int
	id      int
}

type microtaskJobSpec struct {
	children []microtaskChild
	kind     int
}

func FuzzLoopMicrotaskNextTickOrdering(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8})
	f.Add([]byte("nexttick-microtask-alternating-batches"))

	f.Fuzz(func(t *testing.T, data []byte) {
		r := newFuzzReader(data)

		initialCount := 1 + r.intn(8)
		specs := make([]microtaskJobSpec, 0, 48)
		initial := make([]microtaskChild, 0, initialCount)
		for range initialCount {
			id := len(specs)
			kind := r.intn(2)
			specs = append(specs, microtaskJobSpec{kind: kind})
			initial = append(initial, microtaskChild{kind: kind, id: id})
		}
		for i := 0; i < len(specs) && len(specs) < 48; i++ {
			mask := r.byte()
			foreignMask := r.byte()
			for slot := range 3 {
				if len(specs) >= 48 || mask&(1<<slot) == 0 {
					continue
				}
				id := len(specs)
				kind := int((mask >> (3 + slot)) & 1)
				specs[i].children = append(specs[i].children, microtaskChild{
					foreign: foreignMask&(1<<slot) != 0,
					kind:    kind,
					id:      id,
				})
				specs = append(specs, microtaskJobSpec{kind: kind})
			}
		}

		want := simulateMicrotaskTrace(initial, specs)
		loop, err := New(WithAutoExit(true))
		if err != nil {
			panic(err)
		}
		var got []int
		var callbackErrs fuzzErrs
		var scheduleJob func(microtaskChild)
		scheduleJob = func(c microtaskChild) {
			job := func() {
				got = append(got, c.id)
				for _, next := range specs[c.id].children {
					scheduleJob(next)
				}
			}
			var schedule func(func()) error
			if c.kind == 0 {
				schedule = loop.ScheduleNextTick
			} else {
				schedule = loop.ScheduleMicrotask
			}
			var err error
			if c.foreign {
				err = admitForeignCallback(schedule, job)
			} else {
				err = schedule(job)
			}
			if err != nil {
				callbackErrs.add("schedule job %d kind %d: %v", c.id, c.kind, err)
			}
		}
		for _, c := range initial {
			scheduleJob(c)
		}

		if err := runAutoExitLoop(t, loop); err != nil {
			t.Fatalf("Run: %v", err)
		}
		callbackErrs.failNow(t)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("microtask trace mismatch\ninitial %+v\nspecs %+v\ngot  %v\nwant %v", initial, specs, got, want)
		}
	})
}

func simulateMicrotaskTrace(initial []microtaskChild, specs []microtaskJobSpec) []int {
	ingress := append([]microtaskChild(nil), initial...)
	nextTicks := make([]int, 0, len(specs))
	microtasks := make([]int, 0, len(specs))
	trace := make([]int, 0, len(specs))
	materialize := func() {
		for _, job := range ingress {
			if job.kind == 0 {
				nextTicks = append(nextTicks, job.id)
			} else {
				microtasks = append(microtasks, job.id)
			}
		}
		ingress = ingress[:0]
	}
	run := func(id int) {
		trace = append(trace, id)
		for _, child := range specs[id].children {
			if child.foreign {
				ingress = append(ingress, child)
				continue
			}
			// An owner-local schedule first transfers earlier acknowledged
			// foreign ingress, preserving completion order within each queue.
			materialize()
			if child.kind == 0 {
				nextTicks = append(nextTicks, child.id)
			} else {
				microtasks = append(microtasks, child.id)
			}
		}
	}
	for len(ingress) != 0 || len(nextTicks) != 0 || len(microtasks) != 0 {
		materialize()
		for {
			for len(nextTicks) != 0 {
				id := nextTicks[0]
				nextTicks = nextTicks[1:]
				run(id)
			}
			// Acknowledged ingress joins the checkpoint after an exhaustive
			// nextTick owner batch, before a Promise batch may begin.
			materialize()
			if len(nextTicks) == 0 {
				break
			}
		}
		for len(microtasks) != 0 {
			id := microtasks[0]
			microtasks = microtasks[1:]
			run(id)
		}
	}
	return trace
}
