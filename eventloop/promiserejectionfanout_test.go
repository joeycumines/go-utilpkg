package eventloop

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestChainedPromiseTerminalScheduleFailureTransfersReportBeforeChildReject(t *testing.T) {
	for _, mode := range []UnhandledRejectionFallbackMode{
		UnhandledRejectionFallbackIsolated,
		UnhandledRejectionFallbackDisabled,
	} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			loop := New()
			registerLoopCleanupT(t, loop)
			reported := make(chan any, 2)
			js := NewJS(loop,
				WithUnhandledRejection(func(reason any) { reported <- reason }),
				WithUnhandledRejectionFallback(mode),
			)
			parent, _, rejectParent := js.NewChainedPromise()
			child := parent.Then(nil, nil)
			if err := loop.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
			rejectParent("terminal transfer")
			if child.State() != Rejected {
				t.Fatalf("child state = %v, want Rejected", child.State())
			}
			js.rejectionsMu.RLock()
			_, parentTracked := js.unhandledRejections[parent]
			js.rejectionsMu.RUnlock()
			if parentTracked {
				t.Fatal("parent remained tracked after report ownership transferred")
			}
			if mode == UnhandledRejectionFallbackIsolated {
				select {
				case reason := <-reported:
					if reason != "terminal transfer" {
						t.Fatalf("reported reason = %#v, want terminal transfer", reason)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("terminal child rejection was not reported")
				}
				select {
				case reason := <-reported:
					t.Fatalf("terminal parent and child were both reported: %#v", reason)
				default:
				}
			}
		})
	}
}

func TestChainedPromiseFanoutReportOwnership(t *testing.T) {
	tests := []struct {
		name         string
		adopters     bool
		terminal     bool
		handledFirst bool
	}{
		{name: "branches-first-handled", handledFirst: true},
		{name: "branches-second-handled"},
		{name: "adopters-first-handled", adopters: true, handledFirst: true},
		{name: "adopters-second-handled", adopters: true},
		{name: "terminal-first-handled", terminal: true, handledFirst: true},
		{name: "terminal-second-handled", terminal: true},
		{name: "terminal-adopters-first-handled", adopters: true, terminal: true, handledFirst: true},
		{name: "terminal-adopters-second-handled", adopters: true, terminal: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop := New()
			registerLoopCleanupT(t, loop)
			reported := make(chan any, 4)
			js := NewJS(loop,
				WithUnhandledRejection(func(reason any) { reported <- reason }),
				WithUnhandledRejectionFallback(UnhandledRejectionFallbackIsolated),
			)
			reason := "fanout " + test.name
			source, _, rejectSource := js.NewChainedPromise()
			var first, second *ChainedPromise
			if test.adopters {
				var resolveFirst, resolveSecond ResolveFunc
				first, resolveFirst, _ = js.NewChainedPromise()
				second, resolveSecond, _ = js.NewChainedPromise()
				resolveFirst(source)
				resolveSecond(source)
				for index, promise := range []*ChainedPromise{first, second} {
					if state := promise.state.Load(); state != promiseSettlementClaimed {
						t.Fatalf("adopter %d raw state = %d, want settlement claimed", index, state)
					}
					if state := promise.State(); state != Pending {
						t.Fatalf("adopter %d public state = %v, want Pending", index, state)
					}
				}
			} else {
				first = source.Then(nil, nil)
				second = source.Then(nil, nil)
			}

			handled := second
			if test.handledFirst {
				handled = first
			}
			unhandled := first
			if test.handledFirst {
				unhandled = second
			}
			handledDone := make(chan struct{}, 1)
			if test.terminal {
				// A user handler cannot run after terminal scheduling closes. Mark the
				// selected sibling directly so this case isolates propagation ownership
				// without creating a Catch child that must reject on schedule failure.
				handled.rejectionHandled.Store(true)
				if err := loop.Close(); err != nil {
					t.Fatalf("Close: %v", err)
				}
			} else {
				handled.Catch(func(any) any {
					handledDone <- struct{}{}
					return nil
				})
			}

			var runDone chan error
			if !test.terminal {
				runDone = make(chan error, 1)
				go func() { runDone <- loop.Run(context.Background()) }()
				waitLoopOwnerTurnT(t, loop)
			}
			rejectSource(reason)

			for index, promise := range []*ChainedPromise{first, second} {
				select {
				case got := <-promise.ToChannel():
					if got != reason {
						t.Fatalf("sibling %d reason = %#v, want %q", index, got, reason)
					}
				case <-time.After(5 * time.Second):
					t.Fatalf("sibling %d did not settle", index)
				}
				if promise.State() != Rejected {
					t.Fatalf("sibling %d state = %v, want Rejected", index, promise.State())
				}
			}
			if !test.terminal {
				select {
				case <-handledDone:
				case <-time.After(5 * time.Second):
					t.Fatal("handled sibling callback did not run")
				}
			}
			select {
			case got := <-reported:
				if got != reason {
					t.Fatalf("reported reason = %#v, want %q", got, reason)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("unhandled sibling rejection was not reported")
			}

			if !test.terminal {
				if err := loop.Shutdown(context.Background()); err != nil {
					t.Fatalf("Shutdown: %v", err)
				}
				if err := waitContractValue(t, runDone, "fanout ownership Run completion"); err != nil {
					t.Fatalf("Run: %v", err)
				}
			}
			waitTerminalUnhandledRejectionTrackingDrained(t, js)
			if source.rejectionHandled.reported() {
				t.Fatal("source owned the report instead of a fanout descendant")
			}
			if !unhandled.rejectionHandled.reported() {
				t.Fatal("unhandled sibling did not own the report")
			}
			if handled.rejectionHandled.reported() {
				t.Fatal("handled sibling was marked reported")
			}
			select {
			case duplicate := <-reported:
				t.Fatalf("duplicate fanout report: %#v", duplicate)
			default:
			}
		})
	}
}

