package eventloop

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestPromiseAllSettled_ConcurrentRejections tests AllSettled with concurrent rejections
func TestPromiseAllSettled_ConcurrentRejections(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	const numPromises = 10
	promises := make([]*ChainedPromise, numPromises)
	rejectors := make([]func(any), numPromises)

	for i := 0; i < numPromises; i++ {
		p, _, r := js.NewChainedPromise()
		promises[i] = p
		rejectors[i] = r
	}

	result := js.AllSettled(promises)

	// All reject concurrently
	for i := 0; i < numPromises; i++ {
		rejectors[i](errors.New("error" + string(rune('a'+i))))
	}
	loop.tick()

	// Should resolve with all rejection status objects
	var values []any
	handlerDone := make(chan struct{})
	result.Then(func(v any) any {
		values = v.([]any)
		close(handlerDone)
		return nil
	}, nil)
	loop.tick()

	// Wait for handler to complete with timeout
	select {
	case <-handlerDone:
		// Handler completed successfully
	case <-time.After(2 * time.Second):
		t.Fatal("Handler was not called within timeout - possible deadlock")
	}

	if len(values) != numPromises {
		t.Errorf("Expected %d results, got %d", numPromises, len(values))
	}

	// Verify all are rejected
	for i, r := range values {
		status, ok := r.(map[string]interface{})
		if !ok {
			t.Errorf("Expected map, got %T at index %d", r, i)
			continue
		}
		if status["status"] != "rejected" {
			t.Errorf("Expected status 'rejected', got '%v' at index %d", status["status"], i)
		}
	}
}

// TestPromiseAllSettled_ConcurrentResolutions tests AllSettled with concurrent resolutions
func TestPromiseAllSettled_ConcurrentResolutions(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	const numPromises = 10
	promises := make([]*ChainedPromise, numPromises)
	resolvers := make([]func(any), numPromises)

	for i := 0; i < numPromises; i++ {
		p, r, _ := js.NewChainedPromise()
		promises[i] = p
		resolvers[i] = r
	}

	result := js.AllSettled(promises)

	// All resolve concurrently
	for i := 0; i < numPromises; i++ {
		resolvers[i]("value" + string(rune('a'+i)))
	}
	loop.tick()

	// Should resolve with all fulfillment status objects
	var values []any
	handlerDone := make(chan struct{})
	result.Then(func(v any) any {
		values = v.([]any)
		close(handlerDone)
		return nil
	}, nil)
	loop.tick()

	// Wait for handler to complete with timeout
	select {
	case <-handlerDone:
		// Handler completed successfully
	case <-time.After(2 * time.Second):
		t.Fatal("Handler was not called within timeout - possible deadlock")
	}

	if len(values) != numPromises {
		t.Errorf("Expected %d results, got %d", numPromises, len(values))
	}

	// Verify all are fulfilled
	for i, r := range values {
		status, ok := r.(map[string]interface{})
		if !ok {
			t.Errorf("Expected map, got %T at index %d", r, i)
			continue
		}
		if status["status"] != "fulfilled" {
			t.Errorf("Expected status 'fulfilled', got '%v' at index %d", status["status"], i)
		}
	}
}

// TestPromiseAllSettled_MixedFulfillmentAndRejection tests AllSettled with mix
func TestPromiseAllSettled_MixedFulfillmentAndRejection(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p1, resolve1, _ := js.NewChainedPromise()
	p2, _, reject2 := js.NewChainedPromise()
	p3, resolve3, _ := js.NewChainedPromise()

	result := js.AllSettled([]*ChainedPromise{p1, p2, p3})

	// Mixed settlement
	resolve1("fulfilled1")
	reject2(errors.New("rejected"))
	resolve3("fulfilled3")
	loop.tick()

	// Should resolve with correct status objects
	var values []any
	handlerDone := make(chan struct{})
	result.Then(func(v any) any {
		values = v.([]any)
		close(handlerDone)
		return nil
	}, nil)
	loop.tick()

	// Wait for handler to complete with timeout
	select {
	case <-handlerDone:
		// Handler completed successfully
	case <-time.After(2 * time.Second):
		t.Fatal("Handler was not called within timeout - possible deadlock")
	}

	if len(values) != 3 {
		t.Errorf("Expected 3 results, got %d", len(values))
	}

	// Verify each status
	status1, ok := values[0].(map[string]interface{})
	if !ok || status1["status"] != "fulfilled" {
		t.Error("First promise should be fulfilled")
	}

	status2, ok := values[1].(map[string]interface{})
	if !ok || status2["status"] != "rejected" {
		t.Error("Second promise should be rejected")
	}

	status3, ok := values[2].(map[string]interface{})
	if !ok || status3["status"] != "fulfilled" {
		t.Error("Third promise should be fulfilled")
	}
}

