package tournament

import (
	"github.com/joeycumines/go-eventloop"
)

// Promise defines the common interface for tournament promises.
// It reflects the subset of methods we want to benchmark and verify.
type Promise interface {
	Then(onFulfilled, onRejected func(any) any) Promise
	Result() any
}

// PromiseFactory creates a new promise and its resolver/rejector.
// It takes a *eventloop.JS because all our promises depend on it.
type PromiseFactory func(*eventloop.JS) (Promise, eventloop.ResolveFunc, eventloop.RejectFunc)

// PromiseImplementation represents a named promise implementation.
type PromiseImplementation struct { // betteralign:ignore
	Name    string
	Factory PromiseFactory
}
