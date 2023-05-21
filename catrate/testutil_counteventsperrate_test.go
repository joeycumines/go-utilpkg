package catrate

import (
	"golang.org/x/exp/slices"
	"reflect"
	"strconv"
	"testing"
	"time"
)

func countEventsPerRate(rates map[time.Duration]int, now int64, events []int64) map[time.Duration]int {
	r := make(map[time.Duration]int, len(rates))
	for rate := range rates {
		r[rate] = 0
	}
	for _, event := range events {
		if event > now {
			panic("event is in the future")
		}
		for rate := range rates {
			if now-int64(rate) < event {
				r[rate]++
			}
		}
	}
	return r
}

func ratesExceedingOrMeetingLimits(windows map[time.Duration]int, counts map[time.Duration]int) (rates []time.Duration) {
	for rate, limit := range windows {
		if counts[rate] >= limit {
			rates = append(rates, rate)
		}
	}
	slices.Sort(rates)
	return
}

func TestCountEventsPerRate(t *testing.T) {
	tests := []struct {
		name  string
		rates map[time.Duration]int
		// note any events after now will be filtered prior to passing to the test
		events []int64
		// the first key is an input, the rest is the output
		nowResults map[int64]map[time.Duration]int
	}{
		{
			name: "multiple rates and events",
			rates: map[time.Duration]int{
				time.Hour:     2,
				2 * time.Hour: 3,
				3 * time.Hour: 3,
			},
			events: []int64{
				50 * int64(time.Minute),
				120 * int64(time.Minute),
				150 * int64(time.Minute),
				200 * int64(time.Minute),
				250 * int64(time.Minute),
				300 * int64(time.Minute),
				350 * int64(time.Minute),
				400 * int64(time.Minute),
				450 * int64(time.Minute),
				500 * int64(time.Minute),
				600 * int64(time.Minute),
				700 * int64(time.Minute),
				800 * int64(time.Minute),
				900 * int64(time.Minute),
				1000 * int64(time.Minute),
				1100 * int64(time.Minute),
				1200 * int64(time.Minute),
				1300 * int64(time.Minute),
				1400 * int64(time.Minute),
				1500 * int64(time.Minute),
				1600 * int64(time.Minute),
				1700 * int64(time.Minute),
				1800 * int64(time.Minute),
				1900 * int64(time.Minute),
				2000 * int64(time.Minute),
			},
			nowResults: map[int64]map[time.Duration]int{
				50*int64(time.Minute) - 1: {
					time.Hour:     0,
					2 * time.Hour: 0,
					3 * time.Hour: 0,
				},
				50 * int64(time.Minute): {
					time.Hour:     1,
					2 * time.Hour: 1,
					3 * time.Hour: 1,
				},
				60 * int64(time.Minute): {
					time.Hour:     1,
					2 * time.Hour: 1,
					3 * time.Hour: 1,
				},
				90 * int64(time.Minute): {
					time.Hour:     1,
					2 * time.Hour: 1,
					3 * time.Hour: 1,
				},
				120 * int64(time.Minute): {
					time.Hour:     1,
					2 * time.Hour: 2,
					3 * time.Hour: 2,
				},
				150 * int64(time.Minute): {
					time.Hour:     2,
					2 * time.Hour: 3,
					3 * time.Hour: 3,
				},
				180 * int64(time.Minute): {
					time.Hour:     1,
					2 * time.Hour: 2,
					3 * time.Hour: 3,
				},
				210 * int64(time.Minute): {
					time.Hour:     1,
					2 * time.Hour: 3,
					3 * time.Hour: 4,
				},
				// bound check, 1ns before 3h (the largest rate) after the last event
				2180*int64(time.Minute) - 1: {
					time.Hour:     0,
					2 * time.Hour: 0,
					3 * time.Hour: 1,
				},
				// bound check, 3 hours (the largest rate) after the last event
				2180 * int64(time.Minute): {
					time.Hour:     0,
					2 * time.Hour: 0,
					3 * time.Hour: 0,
				},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for now, expected := range tc.nowResults {
				t.Run(strconv.FormatInt(now, 10), func(t *testing.T) {
					var events []int64
					for _, event := range tc.events {
						if event <= now {
							events = append(events, event)
						}
					}
					slices.Sort(events)
					actual := countEventsPerRate(tc.rates, now, events)
					t.Logf("at %s: %v\n%v\n%v", time.Duration(now), tc.rates, actual, ratesExceedingOrMeetingLimits(tc.rates, actual))
					if !reflect.DeepEqual(actual, expected) {
						t.Errorf("Expected count %v, but got %v", expected, actual)
					}
				})
			}
		})
	}
}
