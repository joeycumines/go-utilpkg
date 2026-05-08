package eventloop

import (
	"runtime"
	"testing"
	"time"
)

func TestPromiseCombinatorsCrossLoopSettlementClaimArbitration(t *testing.T) {
	t.Run("normal claim wins", func(t *testing.T) {
		for _, combinator := range promiseCombinatorTestCases() {
			if combinator.name == "AllSettled" {
				continue
			}
			t.Run(combinator.name, func(t *testing.T) {
				reported := make(chan any, 4)
				targetLoop, targetJS := newCombinatorTestAdapter(t, reported)
				normalLoop, normalJS := newCombinatorTestAdapter(t, reported)
				terminalLoop, terminalJS := newCombinatorTestAdapter(t, reported)
				normalSource, resolveNormal, rejectNormal := normalJS.NewChainedPromise()
				terminalSource, resolveTerminal, _ := terminalJS.NewChainedPromise()
				result := combinator.combine(targetJS, []*ChainedPromise{normalSource, terminalSource})
				resultChannel := result.ToChannel()
				result.rejectionHandled.Store(true)
				resolveTerminal("terminal source")

				result.mu.Lock()
				if combinator.name == "All" {
					rejectNormal("normal rejection")
				} else {
					resolveNormal("normal winner")
				}
				normalDone := make(chan struct{})
				go func() {
					normalLoop.tick()
					close(normalDone)
				}()
				waitPromiseSettlementClaimed(t, result)
				if err := terminalLoop.Close(); err != nil {
					t.Fatal(err)
				}
				result.mu.Unlock()
				waitContractSignal(t, normalDone, "normal aggregate settlement publication")

				if combinator.name == "All" {
					if state := result.State(); state != Rejected || result.Reason() != "normal rejection" {
						t.Fatalf("normal All claim lost: state=%v reason=%v", state, result.Reason())
					}
					assertSinglePromiseChannelValue(t, resultChannel, "normal rejection")
				} else {
					if state := result.State(); state != Fulfilled || result.Value() != "normal winner" {
						t.Fatalf("normal %s claim lost: state=%v value=%v", combinator.name, state, result.Value())
					}
					assertSinglePromiseChannelValue(t, resultChannel, "normal winner")
				}
				targetLoop.tick()
				assertUnhandledRejectionTrackingDrained(t, targetJS)
				assertUnhandledRejectionTrackingDrained(t, normalJS)
				waitTerminalUnhandledRejectionTrackingDrained(t, terminalJS)
				select {
				case orphan := <-reported:
					t.Fatalf("normal claim arbitration reported rejection: %v", orphan)
				default:
				}
			})
		}
	})

	t.Run("terminal claim wins", func(t *testing.T) {
		for _, combinator := range promiseCombinatorTestCases() {
			t.Run(combinator.name, func(t *testing.T) {
				reported := make(chan any, 4)
				targetLoop, targetJS := newCombinatorTestAdapter(t, reported)
				normalLoop, normalJS := newCombinatorTestAdapter(t, reported)
				terminalLoop, terminalJS := newCombinatorTestAdapter(t, reported)
				normalSource, resolveNormal, rejectNormal := normalJS.NewChainedPromise()
				terminalSource, resolveTerminal, _ := terminalJS.NewChainedPromise()
				result := combinator.combine(targetJS, []*ChainedPromise{normalSource, terminalSource})
				resultChannel := result.ToChannel()
				result.rejectionHandled.Store(true)
				resolveTerminal("terminal source")
				if combinator.name == "All" {
					rejectNormal("normal rejection")
				} else {
					resolveNormal("normal value")
				}

				result.mu.Lock()
				closeDone := make(chan error, 1)
				go func() { closeDone <- terminalLoop.Close() }()
				waitPromiseSettlementClaimed(t, result)
				normalDone := make(chan struct{})
				go func() {
					normalLoop.tick()
					close(normalDone)
				}()
				waitContractSignal(t, normalDone, "losing normal aggregate reaction")
				result.mu.Unlock()
				if err := waitContractValue(t, closeDone, "terminal aggregate settlement publication"); err != nil {
					t.Fatalf("Close: %v", err)
				}

				assertTerminalCombinatorRejection(t, result, resultChannel)
				targetLoop.tick()
				assertUnhandledRejectionTrackingDrained(t, targetJS)
				assertUnhandledRejectionTrackingDrained(t, normalJS)
				waitTerminalUnhandledRejectionTrackingDrained(t, terminalJS)
				select {
				case orphan := <-reported:
					t.Fatalf("terminal claim arbitration reported private rejection: %v", orphan)
				default:
				}
			})
		}
	})
}

func waitPromiseSettlementClaimed(t *testing.T, promise *ChainedPromise) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for promise.state.Load() != promiseSettlementClaimed {
		select {
		case <-deadline.C:
			t.Fatal("timed out waiting for Promise settlement claim")
		default:
			runtime.Gosched()
		}
	}
}
