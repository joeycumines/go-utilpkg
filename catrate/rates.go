package catrate

import (
	"golang.org/x/exp/slices"
	"time"
)

// parseRates validates rates and limits, and calculates the retention
// duration, where the retention duration is the largest duration for which a
// relevant rate is defined. Any irrelevant rates or invalid rates will
// result in an error. Rates are valid if both the duration and rate are
// greater than zero.
//
// The requirements for rate relevance are:
//
//  1. The rate value (count) must be less than that of any longer durations
//  2. The effective rate (count/duration) must be less than that of any shorter durations
func parseRates(rates map[time.Duration]int) (time.Duration, bool) {
	if len(rates) == 0 {
		return 0, false
	}

	durations := make([]time.Duration, 0, len(rates))
	for duration := range rates {
		durations = append(durations, duration)
	}

	slices.Sort(durations)

	for i, duration := range durations {
		rate := rates[duration]
		if rate <= 0 || duration <= 0 {
			return 0, false
		}

		if (i < len(durations)-1 && rate >= rates[durations[i+1]]) ||
			(i > 0 && float64(rate)/float64(duration) >= float64(rates[durations[i-1]])/float64(durations[i-1])) {
			return 0, false
		}
	}

	return durations[len(durations)-1], true
}