func TestChainedPromiseFanoutPropagationBeatsActiveChecker(t *testing.T) {
	tests := []struct {
		name         string
		adopters     bool
		handledFirst bool
	}{
		{name: "branches-first-handled", handledFirst: true},
		{name: "branches-second-handled"},
		{name: "adopters-first-handled", adopters: true, handledFirst: true},
		{name: "adopters-second-handled", adopters: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop := New()
			registerLoopCleanupT(t, loop)
			reported := make(chan any, 4)
			js := NewJS(loop, WithUnhandledRejection(func(reason any) { reported <- reason }))
			reason := "checker race " + test.name
			source, _, rejectSource := js.NewChainedPromise()
			rejectSource(reason)

			checkerAtSource := make(chan struct{})
			releaseChecker := make(chan struct{})
			release := releaseSignalT(t, releaseChecker)
			loop.testHooks = &loopTestHooks{
				BeforeUnhandledRejectionRecordCheck: func(promise *ChainedPromise) {
					if promise != source {
						return
					}
					select {
					case <-checkerAtSource:
					default:
						close(checkerAtSource)
					}
					<-releaseChecker
				},
			}
			checkerDone := make(chan struct{})
			go func() {
				js.checkUnhandledRejections()
				close(checkerDone)
			}()
			select {
			case <-checkerAtSource:
			case <-time.After(5 * time.Second):
				t.Fatal("checker did not snapshot source")
			}

			var first, second *ChainedPromise
			if test.adopters {
				var resolveFirst, resolveSecond ResolveFunc
				first, resolveFirst, _ = js.NewChainedPromise()
				second, resolveSecond, _ = js.NewChainedPromise()
				resolveFirst(source)
				resolveSecond(source)
			} else {
				first = source.Then(nil, nil)
				second = source.Then(nil, nil)
			}
			handled, unhandled := second, first
			if test.handledFirst {
				handled, unhandled = first, second
			}
			handledDone := make(chan struct{}, 1)
			handled.Catch(func(any) any {
				handledDone <- struct{}{}
				return nil
			})

			release()
			select {
			case <-checkerDone:
			case <-time.After(5 * time.Second):
				t.Fatal("checker did not yield to propagation")
			}
			select {
			case premature := <-reported:
				t.Fatalf("checker reported source before propagation: %#v", premature)
			default:
			}

			runDone := make(chan error, 1)
			go func() { runDone <- loop.Run(context.Background()) }()
			for index, promise := range []*ChainedPromise{first, second} {
				select {
				case got := <-promise.ToChannel():
					if got != reason {
						t.Fatalf("sibling %d reason = %#v, want %q", index, got, reason)
					}
				case <-time.After(5 * time.Second):
					t.Fatalf("sibling %d did not settle", index)
				}
			}
			select {
			case <-handledDone:
			case <-time.After(5 * time.Second):
				t.Fatal("handled sibling callback did not run")
			}
			select {
			case got := <-reported:
				if got != reason {
					t.Fatalf("reported reason = %#v, want %q", got, reason)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("unhandled sibling rejection was not reported")
			}
			if err := loop.Shutdown(context.Background()); err != nil {
				t.Fatalf("Shutdown: %v", err)
			}
			if err := waitContractValue(t, runDone, "fanout propagation Run completion"); err != nil {
				t.Fatalf("Run: %v", err)
			}
			waitTerminalUnhandledRejectionTrackingDrained(t, js)
			if source.rejectionHandled.reported() {
				t.Fatal("source checker won despite published propagation")
			}
			if !unhandled.rejectionHandled.reported() {
				t.Fatal("unhandled sibling did not own the report")
			}
			if handled.rejectionHandled.reported() {
				t.Fatal("handled sibling was marked reported")
			}
			select {
			case duplicate := <-reported:
				t.Fatalf("duplicate checker-race report: %#v", duplicate)
			default:
			}
		})
	}
}

