package eventloop

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// ============================================================================
// Promise.all Tests
// ============================================================================

func TestPromiseAll_EmptyArray(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	result := js.All([]*ChainedPromise{})

	finalResult := false
	result.Then(func(v any) any {
		finalResult = true
		values, ok := v.([]any)
		if !ok {
			t.Errorf("Expected []any, got %T", v)
		}
		if len(values) != 0 {
			t.Errorf("Expected empty slice, got %d elements", len(values))
		}
		return nil
	}, nil)

	loop.tick()

	if !finalResult {
		t.Error("Promise.all with empty array should resolve with empty slice")
	}
}

func TestPromiseAll_SingleElement(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, resolve, _ := js.NewChainedPromise()
	result := js.All([]*ChainedPromise{p})

	finalResult := false
	result.Then(func(v any) any {
		finalResult = true
		values, ok := v.([]any)
		if !ok {
			t.Errorf("Expected []any, got %T", v)
			return nil
		}
		if len(values) != 1 {
			t.Errorf("Expected 1 element, got %d", len(values))
			return nil
		}
		if values[0] != "success" {
			t.Errorf("Expected 'success', got %v", values[0])
		}
		return nil
	}, nil)

	resolve("success")
	loop.tick()

	if !finalResult {
		t.Error("Promise.all with single element should resolve")
	}
}

func TestPromiseAll_MultipleValues_AllResolve(t *testing.T) {
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
	p2, resolve2, _ := js.NewChainedPromise()
	p3, resolve3, _ := js.NewChainedPromise()

	result := js.All([]*ChainedPromise{p1, p2, p3})

	finalResult := make([]any, 3)
	result.Then(func(v any) any {
		values, ok := v.([]any)
		if !ok {
			t.Errorf("Expected []any, got %T", v)
			return nil
		}
		copy(finalResult, values)
		return nil
	}, nil)

	resolve1("first")
	resolve2("second")
	resolve3("third")
	loop.tick()

	if finalResult[0] != "first" {
		t.Errorf("Expected 'first', got %v", finalResult[0])
	}
	if finalResult[1] != "second" {
		t.Errorf("Expected 'second', got %v", finalResult[1])
	}
	if finalResult[2] != "third" {
		t.Errorf("Expected 'third', got %v", finalResult[2])
	}
}

func TestPromiseAll_AnyRejection_RejectsImmediately(t *testing.T) {
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

	result := js.All([]*ChainedPromise{p1, p2, p3})

	rejectionReason := ""
	result.Catch(func(r any) any {
		rejectionReason = r.(string)
		return nil
	})

	reject2("error from p2")
	loop.tick()

	if rejectionReason != "error from p2" {
		t.Errorf("Expected 'error from p2', got '%s'", rejectionReason)
	}

	// Verify other promises can still resolve
	resolve1("first")
	resolve3("third")
	loop.tick()
}

func TestPromiseAll_NestedPromises(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	// Create a chain: base -> then -> then
	base, resolveBase, _ := js.NewChainedPromise()
	chained := base.Then(func(v any) any {
		return v.(string) + " +1"
	}, nil)
	fullyChained := chained.Then(func(v any) any {
		return v.(string) + " +2"
	}, nil)

	result := js.All([]*ChainedPromise{base, chained, fullyChained})

	finalResult := make([]string, 3)
	var wg sync.WaitGroup
	wg.Add(1)

	result.Then(func(v any) any {
		values, ok := v.([]any)
		if !ok {
			t.Errorf("Expected []any, got %T", v)
			return nil
		}
		for i, val := range values {
			finalResult[i] = val.(string)
		}
		wg.Done()
		return nil
	}, nil)

	resolveBase("base")
	loop.tick()

	wg.Wait()

	if finalResult[0] != "base" {
		t.Errorf("Expected 'base', got %s", finalResult[0])
	}
	if finalResult[1] != "base +1" {
		t.Errorf("Expected 'base +1', got %s", finalResult[1])
	}
	if finalResult[2] != "base +1 +2" {
		t.Errorf("Expected 'base +1 +2', got %s", finalResult[2])
	}
}

func TestPromiseAll_StressTest(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	const count = 100
	promises := make([]*ChainedPromise, count)
	resolvers := make([]func(any), count)

	// Create promises
	for i := 0; i < count; i++ {
		promises[i], resolvers[i], _ = js.NewChainedPromise()
	}

	result := js.All(promises)

	var wg sync.WaitGroup
	wg.Add(1)
	results := make([]any, count)

	result.Then(func(v any) any {
		values, ok := v.([]any)
		if !ok {
			t.Errorf("Expected []any, got %T", v)
			wg.Done()
			return nil
		}
		copy(results, values)
		wg.Done()
		return nil
	}, nil)

	// Resolve all promises
	for i := 0; i < count; i++ {
		resolvers[i](i)
	}

	loop.tick()
	wg.Wait()

	// Verify all values were received
	for i := 0; i < count; i++ {
		if results[i] != i {
			t.Errorf("Expected %d, got %v", i, results[i])
		}
	}
}

// ============================================================================
// Promise.race Tests
// ============================================================================

func TestPromiseRace_EmptyArray_NeverSettles(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	result := js.Race([]*ChainedPromise{})

	// After ticking, the promise should still be pending
	loop.tick()

	if result.State() != Pending {
		t.Errorf("Expected Pending state, got %v", result.State())
	}
}

func TestPromiseRace_SinglePromise(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, resolve, _ := js.NewChainedPromise()
	result := js.Race([]*ChainedPromise{p})

	finalResult := ""
	result.Then(func(v any) any {
		finalResult = v.(string)
		return nil
	}, nil)

	resolve("winner")
	loop.tick()

	if finalResult != "winner" {
		t.Errorf("Expected 'winner', got '%s'", finalResult)
	}
}

func TestPromiseRace_MultiplePromises_FirstToSettleWins(t *testing.T) {
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
	p2, resolve2, _ := js.NewChainedPromise()
	p3, resolve3, _ := js.NewChainedPromise()

	result := js.Race([]*ChainedPromise{p1, p2, p3})

	finalResult := ""
	result.Then(func(v any) any {
		finalResult = v.(string)
		return nil
	}, nil)

	resolve2("second wins")
	loop.tick()

	if finalResult != "second wins" {
		t.Errorf("Expected 'second wins', got '%s'", finalResult)
	}

	// Resolve other promises - they should be ignored
	resolve1("first")
	resolve3("third")
	loop.tick()

	if finalResult != "second wins" {
		t.Errorf("Result should still be 'second wins', got '%s'", finalResult)
	}
}

