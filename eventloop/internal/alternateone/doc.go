// Package alternateone implements a "Maximum Safety" event loop variant.
//
// This implementation prioritizes correctness guarantees and defensive
// programming over raw performance. It uses coarse-grained locking
// (single mutex for ingress), always-on invariant validation, phased
// serial shutdown, and write-lock polling. See the tournament results
// in eventloop/docs/tournament/ for performance comparisons.
package alternateone
