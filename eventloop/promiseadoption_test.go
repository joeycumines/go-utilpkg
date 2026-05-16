package eventloop

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"
	"weak"
)

func TestChainedPromiseAdoptionClaimsResolver(t *testing.T) {
	sourceA := &ChainedPromise{}
	sourceA.state.Store(int32(Pending))
	sourceB := &ChainedPromise{}
	sourceB.state.Store(int32(Pending))
	target := &ChainedPromise{}
	target.state.Store(int32(Pending))

	target.resolve(sourceA)
	target.reject("late rejection")
	target.resolve(sourceB)
	sourceB.resolve("wrong source")
	if state := target.State(); state != Pending {
		t.Fatalf("target state before adopted source settlement = %v, want Pending", state)
	}
	sourceA.resolve("adopted value")

	select {
	case got := <-target.ToChannel():
		if got != "adopted value" {
			t.Fatalf("target result = %#v, want adopted value", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("adopted promise did not settle")
	}
	if state := target.State(); state != Fulfilled {
		t.Fatalf("target state = %v, want Fulfilled", state)
	}
}
func TestChainedPromiseCrossAdapterAdoptionUsesTargetOwner(t *testing.T) {
	sourceLoop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, sourceLoop)
	targetLoop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, targetLoop)
	sourceJS, err := NewJS(sourceLoop)
	if err != nil {
		t.Fatal(err)
	}
	targetJS, err := NewJS(targetLoop)
	if err != nil {
		t.Fatal(err)
	}
	source, resolveSource, _ := sourceJS.NewChainedPromise()
	target, resolveTarget, _ := targetJS.NewChainedPromise()
	observed := make(chan bool, 1)
	target.Then(func(any) any {
		observed <- targetLoop.isLoopThread() && !sourceLoop.isLoopThread()
		return nil
	}, nil)
	sourceRun := make(chan error, 1)
	targetRun := make(chan error, 1)
	go func() { sourceRun <- sourceLoop.Run(context.Background()) }()
	go func() { targetRun <- targetLoop.Run(context.Background()) }()

	resolveTarget(source)
	resolveSource("cross-owner")
	select {
	case correctOwner := <-observed:
		if !correctOwner {
			t.Fatal("adopted reaction did not execute on the target owner")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cross-adapter adoption did not settle")
	}
	sourceJS.adoptionsMu.Lock()
	adoptions := len(sourceJS.adoptions)
	sourceJS.adoptionsMu.Unlock()
	if adoptions != 0 {
		t.Fatalf("source pending adoption transfers = %d, want 0", adoptions)
	}
	if err := sourceLoop.Shutdown(context.Background()); err != nil {
		t.Fatalf("source Shutdown: %v", err)
	}
	if err := targetLoop.Shutdown(context.Background()); err != nil {
		t.Fatalf("target Shutdown: %v", err)
	}
	if err := waitContractValue(t, sourceRun, "source adoption Run completion"); err != nil {
		t.Fatalf("source Run: %v", err)
	}
	if err := waitContractValue(t, targetRun, "target adoption Run completion"); err != nil {
		t.Fatalf("target Run: %v", err)
	}
}

func TestChainedPromiseCrossAdapterRejectionUsesTargetOwner(t *testing.T) {
	sourceLoop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, sourceLoop)
	targetLoop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, targetLoop)
	reported := make(chan any, 2)
	sourceJS, err := NewJS(sourceLoop, WithUnhandledRejection(func(reason any) { reported <- reason }))
	if err != nil {
		t.Fatal(err)
	}
	targetJS, err := NewJS(targetLoop, WithUnhandledRejection(func(reason any) { reported <- reason }))
	if err != nil {
		t.Fatal(err)
	}
	source, _, rejectSource := sourceJS.NewChainedPromise()
	target, resolveTarget, _ := targetJS.NewChainedPromise()
	observed := make(chan struct {
		onTarget bool
		state    PromiseState
		reason   any
	}, 1)
	child := target.Catch(func(reason any) any {
		observed <- struct {
			onTarget bool
			state    PromiseState
			reason   any
		}{
			onTarget: targetLoop.isLoopThread() && !sourceLoop.isLoopThread(),
			state:    target.State(),
			reason:   target.Reason(),
		}
		return reason
	})
	sourceRun := make(chan error, 1)
	targetRun := make(chan error, 1)
	go func() { sourceRun <- sourceLoop.Run(context.Background()) }()
	go func() { targetRun <- targetLoop.Run(context.Background()) }()

	resolveTarget(source)
	rejectSource("cross-owner rejection")
	select {
	case got := <-observed:
		if !got.onTarget {
			t.Fatal("adopted rejection reaction did not execute on the target owner")
		}
		if got.state != Rejected || got.reason != "cross-owner rejection" {
			t.Fatalf("reaction observed state=%v reason=%#v", got.state, got.reason)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cross-adapter rejection did not settle")
	}
	select {
	case value := <-child.ToChannel():
		if value != "cross-owner rejection" {
			t.Fatalf("catch child value = %#v, want rejection reason", value)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cross-adapter catch child did not settle")
	}
	sourceJS.adoptionsMu.Lock()
	adoptions := len(sourceJS.adoptions)
	sourceJS.adoptionsMu.Unlock()
	if adoptions != 0 {
		t.Fatalf("source pending adoption transfers = %d, want 0", adoptions)
	}
	if err := sourceLoop.Shutdown(context.Background()); err != nil {
		t.Fatalf("source Shutdown: %v", err)
	}
	if err := targetLoop.Shutdown(context.Background()); err != nil {
		t.Fatalf("target Shutdown: %v", err)
	}
	if err := waitContractValue(t, sourceRun, "source rejection-adoption Run completion"); err != nil {
		t.Fatalf("source Run: %v", err)
	}
	if err := waitContractValue(t, targetRun, "target rejection-adoption Run completion"); err != nil {
		t.Fatalf("target Run: %v", err)
	}
	waitTerminalUnhandledRejectionTrackingDrained(t, sourceJS)
	waitTerminalUnhandledRejectionTrackingDrained(t, targetJS)
	select {
	case reason := <-reported:
		t.Fatalf("handled cross-adapter rejection was reported: %#v", reason)
	default:
	}
}

func TestChainedPromiseAdoptionAfterSourceTermination(t *testing.T) {
	sourceLoop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, sourceLoop)
	targetLoop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, targetLoop)
	sourceReported := make(chan any, 1)
	sourceJS, err := NewJS(sourceLoop, WithUnhandledRejection(func(reason any) { sourceReported <- reason }))
	if err != nil {
		t.Fatal(err)
	}
	targetJS, err := NewJS(targetLoop)
	if err != nil {
		t.Fatal(err)
	}
	source, _, rejectSource := sourceJS.NewChainedPromise()
	target, resolveTarget, _ := targetJS.NewChainedPromise()
	resolveTarget(source)
	child := target.Catch(func(reason any) any { return reason })
	if err := sourceLoop.Close(); err != nil {
		t.Fatalf("source Close: %v", err)
	}
	targetRun := make(chan error, 1)
	go func() { targetRun <- targetLoop.Run(context.Background()) }()
	waitLoopOwnerTurnT(t, targetLoop)

	rejectSource("source terminated")
	select {
	case reason := <-target.ToChannel():
		if reason != "source terminated" {
			t.Fatalf("target reason = %#v, want source terminated", reason)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("source-terminal adoption did not settle target")
	}
	if target.State() != Rejected || target.Reason() != "source terminated" {
		t.Fatalf("target state=%v reason=%#v", target.State(), target.Reason())
	}
	select {
	case value := <-child.ToChannel():
		if value != "source terminated" {
			t.Fatalf("catch child value = %#v, want source terminated", value)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("source-terminal catch child did not settle")
	}
	waitTerminalUnhandledRejectionTrackingDrained(t, sourceJS)
	select {
	case reason := <-sourceReported:
		t.Fatalf("handled source-terminal rejection was reported: %#v", reason)
	default:
	}
	if err := targetLoop.Shutdown(context.Background()); err != nil {
		t.Fatalf("target Shutdown: %v", err)
	}
	if err := waitContractValue(t, targetRun, "source-terminal adoption Run completion"); err != nil {
		t.Fatalf("target Run: %v", err)
	}
}

func TestChainedPromiseAdoptionAfterTargetTermination(t *testing.T) {
	sourceLoop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, sourceLoop)
	targetLoop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, targetLoop)
	sourceJS, err := NewJS(sourceLoop)
	if err != nil {
		t.Fatal(err)
	}
	targetJS, err := NewJS(targetLoop)
	if err != nil {
		t.Fatal(err)
	}
	source, resolveSource, _ := sourceJS.NewChainedPromise()
	target, resolveTarget, _ := targetJS.NewChainedPromise()
	resolveTarget(source)
	if err := targetLoop.Close(); err != nil {
		t.Fatalf("target Close: %v", err)
	}
	targetResult := target.ToChannel()
	sourceRun := make(chan error, 1)
	go func() { sourceRun <- sourceLoop.Run(context.Background()) }()

	resolveSource("target terminated")
	if result := waitContractValue(t, targetResult, "target-terminal adoption settlement"); result != "target terminated" {
		t.Fatalf("target result = %#v, want target terminated", result)
	}
	if target.State() != Fulfilled || target.Value() != "target terminated" {
		t.Fatalf("target state=%v value=%#v", target.State(), target.Value())
	}
	if err := sourceLoop.Shutdown(context.Background()); err != nil {
		t.Fatalf("source Shutdown: %v", err)
	}
	if err := waitContractValue(t, sourceRun, "source Run completion"); err != nil {
		t.Fatalf("source Run: %v", err)
	}
}