func TestPromiseRace_WithSetTimeout(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	// Start with "fast" work
	fastWork, _, _ := js.NewChainedPromise()

	// Create a timeout promise using microtask instead of JS.SetTimeout for deterministic testing
	timeoutPromise, resolveTimeout, _ := js.NewChainedPromise()

	result := js.Race([]*ChainedPromise{fastWork, timeoutPromise})

	var winner string
	var wg sync.WaitGroup
	wg.Add(1)

	result.Then(func(v any) any {
		winner = v.(string)
		wg.Done()
		return nil
	}, nil)

	// Simulate timeout winning
	resolveTimeout("timeout wins")
	loop.tick()

	wg.Wait()

	if winner != "timeout wins" {
		t.Errorf("Expected 'timeout wins', got '%s'", winner)
	}
}

func TestPromiseRace_FasterPromiseWinsOverTimeout(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate actual work that completes quickly
	actualWork, resolveWork, _ := js.NewChainedPromise()

	// Simulate a slow timeout
	timeout, _, rejectTimeout := js.NewChainedPromise()

	result := js.Race([]*ChainedPromise{actualWork, timeout})

	var winner string
	var wg sync.WaitGroup
	wg.Add(1)

	result.Then(func(v any) any {
		winner = v.(string)
		wg.Done()
		return nil
	}, nil)

	// Work completes before timeout
	resolveWork("work complete")
	loop.tick()

	wg.Wait()

	if winner != "work complete" {
		t.Errorf("Expected 'work complete', got '%s'", winner)
	}

	// Timeout would reject later, but should be ignored
	rejectTimeout(errors.New("timeout"))
	loop.tick()

	if winner != "work complete" {
		t.Errorf("Result should still be 'work complete', got '%s'", winner)
	}
}

func TestPromiseRace_RejectionCanWin(t *testing.T) {
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

	result := js.Race([]*ChainedPromise{p1, p2})

	rejectionReason := ""
	result.Catch(func(r any) any {
		rejectionReason = r.(string)
		return nil
	})

	reject2("first rejection wins")
	loop.tick()

	if rejectionReason != "first rejection wins" {
		t.Errorf("Expected 'first rejection wins', got '%s'", rejectionReason)
	}

	// Resolve p1 later - should be ignored
	resolve1("resolve")
	loop.tick()

	if rejectionReason != "first rejection wins" {
		t.Errorf("Result should still be rejection, got '%s'", rejectionReason)
	}
}

// ============================================================================
// Promise.allSettled Tests
// ============================================================================

func TestPromiseAllSettled_EmptyArray(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	result := js.AllSettled([]*ChainedPromise{})

	finalResult := false
	result.Then(func(v any) any {
		finalResult = true
		values, ok := v.([]any)
		if !ok {
			t.Errorf("Expected []any, got %T", v)
		}
		if len(values) != 0 {
			t.Errorf("Expected empty slice, got %d elements", len(values))
		}
		return nil
	}, nil)

	loop.tick()

	if !finalResult {
		t.Error("Promise.allSettled with empty array should resolve with empty slice")
	}
}

func TestPromiseAllSettled_AllFulfilled(t *testing.T) {
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
	p2, resolve2, _ := js.NewChainedPromise()
	p3, resolve3, _ := js.NewChainedPromise()

	result := js.AllSettled([]*ChainedPromise{p1, p2, p3})

	finalResult := make([]any, 3)
	result.Then(func(v any) any {
		values, ok := v.([]any)
		if !ok {
			t.Errorf("Expected []any, got %T", v)
			return nil
		}
		if len(values) != 3 {
			t.Errorf("Expected 3 results, got %d", len(values))
			return nil
		}
		copy(finalResult, values)
		return nil
	}, nil)

	resolve1("first")
	resolve2("second")
	resolve3("third")
	loop.tick()

	// Verify all status objects
	for i, r := range finalResult {
		statusObj, ok := r.(map[string]interface{})
		if !ok {
			t.Errorf("Expected map[string]interface{}, got %T", r)
			continue
		}
		if statusObj["status"] != "fulfilled" {
			t.Errorf("Expected status 'fulfilled', got '%v'", statusObj["status"])
		}
		expectedValues := []interface{}{"first", "second", "third"}
		if statusObj["value"] != expectedValues[i] {
			t.Errorf("Expected value '%v', got '%v'", expectedValues[i], statusObj["value"])
		}
		_, hasReason := statusObj["reason"]
		if hasReason {
			t.Error("Fulfilled promise should not have reason field")
		}
	}
}

func TestPromiseAllSettled_MixedFulfillAndReject(t *testing.T) {
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

	finalResult := make([]any, 3)
	result.Then(func(v any) any {
		values, ok := v.([]any)
		if !ok {
			t.Errorf("Expected []any, got %T", v)
			return nil
		}
		copy(finalResult, values)
		return nil
	}, nil)

	resolve1("first")
	reject2("error from p2")
	resolve3("third")
	loop.tick()

	// Check first (fulfilled)
	status1, ok := finalResult[0].(map[string]interface{})
	if !ok || status1["status"] != "fulfilled" {
		t.Error("First promise should be fulfilled")
	}

	// Check second (rejected)
	status2, ok := finalResult[1].(map[string]interface{})
	if !ok || status2["status"] != "rejected" {
		t.Error("Second promise should be rejected")
	}
	if status2["reason"] != "error from p2" {
		t.Errorf("Expected reason 'error from p2', got '%v'", status2["reason"])
	}

	// Check third (fulfilled)
	status3, ok := finalResult[2].(map[string]interface{})
	if !ok || status3["status"] != "fulfilled" {
		t.Error("Third promise should be fulfilled")
	}
}

func TestPromiseAllSettled_AllRejected(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p1, _, reject1 := js.NewChainedPromise()
	p2, _, reject2 := js.NewChainedPromise()
	p3, _, reject3 := js.NewChainedPromise()

	result := js.AllSettled([]*ChainedPromise{p1, p2, p3})

	finalResult := make([]any, 3)
	result.Then(func(v any) any {
		values, ok := v.([]any)
		if !ok {
			t.Errorf("Expected []any, got %T", v)
			return nil
		}
		copy(finalResult, values)
		return nil
	}, nil)

	reject1("error 1")
	reject2("error 2")
	reject3("error 3")
	loop.tick()

	// Verify all are rejected with status objects
	for i, r := range finalResult {
		statusObj, ok := r.(map[string]interface{})
		if !ok {
			t.Errorf("Expected map[string]interface{}, got %T", r)
			continue
		}
		if statusObj["status"] != "rejected" {
			t.Errorf("Expected status 'rejected', got '%v'", statusObj["status"])
		}
		expectedReasons := []interface{}{"error 1", "error 2", "error 3"}
		if statusObj["reason"] != expectedReasons[i] {
			t.Errorf("Expected reason '%v', got '%v'", expectedReasons[i], statusObj["reason"])
		}
		_, hasValue := statusObj["value"]
		if hasValue {
			t.Error("Rejected promise should not have value field")
		}
	}

	// allSettled should never reject - verify result is fulfilled
	if result.State() != Resolved {
		t.Errorf("Expected Resolved state, got %v", result.State())
	}
}

