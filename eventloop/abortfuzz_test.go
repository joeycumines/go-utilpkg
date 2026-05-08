package eventloop

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

type abortSignalModel struct {
	signal          *AbortSignal
	deps            []int
	reason          any
	expectedReasons []any
	actualReasons   []any
	handlers        int
	actualCalls     int
	aborted         bool
}

func FuzzAbortSignalGraphModel(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte("abort-any-with-preaborted-inputs"))
	f.Add([]byte{255, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9})

	f.Fuzz(func(t *testing.T, data []byte) {
		r := newFuzzReader(data)
		controllers := make([]*AbortController, 0, 8)
		controllerModels := make([]int, 0, 8)
		models := make([]abortSignalModel, 0, 32)
		dependents := make(map[int][]int)

		newController := func() int {
			c := NewAbortController()
			signal := c.Signal()
			controllers = append(controllers, c)
			models = append(models, abortSignalModel{signal: signal})
			idx := len(models) - 1
			controllerModels = append(controllerModels, idx)
			if c.Signal() != signal {
				t.Fatalf("AbortController.Signal did not return stable pointer")
			}
			return idx
		}

		for range 2 + r.intn(4) {
			newController()
		}

		var propagateAbort func(int, any)
		propagateAbort = func(idx int, reason any) {
			if idx < 0 || idx >= len(models) || models[idx].aborted {
				return
			}
			models[idx].aborted = true
			models[idx].reason = reason
			for range models[idx].handlers {
				models[idx].expectedReasons = append(models[idx].expectedReasons, reason)
			}
			for _, dep := range dependents[idx] {
				propagateAbort(dep, reason)
			}
		}

		reasons := []any{
			"user",
			"timeout",
			42,
			errors.New("wrapped abort reason"),
			&AbortError{Reason: "nested"},
			newFuzzListError("non-comparable"),
			newFuzzInterfaceError([]string{"interface field"}),
		}

		ops := 1 + min(len(data)*2, 512)
		for range ops {
			switch r.byte() % 6 {
			case 0:
				newController()

			case 1:
				if len(controllers) == 0 {
					break
				}
				controllerIdx := r.intn(len(controllers))
				modelIdx := controllerModels[controllerIdx]
				reason := reasons[r.intn(len(reasons))]
				if r.byte()%7 == 0 {
					reason = nil
				}
				wasAborted := models[modelIdx].aborted
				previousReason := models[modelIdx].reason
				controllers[controllerIdx].Abort(reason)
				actualReason := controllers[controllerIdx].Signal().Reason()
				if wasAborted {
					if !abortReasonEquivalent(actualReason, previousReason) {
						t.Fatalf("repeated Abort changed source reason: got %#v want %#v", actualReason, previousReason)
					}
				} else if reason == nil {
					defaultReason, ok := actualReason.(*AbortError)
					if !ok || defaultReason.Reason != "Aborted" {
						t.Fatalf("nil Abort source reason = %#v, want *AbortError{Reason: %q}", actualReason, "Aborted")
					}
				} else if !abortReasonEquivalent(actualReason, reason) {
					t.Fatalf("Abort changed source reason identity: got %#v want %#v", actualReason, reason)
				}
				propagateAbort(modelIdx, actualReason)

			case 2:
				if len(models) == 0 {
					break
				}
				idx := r.intn(len(models))
				models[idx].handlers++
				models[idx].signal.OnAbort(func(reason any) {
					models[idx].actualCalls++
					models[idx].actualReasons = append(models[idx].actualReasons, reason)
				})
				if models[idx].aborted {
					models[idx].expectedReasons = append(models[idx].expectedReasons, models[idx].reason)
				}

			case 3:
				if len(models) == 0 || len(models) >= 32 {
					break
				}
				depCount := r.intn(min(len(models), 6) + 1)
				deps := make([]int, 0, depCount)
				signals := make([]*AbortSignal, 0, depCount)
				for range depCount {
					if r.byte()%5 == 0 {
						signals = append(signals, nil)
						continue
					}
					idx := r.intn(len(models))
					deps = append(deps, idx)
					signals = append(signals, models[idx].signal)
				}
				composite := AbortAny(signals)
				models = append(models, abortSignalModel{signal: composite, deps: deps})
				compositeIdx := len(models) - 1
				abortedAtCreation := false
				for _, dep := range deps {
					if models[dep].aborted {
						propagateAbort(compositeIdx, models[dep].reason)
						abortedAtCreation = true
						break
					}
				}
				if !abortedAtCreation {
					for _, dep := range deps {
						dependents[dep] = append(dependents[dep], compositeIdx)
					}
				}

			case 4:
				if len(models) == 0 {
					break
				}
				idx := r.intn(len(models))
				models[idx].handlers++
				models[idx].signal.OnAbort(func(reason any) {
					models[idx].actualCalls++
					models[idx].actualReasons = append(models[idx].actualReasons, reason)
				})
				if models[idx].aborted {
					models[idx].expectedReasons = append(models[idx].expectedReasons, models[idx].reason)
				}

			case 5:
				if len(models) == 0 {
					break
				}
				idx := r.intn(len(models))
				err := models[idx].signal.ThrowIfAborted()
				if models[idx].aborted {
					if reasonErr, ok := models[idx].reason.(error); ok {
						if !abortReasonEquivalent(err, reasonErr) {
							t.Fatalf("ThrowIfAborted = %#v, want exact reason %#v", err, reasonErr)
						}
					} else {
						var abortErr *AbortError
						if !errors.As(err, &abortErr) {
							t.Fatalf("ThrowIfAborted on aborted signal returned %T, want *AbortError", err)
						}
						if !abortReasonEquivalent(abortErr.Reason, models[idx].reason) {
							t.Fatalf("ThrowIfAborted reason = %#v, want %#v", abortErr.Reason, models[idx].reason)
						}
					}
				} else if err != nil {
					t.Fatalf("ThrowIfAborted on non-aborted signal returned %v", err)
				}
			}

			for i := range models {
				gotAborted := models[i].signal.Aborted()
				if gotAborted != models[i].aborted {
					t.Fatalf("signal %d Aborted = %v, want %v", i, gotAborted, models[i].aborted)
				}
				if !abortReasonEquivalent(models[i].signal.Reason(), models[i].reason) {
					t.Fatalf("signal %d Reason = %#v, want %#v", i, models[i].signal.Reason(), models[i].reason)
				}
				if models[i].actualCalls != len(models[i].actualReasons) {
					t.Fatalf("signal %d handler accounting mismatch: calls=%d reasons=%d", i, models[i].actualCalls, len(models[i].actualReasons))
				}
				if len(models[i].actualReasons) != len(models[i].expectedReasons) {
					t.Fatalf("signal %d handler reason count = %d, want %d", i, len(models[i].actualReasons), len(models[i].expectedReasons))
				}
				for j, reason := range models[i].actualReasons {
					if !abortReasonEquivalent(reason, models[i].expectedReasons[j]) {
						t.Fatalf("signal %d handler reason[%d] = %#v, want %#v", i, j, reason, models[i].expectedReasons[j])
					}
				}
			}
		}
	})
}

