package eventloop

import (
	"runtime"
	"strconv"
	"testing"
)

func BenchmarkEventTargetDispatchReusedEvent(b *testing.B) {
	target := NewEventTarget()
	target.AddEventListener("event", func(*Event) {})
	event := NewEvent("event")
	if !target.DispatchEvent(event) {
		b.Fatal("warmup dispatch was canceled")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if !target.DispatchEvent(event) {
			b.Fatal("dispatch was canceled")
		}
	}
}

func BenchmarkEventTargetDispatchFreshEvent(b *testing.B) {
	target := NewEventTarget()
	target.AddEventListener("event", func(*Event) {})
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if !target.DispatchEvent(NewEvent("event")) {
			b.Fatal("dispatch was canceled")
		}
	}
}

func BenchmarkEventTargetDispatchEmptyReusedEvent(b *testing.B) {
	target := NewEventTarget()
	event := NewEvent("event")
	if !target.DispatchEvent(event) {
		b.Fatal("warmup dispatch was canceled")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if !target.DispatchEvent(event) {
			b.Fatal("dispatch was canceled")
		}
	}
}

func BenchmarkEventTargetDispatchParallelDistinctEvents(b *testing.B) {
	target := NewEventTarget()
	target.AddEventListener("event", func(*Event) {})
	b.ReportAllocs()
	b.RunParallel(func(parallel *testing.PB) {
		event := NewEvent("event")
		for parallel.Next() {
			if !target.DispatchEvent(event) {
				panic("eventloop: benchmark dispatch was canceled")
			}
		}
	})
}

func BenchmarkEventTargetDispatchParallelDistinctEventsControl(b *testing.B) {
	target := NewEventTarget()
	target.AddEventListener("event", func(event *Event) {
		event.PreventDefault()
		event.StopImmediatePropagation()
		if !event.PropagationStopped() || !event.ImmediatePropagationStopped() {
			panic("eventloop: benchmark control state was not observable")
		}
	})
	b.ReportAllocs()
	b.RunParallel(func(parallel *testing.PB) {
		event := &Event{Type: "event", Cancelable: true}
		for parallel.Next() {
			if target.DispatchEvent(event) {
				panic("eventloop: benchmark cancelable dispatch was not canceled")
			}
		}
	})
}

var benchmarkEventTargetTypes = [...]string{
	"live-0",
	"live-1",
	"live-2",
	"live-3",
	"live-4",
	"live-5",
	"live-6",
}

func BenchmarkEventTargetListenerRegistrationWithLiveSet(b *testing.B) {
	for _, liveCount := range []int{0, 100, 1000, 10000} {
		b.Run(strconv.Itoa(liveCount), func(b *testing.B) {
			target := NewEventTarget()
			for i := range liveCount {
				target.AddEventListener(benchmarkEventTargetTypes[i%len(benchmarkEventTargetTypes)], func(*Event) {})
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				id := target.AddEventListener("temporary", func(*Event) {})
				if !target.RemoveEventListenerByID("temporary", id) {
					b.Fatal("failed to remove temporary listener")
				}
			}
			runtime.KeepAlive(target)
		})
	}
}

func BenchmarkEventTargetListenerConstruction(b *testing.B) {
	for _, listenerCount := range []int{100, 1000, 5000, 10000} {
		b.Run(strconv.Itoa(listenerCount), func(b *testing.B) {
			b.ReportAllocs()
			b.ReportMetric(float64(listenerCount), "listeners/op")
			var target *EventTarget
			for b.Loop() {
				target = NewEventTarget()
				for i := range listenerCount {
					target.AddEventListener(benchmarkEventTargetTypes[i%len(benchmarkEventTargetTypes)], func(*Event) {})
				}
			}
			runtime.KeepAlive(target)
		})
	}
}