func TestPromiseAllSettled_NestedPromises(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	base, resolveBase, _ := js.NewChainedPromise()
	chained := base.Then(func(v any) any {
		return v.(string) + " transformed"
	}, nil)

	result := js.AllSettled([]*ChainedPromise{base, chained})

	finalResult := make([]any, 2)
	result.Then(func(v any) any {
		values, ok := v.([]any)
		if !ok {
			t.Errorf("Expected []any, got %T", v)
			return nil
		}
		copy(finalResult, values)
		return nil
	}, nil)

	resolveBase("base")
	loop.tick()

	// Check base promise
	status1, ok := finalResult[0].(map[string]interface{})
	if !ok || status1["status"] != "fulfilled" {
		t.Error("Base promise should be fulfilled")
	}
	if status1["value"] != "base" {
		t.Errorf("Expected value 'base', got '%v'", status1["value"])
	}

	// Check chained promise
	status2, ok := finalResult[1].(map[string]interface{})
	if !ok || status2["status"] != "fulfilled" {
		t.Error("Chained promise should be fulfilled")
	}
	if status2["value"] != "base transformed" {
		t.Errorf("Expected value 'base transformed', got '%v'", status2["value"])
	}
}

// ============================================================================
// Promise.any Tests
// ============================================================================

func TestPromiseAny_EmptyArray_RejectsWithAggregateError(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	result := js.Any([]*ChainedPromise{})

	rejectionReason := ""
	result.Catch(func(r any) any {
		aggErr, ok := r.(*AggregateError)
		if !ok {
			t.Errorf("Expected *AggregateError, got %T", r)
			return nil
		}
		// For empty array, check the inner error in Errors[0]
		if len(aggErr.Errors) == 1 {
			rejectionReason = aggErr.Errors[0].Error()
		}
		return nil
	})

	loop.tick()

	if rejectionReason == "" {
		t.Error("Promise.any with empty array should reject with AggregateError")
	}
	if rejectionReason != "No promises were provided" {
		t.Errorf("Expected 'No promises were provided', got '%s'", rejectionReason)
	}
}

func TestPromiseAny_SingleValue(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, resolve, _ := js.NewChainedPromise()
	result := js.Any([]*ChainedPromise{p})

	finalResult := ""
	result.Then(func(v any) any {
		finalResult = v.(string)
		return nil
	}, nil)

	resolve("success")
	loop.tick()

	if finalResult != "success" {
		t.Errorf("Expected 'success', got '%s'", finalResult)
	}
}

func TestPromiseAny_FirstFulfillmentWins(t *testing.T) {
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
	p2, resolve2, _ := js.NewChainedPromise()
	p3, resolve3, _ := js.NewChainedPromise()

	result := js.Any([]*ChainedPromise{p1, p2, p3})

	finalResult := ""
	result.Then(func(v any) any {
		finalResult = v.(string)
		return nil
	}, nil)

	resolve2("second fulfillment wins")
	loop.tick()

	if finalResult != "second fulfillment wins" {
		t.Errorf("Expected 'second fulfillment wins', got '%s'", finalResult)
	}

	// Other fulfillments should be ignored
	resolve1("first")
	resolve3("third")
	loop.tick()

	if finalResult != "second fulfillment wins" {
		t.Errorf("Result should still be 'second fulfillment wins', got '%s'", finalResult)
	}
}

func TestPromiseAny_AllReject_RejectsWithAggregateError(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p1, _, reject1 := js.NewChainedPromise()
	p2, _, reject2 := js.NewChainedPromise()
	p3, _, reject3 := js.NewChainedPromise()

	result := js.Any([]*ChainedPromise{p1, p2, p3})

	var aggregateErr *AggregateError
	result.Catch(func(r any) any {
		var ok bool
		aggregateErr, ok = r.(*AggregateError)
		if !ok {
			t.Errorf("Expected *AggregateError, got %T", r)
		}
		return nil
	})

	reject1("error 1")
	reject2("error 2")
	reject3("error 3")
	loop.tick()

	if aggregateErr == nil {
		t.Fatal("Expected AggregateError, got nil")
	}

	// Verify aggregate contains all errors
	if len(aggregateErr.Errors) != 3 {
		t.Errorf("Expected 3 errors in AggregateError, got %d", len(aggregateErr.Errors))
	}

	// Convert errors to strings for comparison
	errorMessages := make([]string, len(aggregateErr.Errors))
	for i, err := range aggregateErr.Errors {
		if wrapper, ok := err.(*errorWrapper); ok {
			errorMessages[i] = wrapper.Error()
		} else {
			errorMessages[i] = err.Error()
		}
	}

	expectedMessages := []string{"error 1", "error 2", "error 3"}
	for i, msg := range expectedMessages {
		if errorMessages[i] != msg {
			t.Errorf("Expected error[%d]='%s', got '%s'", i, msg, errorMessages[i])
		}
	}

	if aggregateErr.Message != "All promises were rejected" {
		t.Errorf("Expected 'All promises were rejected', got '%s'", aggregateErr.Message)
	}
}

func TestPromiseAny_RejectThenFulfillment_FulfillmentWins(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p1, _, reject1 := js.NewChainedPromise()
	p2, resolve2, _ := js.NewChainedPromise()
	p3, _, reject3 := js.NewChainedPromise()

	result := js.Any([]*ChainedPromise{p1, p2, p3})

	finalResult := ""
	result.Then(func(v any) any {
		finalResult = v.(string)
		return nil
	}, nil)

	// Reject some first
	reject1("error 1")
	reject3("error 3")
	loop.tick()

	// Then fulfill - should resolve
	resolve2("fulfillment after rejections")
	loop.tick()

	if finalResult != "fulfillment after rejections" {
		t.Errorf("Expected 'fulfillment after rejections', got '%s'", finalResult)
	}

	// Result should be fulfilled, not rejected
	if result.State() != Resolved {
		t.Errorf("Expected Resolved state, got %v", result.State())
	}
}