func abortReasonEquivalent(got, want any) bool {
	gotError, gotIsError := got.(error)
	wantError, wantIsError := want.(error)
	if gotIsError || wantIsError {
		if !gotIsError || !wantIsError {
			return false
		}
		if reflect.TypeOf(gotError) != reflect.TypeOf(wantError) {
			return false
		}
		if reflect.ValueOf(gotError).Comparable() && reflect.ValueOf(wantError).Comparable() {
			return gotError == wantError
		}
		gotWitness, gotHasWitness := gotError.(fuzzReasonIdentityWitness)
		wantWitness, wantHasWitness := wantError.(fuzzReasonIdentityWitness)
		if !gotHasWitness || !wantHasWitness || gotWitness.fuzzReasonIdentity() == nil ||
			gotWitness.fuzzReasonIdentity() != wantWitness.fuzzReasonIdentity() {
			return false
		}
		return reflect.DeepEqual(gotError, wantError)
	}
	return reflect.DeepEqual(got, want)
}

// fuzzReasonIdentityWitness gives the fuzz oracle a stable identity for error
// values whose dynamic representation cannot be compared with ==. Unknown
// non-comparable error types fail closed instead of weakening identity checks to
// structural equality.
type fuzzReasonIdentityWitness interface {
	fuzzReasonIdentity() *fuzzReasonIdentityToken
}

// fuzzReasonIdentityToken is deliberately non-zero-sized: distinct zero-sized
// allocations are permitted to have equal pointers.
type fuzzReasonIdentityToken struct {
	marker byte
}

type fuzzListError struct {
	identity *fuzzReasonIdentityToken
	values   []string
}

func newFuzzListError(values ...string) fuzzListError {
	return fuzzListError{
		identity: &fuzzReasonIdentityToken{marker: 1},
		values:   values,
	}
}

func (e fuzzListError) Error() string {
	return strings.Join(e.values, ",")
}

func (e fuzzListError) fuzzReasonIdentity() *fuzzReasonIdentityToken {
	return e.identity
}