func TestChainedPromiseFanoutCheckerWinsBeforeLateDescendants(t *testing.T) {
	for _, test := range []struct {
		name     string
		adopters bool
	}{
		{name: "branches"},
		{name: "adopters", adopters: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			loop := New()
			registerLoopCleanupT(t, loop)
			reported := make(chan any, 4)
			js := NewJS(loop, WithUnhandledRejection(func(reason any) { reported <- reason }))
			source, _, rejectSource := js.NewChainedPromise()
			rejectSource("checker owns fanout")
			checkerClaimed := make(chan struct{})
			releaseCallback := make(chan struct{})
			release := releaseSignalT(t, releaseCallback)
			loop.testHooks = &loopTestHooks{
				BeforeUnhandledRejectionCallback: func() {
					select {
					case <-checkerClaimed:
					default:
						close(checkerClaimed)
					}
					<-releaseCallback
				},
			}
			checkerDone := make(chan struct{})
			go func() {
				js.checkUnhandledRejections()
				close(checkerDone)
			}()
			select {
			case <-checkerClaimed:
			case <-time.After(5 * time.Second):
				t.Fatal("checker did not claim source report")
			}

			var first, second *ChainedPromise
			if test.adopters {
				var resolveFirst, resolveSecond ResolveFunc
				first, resolveFirst, _ = js.NewChainedPromise()
				second, resolveSecond, _ = js.NewChainedPromise()
				resolveFirst(source)
				resolveSecond(source)
			} else {
				first = source.Then(nil, nil)
				second = source.Then(nil, nil)
			}
			for index, promise := range []*ChainedPromise{first, second} {
				wantRawState := int32(Pending)
				if test.adopters {
					wantRawState = promiseSettlementClaimed
				}
				if state := promise.state.Load(); state != wantRawState {
					t.Fatalf("descendant %d raw state = %d, want %d", index, state, wantRawState)
				}
				if state := promise.State(); state != Pending {
					t.Fatalf("descendant %d public state = %v, want Pending", index, state)
				}
			}

			release()
			select {
			case <-checkerDone:
			case <-time.After(5 * time.Second):
				t.Fatal("checker did not finish source report")
			}
			select {
			case got := <-reported:
				if got != "checker owns fanout" {
					t.Fatalf("source report = %#v", got)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("source report missing")
			}

			runDone := make(chan error, 1)
			go func() { runDone <- loop.Run(context.Background()) }()
			for index, promise := range []*ChainedPromise{first, second} {
				select {
				case got := <-promise.ToChannel():
					if got != "checker owns fanout" {
						t.Fatalf("descendant %d reason = %#v", index, got)
					}
				case <-time.After(5 * time.Second):
					t.Fatalf("descendant %d did not settle", index)
				}
				if state := promise.State(); state != Rejected {
					t.Fatalf("descendant %d state = %v, want Rejected", index, state)
				}
			}
			if err := loop.Shutdown(context.Background()); err != nil {
				t.Fatalf("Shutdown: %v", err)
			}
			if err := waitContractValue(t, runDone, "checker-owned fanout Run completion"); err != nil {
				t.Fatalf("Run: %v", err)
			}
			waitTerminalUnhandledRejectionTrackingDrained(t, js)
			if !source.rejectionHandled.reported() {
				t.Fatal("source was not marked reported by checker")
			}
			if !first.rejectionHandled.reported() || !second.rejectionHandled.reported() {
				t.Fatal("checker-owned descendants were not suppressed")
			}
			select {
			case duplicate := <-reported:
				t.Fatalf("checker-owned descendants produced report: %#v", duplicate)
			default:
			}
		})
	}
}

