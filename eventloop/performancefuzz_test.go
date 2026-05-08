package eventloop

import (
	"math"
	"testing"
)

func FuzzPerformanceTimelineModel(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte("mark-measure-clear"))
	f.Add([]byte{255, 1, 2, 3, 4, 5, 6, 7})

	f.Fuzz(func(t *testing.T, data []byte) {
		r := newFuzzReader(data)
		perf := NewPerformance()
		names := []string{"", "start", "end", "measure", "α"}

		lastNow := perf.Now()
		if origin := perf.TimeOrigin(); math.IsNaN(origin) || math.IsInf(origin, 0) || origin <= 0 {
			t.Fatalf("invalid TimeOrigin: %v", origin)
		}

		ops := 1 + min(len(data)*3, 512)
		for range ops {
			name := names[r.intn(len(names))]
			if r.bool() {
				name = r.smallString(10)
			}
			switch r.byte() % 7 {
			case 0:
				perf.Mark(name)
			case 1:
				perf.MarkWithDetail(name, map[string]any{"name": name})
			case 2, 3:
				start := names[r.intn(len(names))]
				end := names[r.intn(len(names))]
				if r.bool() {
					start = ""
				}
				if r.bool() {
					end = ""
				}
				err := perf.MeasureWithDetail(name, start, end, r.smallString(4))
				if err == nil {
					entries := perf.GetEntriesByName(name, "measure")
					if len(entries) == 0 {
						t.Fatalf("successful MeasureWithDetail(%q) did not create a measure entry", name)
					}
				} else if start == "" && end == "" {
					t.Fatalf("MeasureWithDetail using origin/current failed: %v", err)
				}
			case 4:
				perf.ClearMarks(name)
			case 5:
				perf.ClearMeasures(name)
			case 6:
				perf.ClearResourceTimings()
			}

			now := perf.Now()
			if now < lastNow {
				t.Fatalf("Performance.Now moved backwards: %v < %v", now, lastNow)
			}
			lastNow = now

			entries := perf.GetEntries()
			if len(entries) > 0 {
				mutated := entries
				mutated[0].Name = "mutated-by-test"
				if len(perf.GetEntries()) > 0 && perf.GetEntries()[0].Name == "mutated-by-test" {
					t.Fatalf("GetEntries did not return a defensive copy")
				}
			}
			for _, entry := range perf.GetEntriesByType("mark") {
				if entry.EntryType != "mark" || entry.Duration != 0 {
					t.Fatalf("invalid mark entry: %+v", entry)
				}
			}
			for _, entry := range perf.GetEntriesByType("measure") {
				if entry.EntryType != "measure" || math.IsNaN(entry.Duration) || math.IsInf(entry.Duration, 0) {
					t.Fatalf("invalid measure entry: %+v", entry)
				}
			}
			json := perf.ToJSON()
			if _, ok := json["timeOrigin"].(float64); !ok {
				t.Fatalf("ToJSON missing float64 timeOrigin: %#v", json)
			}
		}
	})
}
