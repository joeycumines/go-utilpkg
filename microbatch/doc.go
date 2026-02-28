// Package microbatch groups tasks into small batches, e.g. to reduce the
// number of round trips.
//
// See also [github.com/joeycumines/go-longpoll], for a similar, lower-level
// implementation, e.g. if you require more control over the batching or
// concurrency behavior.
package microbatch
