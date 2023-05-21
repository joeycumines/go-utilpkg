// Package catrate implements multi-window rate limiting per (arbitrary)
// "category". Rates are applied independently, to all categories, with
// separate buckets per category. It uses a simple but potentially poorly
// optimized strategy, involving tracking discrete events, within a sliding
// window.
//
// It is intended for use cases that don't lend themselves well to any of the
// more complex solutions, e.g. token buckets, sliding/fixed window counters,
// or probabilistic rate limiting (i.e. bloom filters).
package catrate
