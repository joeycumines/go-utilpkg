package catrate

import (
	"golang.org/x/exp/slices"
	"math/rand"
	"reflect"
	"testing"
	"time"
)

func int64SliceEqual(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}

// adapt tests for the slice version
func filterEventsTestAdapter(now time.Time, rates map[time.Duration]int, events []int64) (_ []int64, remaining time.Duration) {
	rb := newRingBufferFrom(events)
	remaining = filterEvents(now, rates, rb)
	return rb.Slice(), remaining
}

//// for benchmarking
//func filterEventsSlice(now time.Time, rates map[time.Duration]int, events []int64) (_ []int64, remaining time.Duration) {
//	indexFirstRelevant := len(events)
//	for rate, limit := range rates {
//		if limit <= 0 || rate <= 0 {
//			continue
//		}
//		boundary := now.Add(-rate)
//		index, _ := slices.BinarySearch(events, boundary.UnixNano()+1)
//		if index < indexFirstRelevant {
//			indexFirstRelevant = index
//		}
//		if limit <= len(events)-index {
//			offset := time.Unix(0, events[len(events)-limit]).Sub(boundary)
//			if offset > remaining {
//				remaining = offset
//			}
//		}
//	}
//	return events[indexFirstRelevant:], remaining
//}

func TestFilterEvents_notLimited(t *testing.T) {
	rates := map[time.Duration]int{
		1 * time.Second: 2,
		2 * time.Second: 3,
	}

	now := time.Unix(123456789, 123456789)
	events := []int64{
		now.Add(-3 * time.Second).UnixNano(),
		now.Add(-2 * time.Second).UnixNano(),
		now.Add(-1 * time.Second).UnixNano(),
		now.UnixNano(),
	}

	// expecting 2 most recent events (as per 1-second rate), remaining time should be 0
	wantEvents := []int64{now.Add(-1 * time.Second).UnixNano(), now.UnixNano()}
	wantRemaining := time.Duration(0)

	gotEvents, gotRemaining := filterEventsTestAdapter(now, rates, events)

	if !reflect.DeepEqual(gotEvents, wantEvents) {
		t.Errorf("filterEvents() = %v, want %v", gotEvents, wantEvents)
	}
	if gotRemaining != wantRemaining {
		t.Errorf("filterEvents() = %v, want %v", gotRemaining, wantRemaining)
	}
}

