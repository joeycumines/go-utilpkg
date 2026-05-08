package eventloop

import "testing"

var (
	abortBenchmarkReason     = &struct{}{}
	abortBenchmarkSignalSink *AbortSignal
)

func BenchmarkAbortAnyEmpty(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		abortBenchmarkSignalSink = AbortAny(nil)
	}
}

func BenchmarkAbortAnyPreSettled(b *testing.B) {
	controller := NewAbortController()
	controller.Abort(abortBenchmarkReason)
	signal := controller.Signal()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		abortBenchmarkSignalSink = AbortAny([]*AbortSignal{signal})
	}
}

func BenchmarkAbortAnyPendingSettlement(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		controller := NewAbortController()
		composite := AbortAny([]*AbortSignal{controller.Signal()})
		controller.Abort(abortBenchmarkReason)
		abortBenchmarkSignalSink = composite
	}
}