func TestChainedPromiseRejectedCheckerAdmissionLateDescendantReportsExactlyOnce(t *testing.T) {
	for _, test := range []struct {
		name    string
		adopter bool
	}{
		{name: "branch"},
		{name: "adopter", adopter: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			loop := New()
			registerLoopCleanupT(t, loop)
			reported := make(chan any, 4)
			js := NewJS(loop,
				WithUnhandledRejection(func(reason any) { reported <- reason }),
				WithUnhandledRejectionFallback(UnhandledRejectionFallbackIsolated),
			)

			admissionReached := make(chan struct{})
			releaseAdmission := make(chan struct{})
			releaseAdmissionFn := releaseSignalT(t, releaseAdmission)
			checkCleared := make(chan struct{})
			releaseCheck := make(chan struct{})
			releaseCheckFn := releaseSignalT(t, releaseCheck)
			closeTransitioned := make(chan struct{})
			var admissionOnce sync.Once
			var checkOnce sync.Once
			loop.testHooks = &loopTestHooks{
				BeforeUnhandledRejectionCallback: func() {
					admissionOnce.Do(func() {
						close(admissionReached)
						<-releaseAdmission
					})
				},
				AfterUnhandledRejectionCheckClear: func() {
					checkOnce.Do(func() {
						close(checkCleared)
						<-releaseCheck
					})
				},
				AfterCloseStateTerminating: func() { close(closeTransitioned) },
			}

			reason := "rejected checker admission " + test.name
			source, _, rejectSource := js.NewChainedPromise()
			rejectSource(reason)
			runDone := make(chan error, 1)
			go func() { runDone <- loop.Run(context.Background()) }()
			select {
			case <-admissionReached:
			case <-time.After(5 * time.Second):
				t.Fatal("normal checker did not reach callback admission")
			}

			closeDone := make(chan error, 1)
			go func() { closeDone <- loop.Close() }()
			waitContractSignal(t, closeTransitioned, "Close StateTerminating publication")
			releaseAdmissionFn()
			select {
			case <-checkCleared:
			case <-time.After(5 * time.Second):
				t.Fatal("normal checker did not pause after rejected callback admission")
			}
			select {
			case got := <-reported:
				t.Fatalf("diagnostic callback started despite rejected admission: %#v", got)
			default:
			}

			var descendant *ChainedPromise
			if test.adopter {
				var resolveDescendant ResolveFunc
				descendant, resolveDescendant, _ = js.NewChainedPromise()
				resolveDescendant(source)
			} else {
				descendant = source.Then(nil, nil)
			}
			releaseCheckFn()

			select {
			case got := <-descendant.ToChannel():
				if got != reason {
					t.Fatalf("descendant reason = %#v, want %q", got, reason)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("late descendant did not settle")
			}
			if state := descendant.State(); state != Rejected {
				t.Fatalf("descendant state = %v, want Rejected", state)
			}
			select {
			case err := <-closeDone:
				if err != nil {
					t.Fatalf("Close: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Close did not finish after fallback handoff")
			}
			select {
			case err := <-runDone:
				if err != nil {
					t.Fatalf("Run: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Run did not finish after Close")
			}
			select {
			case got := <-reported:
				if got != reason {
					t.Fatalf("reported reason = %#v, want %q", got, reason)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("rejected normal admission and late descendant lost the diagnostic")
			}
			waitTerminalUnhandledRejectionTrackingDrained(t, js)
			if !source.rejectionHandled.reported() {
				t.Fatal("source did not retain checker report ownership")
			}
			if !descendant.rejectionHandled.reported() {
				t.Fatal("checker-owned descendant was not suppressed")
			}
			select {
			case duplicate := <-reported:
				t.Fatalf("duplicate handoff diagnostic: %#v", duplicate)
			default:
			}
		})
	}
}
