# go-microbatch

Package microbatch groups tasks into small batches, e.g. to reduce the number
of round trips.

See the [docs](https://pkg.go.dev/github.com/joeycumines/go-microbatch) for
example usage.

See also
[github.com/joeycumines/go-longpoll](github.com/joeycumines/go-longpoll), for a
similar, lower-level implementation, e.g. if you require more control over the
batching or concurrency behavior.
