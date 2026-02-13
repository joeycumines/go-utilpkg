package tournament

import (
	"github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/go-eventloop/internal/promisealtfour"
	"github.com/joeycumines/go-eventloop/internal/promisealtone"
	"github.com/joeycumines/go-eventloop/internal/promisealtthree"
	"github.com/joeycumines/go-eventloop/internal/promisealttwo"
)

// ChainedPromiseAdapter adapts eventloop.ChainedPromise
type ChainedPromiseAdapter struct {
	p *eventloop.ChainedPromise
}

func (a *ChainedPromiseAdapter) Then(onFulfilled, onRejected func(any) any) Promise {
	return &ChainedPromiseAdapter{p: a.p.Then(onFulfilled, onRejected)}
}

func (a *ChainedPromiseAdapter) Result() any {
	switch a.p.State() {
	case eventloop.Fulfilled:
		return a.p.Value()
	case eventloop.Rejected:
		return a.p.Reason()
	default:
		return nil
	}
}

// PromiseAltOneAdapter adapts promisealtone.Promise
type PromiseAltOneAdapter struct {
	p *promisealtone.Promise
}

func (a *PromiseAltOneAdapter) Then(onFulfilled, onRejected func(any) any) Promise {
	return &PromiseAltOneAdapter{p: a.p.Then(onFulfilled, onRejected)}
}

func (a *PromiseAltOneAdapter) Result() any {
	return a.p.Result()
}

// PromiseAltTwoAdapter adapts promisealttwo.Promise
type PromiseAltTwoAdapter struct {
	p *promisealttwo.Promise
}

func (a *PromiseAltTwoAdapter) Then(onFulfilled, onRejected func(any) any) Promise {
	return &PromiseAltTwoAdapter{p: a.p.Then(onFulfilled, onRejected)}
}

func (a *PromiseAltTwoAdapter) Result() any {
	return a.p.Result()
}

// PromiseAltThreeAdapter adapts promisealtthree.Promise
type PromiseAltThreeAdapter struct {
	p *promisealtthree.Promise
}

func (a *PromiseAltThreeAdapter) Then(onFulfilled, onRejected func(any) any) Promise {
	return &PromiseAltThreeAdapter{p: a.p.Then(onFulfilled, onRejected)}
}

func (a *PromiseAltThreeAdapter) Result() any {
	return a.p.Result()
}

// PromiseAltFourAdapter adapts promisealtfour.Promise
type PromiseAltFourAdapter struct {
	p *promisealtfour.Promise
}

func (a *PromiseAltFourAdapter) Then(onFulfilled, onRejected func(any) any) Promise {
	return &PromiseAltFourAdapter{p: a.p.Then(onFulfilled, onRejected)}
}

func (a *PromiseAltFourAdapter) Result() any {
	return a.p.Result()
}

// PromiseImplementations returns the list of promise implementations.
func PromiseImplementations() []PromiseImplementation {
	return []PromiseImplementation{
		{
			Name: "ChainedPromise",
			Factory: func(js *eventloop.JS) (Promise, eventloop.ResolveFunc, eventloop.RejectFunc) {
				p, resolve, reject := js.NewChainedPromise()
				return &ChainedPromiseAdapter{p: p}, resolve, reject
			},
		},
		{
			Name: "PromiseAltOne",
			Factory: func(js *eventloop.JS) (Promise, eventloop.ResolveFunc, eventloop.RejectFunc) {
				p, r1, r2 := promisealtone.New(js)
				return &PromiseAltOneAdapter{p: p}, eventloop.ResolveFunc(r1), eventloop.RejectFunc(r2)
			},
		},
		{
			Name: "PromiseAltTwo",
			Factory: func(js *eventloop.JS) (Promise, eventloop.ResolveFunc, eventloop.RejectFunc) {
				p, r1, r2 := promisealttwo.New(js)
				return &PromiseAltTwoAdapter{p: p}, eventloop.ResolveFunc(r1), eventloop.RejectFunc(r2)
			},
		},
		{
			Name: "PromiseAltThree",
			Factory: func(js *eventloop.JS) (Promise, eventloop.ResolveFunc, eventloop.RejectFunc) {
				p, r1, r2 := promisealtthree.New(js)
				return &PromiseAltThreeAdapter{p: p}, eventloop.ResolveFunc(r1), eventloop.RejectFunc(r2)
			},
		},
		{
			Name: "PromiseAltFour",
			Factory: func(js *eventloop.JS) (Promise, eventloop.ResolveFunc, eventloop.RejectFunc) {
				p, r1, r2 := promisealtfour.New(js)
				return &PromiseAltFourAdapter{p: p}, eventloop.ResolveFunc(r1), eventloop.RejectFunc(r2)
			},
		},
	}
}
