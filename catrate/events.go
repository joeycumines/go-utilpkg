package catrate

import (
	"time"
)

// filterEvents filters an array of event timestamps (represented in UnixNano)
// based on a map of rates, which specify how many events are allowed per
// certain duration. The function returns only the relevant events that
// fall within the defined rate limits, and also calculates the time remaining
// until the next event can occur without violating the rate limits.
// Returns an array of relevant event timestamps (int64 format) and the
// shortest duration to wait before the next event can occur without violating
// the rate limits.
func filterEvents(now time.Time, rates map[time.Duration]int, events *ringBuffer[int64]) (remaining time.Duration) {
	// Initializing the index for the first relevant event.
	// All events before this index will be discarded as they fall outside the rate limits.
	indexFirstRelevant := events.Len()

	// Looping through each rate limit.
	for rate, limit := range rates {
		if limit <= 0 || rate <= 0 {
			// If the rate limit is invalid, skip it.
			continue
		}

		// Define the boundary of the window for this rate.
		// Events equal to or older than this boundary are irrelevant.
		boundary := now.Add(-rate)

		// Using binary search to find the index of the first event
		// that is newer than the boundary.
		// This index is the first relevant event, for rate.
		index := events.Search(boundary.UnixNano() + 1)
		if index < indexFirstRelevant {
			indexFirstRelevant = index
		}

		// If the limit has been reached or exceeded, calculate the offset
		// between the boundary and the timestamp of the last event that would
		// exceed the limit, if another event were to occur.
		// The offset is the remaining (time until the next event), for rate.
		if limit <= events.Len()-index {
			offset := time.Unix(0, events.Get(events.Len()-limit)).Sub(boundary)
			if offset > remaining {
				remaining = offset
			}
		}
	}

	// Discard all events before the first relevant event.
	events.RemoveBefore(indexFirstRelevant)

	// Return the remaining (time until the next event).
	return remaining
}