// TestPromiseAllSettled_TerminatedLoop tests AllSettled on terminated loop
func TestPromiseAllSettled_TerminatedLoop(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		loop.Run(ctx)
		close(done)
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()
	<-done

	js, _ := NewJS(loop)
	p1, _, _ := js.NewChainedPromise()

	result := js.AllSettled([]*ChainedPromise{p1})

	// Should handle gracefully
	if result.State() == Rejected {
		t.Error("AllSettled should not reject on terminated loop")
	}

	loop.Shutdown(context.Background())
}

// TestPromiseAllSettled_PanicInHandler tests AllSettled with panic in handler
func TestPromiseAllSettled_PanicInHandler(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p1, resolve1, _ := js.NewChainedPromise()

	result := js.AllSettled([]*ChainedPromise{p1})

	// Handler that panics
	result.Then(func(v any) any {
		panic("handler panic")
	}, nil)

	// Should not crash
	resolve1("value")
	loop.tick()
}

// TestPromiseAllSettled_OnePromise tests AllSettled with single promise
func TestPromiseAllSettled_OnePromise(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p1, resolve1, _ := js.NewChainedPromise()

	result := js.AllSettled([]*ChainedPromise{p1})

	var values []any
	result.Then(func(v any) any {
		values = v.([]any)
		return nil
	}, nil)

	resolve1("value")
	loop.tick()

	if len(values) != 1 {
		t.Errorf("Expected 1 result, got %d", len(values))
	}

	status, ok := values[0].(map[string]interface{})
	if !ok || status["status"] != "fulfilled" {
		t.Error("Promise should be fulfilled")
	}
}

// TestPromiseAllSettled_AlreadySettledPromises tests AllSettled with already-settled promises
func TestPromiseAllSettled_AlreadySettledPromises(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	// Create already-settled promises
	p1, resolve1, _ := js.NewChainedPromise()
	p2, _, reject2 := js.NewChainedPromise()

	resolve1("value1")
	reject2(errors.New("error"))

	result := js.AllSettled([]*ChainedPromise{p1, p2})

	// Should resolve immediately
	var values []any
	result.Then(func(v any) any {
		values = v.([]any)
		return nil
	}, nil)

	loop.tick()

	if len(values) != 2 {
		t.Errorf("Expected 2 results, got %d", len(values))
	}
}

// TestPromiseAllSettled_WithThenHandler tests AllSettled with immediate then handler
func TestPromiseAllSettled_WithThenHandler(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p1, resolve1, _ := js.NewChainedPromise()

	result := js.AllSettled([]*ChainedPromise{p1})

	var gotResult bool
	var values []any

	result.Then(func(v any) any {
		gotResult = true
		values = v.([]any)
		return nil
	}, nil)

	resolve1("value")
	loop.tick()

	if !gotResult {
		t.Error("Then handler should have been called")
	}
	if len(values) != 1 {
		t.Errorf("Expected 1 result, got %d", len(values))
	}
}

