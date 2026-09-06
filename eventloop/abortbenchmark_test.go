package eventloop

import "testing"

var (
	abortBenchmarkReason     = &struct{}{}
	abortBenchmarkSignalSink *AbortSignal
)

func BenchmarkAbortAnyEmpty(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		abortBenchmarkSignalSink = AbortAny()
	}
}

func BenchmarkAbortAnyPreSettled(b *testing.B) {
	controller := NewAbortController()
	controller.Abort(abortBenchmarkReason)
	signal := controller.Signal()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		abortBenchmarkSignalSink = AbortAny(signal)
	}
}

func BenchmarkAbortAnyPendingSettlement(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		controller := NewAbortController()
		composite := AbortAny(controller.Signal())
		controller.Abort(abortBenchmarkReason)
		abortBenchmarkSignalSink = composite
	}
}
