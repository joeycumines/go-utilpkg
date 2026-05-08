package gojagrpc

import (
	"errors"
	"fmt"
	"runtime"
	"testing"
	"time"
)

type serverRetentionSnapshot struct {
	callbackEffects int
	controlSlots    int
	disposals       int
	effects         int
	fences          int
	ownerRoots      int
	registrations   int
	serverPlans     int
	tombstones      int
}

func captureServerRetention(module *Module) serverRetentionSnapshot {
	return serverRetentionSnapshot{
		callbackEffects: syncMapSize(&module.owner.callbackEffects),
		controlSlots:    syncMapSize(&module.executor.slots),
		disposals:       len(module.owner.disposals),
		effects:         syncMapSize(&module.owner.effects),
		fences:          syncMapSize(&module.owner.fences),
		ownerRoots:      len(module.owner.roots),
		registrations: supervisorKindCount(
			module,
			supervisorServerRegistration,
		),
		serverPlans: len(module.owner.serverPlans),
		tombstones:  len(module.owner.tombstones),
	}
}

func assertServerRetention(
	t *testing.T,
	got serverRetentionSnapshot,
	want serverRetentionSnapshot,
) {
	t.Helper()
	if got != want {
		t.Fatalf("server retention = %+v, want %+v", got, want)
	}
}

func allocateTestServerPlan(
	module *Module,
	admission *serverRegistrationAdmission,
) error {
	id, err := module.allocateServerMethodPlan(new(serverMethodPlan))
	if err != nil {
		return err
	}
	admission.plans = append(admission.plans, id)
	return nil
}

func TestServerRegistrationAdmissionNonreturnRollsBack(t *testing.T) {
	testErr := errors.New("test registration failure")
	panicValue := "test registration panic"
	tests := []struct {
		name string
		run  func(*Module, *serverRegistrationAdmission) error
	}{
		{
			name: "error",
			run: func(
				_ *Module,
				_ *serverRegistrationAdmission,
			) error {
				return testErr
			},
		},
		{
			name: "panic",
			run: func(
				_ *Module,
				_ *serverRegistrationAdmission,
			) error {
				panic(panicValue)
			},
		},
		{
			name: "Goexit",
			run: func(
				_ *Module,
				_ *serverRegistrationAdmission,
			) error {
				runtime.Goexit()
				return nil
			},
		},
		{
			name: "idempotent",
			run: func(
				module *Module,
				admission *serverRegistrationAdmission,
			) error {
				module.rollbackServerRegistrationOwner(admission)
				return testErr
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			env := newGrpcTestEnv(t)
			defer env.shutdown()

			before := captureServerRetention(env.grpcMod)
			returned := make(chan error, 1)
			panicResult := make(chan any, 1)
			done := make(chan struct{})
			go func() {
				defer close(done)
				defer func() {
					panicResult <- recover()
				}()
				err := env.grpcMod.admitServerRegistration(
					func(admission *serverRegistrationAdmission) error {
						if err := allocateTestServerPlan(
							env.grpcMod,
							admission,
						); err != nil {
							return err
						}
						return test.run(env.grpcMod, admission)
					},
				)
				returned <- err
			}()
			select {
			case <-done:
			case <-time.After(defaultTimeout):
				t.Fatal("nonreturning registration did not unwind")
			}
			reason := <-panicResult
			switch test.name {
			case "panic":
				if fmt.Sprint(reason) != panicValue {
					t.Fatalf("panic = %#v, want %q", reason, panicValue)
				}
			case "Goexit":
				if reason != nil {
					t.Fatalf("Goexit recovered reason = %#v, want nil", reason)
				}
				select {
				case err := <-returned:
					t.Fatalf("Goexit returned %v", err)
				default:
				}
			default:
				if reason != nil {
					t.Fatalf("registration panic = %#v, want nil", reason)
				}
				select {
				case err := <-returned:
					if !errors.Is(err, testErr) {
						t.Fatalf("registration error = %v, want %v", err, testErr)
					}
				default:
					t.Fatal("registration did not return an error")
				}
			}
			assertServerRetention(
				t,
				captureServerRetention(env.grpcMod),
				before,
			)
		})
	}
}

func TestServerRegistrationNonreturnCloseRace(t *testing.T) {
	const panicValue = "registration boundary panic"
	tests := []struct {
		name string
		exit func()
	}{
		{
			name: "panic",
			exit: func() { panic(panicValue) },
		},
		{
			name: "Goexit",
			exit: runtime.Goexit,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			env := newGrpcTestEnv(t)
			defer env.shutdown()
			before := captureServerRetention(env.grpcMod)

			entered := make(chan struct{})
			release := make(chan struct{})
			type result struct {
				panicValue any
				returned   bool
				err        error
			}
			admissionDone := make(chan result, 1)
			go func() {
				current := result{}
				defer func() {
					current.panicValue = recover()
					admissionDone <- current
				}()
				current.err = env.grpcMod.admitServerRegistration(
					func(admission *serverRegistrationAdmission) error {
						if err := allocateTestServerPlan(
							env.grpcMod,
							admission,
						); err != nil {
							return err
						}
						close(entered)
						<-release
						test.exit()
						return nil
					},
				)
				current.returned = true
			}()
			select {
			case <-entered:
			case <-time.After(defaultTimeout):
				t.Fatal("registration did not enter the compound boundary")
			}

			boundaryContended := make(chan bool, 1)
			closeDone := make(chan error, 1)
			go func() {
				acquired := env.grpcMod.control.boundaryMu.TryLock()
				if acquired {
					env.grpcMod.control.boundaryMu.Unlock()
				}
				boundaryContended <- !acquired
				closeDone <- env.grpcMod.Close()
			}()
			if contended := <-boundaryContended; !contended {
				t.Fatal("registration did not retain the close boundary")
			}
			runtime.Gosched()
			select {
			case err := <-closeDone:
				t.Fatalf("Close returned inside registration boundary: %v", err)
			default:
			}
			close(release)

			var got result
			select {
			case got = <-admissionDone:
			case <-time.After(defaultTimeout):
				t.Fatal("nonreturning registration did not unwind")
			}
			switch test.name {
			case "panic":
				if got.panicValue != panicValue {
					t.Fatalf("registration panic = %#v, want %q", got.panicValue, panicValue)
				}
			case "Goexit":
				if got.panicValue != nil {
					t.Fatalf("Goexit recovered panic = %#v, want nil", got.panicValue)
				}
			}
			if got.returned {
				t.Fatalf("%s registration returned", test.name)
			}
			if got.err != nil {
				t.Fatalf("%s registration error = %v, want no return", test.name, got.err)
			}
			select {
			case err := <-closeDone:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(defaultTimeout):
				t.Fatal("Close did not complete after registration rollback")
			}
			assertServerRetention(t, captureServerRetention(env.grpcMod), before)
		})
	}
}