func TestPromiseAny_NestedPromises(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	// Create a chain that will resolve
	base, resolveBase, _ := js.NewChainedPromise()
	chained := base.Then(func(v any) any {
		return v.(string) + " chained"
	}, nil)

	// Also create a rejected promise
	_, _, reject2 := js.NewChainedPromise()
	reject2("ignore me")

	result := js.Any([]*ChainedPromise{chained, js.Reject("ignore me")})

	finalResult := ""
	result.Then(func(v any) any {
		finalResult = v.(string)
		return nil
	}, nil)

	resolveBase("base")
	loop.tick()

	if finalResult != "base chained" {
		t.Errorf("Expected 'base chained', got '%s'", finalResult)
	}
}

// ============================================================================
// Edge Case and Behavior Tests
// ============================================================================

func TestPromiseCombinators_TableDriven(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		fn     func() *ChainedPromise
		expect string // "resolve" or "reject"
	}{
		{
			name: "All with all fulfill",
			fn: func() *ChainedPromise {
				p1, resolve1, _ := js.NewChainedPromise()
				p2, resolve2, _ := js.NewChainedPromise()
				resolve1("a")
				resolve2("b")
				// Note: Don't call loop.tick() - let event loop goroutine handle it
				return js.All([]*ChainedPromise{p1, p2})
			},
			expect: "resolve",
		},
		{
			name: "All with any reject",
			fn: func() *ChainedPromise {
				p1, _, reject1 := js.NewChainedPromise()
				p2, _, _ := js.NewChainedPromise()
				reject1("error")
				// Note: Don't call loop.tick() - let event loop goroutine handle it
				return js.All([]*ChainedPromise{p1, p2})
			},
			expect: "reject",
		},
		{
			name: "AllSettled always resolves",
			fn: func() *ChainedPromise {
				p1, _, reject1 := js.NewChainedPromise()
				p2, resolve2, _ := js.NewChainedPromise()
				reject1("error")
				resolve2("success")
				// Note: Don't call loop.tick() - let event loop goroutine handle it
				return js.AllSettled([]*ChainedPromise{p1, p2})
			},
			expect: "resolve",
		},
		{
			name: "AllSettled with empty array",
			fn: func() *ChainedPromise {
				return js.AllSettled([]*ChainedPromise{})
			},
			expect: "resolve",
		},
		{
			name: "Any with any fulfill",
			fn: func() *ChainedPromise {
				p1, resolve1, _ := js.NewChainedPromise()
				p2, _, _ := js.NewChainedPromise()
				resolve1("success")
				// Note: Don't call loop.tick() - let event loop goroutine handle it
				return js.Any([]*ChainedPromise{p1, p2})
			},
			expect: "resolve",
		},
		{
			name: "Any with all reject",
			fn: func() *ChainedPromise {
				p1, _, reject1 := js.NewChainedPromise()
				p2, _, reject2 := js.NewChainedPromise()
				reject1("err1")
				reject2("err2")
				// Note: Don't call loop.tick() - let event loop goroutine handle it
				return js.Any([]*ChainedPromise{p1, p2})
			},
			expect: "reject",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			promise := tt.fn()

			// Process microtasks to settle promise combinator
			// Use synchronous tick() to execute microtasks immediately
			loop.tick()

			actualState := promise.State()
			if tt.expect == "resolve" && actualState != Resolved {
				t.Errorf("Expected Resolved, got %v", actualState)
			}
			if tt.expect == "reject" && actualState != Rejected {
				t.Errorf("Expected Rejected, got %v", actualState)
			}
		})
	}
}

func TestPromiseCombinators_ErrorPropagation(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("All propagates first error", func(t *testing.T) {
		p1, _, reject1 := js.NewChainedPromise()
		p2, resolve2, _ := js.NewChainedPromise()
		p3, _, reject3 := js.NewChainedPromise()

		result := js.All([]*ChainedPromise{p1, p2, p3})

		var caught error
		result.Catch(func(r any) any {
			if err, ok := r.(error); ok {
				caught = err
			}
			return nil
		})

		reject1(errors.New("first error"))
		loop.tick()

		if caught == nil {
			t.Error("Expected error to be caught")
		}
		if caught.Error() != "first error" {
			t.Errorf("Expected 'first error', got '%v'", caught)
		}

		// Other promises can still settle
		reject3(errors.New("third error"))
		resolve2("success")
		loop.tick()
	})

	t.Run("Any aggregates all errors", func(t *testing.T) {
		p1, _, reject1 := js.NewChainedPromise()
		p2, _, reject2 := js.NewChainedPromise()

		result := js.Any([]*ChainedPromise{p1, p2})

		var caught *AggregateError
		result.Catch(func(r any) any {
			if agg, ok := r.(*AggregateError); ok {
				caught = agg
			}
			return nil
		})

		reject1(errors.New("error 1"))
		reject2(errors.New("error 2"))
		loop.tick()

		if caught == nil {
			t.Fatal("Expected AggregateError")
		}
		if len(caught.Errors) != 2 {
			t.Errorf("Expected 2 errors, got %d", len(caught.Errors))
		}
	})
}

func TestPromiseCombinators_OrderPreservation(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("All preserves order", func(t *testing.T) {
		p1, resolve1, _ := js.NewChainedPromise()
		p2, resolve2, _ := js.NewChainedPromise()
		p3, resolve3, _ := js.NewChainedPromise()

		// Resolve out of order
		resolve3("third")
		resolve1("first")
		resolve2("second")

		result := js.All([]*ChainedPromise{p1, p2, p3})

		order := make([]string, 3)
		result.Then(func(v any) any {
			values, ok := v.([]any)
			if !ok {
				return nil
			}
			for i, val := range values {
				order[i] = val.(string)
			}
			return nil
		}, nil)

		loop.tick()

		expected := []string{"first", "second", "third"}
		for i, v := range order {
			if v != expected[i] {
				t.Errorf("Order[%d]: expected '%s', got '%s'", i, expected[i], v)
			}
		}
	})

	t.Run("AllSettled preserves order", func(t *testing.T) {
		p1, resolve1, _ := js.NewChainedPromise()
		p2, _, reject2 := js.NewChainedPromise()
		p3, resolve3, _ := js.NewChainedPromise()

		// Settle out of order
		resolve3("third")
		reject2("second")
		resolve1("first")

		result := js.AllSettled([]*ChainedPromise{p1, p2, p3})

		statuses := make([]string, 3)
		result.Then(func(v any) any {
			values, ok := v.([]any)
			if !ok {
				return nil
			}
			for i, val := range values {
				if statusObj, ok := val.(map[string]interface{}); ok {
					statuses[i] = statusObj["status"].(string)
				}
			}
			return nil
		}, nil)

		loop.tick()

		expected := []string{"fulfilled", "rejected", "fulfilled"}
		for i, v := range statuses {
			if v != expected[i] {
				t.Errorf("Status[%d]: expected '%s', got '%s'", i, expected[i], v)
			}
		}
	})
}

