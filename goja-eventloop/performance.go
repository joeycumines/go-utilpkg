package gojaeventloop

import (
	"fmt"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

// bindPerformance installs the retained High Resolution Time subset. User
// Timing and Performance Timeline APIs are intentionally outside the profile.
func (a *Adapter) bindPerformance(journal *installationJournal, eventTargetPrototype *goja.Object) (*goja.Object, error) {
	if journal == nil {
		return nil, fmt.Errorf("initialize Performance: installation journal is unavailable")
	}

	if eventTargetPrototype == nil {
		return nil, fmt.Errorf("initialize Performance: EventTarget prototype is unavailable")
	}
	clock := a.performance
	if clock == nil {
		clock = goeventloop.NewLoopPerformance(a.loop)
	}

	constructorValue := a.runtime.ToValue(func(goja.ConstructorCall) *goja.Object {
		panic(a.performanceTypeError("Illegal constructor"))
	})
	constructor, ok := constructorValue.(*goja.Object)
	if !ok || constructor == nil {
		return nil, fmt.Errorf("initialize Performance: constructor is not an object")
	}
	if err := defineWebFunction(a.runtime, constructor, "Performance", 0, "define Performance constructor"); err != nil {
		return nil, err
	}
	prototype, ok := constructor.Get("prototype").(*goja.Object)
	if !ok || prototype == nil {
		return nil, fmt.Errorf("initialize Performance: prototype is not an object")
	}
	if err := lockWebConstructorPrototype(a.runtime, constructor, "Performance"); err != nil {
		return nil, err
	}
	if err := prototype.SetPrototype(eventTargetPrototype); err != nil {
		return nil, fmt.Errorf("set Performance prototype inheritance: %w", err)
	}

	performance := a.runtime.NewObject()
	if err := performance.SetPrototype(prototype); err != nil {
		return nil, fmt.Errorf("set performance prototype: %w", err)
	}
	a.initEventTargetObject(performance)
	requireReceiver := func(value goja.Value) {
		obj, ok := value.(*goja.Object)
		if !ok || obj == nil || obj != performance {
			panic(a.performanceTypeError("Value of \"this\" must be of type Performance"))
		}
	}

	if err := defineWebMethod(a.runtime, prototype, "now", 0, true, func(call goja.FunctionCall) goja.Value {
		requireReceiver(call.This)
		return a.runtime.ToValue(clock.Now())
	}); err != nil {
		return nil, fmt.Errorf("bind Performance.prototype.now: %w", err)
	}
	if err := defineWebAccessor(a.runtime, prototype, "timeOrigin", true, func(call goja.FunctionCall) goja.Value {
		requireReceiver(call.This)
		return a.runtime.ToValue(clock.TimeOrigin())
	}, nil); err != nil {
		return nil, fmt.Errorf("bind Performance.prototype.timeOrigin: %w", err)
	}
	if err := defineWebMethod(a.runtime, prototype, "toJSON", 0, true, func(call goja.FunctionCall) goja.Value {
		requireReceiver(call.This)
		result := a.runtime.NewObject()
		if err := result.DefineDataProperty("timeOrigin", a.runtime.ToValue(clock.TimeOrigin()), goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_TRUE); err != nil {
			a.panicJSException(wrapRuntimeError("define Performance toJSON timeOrigin", err))
		}
		return result
	}); err != nil {
		return nil, fmt.Errorf("bind Performance.prototype.toJSON: %w", err)
	}
	if err := defineWebTag(a.runtime, prototype, "Performance"); err != nil {
		return nil, err
	}
	if _, err := verifyBrandedSingletonObject(a, performance, constructor, "Performance", []string{"now", "toJSON"}, []string{"timeOrigin"}); err != nil {
		return nil, err
	}

	// Publish only after the detached constructor, prototype, and singleton are
	// complete. Bind's ownership transaction journals both global mutations.
	if err := journal.setGlobal("Performance", constructor); err != nil {
		return nil, err
	}
	if err := journal.setGlobal("performance", performance); err != nil {
		return nil, err
	}
	a.performance = clock
	return performance, nil
}

func (a *Adapter) performanceTypeError(message string) *goja.Object {
	return a.runtime.NewTypeError(message)
}