func TestChainedPromiseIndirectNativeCycleRemainsPending(t *testing.T) {
	first := &ChainedPromise{}
	first.state.Store(int32(Pending))
	second := &ChainedPromise{}
	second.state.Store(int32(Pending))
	first.resolve(second)
	second.resolve(first)

	if first.State() != Pending || second.State() != Pending {
		t.Fatalf("cycle states = (%v, %v), want both Pending", first.State(), second.State())
	}
	select {
	case result := <-first.ToChannel():
		t.Fatalf("indirect cycle unexpectedly settled with %#v", result)
	default:
	}
}
func TestChainedPromiseAcceptedAdoptionSurvivesImmediateClose(t *testing.T) {
	for _, mode := range []UnhandledRejectionFallbackMode{
		UnhandledRejectionFallbackIsolated,
		UnhandledRejectionFallbackDisabled,
	} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			loop, err := New()
			if err != nil {
				t.Fatal(err)
			}
			registerLoopCleanupT(t, loop)
			reported := make(chan any, 2)
			js, err := NewJS(loop,
				WithUnhandledRejection(func(reason any) { reported <- reason }),
				WithUnhandledRejectionFallback(mode),
			)
			if err != nil {
				t.Fatal(err)
			}
			source, _, rejectSource := js.NewChainedPromise()
			adopter, resolveAdopter, _ := js.NewChainedPromise()
			resolveAdopter(source)
			rejectSource("discarded adoption")
			if state := adopter.state.Load(); state != promiseSettlementClaimed {
				t.Fatalf("adopter raw state = %d, want settlement claimed", state)
			}
			if state := adopter.State(); state != Pending {
				t.Fatalf("adopter public state = %v, want Pending before Close", state)
			}

			if err := loop.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
			select {
			case got := <-adopter.ToChannel():
				if got != "discarded adoption" {
					t.Fatalf("adopter reason = %#v, want discarded adoption", got)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("accepted adoption was discarded by immediate Close")
			}
			if state := adopter.State(); state != Rejected {
				t.Fatalf("adopter state = %v, want Rejected", state)
			}
			js.adoptionsMu.Lock()
			adoptions := len(js.adoptions)
			js.adoptionsMu.Unlock()
			if adoptions != 0 {
				t.Fatalf("pending adoption transfers = %d, want 0", adoptions)
			}
			if mode == UnhandledRejectionFallbackIsolated {
				select {
				case got := <-reported:
					if got != "discarded adoption" {
						t.Fatalf("reported reason = %#v, want discarded adoption", got)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("adopter terminal diagnostic was not delivered")
				}
			}
			waitTerminalUnhandledRejectionTrackingDrained(t, js)
			if source.rejectionHandled.reported() {
				t.Fatal("handled source owned the adoption diagnostic")
			}
			if !adopter.rejectionHandled.reported() {
				t.Fatal("adopter did not own the terminal diagnostic")
			}
			select {
			case duplicate := <-reported:
				t.Fatalf("duplicate adoption diagnostic: %#v", duplicate)
			default:
			}
		})
	}
}

