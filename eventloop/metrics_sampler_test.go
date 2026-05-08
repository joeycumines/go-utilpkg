package eventloop

import (
	"testing"
	"time"
)

// latencyTestSampler adapts the production runtime sampler for focused
// algorithm tests without restoring mutation methods to public snapshots.
type latencyTestSampler struct {
	LatencyMetrics
	runtime runtimeMetrics
}

func (s *latencyTestSampler) Record(duration time.Duration) {
	s.runtime.recordCallback(duration, time.Time{}, false)
}

func (s *latencyTestSampler) Sample() int {
	s.LatencyMetrics = s.runtime.snapshot().Latency
	return int(s.Count)
}

// queueTestSampler adapts the production queue sampler for focused tests.
type queueTestSampler struct {
	QueueMetrics
	runtime                      runtimeMetrics
	ingress, internal, microtask int
}

func (s *queueTestSampler) UpdateIngress(depth int) {
	s.ingress = depth
	s.update()
}

func (s *queueTestSampler) UpdateInternal(depth int) {
	s.internal = depth
	s.update()
}

func (s *queueTestSampler) UpdateMicrotask(depth int) {
	s.microtask = depth
	s.update()
}

func (s *queueTestSampler) updateDepths(ingress, internal, microtask int) {
	s.ingress = ingress
	s.internal = internal
	s.microtask = microtask
	s.update()
}

func (s *queueTestSampler) update() {
	s.runtime.recordQueueDepths(s.ingress, s.internal, s.microtask)
	s.QueueMetrics = s.runtime.snapshot().Queue
}

func TestQueueMetricsSampler(t *testing.T) {
	var sampler queueTestSampler
	if sampler.IngressCurrent != 0 || sampler.InternalCurrent != 0 || sampler.MicrotaskCurrent != 0 {
		t.Fatalf("initial queue depths = (%d, %d, %d), want all zero", sampler.IngressCurrent, sampler.InternalCurrent, sampler.MicrotaskCurrent)
	}

	sampler.updateDepths(10, 20, 30)
	if sampler.IngressCurrent != 10 || sampler.InternalCurrent != 20 || sampler.MicrotaskCurrent != 30 {
		t.Fatalf("initialized queue currents = (%d, %d, %d), want (10, 20, 30)", sampler.IngressCurrent, sampler.InternalCurrent, sampler.MicrotaskCurrent)
	}
	if sampler.IngressAvg != 10 || sampler.InternalAvg != 20 || sampler.MicrotaskAvg != 30 {
		t.Fatalf("initialized queue averages = (%v, %v, %v), want (10, 20, 30)", sampler.IngressAvg, sampler.InternalAvg, sampler.MicrotaskAvg)
	}

	sampler.updateDepths(0, 0, 0)
	if sampler.IngressCurrent != 0 || sampler.InternalCurrent != 0 || sampler.MicrotaskCurrent != 0 {
		t.Fatalf("updated queue currents = (%d, %d, %d), want all zero", sampler.IngressCurrent, sampler.InternalCurrent, sampler.MicrotaskCurrent)
	}
	if sampler.IngressMax != 10 || sampler.InternalMax != 20 || sampler.MicrotaskMax != 30 {
		t.Fatalf("queue maxima = (%d, %d, %d), want (10, 20, 30)", sampler.IngressMax, sampler.InternalMax, sampler.MicrotaskMax)
	}
	if sampler.IngressAvg != 9 || sampler.InternalAvg != 18 || sampler.MicrotaskAvg != 27 {
		t.Fatalf("queue EMA recurrence = (%v, %v, %v), want (9, 18, 27)", sampler.IngressAvg, sampler.InternalAvg, sampler.MicrotaskAvg)
	}
}