// ============================================================================
// Coverage Improvement Tests (Task COVERAGE_1.2)
// ============================================================================

// Test ChainedPromise.State() method - covers promise.go:265
func TestChainedPromise_State_Lifecycle(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("Pending state", func(t *testing.T) {
		p, _, _ := js.NewChainedPromise()
		if p.State() != Pending {
			t.Errorf("Initial state should be Pending, got %v", p.State())
		}
	})

	t.Run("Fulfilled state", func(t *testing.T) {
		p, resolve, _ := js.NewChainedPromise()
		resolve("value")
		loop.tick()
		if p.State() != Fulfilled {
			t.Errorf("State should be Fulfilled, got %v", p.State())
		}
	})

	t.Run("Rejected state", func(t *testing.T) {
		p, _, reject := js.NewChainedPromise()
		reject("error")
		loop.tick()
		if p.State() != Rejected {
			t.Errorf("State should be Rejected, got %v", p.State())
		}
	})
}

// Test ChainedPromise.Value() and Reason() methods - covers promise.go:272,284
func TestChainedPromise_ValueAndReason_Accessors(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("Value() returns fulfillment value", func(t *testing.T) {
		p, resolve, _ := js.NewChainedPromise()
		resolve("test value")
		loop.tick()

		val := p.Value()
		if val != "test value" {
			t.Errorf("Expected 'test value', got %v", val)
		}
	})

	t.Run("Value() returns nil for pending", func(t *testing.T) {
		p, _, _ := js.NewChainedPromise()
		if p.Value() != nil {
			t.Errorf("Pending promise Value() should return nil, got %v", p.Value())
		}
	})

	t.Run("Value() returns nil for rejected", func(t *testing.T) {
		p, _, reject := js.NewChainedPromise()
		reject("error")
		loop.tick()

		val := p.Value()
		if val != nil {
			t.Errorf("Rejected promise Value() should return nil, got %v", val)
		}
	})

	t.Run("Reason() returns rejection reason", func(t *testing.T) {
		p, _, reject := js.NewChainedPromise()
		reject("reason value")
		loop.tick()

		reason := p.Reason()
		if reason != "reason value" {
			t.Errorf("Expected 'reason value', got %v", reason)
		}
	})

	t.Run("Reason() returns nil for pending", func(t *testing.T) {
		p, _, _ := js.NewChainedPromise()
		if p.Reason() != nil {
			t.Errorf("Pending promise Reason() should return nil, got %v", p.Reason())
		}
	})

	t.Run("Reason() returns nil for fulfilled", func(t *testing.T) {
		p, resolve, _ := js.NewChainedPromise()
		resolve("value")
		loop.tick()

		reason := p.Reason()
		if reason != nil {
			t.Errorf("Fulfilled promise Reason() should return nil, got %v", reason)
		}
	})
}

// Test Chaining Cycle Detection - covers promise.go:296-299
func TestChainedPromise_CycleDetection(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, resolve, _ := js.NewChainedPromise()

	// Attempt to resolve promise with itself
	resolve(p)
	loop.tick()

	// Should reject with TypeError
	if p.State() != Rejected {
		t.Errorf("Self-resolution should reject, got state %v", p.State())
	}

	// Verify error message contains "cycle"
	reason := p.Reason()
	// Cycle detection creates error via fmt.Errorf, so it's *errors.errorString
	if err, ok := reason.(error); ok {
		errStr := err.Error()
		if !strings.Contains(errStr, "cycle") {
			t.Errorf("Error should mention cycle, got: %s", errStr)
		}
		if !strings.Contains(errStr, "TypeError") {
			t.Errorf("Error should be TypeError, got: %s", errStr)
		}
	} else {
		t.Errorf("Rejection reason should be error, got %T", reason)
	}
}

// Test Promise Adopts State from Another Promise - covers promise.go:304-318
func TestChainedPromise_AdoptsState(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("Adopts fulfilled state", func(t *testing.T) {
		p1, resolve1, _ := js.NewChainedPromise()
		p2, resolve2, _ := js.NewChainedPromise()

		// Resolve p2 with p1
		resolve2(p1)

		// Now settle p1
		resolve1("adopted value")
		loop.tick()

		// Both should be fulfilled with same value
		if p1.State() != Fulfilled {
			t.Errorf("p1 should be fulfilled, got state %v", p1.State())
		}
		if p2.State() != Fulfilled {
			t.Errorf("p2 should be fulfilled (adopted), got state %v", p2.State())
		}
		if p1.Value() != "adopted value" {
			t.Errorf("p1 value mismatch, got: %v", p1.Value())
		}
		if p2.Value() != "adopted value" {
			t.Errorf("p2 should adopt p1's value, got: %v", p2.Value())
		}
	})

	t.Run("Adopts rejected state", func(t *testing.T) {
		p1, _, reject1 := js.NewChainedPromise()
		p2, resolve2, _ := js.NewChainedPromise()

		resolve2(p1)
		reject1("adopted error")
		loop.tick()

		// Both should be rejected with same reason
		if p1.State() != Rejected {
			t.Errorf("p1 should be rejected, got state %v", p1.State())
		}
		if p2.State() != Rejected {
			t.Errorf("p2 should be rejected (adopted), got state %v", p2.State())
		}
		if p2.Reason() != "adopted error" {
			t.Errorf("p2 should adopt p1's reason, got: %v", p2.Reason())
		}
	})
}

// Test Nil Handler Pass-Through - covers tryCall promise.go:680-684
func TestChainedPromise_NilHandlerPassThrough(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("Then with nil handlers passes value through", func(t *testing.T) {
		p, resolve, _ := js.NewChainedPromise()
		result := p.Then(nil, nil) // Both handlers nil
		resolve("original value")
		loop.tick()

		// Should pass through without modification
		val := result.Value()
		if val != "original value" {
			t.Errorf("Nil handler should pass-through value, got: %v", val)
		}
	})

	t.Run("Catch with nil handler passes reason through", func(t *testing.T) {
		p, _, reject := js.NewChainedPromise()
		result := p.Catch(nil) // nil handler
		reject("original error")
		loop.tick()

		reason := result.Reason()
		if reason != "original error" {
			t.Errorf("Nil Catch handler should pass-through reason, got: %v", reason)
		}
	})
}