// TestPromiseAllSettled_LargeNumberOfPromises tests AllSettled with many promises
func TestPromiseAllSettled_LargeNumberOfPromises(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	const numPromises = 50
	promises := make([]*ChainedPromise, numPromises)
	resolvers := make([]func(any), numPromises)

	for i := 0; i < numPromises; i++ {
		p, r, _ := js.NewChainedPromise()
		promises[i] = p
		resolvers[i] = r
	}

	result := js.AllSettled(promises)

	// All resolve
	for i := 0; i < numPromises; i++ {
		resolvers[i]("value" + string(rune('a'+i%26)))
	}
	loop.tick()

	// Verify all resolved
	var values []any
	handlerDone := make(chan struct{})
	result.Then(func(v any) any {
		values = v.([]any)
		close(handlerDone)
		return nil
	}, nil)
	loop.tick()

	// Wait for handler to complete with timeout
	select {
	case <-handlerDone:
		// Handler completed successfully
	case <-time.After(2 * time.Second):
		t.Fatal("Handler was not called within timeout - possible deadlock")
	}

	if len(values) != numPromises {
		t.Errorf("Expected %d results, got %d", numPromises, len(values))
	}
}

// TestPromiseAllSettled_ChainedPromises tests AllSettled with chained promises
func TestPromiseAllSettled_ChainedPromises(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p1, resolve1, _ := js.NewChainedPromise()
	p2, _, reject2 := js.NewChainedPromise()

	// Create chains
	chained1 := p1.Then(func(v any) any {
		return v.(string) + "-chained1"
	}, nil)
	// Note: chained2 uses a catch handler that returns a value.
	// When a catch handler returns a value, the promise becomes fulfilled
	// with that value (it "caught" the rejection).
	chained2 := p2.Then(nil, func(r any) any {
		return r.(error).Error() + "-chained2"
	})

	result := js.AllSettled([]*ChainedPromise{chained1, chained2})

	// Settle parents
	resolve1("value")
	reject2(errors.New("error"))
	loop.tick()

	// Verify chained results
	var values []any
	handlerDone := make(chan struct{})
	result.Then(func(v any) any {
		values = v.([]any)
		close(handlerDone)
		return nil
	}, nil)
	loop.tick()

	// Wait for handler to complete with timeout
	select {
	case <-handlerDone:
		// Handler completed successfully
	case <-time.After(2 * time.Second):
		t.Fatal("Handler was not called within timeout - possible deadlock")
	}

	if len(values) != 2 {
		t.Errorf("Expected 2 results, got %d", len(values))
	}

	status1, ok := values[0].(map[string]interface{})
	if !ok || status1["status"] != "fulfilled" {
		t.Error("First chained promise should be fulfilled")
	}
	if status1["value"] != "value-chained1" {
		t.Errorf("Expected 'value-chained1', got %v", status1["value"])
	}

	// chained2 is fulfilled because the catch handler returned a value
	status2, ok := values[1].(map[string]interface{})
	if !ok || status2["status"] != "fulfilled" {
		t.Error("Second chained promise should be fulfilled (catch handler returned a value)")
	}
	if status2["value"] != "error-chained2" {
		t.Errorf("Expected 'error-chained2', got %v", status2["value"])
	}
}

// TestPromiseAllSettled_RejectionAfterFulfillment tests AllSettled with late rejection
func TestPromiseAllSettled_RejectionAfterFulfillment(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p1, resolve1, _ := js.NewChainedPromise()
	p2, _, reject2 := js.NewChainedPromise()

	result := js.AllSettled([]*ChainedPromise{p1, p2})

	// First promise fulfills
	resolve1("fulfilled")
	loop.tick()

	// Second promise rejects
	reject2(errors.New("error"))
	loop.tick()

	// Should have both results
	var values []any
	handlerDone := make(chan struct{})
	result.Then(func(v any) any {
		values = v.([]any)
		close(handlerDone)
		return nil
	}, nil)
	loop.tick()

	// Wait for handler to complete with timeout
	select {
	case <-handlerDone:
		// Handler completed successfully
	case <-time.After(2 * time.Second):
		t.Fatal("Handler was not called within timeout - possible deadlock")
	}

	if len(values) != 2 {
		t.Errorf("Expected 2 results, got %d", len(values))
	}

	status1, ok := values[0].(map[string]interface{})
	if !ok || status1["status"] != "fulfilled" {
		t.Error("First promise should be fulfilled")
	}

	status2, ok := values[1].(map[string]interface{})
	if !ok || status2["status"] != "rejected" {
		t.Error("Second promise should be rejected")
	}
}
