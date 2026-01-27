package promisealtone

import "github.com/joeycumines/go-eventloop"

func NewPromiseForTesting(js *eventloop.JS) *Promise {
	return newPromise(js)
}