// Test Resolve/Reject Idempotency - covers promise.go:322,363
func TestChainedPromise_ResolveRejectIdempotency(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("Resolve only accepts first call", func(t *testing.T) {
		p1, resolve1, _ := js.NewChainedPromise()

		resolve1("first")
		resolve1("second") // Should be ignored
		resolve1("third")  // Should be ignored

		loop.tick()

		if p1.Value() != "first" {
			t.Errorf("Should only use first resolve, got: %v", p1.Value())
		}
	})

	t.Run("Reject only accepts first call", func(t *testing.T) {
		p2, _, reject2 := js.NewChainedPromise()

		reject2("first error")
		reject2("second error") // Should be ignored
		reject2("third error")  // Should be ignored

		loop.tick()

		if p2.Reason() != "first error" {
			t.Errorf("Should only use first reject, got: %v", p2.Reason())
		}
	})

	t.Run("Resolve after reject has no effect", func(t *testing.T) {
		p3, resolve3, reject3 := js.NewChainedPromise()

		reject3("rejected")
		resolve3("resolved") // Should be ignored

		loop.tick()

		if p3.State() != Rejected {
			t.Errorf("State should remain Rejected, got: %v", p3.State())
		}
	})

	t.Run("Reject after resolve has no effect", func(t *testing.T) {
		p4, resolve4, reject4 := js.NewChainedPromise()

		resolve4("resolved")
		reject4("rejected") // Should be ignored

		loop.tick()

		if p4.State() != Fulfilled {
			t.Errorf("State should remain Fulfilled, got: %v", p4.State())
		}
	})
}

// Test Then method - covers promise.go Then
func TestChainedPromise_Then(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js1, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	// Create promise with js1
	p := js1.Resolve("original value")

	// Attach handler using Then (uses promise's stored js)
	result := p.Then(
		func(v any) any {
			return v.(string) + " processed by js2"
		},
		nil,
	)

	loop.tick()

	// Verify handler executed with correct transform
	if result.State() != Fulfilled {
		t.Errorf("Result should be fulfilled, got state: %v", result.State())
	}

	val := result.Value()
	expected := "original value processed by js2"
	if val != expected {
		t.Errorf("Expected '%s', got: %v", expected, val)
	}
}

// Test AggregateError types - covers promise.go:1060
func TestAggregateError_ErrorMessages(t *testing.T) {
	t.Run("Default message", func(t *testing.T) {
		agg := &AggregateError{
			Errors: []error{errors.New("error1"), errors.New("error2")},
		}
		msg := agg.Error()
		if msg != "All promises were rejected" {
			t.Errorf("Expected default message, got: %s", msg)
		}
	})

	t.Run("Custom message", func(t *testing.T) {
		agg := &AggregateError{
			Message: "custom error message",
			Errors:  []error{errors.New("error")},
		}
		msg := agg.Error()
		if msg != "custom error message" {
			t.Errorf("Expected custom message, got: %s", msg)
		}
	})
}

// Test errNoPromiseResolved type - covers promise.go:1065
func Test_errNoPromiseResolved_Error(t *testing.T) {
	err := errNoPromiseResolved{}
	msg := err.Error()
	if msg != "No promises were provided" {
		t.Errorf("Expected 'No promises were provided', got: %s", msg)
	}
}

// Test errorWrapper with various types - covers promise.go:1076
func Test_errorWrapper_VariousTypes(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
	}{
		{"string error", "error string"},
		{"int error", 12345},
		{"struct error", struct{ Field int }{Field: 99}},
		{"nil error", nil},
		{"float error", 3.14159},
		{"bool error", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapper := &errorWrapper{Value: tt.input}
			errStr := wrapper.Error()
			if errStr == "" {
				t.Error("errorWrapper should produce non-empty string")
			}
			// Verify format contains value info (not type info - errorWrapper uses %v format)
			expectedValue := fmt.Sprintf("%v", tt.input)
			if !strings.Contains(errStr, expectedValue) && errStr != expectedValue {
				t.Errorf("Error string should contain value '%v', got: %s", expectedValue, errStr)
			}
		})
	}
}

// Test Combinators with nil values
func TestPromiseCombinators_NilValues(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("All with nil fulfillment values", func(t *testing.T) {
		result := js.All([]*ChainedPromise{
			js.Resolve("value1"),
			js.Resolve(nil),
			js.Resolve("value3"),
		})
		loop.tick()

		values := result.Value().([]any)
		if values[1] != nil {
			t.Errorf("Should preserve nil value, got: %v", values[1])
		}
	})

	t.Run("Race with nil value winning", func(t *testing.T) {
		p1, resolve1, _ := js.NewChainedPromise()
		p2, _, _ := js.NewChainedPromise()

		result := js.Race([]*ChainedPromise{p1, p2})
		resolve1(nil) // First settles with nil
		loop.tick()

		if result.Value() != nil {
			t.Errorf("Race should preserve nil, got: %v", result.Value())
		}
	})

	t.Run("AllSettled with nil fulfillment", func(t *testing.T) {
		result := js.AllSettled([]*ChainedPromise{js.Resolve(nil)})
		loop.tick()

		outcomes := result.Value().([]any)
		status := outcomes[0].(map[string]interface{})
		if status["value"] != nil {
			t.Errorf("AllSettled should preserve nil value, got: %v", status["value"])
		}
		if status["status"] != "fulfilled" {
			t.Errorf("Status should be fulfilled, got: %v", status["status"])
		}
	})

	t.Run("AllSettled with nil rejection reason", func(t *testing.T) {
		result := js.AllSettled([]*ChainedPromise{js.Reject(nil)})
		loop.tick()

		outcomes := result.Value().([]any)
		status := outcomes[0].(map[string]interface{})
		if status["reason"] != nil {
			t.Errorf("AllSettled should preserve nil reason, got: %v", status["reason"])
		}
		if status["status"] != "rejected" {
			t.Errorf("Status should be rejected, got: %v", status["status"])
		}
	})

	t.Run("Any with first nil value fulfilling", func(t *testing.T) {
		p1, resolve1, _ := js.NewChainedPromise()
		p2, _, _ := js.NewChainedPromise()

		result := js.Any([]*ChainedPromise{p1, p2})
		resolve1(nil) // First resolves with nil
		loop.tick()

		// Nil should be a valid fulfillment for Any
		if result.State() != Fulfilled {
			t.Errorf("Any with nil resolution should fulfill, got state: %v", result.State())
		}
		if result.Value() != nil {
			t.Errorf("Any should preserve nil value, got: %v", result.Value())
		}
	})
}

