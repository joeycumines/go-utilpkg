package eventloop

import (
	"math"
)

// pSquareQuantile implements the P-Square algorithm for streaming quantile estimation.
// This algorithm provides O(1) per-observation updates and O(1) quantile retrieval,
// compared to O(n log n) for sorting-based approaches.
//
// Reference:
// Jain, R. and Chlamtac, I. (1985). "The P² Algorithm for Dynamic Calculation
// of Quantiles and Histograms Without Storing Observations". Communications
// of the ACM, 28(10), pp. 1076-1085.
//
// Thread Safety: NOT thread-safe. Caller must ensure synchronization.
type pSquareQuantile struct {
	// p is the target quantile (0.0 to 1.0)
	p float64

	// q stores the 5 marker heights (values at markers)
	q [5]float64

	// n stores the 5 marker positions (actual positions, 0-indexed)
	n [5]int

	// np stores the 5 desired marker positions (idealized, floats)
	np [5]float64

	// dn stores the increments for desired marker positions
	dn [5]float64

	// initialized tracks whether we have enough observations
	initialized bool

	// count is the total number of observations received
	count int

	// initBuffer stores first 5 observations before algorithm starts
	initBuffer [5]float64
}

// newPSquareQuantile creates a new P-Square quantile estimator for the given percentile p.
// The percentile should be in the range [0.0, 1.0] (e.g., 0.50 for P50, 0.99 for P99).
func newPSquareQuantile(p float64) *pSquareQuantile {
	if p < 0 {
		p = 0
	}
	if p > 1 {
		p = 1
	}

	return &pSquareQuantile{
		p:  p,
		dn: [5]float64{0, p / 2, p, (1 + p) / 2, 1},
	}
}

// Update adds a new observation to the quantile estimator.
// This is an O(1) operation.
func (ps *pSquareQuantile) Update(x float64) {
	ps.count++

	// Collect first 5 observations before starting the algorithm
	if ps.count <= 5 {
		ps.initBuffer[ps.count-1] = x
		if ps.count == 5 {
			ps.initialize()
		}
		return
	}

	// Find the cell k such that q[k] <= x < q[k+1]
	var k int
	if x < ps.q[0] {
		// x is new minimum
		ps.q[0] = x
		k = 0
	} else if x >= ps.q[4] {
		// x is new maximum
		ps.q[4] = x
		k = 3
	} else {
		// Binary search for the cell
		for k = 0; k < 4; k++ {
			if ps.q[k] <= x && x < ps.q[k+1] {
				break
			}
		}
	}

	// Increment positions of markers k+1 through 4
	for i := k + 1; i < 5; i++ {
		ps.n[i]++
	}

	// Update desired positions
	for i := 0; i < 5; i++ {
		ps.np[i] += ps.dn[i]
	}

	// Adjust marker heights if necessary
	for i := 1; i < 4; i++ {
		d := ps.np[i] - float64(ps.n[i])
		if (d >= 1 && ps.n[i+1]-ps.n[i] > 1) || (d <= -1 && ps.n[i-1]-ps.n[i] < -1) {
			sign := 1
			if d < 0 {
				sign = -1
			}

			// Try parabolic adjustment
			qPrime := ps.parabolic(i, sign)

			// Check if parabolic adjustment is valid
			if ps.q[i-1] < qPrime && qPrime < ps.q[i+1] {
				ps.q[i] = qPrime
			} else {
				// Use linear adjustment
				ps.q[i] = ps.linear(i, sign)
			}
			ps.n[i] += sign
		}
	}
}

// initialize sets up the markers from the first 5 observations.
func (ps *pSquareQuantile) initialize() {
	// Sort the first 5 observations (insertion sort for small array)
	for i := 1; i < 5; i++ {
		key := ps.initBuffer[i]
		j := i - 1
		for j >= 0 && ps.initBuffer[j] > key {
			ps.initBuffer[j+1] = ps.initBuffer[j]
			j--
		}
		ps.initBuffer[j+1] = key
	}

	// Initialize marker heights
	for i := 0; i < 5; i++ {
		ps.q[i] = ps.initBuffer[i]
		ps.n[i] = i
	}

	// Initialize desired positions
	ps.np = [5]float64{0, 2 * ps.p, 4 * ps.p, 2 + 2*ps.p, 4}

	ps.initialized = true
}

