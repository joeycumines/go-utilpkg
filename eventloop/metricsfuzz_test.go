package eventloop

import (
	"math"
	"testing"
	"time"
)

func FuzzMetricsAndPSquareInvariants(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9})
	f.Add([]byte("psquare-latency-queue-tps"))

	f.Fuzz(func(t *testing.T, data []byte) {
		r := newFuzzReader(data)
		q := newPSquareQuantile(float64(int(r.byte())-32) / 16.0)
		m := newPSquareMultiQuantile(0, 0.5, 0.9, 0.99, 1)
		latency := &latencyTestSampler{}
		queue := &queueTestSampler{}
		windowBuckets := 1 + r.intn(20)
		bucket := time.Duration(1+r.intn(50)) * time.Millisecond
		tps := newTPSCounter(time.Duration(windowBuckets)*bucket, bucket)
		base := time.Unix(1700000000, 0)

		count := 1 + min(len(data)*2, 1000)
		minVal := math.Inf(1)
		maxVal := math.Inf(-1)
		for i := range count {
			v := float64(r.uint64()%1_000_000) / 10.0
			if r.bool() {
				v = -v
			}
			minVal = math.Min(minVal, v)
			maxVal = math.Max(maxVal, v)
			q.Update(v)
			m.Update(v)

			duration := time.Duration(r.uint64()%uint64(time.Second)) + time.Nanosecond
			latency.Record(duration)
			queue.UpdateIngress(int(r.uint64() % 10000))
			queue.UpdateInternal(int(r.uint64() % 10000))
			queue.UpdateMicrotask(int(r.uint64() % 10000))
			tOffset := time.Duration(int64(r.uint64()%uint64(2*time.Second)) - int64(time.Second))
			tps.IncrementAt(base.Add(time.Duration(i)*bucket + tOffset))
		}

		if got := q.Count(); got != uint64(count) {
			t.Fatalf("pSquare count = %d, want %d", got, count)
		}
		quantile := q.Quantile()
		if math.IsNaN(quantile) || math.IsInf(quantile, 0) || quantile < minVal || quantile > maxVal {
			t.Fatalf("pSquare quantile out of bounds: q=%v min=%v max=%v", quantile, minVal, maxVal)
		}
		if gotMax := q.Max(); gotMax != maxVal {
			t.Fatalf("pSquare max = %v, want %v", gotMax, maxVal)
		}

		for i := range 5 {
			got := m.Quantile(i)
			if math.IsNaN(got) || math.IsInf(got, 0) || got < minVal || got > maxVal {
				t.Fatalf("multi quantile %d out of bounds: %v min=%v max=%v", i, got, minVal, maxVal)
			}
		}
		samples := latency.Sample()
		if samples != count {
			t.Fatalf("latency Sample count = %d, want %d", samples, count)
		}
		if latency.Max <= 0 || latency.Mean <= 0 || latency.P50 <= 0 || latency.P99 > latency.Max {
			t.Fatalf("invalid latency metrics: %+v", latency)
		}
		if queue.IngressCurrent < 0 || queue.InternalCurrent < 0 || queue.MicrotaskCurrent < 0 {
			t.Fatalf("negative queue metric: %+v", queue)
		}
		if queue.IngressCurrent > queue.IngressMax || queue.InternalCurrent > queue.InternalMax || queue.MicrotaskCurrent > queue.MicrotaskMax {
			t.Fatalf("queue current exceeded max: %+v", queue)
		}
		gotTPS := tps.TPS()
		if math.IsNaN(gotTPS) || math.IsInf(gotTPS, 0) || gotTPS < 0 {
			t.Fatalf("invalid TPS: %v", gotTPS)
		}
	})
}