// Test Combinators with already-settled promises
func TestPromiseCombinators_AlreadySettled(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("All with some already fulfilled", func(t *testing.T) {
		p1 := js.Resolve("already1")
		p2, resolve2, _ := js.NewChainedPromise()
		p3 := js.Resolve("already3")

		result := js.All([]*ChainedPromise{p1, p2, p3})

		// Resolve pending one
		resolve2("pending2")
		loop.tick()

		values := result.Value().([]any)
		expected := []any{"already1", "pending2", "already3"}
		for i, v := range values {
			if v != expected[i] {
				t.Errorf("Value[%d]: expected %v, got %v", i, expected[i], v)
			}
		}
	})

	t.Run("Race with all already fulfilled", func(t *testing.T) {
		p1 := js.Resolve("first")
		p2 := js.Resolve("second")

		result := js.Race([]*ChainedPromise{p1, p2})
		loop.tick()

		// First in array should win (microtask order)
		val := result.Value()
		if val != "first" && val != "second" {
			t.Errorf("Unexpected race result: %v", val)
		}
	})

	t.Run("AllSettled with mixed already settled", func(t *testing.T) {
		p1 := js.Resolve("fulfilled")
		p2 := js.Reject("rejected")
		p3, resolve3, _ := js.NewChainedPromise()

		result := js.AllSettled([]*ChainedPromise{p1, p2, p3})

		resolve3("later fulfilled")
		loop.tick()

		outcomes := result.Value().([]any)

		status1 := outcomes[0].(map[string]interface{})
		if status1["status"] != "fulfilled" || status1["value"] != "fulfilled" {
			t.Errorf("Outcome[0] incorrect: %v", status1)
		}

		status2 := outcomes[1].(map[string]interface{})
		if status2["status"] != "rejected" || status2["reason"] != "rejected" {
			t.Errorf("Outcome[1] incorrect: %v", status2)
		}

		status3 := outcomes[2].(map[string]interface{})
		if status3["status"] != "fulfilled" || status3["value"] != "later fulfilled" {
			t.Errorf("Outcome[2] incorrect: %v", status3)
		}
	})
}

// Test js.Resolve() and js.Reject() helpers - covers js.go:522,533
func TestJS_ConvenienceHelpers(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("js.Resolve() returns already fulfilled promise", func(t *testing.T) {
		p := js.Resolve("immediate value")

		if p.State() != Fulfilled {
			t.Errorf("Should be already fulfilled, got state: %v", p.State())
		}

		if p.Value() != "immediate value" {
			t.Errorf("Value should be 'immediate value', got: %v", p.Value())
		}

		// Should not require loop.tick()
		alreadySettled := p.Value() != ""
		if !alreadySettled {
			t.Error("js.Resolve() should create pre-fulfilled promise")
		}
	})

	t.Run("js.Resolve() with nil", func(t *testing.T) {
		p := js.Resolve(nil)

		if p.State() != Fulfilled {
			t.Errorf("Should be fulfilled, got state: %v", p.State())
		}

		if p.Value() != nil {
			t.Errorf("Value should be nil, got: %v", p.Value())
		}
	})

	t.Run("js.Reject() returns already rejected promise", func(t *testing.T) {
		p := js.Reject("immediate error")

		if p.State() != Rejected {
			t.Errorf("Should be already rejected, got state: %v", p.State())
		}

		if p.Reason() != "immediate error" {
			t.Errorf("Reason should be 'immediate error', got: %v", p.Reason())
		}

		// Should not require loop.tick()
		alreadySettled := p.Reason() != ""
		if !alreadySettled {
			t.Error("js.Reject() should create pre-rejected promise")
		}
	})

	t.Run("js.Reject() with error type", func(t *testing.T) {
		expectedErr := errors.New("actual error")
		p := js.Reject(expectedErr)

		if p.State() != Rejected {
			t.Errorf("Should be rejected, got state: %v", p.State())
		}

		// Error is passed through directly (NOT wrapped)
		reason := p.Reason()
		if reason != expectedErr {
			t.Errorf("Reason should be original error, got: %v (type %T)", reason, reason)
		}

		// Verify it's the exact same error instance
		if err, ok := reason.(error); ok {
			if err.Error() != expectedErr.Error() {
				t.Errorf("Error message mismatch: got '%s', expected '%s'", err.Error(), expectedErr.Error())
			}
		} else {
			t.Errorf("Reason should be error type, got %T", reason)
		}
	})

	t.Run("js.Resolve() promise can be chained", func(t *testing.T) {
		p1 := js.Resolve("first")

		var chainCalled bool
		p2 := p1.Then(func(v any) any {
			chainCalled = true
			return v.(string) + " chained"
		}, nil)

		loop.tick()

		if !chainCalled {
			t.Error("Chain handler should execute")
		}

		if p2.Value() != "first chained" {
			t.Errorf("Chained value incorrect, got: %v", p2.Value())
		}
	})

	t.Run("js.Reject() promise can be caught", func(t *testing.T) {
		p1 := js.Reject("error")

		var catchCalled bool
		p2 := p1.Catch(func(r any) any {
			catchCalled = true
			return "recovered: " + r.(string)
		})

		loop.tick()

		if !catchCalled {
			t.Error("Catch handler should execute")
		}

		if p2.Value() != "recovered: error" {
			t.Errorf("Recovered value incorrect, got: %v", p2.Value())
		}
	})
}

// Test Combinators with JS.Resolve/Reject shortcuts
func TestPromiseCombinators_WithConvenienceHelpers(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("All with mixed js.Resolve and new promises", func(t *testing.T) {
		p1 := js.Resolve("already1")
		p2, resolve2, _ := js.NewChainedPromise()
		p3 := js.Resolve("already3")

		result := js.All([]*ChainedPromise{p1, p2, p3})

		resolve2("resolved2")
		loop.tick()

		values := result.Value().([]any)
		expected := []any{"already1", "resolved2", "already3"}
		for i, v := range values {
			if v != expected[i] {
				t.Errorf("Value[%d]: expected %v, got %v", i, expected[i], v)
			}
		}
	})

	t.Run("Race with js.Resolve winning", func(t *testing.T) {
		p1 := js.Resolve("immediate winner")
		p2, _, _ := js.NewChainedPromise()

		result := js.Race([]*ChainedPromise{p1, p2})
		loop.tick()

		// js.Resolve is already settled, should win
		if result.Value() != "immediate winner" {
			t.Errorf("Race result incorrect, got: %v", result.Value())
		}
	})

	t.Run("AllSettled with mixed js.Resolve and js.Reject", func(t *testing.T) {
		p1 := js.Resolve("fulfilled")
		p2 := js.Reject("rejected")
		p3, resolve3, _ := js.NewChainedPromise()

		result := js.AllSettled([]*ChainedPromise{p1, p2, p3})

		resolve3("later resolved")
		loop.tick()

		outcomes := result.Value().([]any)

		// Verify all outcomes are represented
		if len(outcomes) != 3 {
			t.Errorf("Expected 3 outcomes, got %d", len(outcomes))
		}

		// Check first outcome (fulfilled)
		status1 := outcomes[0].(map[string]interface{})
		if status1["status"] != "fulfilled" {
			t.Errorf("Outcome[0] status incorrect: %v", status1)
		}

		// Check second outcome (rejected)
		status2 := outcomes[1].(map[string]interface{})
		if status2["status"] != "rejected" {
			t.Errorf("Outcome[1] status incorrect: %v", status2)
		}

		// Check third outcome (fulfilled)
		status3 := outcomes[2].(map[string]interface{})
		if status3["status"] != "fulfilled" {
			t.Errorf("Outcome[2] status incorrect: %v", status3)
		}
	})

	t.Run("Any with pre-fulfilled promise", func(t *testing.T) {
		p1 := js.Resolve("first winner")
		p2, _, _ := js.NewChainedPromise()
		p3, _, _ := js.NewChainedPromise()

		result := js.Any([]*ChainedPromise{p1, p2, p3})
		loop.tick()

		// Pre-fulfilled should win
		if result.Value() != "first winner" {
			t.Errorf("Any result incorrect, got: %v", result.Value())
		}
	})
}

