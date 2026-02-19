// Package alternatetwo implements a "Maximum Performance" event loop variant.
//
// This implementation prioritizes throughput, zero allocations, and minimal
// latency over defensive safety measures. It uses lock-free MPSC queues,
// cache-line padding, direct FD array indexing, and arena-based task
// allocation. See the tournament results in eventloop/docs/tournament/
// for performance comparisons.
package alternatetwo