func TestChainedPromiseAcceptedFulfillmentAdoptionSurvivesImmediateClose(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}
	source, resolveSource, _ := js.NewChainedPromise()
	adopter, resolveAdopter, _ := js.NewChainedPromise()
	resolveAdopter(source)
	resolveSource("discarded adoption fulfillment")
	if state := adopter.state.Load(); state != promiseSettlementClaimed {
		t.Fatalf("adopter raw state = %d, want settlement claimed", state)
	}
	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case got := <-adopter.ToChannel():
		if got != "discarded adoption fulfillment" {
			t.Fatalf("adopter value = %#v, want discarded adoption fulfillment", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("accepted fulfillment adoption was discarded by immediate Close")
	}
	if state := adopter.State(); state != Fulfilled {
		t.Fatalf("adopter state = %v, want Fulfilled", state)
	}
	js.adoptionsMu.Lock()
	adoptions := len(js.adoptions)
	js.adoptionsMu.Unlock()
	if adoptions != 0 {
		t.Fatalf("pending adoption transfers = %d, want 0", adoptions)
	}
}

func TestTerminalTransitionSettlesAcceptedAdoptionNeededByRunningCallback(t *testing.T) {
	settlementCases := []struct {
		name   string
		value  any
		settle func(ResolveFunc, RejectFunc, any)
	}{
		{
			name:  "fulfilled",
			value: "adopted value",
			settle: func(resolve ResolveFunc, _ RejectFunc, value any) {
				resolve(value)
			},
		},
		{
			name:  "rejected",
			value: "adopted reason",
			settle: func(_ ResolveFunc, reject RejectFunc, value any) {
				reject(value)
			},
		},
	}
	terminationCases := []struct {
		name      string
		terminate func(*Loop) error
	}{
		{name: "close", terminate: func(loop *Loop) error { return loop.Close() }},
		{name: "shutdown", terminate: func(loop *Loop) error { return loop.Shutdown(context.Background()) }},
	}

	for _, terminationCase := range terminationCases {
		for _, settlementCase := range settlementCases {
			t.Run(terminationCase.name+"/"+settlementCase.name, func(t *testing.T) {
				loop, err := New()
				if err != nil {
					t.Fatal(err)
				}
				registerLoopCleanupT(t, loop)
				js, err := NewJS(loop, WithUnhandledRejectionFallback(UnhandledRejectionFallbackDisabled))
				if err != nil {
					t.Fatal(err)
				}
				callbackStarted := make(chan struct{})
				callbackResult := make(chan any, 1)
				releaseCallback := make(chan struct{})
				if err := loop.Submit(func() {
					source, resolveSource, rejectSource := js.NewChainedPromise()
					target, resolveTarget, _ := js.NewChainedPromise()
					resolveTarget(source)
					settlementCase.settle(resolveSource, rejectSource, settlementCase.value)
					close(callbackStarted)
					select {
					case value := <-target.ToChannel():
						callbackResult <- value
					case <-releaseCallback:
						callbackResult <- releaseCallback
					}
				}); err != nil {
					t.Fatal(err)
				}

				runDone := make(chan error, 1)
				go func() { runDone <- loop.Run(context.Background()) }()
				waitContractSignal(t, callbackStarted, "settled adoption callback entry")
				terminalDone := make(chan error, 1)
				go func() { terminalDone <- terminationCase.terminate(loop) }()

				select {
				case value := <-callbackResult:
					if value != settlementCase.value {
						t.Fatalf("adoption result = %#v, want %#v", value, settlementCase.value)
					}
				case <-time.After(2 * time.Second):
					close(releaseCallback)
					<-callbackResult
					<-runDone
					<-terminalDone
					t.Fatalf("%s waited for loopDone before recovering the %s adoption needed by the running callback", terminationCase.name, settlementCase.name)
				}
				if err := waitContractValue(t, runDone, "Run after adoption dependency settlement"); err != nil {
					t.Fatalf("Run: %v", err)
				}
				if err := waitContractValue(t, terminalDone, terminationCase.name+" after adoption dependency settlement"); err != nil {
					t.Fatalf("%s: %v", terminationCase.name, err)
				}
			})
		}
	}
}