// Test Promise chaining edge cases
func TestChainedPromise_ChainingEdgeCases(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("Long chain of Thens", func(t *testing.T) {
		p := js.Resolve("start")

		current := p
		for i := 0; i < 10; i++ {
			current = current.Then(func(v any) any {
				return v.(string) + " +1"
			}, nil)
		}

		loop.tick()

		expected := "start"
		for i := 0; i < 10; i++ {
			expected = expected + " +1"
		}

		if current.Value() != expected {
			t.Errorf("Long chain result incorrect, got: %v", current.Value())
		}
	})

	t.Run("Chain with mixed resolve and reject", func(t *testing.T) {
		p, _, reject := js.NewChainedPromise()

		p1 := p.Then(func(v any) any {
			return "should not execute"
		}, nil)

		p2 := p1.Catch(func(r any) any {
			return "caught: " + r.(string)
		})

		p3 := p2.Then(func(v any) any {
			return "then after catch: " + v.(string)
		}, nil)

		reject("initial error")
		loop.tick()

		if p3.Value() != "then after catch: caught: initial error" {
			t.Errorf("Chain result incorrect, got: %v", p3.Value())
		}
	})

	t.Run("Finally executes on fulfilled", func(t *testing.T) {
		var finallyCalled bool
		finallyExecuted := make(chan struct{})

		p, resolve, _ := js.NewChainedPromise()

		result := p.Finally(func() {
			finallyCalled = true
			close(finallyExecuted)
		})

		resolve("value")
		loop.tick() // MUST run microtask queue for Finally to execute

		// Wait for finally to execute
		<-finallyExecuted

		if !finallyCalled {
			t.Error("Finally should execute on fulfilled promise")
		}

		// Finally should not change value
		if result.Value() != "value" {
			t.Errorf("Finally should not modify value, got: %v", result.Value())
		}
	})

	t.Run("Finally executes on rejected", func(t *testing.T) {
		var finallyCalled bool
		finallyExecuted := make(chan struct{})

		p, _, reject := js.NewChainedPromise()

		result := p.Finally(func() {
			finallyCalled = true
			close(finallyExecuted)
		})

		reject("error")
		loop.tick() // MUST run microtask queue for Finally to execute

		// Wait for finally to execute
		<-finallyExecuted

		if !finallyCalled {
			t.Error("Finally should execute on rejected promise")
		}

		// Finally should not change rejection reason
		if result.Reason() != "error" {
			t.Errorf("Finally should not modify reason, got: %v", result.Reason())
		}
	})

	t.Run("Finally after catch", func(t *testing.T) {
		var catchCalled, finallyCalled bool
		catchExecuted := make(chan struct{})
		finallyExecuted := make(chan struct{})

		p, _, reject := js.NewChainedPromise()

		result := p.Catch(func(r any) any {
			catchCalled = true
			close(catchExecuted)
			return "recovered"

		}).Finally(func() {
			finallyCalled = true
			close(finallyExecuted)
		})

		reject("error")
		loop.tick() // MUST run microtask queue for Finally to execute

		<-catchExecuted
		<-finallyExecuted

		if !catchCalled {
			t.Error("Catch should execute")
		}
		if !finallyCalled {
			t.Error("Finally should execute after catch")
		}

		if result.Value() != "recovered" {
			t.Errorf("Result value incorrect, got: %v", result.Value())
		}
	})
}

// Test Then returns new promise (identity)
func TestChainedPromise_ThenReturnsNewPromise(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p1, resolve, _ := js.NewChainedPromise()
	p2 := p1.Then(func(v any) any {
		return v.(string) + " modified"
	}, nil)

	resolve("original")
	loop.tick()

	// Both promises should be separate instances
	if p1 == p2 {
		t.Error("Then should return new promise instance")
	}

	// p1 should be original value
	if p1.Value() != "original" {
		t.Errorf("p1 value should be original, got: %v", p1.Value())
	}

	// p2 should be modified value
	if p2.Value() != "original modified" {
		t.Errorf("p2 value should be modified, got: %v", p2.Value())
	}
}

// Test Promise value transformations
func TestChainedPromise_ValueTransformations(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("Transform string to int", func(t *testing.T) {
		p, resolve, _ := js.NewChainedPromise()

		result := p.Then(func(v any) any {
			return len(v.(string))
		}, nil)

		resolve("hello")
		loop.tick()

		val := result.Value()
		if val != 5 {
			t.Errorf("Expected 5, got %v", val)
		}
	})

	t.Run("Transform int to string", func(t *testing.T) {
		p, resolve, _ := js.NewChainedPromise()

		result := p.Then(func(v any) any {
			num := v.(int)
			return fmt.Sprintf("%d", num)
		}, nil)

		resolve(123)
		loop.tick()

		val := result.Value()
		if val != "123" {
			t.Errorf("Expected '123', got %v", val)
		}
	})

	t.Run("Transform to map", func(t *testing.T) {
		p, resolve, _ := js.NewChainedPromise()

		result := p.Then(func(v any) any {
			return map[string]interface{}{"value": v}
		}, nil)

		resolve("original")
		loop.tick()

		val := result.Value()
		if m, ok := val.(map[string]interface{}); ok {
			if m["value"] != "original" {
				t.Errorf("Map value incorrect, got: %v", m)
			}
		} else {
			t.Errorf("Expected map[string]interface{}, got %T", val)
		}
	})
}