func TestFilterEvents(t *testing.T) {
	now := time.Unix(3, 0)
	oneSecondAgo := now.Add(-time.Second)
	twoSecondsAgo := now.Add(-2 * time.Second)
	threeSecondsAgo := now.Add(-3 * time.Second)

	for _, tt := range [...]struct {
		name             string
		now              time.Time
		rates            map[time.Duration]int
		events           []int64
		expectedEvents   []int64
		expectedDuration time.Duration
	}{
		{
			name:             "no rates",
			now:              now,
			rates:            map[time.Duration]int{},
			events:           []int64{twoSecondsAgo.UnixNano(), oneSecondAgo.UnixNano()},
			expectedEvents:   []int64{},
			expectedDuration: 0,
		},
		{
			name: "invalid rates",
			now:  now,
			rates: map[time.Duration]int{
				time.Second: 0,
				0:           1,
			},
			events:           []int64{twoSecondsAgo.UnixNano(), oneSecondAgo.UnixNano()},
			expectedEvents:   []int64{},
			expectedDuration: 0,
		},
		{
			name:             "no events",
			now:              now,
			rates:            map[time.Duration]int{time.Second: 1},
			events:           []int64{},
			expectedEvents:   []int64{},
			expectedDuration: 0,
		},
		{
			name: "one event is on the boundary of expiration and is therefore irrelevant",
			now:  now,
			rates: map[time.Duration]int{
				2 * time.Second: 2,
			},
			events:           []int64{twoSecondsAgo.UnixNano(), oneSecondAgo.UnixNano()},
			expectedEvents:   []int64{oneSecondAgo.UnixNano()},
			expectedDuration: 0,
		},
		{
			name: "all events are relevant and there is need to wait",
			now:  now,
			rates: map[time.Duration]int{
				2 * time.Second: 2,
			},
			events:           []int64{twoSecondsAgo.UnixNano() + 1, oneSecondAgo.UnixNano()},
			expectedEvents:   []int64{twoSecondsAgo.UnixNano() + 1, oneSecondAgo.UnixNano()},
			expectedDuration: 1,
		},
		{
			name: "all events are irrelevant",
			now:  now,
			rates: map[time.Duration]int{
				time.Second: 1,
			},
			events:           []int64{threeSecondsAgo.UnixNano(), twoSecondsAgo.UnixNano()},
			expectedEvents:   []int64{},
			expectedDuration: 0,
		},
		{
			name: "mixed relevant and irrelevant events",
			now:  now,
			rates: map[time.Duration]int{
				2 * time.Second: 1,
			},
			events:           []int64{threeSecondsAgo.UnixNano(), twoSecondsAgo.UnixNano(), oneSecondAgo.UnixNano()},
			expectedEvents:   []int64{oneSecondAgo.UnixNano()},
			expectedDuration: time.Second,
		},
		{
			name: "multiple rates, all events relevant",
			now:  now,
			rates: map[time.Duration]int{
				2 * time.Second:   2,
				3*time.Second + 1: 3,
			},
			events:           []int64{threeSecondsAgo.UnixNano(), twoSecondsAgo.UnixNano(), oneSecondAgo.UnixNano()},
			expectedEvents:   []int64{threeSecondsAgo.UnixNano(), twoSecondsAgo.UnixNano(), oneSecondAgo.UnixNano()},
			expectedDuration: 1,
		},
		{
			name: "multiple rates, all relevant, due to different rates",
			now:  now,
			rates: map[time.Duration]int{
				3*time.Second + 1:                    2,
				1*time.Second + 500*time.Millisecond: 1,
			},
			events:           []int64{threeSecondsAgo.UnixNano(), twoSecondsAgo.UnixNano(), oneSecondAgo.UnixNano()},
			expectedEvents:   []int64{threeSecondsAgo.UnixNano(), twoSecondsAgo.UnixNano(), oneSecondAgo.UnixNano()},
			expectedDuration: time.Second + 1,
		},
		{
			name: "multiple rates, no events relevant",
			now:  now,
			rates: map[time.Duration]int{
				1 * time.Second: 1,
				2 * time.Second: 1,
			},
			events:           []int64{threeSecondsAgo.UnixNano(), twoSecondsAgo.UnixNano()},
			expectedEvents:   []int64{},
			expectedDuration: 0,
		},
		{
			name: "multiple rates, overlapping windows",
			now:  now,
			rates: map[time.Duration]int{
				2 * time.Second: 1,
				3 * time.Second: 2,
			},
			events:           []int64{threeSecondsAgo.UnixNano(), twoSecondsAgo.UnixNano(), oneSecondAgo.UnixNano()},
			expectedEvents:   []int64{twoSecondsAgo.UnixNano(), oneSecondAgo.UnixNano()},
			expectedDuration: time.Second,
		},
		{
			name: "multiple rates, non-overlapping windows",
			now:  now,
			rates: map[time.Duration]int{
				1*time.Second + 1: 1,
				3 * time.Second:   1,
			},
			events:           []int64{threeSecondsAgo.UnixNano(), twoSecondsAgo.UnixNano(), oneSecondAgo.UnixNano()},
			expectedEvents:   []int64{twoSecondsAgo.UnixNano(), oneSecondAgo.UnixNano()},
			expectedDuration: 2 * time.Second,
		},
		{
			name: "multiple rates, identical limits different durations",
			now:  now,
			rates: map[time.Duration]int{
				1 * time.Second: 1,
				2 * time.Second: 1,
			},
			events:           []int64{threeSecondsAgo.UnixNano(), twoSecondsAgo.UnixNano(), oneSecondAgo.UnixNano()},
			expectedEvents:   []int64{oneSecondAgo.UnixNano()},
			expectedDuration: time.Second,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			events, remaining := filterEventsTestAdapter(tt.now, tt.rates, tt.events)
			if !int64SliceEqual(events, tt.expectedEvents) {
				t.Errorf("expected events %v, got %v", tt.expectedEvents, events)
			}
			if remaining != tt.expectedDuration {
				t.Errorf("expected remaining duration %v, got %v", tt.expectedDuration, remaining)
			}
		})
	}
}