func TestChainedPromisePendingAdoptionDoesNotRetainAbandonedPromises(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	const total = 256
	sourceRefs := make([]weak.Pointer[ChainedPromise], total)
	adopterRefs := make([]weak.Pointer[ChainedPromise], total)
	for i := range total {
		sourceRefs[i], adopterRefs[i] = abandonPendingAdoption(js)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		js.adoptionsMu.Lock()
		adoptions := len(js.adoptions)
		js.adoptionsMu.Unlock()
		sourceAlive := 0
		adopterAlive := 0
		for i := range total {
			if sourceRefs[i].Value() != nil {
				sourceAlive++
			}
			if adopterRefs[i].Value() != nil {
				adopterAlive++
			}
		}
		if sourceAlive == 0 && adopterAlive == 0 && adoptions == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("abandoned pending adoptions retained: sources=%d adopters=%d transfers=%d",
				sourceAlive, adopterAlive, adoptions)
		}
		runtime.GC()
		runtime.Gosched()
	}
}

func abandonPendingAdoption(js *JS) (weak.Pointer[ChainedPromise], weak.Pointer[ChainedPromise]) {
	source, _, _ := js.NewChainedPromise()
	adopter, resolveAdopter, _ := js.NewChainedPromise()
	resolveAdopter(source)
	sourceRef := weak.Make(source)
	adopterRef := weak.Make(adopter)
	runtime.KeepAlive(source)
	runtime.KeepAlive(adopter)
	return sourceRef, adopterRef
}

