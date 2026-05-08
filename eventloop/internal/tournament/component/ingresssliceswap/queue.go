// Package ingresssliceswap materializes the early Main fast-path ingress
// storage that admitted callbacks into a mutex-owned slice and swapped it with
// an owner-recycled spare slice.
package ingresssliceswap

// Queue is not thread-safe. The source event loop serialized Push, Take, and
// Length with its external mutex; only the loop owner recycled drained batches.
type Queue struct {
	jobs  []func()
	spare []func()
}

// Push appends one callback. Nil is a source-valid queued value.
func (q *Queue) Push(fn func()) {
	q.jobs = append(q.jobs, fn)
}

// Take transfers the current phase snapshot to the owner and installs the
// recycled spare as the producer slice.
func (q *Queue) Take() []func() {
	jobs := q.jobs
	q.jobs = q.spare
	return jobs
}

// Recycle clears a drained snapshot and makes its storage the next spare.
// The owner must not recycle a batch before it has finished reading it.
func (q *Queue) Recycle(jobs []func()) {
	clear(jobs)
	q.spare = jobs[:0]
}

// Length returns the number of callbacks in the current producer snapshot.
func (q *Queue) Length() int {
	return len(q.jobs)
}
