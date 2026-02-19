# go-catrate

Package catrate implements multi-window rate limiting per arbitrary category.
Rates are applied independently to all categories, with separate sliding window
buckets per category.

It is intended for use cases that don't lend themselves well to token buckets,
sliding/fixed window counters, or probabilistic rate limiters.

## Documentation

Available [here](https://pkg.go.dev/github.com/joeycumines/go-catrate).