func TestChainedPromiseCrossAdapterPendingAdoptionDoesNotRetainSourceOwner(t *testing.T) {
	targetLoop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, targetLoop)
	targetJS, err := NewJS(targetLoop)
	if err != nil {
		t.Fatal(err)
	}

	target, sourceRef, sourceJSRef, sourceLoopRef := abandonCrossAdapterPendingAdoption(t, targetJS)
	deadline := time.Now().Add(5 * time.Second)
	for {
		sourceAlive := sourceRef.Value() != nil
		sourceJSAlive := sourceJSRef.Value() != nil
		sourceLoopAlive := sourceLoopRef.Value() != nil
		if !sourceAlive && !sourceJSAlive && !sourceLoopAlive {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("foreign target retained source owner: source=%t js=%t loop=%t",
				sourceAlive, sourceJSAlive, sourceLoopAlive)
		}
		if state := target.State(); state != Pending {
			t.Fatalf("target state while source owner is collected = %v, want Pending", state)
		}
		runtime.GC()
		runtime.Gosched()
	}
	if state := target.State(); state != Pending {
		t.Fatalf("target state after source owner collection = %v, want Pending", state)
	}
	runtime.KeepAlive(targetJS)
}

func abandonCrossAdapterPendingAdoption(
	t *testing.T,
	targetJS *JS,
) (*ChainedPromise, weak.Pointer[ChainedPromise], weak.Pointer[JS], weak.Pointer[Loop]) {
	t.Helper()
	sourceLoop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	sourceJS, err := NewJS(sourceLoop)
	if err != nil {
		t.Fatal(err)
	}
	source, _, _ := sourceJS.NewChainedPromise()
	target, resolveTarget, _ := targetJS.NewChainedPromise()
	resolveTarget(source)
	sourceRef := weak.Make(source)
	sourceJSRef := weak.Make(sourceJS)
	sourceLoopRef := weak.Make(sourceLoop)
	if err := sourceLoop.Close(); err != nil {
		t.Fatalf("source Close: %v", err)
	}
	runtime.KeepAlive(source)
	runtime.KeepAlive(sourceJS)
	runtime.KeepAlive(sourceLoop)
	return target, sourceRef, sourceJSRef, sourceLoopRef
}