func FuzzFilterEvents(f *testing.F) {
	f.Add(int64(0), 0, 0, int64(1234))
	f.Add(int64(0), 1, 1, int64(1234))
	f.Add(int64(574221), 3, 50, int64(-23434245))
	f.Add(int64(0), 10, 500, int64(4))

	// note: can't have slices maps structs etc as values in the seed corpus
	f.Fuzz(func(t *testing.T, nowUnixNano int64, ratesCount int, eventsCount int, randomSeed int64) {
		if nowUnixNano < 0 || ratesCount < 0 || ratesCount > 10 || eventsCount < 0 || eventsCount > 2048 {
			t.Skipf(`invalid input: nowUnixNano=%d, ratesCount=%d, eventsCount=%d`, nowUnixNano, ratesCount, eventsCount)
		}

		now := time.Unix(0, nowUnixNano)

		// needs to be deterministic
		r := rand.New(rand.NewSource(randomSeed))

		const maxDur = time.Hour

		rates := make(map[time.Duration]int)
		for i := 0; i < ratesCount; i++ {
			duration := time.Duration(r.Int63n(int64(maxDur))) + 1 // Random durations 1ns-1h
			limit := r.Intn(10) + 1                                // Random limits 1-10
			rates[duration] = limit
		}

		events := make([]int64, eventsCount)
		for i := 0; i < eventsCount; i++ {
			// random events up to and including now, from 1h ago
			events[i] = nowUnixNano - int64(maxDur) + r.Int63n(int64(maxDur+1))
		}
		slices.Sort(events)

		counts := countEventsPerRate(rates, now.UnixNano(), events)
		limited := ratesExceedingOrMeetingLimits(rates, counts)

		filtered, remaining := filterEventsTestAdapter(now, rates, events)

		// log the test case on error for debugging
		defer func() {
			if t.Failed() {
				t.Logf("f.Add(int64(%d), %d, %d, int64(%d))\nnow=%v\nrates=%v\nevents=%v\nfiltered=%v\nremaining=%v\ncounts=%v\nlimited=%v", nowUnixNano, ratesCount, eventsCount, randomSeed, now.UnixNano(), rates, events, filtered, remaining, counts, limited)
			}
		}()

		if remaining < 0 {
			t.Errorf("expected remaining to be >= 0, got %v", remaining)
		}

		if remaining > 0 && len(filtered) == 0 {
			t.Errorf("expected filtered to be non-empty if remaining is non-zero")
		}

		if len(filtered) != 0 && len(events) == 0 {
			t.Error(`filtered non-empty but events isn't`)
		}

		if !reflect.DeepEqual(counts, countEventsPerRate(rates, now.UnixNano(), filtered)) {
			t.Errorf("expected counts to be the same for events and filtered")
		}

		if (len(limited) == 0) != (remaining == 0) {
			t.Errorf("expected remaining to be 0 if and only if there are no rates limited")
		}

		if remaining > 1 {
			// simulate waiting until just before it would be no longer limited, and check that it is still limited
			nowNext := now.Add(remaining - 1)
			filteredNext, remainingNext := filterEventsTestAdapter(nowNext, rates, events)
			if len(filteredNext) == 0 || remainingNext != 1 {
				t.Errorf("unexpected result when waiting until just before remaining would be 0: filtered=%v, remaining=%v", filteredNext, remainingNext)
			}
			countsNext := countEventsPerRate(rates, nowNext.UnixNano(), events)
			limitedNext := ratesExceedingOrMeetingLimits(rates, countsNext)
			if len(limitedNext) == 0 {
				t.Errorf("expected rates to reach their limits after waiting until just before remaining would be 0\nfiltered=%v, remaining=%v\n%v\n%v", filteredNext, remainingNext, countsNext, limitedNext)
			}
		}

		if remaining > 0 {
			// simulate waiting for remaining, and check that it is no longer limited
			nowNext := now.Add(remaining)
			filteredNext, remainingNext := filterEventsTestAdapter(nowNext, rates, events)
			if remainingNext != 0 {
				t.Errorf("expected remaining to be 0 after waiting for remaining: filtered=%v, remaining=%v", filteredNext, remainingNext)
			}
			countsNext := countEventsPerRate(rates, nowNext.UnixNano(), events)
			limitedNext := ratesExceedingOrMeetingLimits(rates, countsNext)
			if len(limitedNext) != 0 {
				t.Errorf("expected rates to not reach their limits after waiting for remaining\nfiltered=%v, remaining=%v\n%v\n%v", filteredNext, remainingNext, countsNext, limitedNext)
			}
		}
	})
}
