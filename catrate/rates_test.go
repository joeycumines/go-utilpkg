package catrate

import (
	"testing"
	"time"
)

// TestParseRates tests the parseRates function with multiple cases
func TestParseRates(t *testing.T) {
	tests := []struct {
		name     string
		rates    map[time.Duration]int
		expected time.Duration
		isValid  bool
	}{
		{
			name:     "empty_rates",
			rates:    map[time.Duration]int{},
			expected: 0,
			isValid:  false,
		},
		{
			name:     "rates_with_zero_values",
			rates:    map[time.Duration]int{time.Second: 0, time.Minute: 0},
			expected: 0,
			isValid:  false,
		},
		{
			name:     "rates_with_negative_values",
			rates:    map[time.Duration]int{time.Second: -1, time.Minute: -2},
			expected: 0,
			isValid:  false,
		},
		{
			name:     "rates_with_non_relevant_values",
			rates:    map[time.Duration]int{time.Second: 10, time.Minute: 9},
			expected: 0,
			isValid:  false,
		},
		{
			name: "valid_rates",
			rates: map[time.Duration]int{
				time.Second: 3,
				time.Minute: 120,
				time.Hour:   3600,
			},
			expected: time.Hour,
			isValid:  true,
		},
		{
			name: "realistic_rate_limiting",
			rates: map[time.Duration]int{
				time.Second:         1,
				time.Minute:         10,
				time.Hour:           500,
				24 * time.Hour:      8000,
				7 * 24 * time.Hour:  50000,
				30 * 24 * time.Hour: 200000,
			},
			expected: 30 * 24 * time.Hour,
			isValid:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, valid := parseRates(tt.rates)

			if got != tt.expected || valid != tt.isValid {
				t.Errorf("parseRates(%v) = %v, %v; want %v, %v", tt.rates, got, valid, tt.expected, tt.isValid)
			}
		})
	}
}