func TestChainedPromiseCrossAdapterAcceptedAdoptionSurvivesSourceClose(t *testing.T) {
	sourceLoop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, sourceLoop)
	targetLoop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, targetLoop)
	sourceReported := make(chan any, 1)
	targetReported := make(chan any, 2)
	sourceJS, err := NewJS(sourceLoop, WithUnhandledRejection(func(reason any) { sourceReported <- reason }))
	if err != nil {
		t.Fatal(err)
	}
	targetJS, err := NewJS(targetLoop, WithUnhandledRejection(func(reason any) { targetReported <- reason }))
	if err != nil {
		t.Fatal(err)
	}
	source, _, rejectSource := sourceJS.NewChainedPromise()
	adopter, resolveAdopter, _ := targetJS.NewChainedPromise()
	resolveAdopter(source)
	rejectSource("cross-owner discarded adoption")
	if state := adopter.state.Load(); state != promiseSettlementClaimed {
		t.Fatalf("adopter raw state = %d, want settlement claimed", state)
	}

	targetRun := make(chan error, 1)
	go func() { targetRun <- targetLoop.Run(context.Background()) }()
	waitLoopOwnerTurnT(t, targetLoop)
	if err := sourceLoop.Close(); err != nil {
		t.Fatalf("source Close: %v", err)
	}
	select {
	case got := <-adopter.ToChannel():
		if got != "cross-owner discarded adoption" {
			t.Fatalf("adopter reason = %#v, want cross-owner discarded adoption", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("source Close discarded the cross-owner adoption")
	}
	select {
	case got := <-targetReported:
		if got != "cross-owner discarded adoption" {
			t.Fatalf("target report = %#v, want cross-owner discarded adoption", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("target owner did not report the recovered adoption")
	}
	if err := targetLoop.Shutdown(context.Background()); err != nil {
		t.Fatalf("target Shutdown: %v", err)
	}
	if err := waitContractValue(t, targetRun, "source-close adoption Run completion"); err != nil {
		t.Fatalf("target Run: %v", err)
	}
	waitTerminalUnhandledRejectionTrackingDrained(t, sourceJS)
	waitTerminalUnhandledRejectionTrackingDrained(t, targetJS)
	sourceJS.adoptionsMu.Lock()
	adoptions := len(sourceJS.adoptions)
	sourceJS.adoptionsMu.Unlock()
	if adoptions != 0 {
		t.Fatalf("source pending adoption transfers = %d, want 0", adoptions)
	}
	if source.rejectionHandled.reported() {
		t.Fatal("source owner reported the handled adoption")
	}
	if !adopter.rejectionHandled.reported() {
		t.Fatal("cross-owner adopter did not own the report")
	}
	select {
	case duplicate := <-sourceReported:
		t.Fatalf("source owner produced adoption diagnostic: %#v", duplicate)
	default:
	}
	select {
	case duplicate := <-targetReported:
		t.Fatalf("target owner produced duplicate diagnostic: %#v", duplicate)
	default:
	}
}