// parabolic computes the P-Square parabolic adjustment formula.
func (ps *pSquareQuantile) parabolic(i, d int) float64 {
	df := float64(d)
	ni := float64(ps.n[i])
	niPrev := float64(ps.n[i-1])
	niNext := float64(ps.n[i+1])

	term1 := df / (niNext - niPrev)
	term2 := (ni - niPrev + df) * (ps.q[i+1] - ps.q[i]) / (niNext - ni)
	term3 := (niNext - ni - df) * (ps.q[i] - ps.q[i-1]) / (ni - niPrev)

	return ps.q[i] + term1*(term2+term3)
}

// linear computes the P-Square linear adjustment formula.
func (ps *pSquareQuantile) linear(i, d int) float64 {
	if d == 1 {
		return ps.q[i] + (ps.q[i+1]-ps.q[i])/float64(ps.n[i+1]-ps.n[i])
	}
	return ps.q[i] - (ps.q[i]-ps.q[i-1])/float64(ps.n[i]-ps.n[i-1])
}

// Quantile returns the current estimated quantile value.
// This is an O(1) operation.
func (ps *pSquareQuantile) Quantile() float64 {
	if ps.count == 0 {
		return 0
	}

	if ps.count < 5 {
		// Not enough observations, use simple approach
		// Sort buffer and return closest position
		sorted := make([]float64, ps.count)
		copy(sorted, ps.initBuffer[:ps.count])
		for i := 1; i < ps.count; i++ {
			key := sorted[i]
			j := i - 1
			for j >= 0 && sorted[j] > key {
				sorted[j+1] = sorted[j]
				j--
			}
			sorted[j+1] = key
		}
		index := int(float64(ps.count-1) * ps.p)
		if index >= ps.count {
			index = ps.count - 1
		}
		return sorted[index]
	}

	// The quantile is at marker 2 (the middle marker for the target quantile)
	return ps.q[2]
}

// Count returns the number of observations received.
func (ps *pSquareQuantile) Count() int {
	return ps.count
}

// Max returns the maximum observed value.
func (ps *pSquareQuantile) Max() float64 {
	if ps.count == 0 {
		return 0
	}
	if ps.count < 5 {
		max := ps.initBuffer[0]
		for i := 1; i < ps.count; i++ {
			if ps.initBuffer[i] > max {
				max = ps.initBuffer[i]
			}
		}
		return max
	}
	return ps.q[4]
}

// pSquareMultiQuantile tracks multiple quantiles efficiently.
// It maintains separate P-Square estimators for each target percentile.
//
// Thread Safety: NOT thread-safe. Caller must ensure synchronization.
type pSquareMultiQuantile struct {
	estimators []*pSquareQuantile
	sum        float64
	count      int
	max        float64
}

// newPSquareMultiQuantile creates a new multi-quantile estimator.
// percentiles should be in range [0.0, 1.0].
func newPSquareMultiQuantile(percentiles ...float64) *pSquareMultiQuantile {
	m := &pSquareMultiQuantile{
		estimators: make([]*pSquareQuantile, len(percentiles)),
		max:        -math.MaxFloat64,
	}
	for i, p := range percentiles {
		m.estimators[i] = newPSquareQuantile(p)
	}
	return m
}

// Update adds a new observation to all quantile estimators.
// This is an O(k) operation where k is the number of percentiles tracked.
func (m *pSquareMultiQuantile) Update(x float64) {
	m.count++
	m.sum += x
	if x > m.max {
		m.max = x
	}
	for _, est := range m.estimators {
		est.Update(x)
	}
}

// Quantile returns the estimated quantile for the i-th percentile.
func (m *pSquareMultiQuantile) Quantile(i int) float64 {
	if i < 0 || i >= len(m.estimators) {
		return 0
	}
	return m.estimators[i].Quantile()
}

// Count returns the total number of observations.
func (m *pSquareMultiQuantile) Count() int {
	return m.count
}

// Sum returns the sum of all observations.
func (m *pSquareMultiQuantile) Sum() float64 {
	return m.sum
}

// Max returns the maximum observed value.
func (m *pSquareMultiQuantile) Max() float64 {
	if m.count == 0 {
		return 0
	}
	return m.max
}

// Mean returns the arithmetic mean of all observations.
func (m *pSquareMultiQuantile) Mean() float64 {
	if m.count == 0 {
		return 0
	}
	return m.sum / float64(m.count)
}

// Reset clears all state for reuse.
func (m *pSquareMultiQuantile) Reset() {
	m.sum = 0
	m.count = 0
	m.max = -math.MaxFloat64
	for _, est := range m.estimators {
		*est = *newPSquareQuantile(est.p)
	}
}