type fuzzInterfaceError struct {
	identity *fuzzReasonIdentityToken
	value    any
}

func newFuzzInterfaceError(value any) fuzzInterfaceError {
	return fuzzInterfaceError{
		identity: &fuzzReasonIdentityToken{marker: 1},
		value:    value,
	}
}

func (e fuzzInterfaceError) Error() string {
	return fmt.Sprint(e.value)
}

func (e fuzzInterfaceError) fuzzReasonIdentity() *fuzzReasonIdentityToken {
	return e.identity
}

type fuzzUnknownListError []string

func (e fuzzUnknownListError) Error() string {
	return strings.Join(e, ",")
}

func TestAbortReasonEquivalentSupportsNonComparableErrors(t *testing.T) {
	t.Run("slice error", func(t *testing.T) {
		reason := newFuzzListError("failure")
		if !abortReasonEquivalent(reason, reason) {
			t.Fatal("reason did not match itself")
		}
		if abortReasonEquivalent(reason, newFuzzListError("failure")) {
			t.Fatal("distinct cloned slice error matched by structure")
		}
	})

	t.Run("comparable type with non-comparable interface value", func(t *testing.T) {
		reason := newFuzzInterfaceError([]string{"failure"})
		if !abortReasonEquivalent(reason, reason) {
			t.Fatal("reason did not match itself")
		}
		if abortReasonEquivalent(reason, newFuzzInterfaceError([]string{"failure"})) {
			t.Fatal("distinct cloned interface error matched by structure")
		}
	})

	t.Run("asymmetric runtime comparability", func(t *testing.T) {
		identity := &fuzzReasonIdentityToken{marker: 1}
		comparable := fuzzInterfaceError{identity: identity, value: "failure"}
		nonComparable := fuzzInterfaceError{identity: identity, value: []string{"failure"}}
		if abortReasonEquivalent(comparable, nonComparable) {
			t.Fatal("comparable and non-comparable payloads matched")
		}
		if abortReasonEquivalent(nonComparable, comparable) {
			t.Fatal("non-comparable and comparable payloads matched")
		}
	})

	t.Run("unknown non-comparable error fails closed", func(t *testing.T) {
		reason := fuzzUnknownListError{"failure"}
		if abortReasonEquivalent(reason, reason) {
			t.Fatal("non-comparable error without an identity witness matched")
		}
	})
}

func FuzzAbortTimeoutReasonStability(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{1, 2, 3, 4})
	f.Add([]byte("manual-before-timeout"))
	f.Add([]byte("manual-races-timeout-publication"))

	f.Fuzz(func(t *testing.T, data []byte) {
		r := newFuzzReader(data)
		loop := New(WithAutoExit(true))
		controller, err := AbortTimeout(loop, 0)
		if err != nil {
			t.Fatalf("AbortTimeout: %v", err)
		}
		signal := controller.Signal()
		mode := r.byte() % 3
		manualReason := any("manual")
		if r.bool() {
			manualReason = errors.New("manual abort")
		}
		if mode == 1 {
			controller.Abort(manualReason)
		}
		manualDone := make(chan struct{})
		if mode == 2 {
			go func() {
				controller.Abort(manualReason)
				close(manualDone)
			}()
		} else {
			close(manualDone)
		}

		if err := runAutoExitLoop(t, loop); err != nil {
			t.Fatalf("Run: %v", err)
		}
		select {
		case <-manualDone:
		case <-time.After(fuzzLoopRunTimeout):
			t.Fatal("concurrent manual abort did not join timeout publication")
		}
		if !signal.Aborted() {
			t.Fatalf("timeout signal was not aborted")
		}
		if reasonErr, ok := signal.Reason().(error); ok {
			if got := signal.ThrowIfAborted(); got != reasonErr {
				t.Fatalf("ThrowIfAborted = %#v, want exact stored error %#v", got, reasonErr)
			}
		}
		if mode == 1 {
			if !abortReasonEquivalent(signal.Reason(), manualReason) {
				t.Fatalf("manual abort reason changed after timeout: got %#v want %#v", signal.Reason(), manualReason)
			}
		} else if mode == 0 {
			var timeoutErr *TimeoutError
			if !errors.As(signal.ThrowIfAborted(), &timeoutErr) {
				t.Fatalf("timeout ThrowIfAborted did not return TimeoutError")
			}
		} else if !abortReasonEquivalent(signal.Reason(), manualReason) {
			var timeoutErr *TimeoutError
			if !errors.As(signal.ThrowIfAborted(), &timeoutErr) {
				t.Fatalf("racing result = %#v, want manual identity or TimeoutError", signal.Reason())
			}
		}
	})
}
