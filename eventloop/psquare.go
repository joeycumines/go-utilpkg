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

	// n stores the 5 marker positions (actual positions, 0-indexed).
	n [5]uint64

	// np stores the 5 desired marker positions as fixed-point values so long-lived
	// samplers do not stop advancing after float64 loses integral precision.
	np [5]pSquarePosition

	// dn stores the fixed-point increments for desired marker positions.
	dn [5]pSquarePosition

	// initialized tracks whether we have enough observations
	initialized bool

	// count is the total number of observations received
	count uint64

	// initBuffer stores first 5 observations before algorithm starts
	initBuffer [5]float64
}

// newPSquareQuantile creates a new P-Square quantile estimator for the given percentile p.
// The percentile should be in the range [0.0, 1.0] (e.g., 0.50 for P50, 0.99 for P99).
func newPSquareQuantile(p float64) *pSquareQuantile {
	if math.IsNaN(p) {
		p = 0
	}
	if p < 0 {
		p = 0
	}
	if p > 1 {
		p = 1
	}

	return &pSquareQuantile{
		p: p,
		dn: [5]pSquarePosition{
			newPSquarePosition(0),
			newPSquarePosition(p / 2),
			newPSquarePosition(p),
			newPSquarePosition((1 + p) / 2),
			newPSquarePosition(1),
		},
	}
}

// Update adds a new observation to the quantile estimator.
// This is an O(1) operation.
func (ps *pSquareQuantile) Update(x float64) {
	if ps.count == ^uint64(0) {
		return
	}
	ps.count++

	// Collect first 5 observations before starting the algorithm
	if ps.count <= 5 {
		ps.initBuffer[int(ps.count-1)] = x
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
		for k = range 4 {
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
	for i := range 5 {
		ps.np[i].add(ps.dn[i])
	}

	// Adjust marker heights if necessary
	for i := 1; i < 4; i++ {
		sign := ps.np[i].adjustment(ps.n[i])
		if (sign > 0 && ps.n[i+1]-ps.n[i] > 1) ||
			(sign < 0 && ps.n[i]-ps.n[i-1] > 1) {

			// Try parabolic adjustment
			qPrime := ps.parabolic(i, sign)

			// Check if parabolic adjustment is valid
			if ps.q[i-1] < qPrime && qPrime < ps.q[i+1] {
				ps.q[i] = qPrime
			} else {
				// Use linear adjustment
				ps.q[i] = ps.linear(i, sign)
			}
			if sign > 0 {
				ps.n[i]++
			} else {
				ps.n[i]--
			}
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
	for i := range 5 {
		ps.q[i] = ps.initBuffer[i]
		ps.n[i] = uint64(i)
	}

	// Initialize desired positions
	ps.np = [5]pSquarePosition{
		newPSquarePosition(0),
		newPSquarePosition(2 * ps.p),
		newPSquarePosition(4 * ps.p),
		newPSquarePosition(2 + 2*ps.p),
		newPSquarePosition(4),
	}

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
		count := int(ps.count)
		sorted := ps.initBuffer
		for i := 1; i < count; i++ {
			key := sorted[i]
			j := i - 1
			for j >= 0 && sorted[j] > key {
				sorted[j+1] = sorted[j]
				j--
			}
			sorted[j+1] = key
		}
		index := int(float64(count-1) * ps.p)
		if index >= count {
			index = count - 1
		}
		return sorted[index]
	}

	// The quantile is at marker 2 (the middle marker for the target quantile)
	return ps.q[2]
}

// Count returns the number of observations received.
func (ps *pSquareQuantile) Count() uint64 {
	return ps.count
}

// Max returns the maximum observed value.
func (ps *pSquareQuantile) Max() float64 {
	if ps.count == 0 {
		return 0
	}
	if ps.count < 5 {
		max := ps.initBuffer[0]
		for i := 1; i < int(ps.count); i++ {
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
}

// newPSquareMultiQuantile creates a new multi-quantile estimator.
// percentiles should be in range [0.0, 1.0].
func newPSquareMultiQuantile(percentiles ...float64) *pSquareMultiQuantile {
	m := &pSquareMultiQuantile{estimators: make([]*pSquareQuantile, len(percentiles))}
	for i, p := range percentiles {
		m.estimators[i] = newPSquareQuantile(p)
	}
	return m
}

// Update adds a new observation to all quantile estimators.
// This is an O(k) operation where k is the number of percentiles tracked.
func (m *pSquareMultiQuantile) Update(x float64) {
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

const pSquarePositionScale = uint64(1) << 32

type pSquarePosition struct {
	whole    uint64
	fraction uint64
}

func newPSquarePosition(value float64) pSquarePosition {
	if math.IsNaN(value) || value <= 0 {
		return pSquarePosition{}
	}
	whole, fraction := math.Modf(value)
	position := pSquarePosition{
		whole:    uint64(whole),
		fraction: uint64(fraction * float64(pSquarePositionScale)),
	}
	if position.fraction >= pSquarePositionScale {
		position.whole++
		position.fraction = 0
	}
	return position
}

func (p *pSquarePosition) add(increment pSquarePosition) {
	p.whole += increment.whole
	p.fraction += increment.fraction
	if p.fraction >= pSquarePositionScale {
		p.whole++
		p.fraction -= pSquarePositionScale
	}
}

func (p pSquarePosition) adjustment(actual uint64) int {
	if p.whole > actual {
		return 1
	}
	if p.whole < actual {
		gap := actual - p.whole
		if gap > 1 || (gap == 1 && p.fraction == 0) {
			return -1
		}
	}
	return 0
}
