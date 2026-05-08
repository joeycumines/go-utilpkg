package eventloop

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestPromiseCombinatorsRejectTerminalReactionScheduleFailure(t *testing.T) {
	for _, combinator := range promiseCombinatorTestCases() {
		t.Run(combinator.name, func(t *testing.T) {
			for _, attachment := range []struct {
				name       string
				afterClose bool
			}{
				{name: "attached before Close"},
				{name: "attached after Close", afterClose: true},
			} {
				t.Run(attachment.name, func(t *testing.T) {
					for _, settlement := range []struct {
						name       string
						reject     bool
						wantSource PromiseState
					}{
						{name: "fulfilled source", wantSource: Fulfilled},
						{name: "rejected source", reject: true, wantSource: Rejected},
					} {
						t.Run(settlement.name, func(t *testing.T) {
							reported := make(chan any, 2)
							loop := New()
							js := NewJS(loop,
								WithUnhandledRejection(func(reason any) { reported <- reason }),
								WithUnhandledRejectionFallback(UnhandledRejectionFallbackIsolated),
							)
							source, resolveSource, rejectSource := js.NewChainedPromise()

							if attachment.afterClose {
								if err := loop.Close(); err != nil {
									t.Fatal(err)
								}
							}
							result := combinator.combine(js, []*ChainedPromise{source})
							resultChannel := result.ToChannel()
							// Suppress only the expected public aggregate diagnostic so any
							// inaccessible internal reaction child remains observable.
							result.rejectionHandled.Store(true)
							if !attachment.afterClose {
								if err := loop.Close(); err != nil {
									t.Fatal(err)
								}
							}

							if settlement.reject {
								rejectSource("late rejection")
							} else {
								resolveSource("late fulfillment")
							}

							if source.State() != settlement.wantSource {
								t.Fatalf("source state = %v, want %v", source.State(), settlement.wantSource)
							}
							if result.State() != Rejected {
								t.Fatalf("aggregate state = %v, want Rejected", result.State())
							}
							reason, ok := result.Reason().(error)
							if !ok || !errors.Is(reason, ErrLoopTerminated) {
								t.Fatalf("aggregate reason = %T %v, want ErrLoopTerminated", result.Reason(), result.Reason())
							}
							got := waitContractValue(t, resultChannel, "terminal combinator settlement")
							channelErr, ok := got.(error)
							if !ok || !errors.Is(channelErr, ErrLoopTerminated) {
								t.Fatalf("aggregate channel result = %T %v, want ErrLoopTerminated", got, got)
							}

							waitTerminalUnhandledRejectionTrackingDrained(t, js)
							select {
							case orphan := <-reported:
								t.Fatalf("terminal combinator created an orphaned internal rejection: %v", orphan)
							default:
							}
						})
					}
				})
			}
		})
	}
}

func TestPromiseCombinatorsRejectTerminalSettledSourceAttachment(t *testing.T) {
	for _, combinator := range promiseCombinatorTestCases() {
		t.Run(combinator.name, func(t *testing.T) {
			for _, settlement := range []struct {
				name   string
				reject bool
			}{
				{name: "fulfilled source"},
				{name: "rejected source", reject: true},
			} {
				t.Run(settlement.name, func(t *testing.T) {
					reported := make(chan any, 2)
					recordReached := make(chan struct{})
					releaseRecord := make(chan struct{})
					releaseRecordFn := releaseSignalT(t, releaseRecord)
					var blockRecord atomic.Bool
					var recordOnce sync.Once
					loop := New()
					registerLoopCleanupT(t, loop)
					js := NewJS(loop,
						WithUnhandledRejection(func(reason any) { reported <- reason }),
						WithUnhandledRejectionFallback(UnhandledRejectionFallbackIsolated),
					)
					loop.testHooks = &loopTestHooks{
						BeforeUnhandledRejectionRecordCheck: func(*ChainedPromise) {
							if blockRecord.Load() {
								recordOnce.Do(func() {
									close(recordReached)
									<-releaseRecord
								})
							}
						},
					}
					source, resolveSource, rejectSource := js.NewChainedPromise()
					if settlement.reject {
						source.rejectionHandled.Store(true)
						rejectSource("stable rejection")
					} else {
						resolveSource("stable fulfillment")
					}
					if err := loop.Close(); err != nil {
						t.Fatal(err)
					}
					waitTerminalUnhandledRejectionTrackingDrained(t, js)

					blockRecord.Store(true)
					result := combinator.combine(js, []*ChainedPromise{source})
					resultChannel := result.ToChannel()
					result.rejectionHandled.Store(true)
					waitContractSignal(t, recordReached, "settled-source aggregate rejection check")
					releaseRecordFn()

					assertTerminalCombinatorRejection(t, result, resultChannel)
					waitTerminalUnhandledRejectionTrackingDrained(t, js)
					select {
					case orphan := <-reported:
						t.Fatalf("settled-source combinator created an orphaned rejection: %v", orphan)
					default:
					}
				})
			}
		})
	}
}

func TestPromiseCombinatorsCrossLoopTerminalOrdering(t *testing.T) {
	for _, combinator := range promiseCombinatorTestCases() {
		t.Run(combinator.name, func(t *testing.T) {
			for _, ordering := range []struct {
				name         string
				normalFirst  bool
				normalQueued bool
			}{
				{name: "normal reaction first", normalFirst: true},
				{name: "normal source first but reaction queued", normalQueued: true},
				{name: "terminal failure first"},
			} {
				t.Run(ordering.name, func(t *testing.T) {
					reported := make(chan any, 4)
					targetLoop, targetJS := newCombinatorTestAdapter(t, reported)
					normalLoop, normalJS := newCombinatorTestAdapter(t, reported)
					terminalLoop, terminalJS := newCombinatorTestAdapter(t, reported)
					normalSource, resolveNormal, _ := normalJS.NewChainedPromise()
					terminalSource, resolveTerminal, _ := terminalJS.NewChainedPromise()
					result := combinator.combine(targetJS, []*ChainedPromise{normalSource, terminalSource})
					resultChannel := result.ToChannel()
					result.rejectionHandled.Store(true)

					if ordering.normalFirst {
						resolveNormal("normal winner")
						normalLoop.tick()
						if combinator.name == "Race" || combinator.name == "Any" {
							if state := result.State(); state != Fulfilled {
								t.Fatalf("normal-first intermediate state = %v, want Fulfilled", state)
							}
						} else if state := result.State(); state != Pending {
							t.Fatalf("normal-first intermediate state = %v, want Pending", state)
						}
						if err := terminalLoop.Close(); err != nil {
							t.Fatal(err)
						}
						resolveTerminal("terminal source")
					} else if ordering.normalQueued {
						resolveNormal("queued normal source")
						if state := result.State(); state != Pending {
							t.Fatalf("queued-normal intermediate state = %v, want Pending before its reaction executes", state)
						}
						if err := terminalLoop.Close(); err != nil {
							t.Fatal(err)
						}
						resolveTerminal("terminal source")
						normalLoop.tick()
					} else {
						if err := terminalLoop.Close(); err != nil {
							t.Fatal(err)
						}
						resolveTerminal("terminal source")
						resolveNormal("late normal source")
						normalLoop.tick()
					}

					targetLoop.tick()
					if ordering.normalFirst && (combinator.name == "Race" || combinator.name == "Any") {
						if state := result.State(); state != Fulfilled || result.Value() != "normal winner" {
							t.Fatalf("normal winner changed after terminal failure: state=%v value=%v", state, result.Value())
						}
						assertSinglePromiseChannelValue(t, resultChannel, "normal winner")
					} else {
						assertTerminalCombinatorRejection(t, result, resultChannel)
					}
					assertUnhandledRejectionTrackingDrained(t, targetJS)
					assertUnhandledRejectionTrackingDrained(t, normalJS)
					assertUnhandledRejectionTrackingDrained(t, terminalJS)
					select {
					case orphan := <-reported:
						t.Fatalf("cross-loop combinator created an orphaned rejection: %v", orphan)
					default:
					}
				})
			}
		})
	}
}

func TestPromiseCombinatorsTwoTerminalFailuresSettleOnce(t *testing.T) {
	for _, combinator := range promiseCombinatorTestCases() {
		t.Run(combinator.name, func(t *testing.T) {
			reported := make(chan any, 4)
			targetLoop, targetJS := newCombinatorTestAdapter(t, reported)
			firstLoop, firstJS := newCombinatorTestAdapter(t, reported)
			secondLoop, secondJS := newCombinatorTestAdapter(t, reported)
			first, resolveFirst, _ := firstJS.NewChainedPromise()
			second, resolveSecond, _ := secondJS.NewChainedPromise()
			result := combinator.combine(targetJS, []*ChainedPromise{first, second})
			resultChannel := result.ToChannel()
			result.rejectionHandled.Store(true)

			if err := firstLoop.Close(); err != nil {
				t.Fatal(err)
			}
			if err := secondLoop.Close(); err != nil {
				t.Fatal(err)
			}
			resolveFirst("first terminal source")
			resolveSecond("second terminal source")
			targetLoop.tick()

			assertTerminalCombinatorRejection(t, result, resultChannel)
			assertUnhandledRejectionTrackingDrained(t, targetJS)
			assertUnhandledRejectionTrackingDrained(t, firstJS)
			assertUnhandledRejectionTrackingDrained(t, secondJS)
			select {
			case orphan := <-reported:
				t.Fatalf("multiple terminal failures created an orphaned rejection: %v", orphan)
			default:
			}
		})
	}
}

func TestPromiseCombinatorsAcceptedReactionCloseDisposition(t *testing.T) {
	for _, combinator := range promiseCombinatorTestCases() {
		t.Run(combinator.name, func(t *testing.T) {
			reported := make(chan any, 2)
			loop := New(WithLogger(nil))
			registerLoopCleanupT(t, loop)
			js := NewJS(loop,
				WithUnhandledRejection(func(reason any) { reported <- reason }),
				WithUnhandledRejectionFallback(UnhandledRejectionFallbackIsolated),
			)

			admissionEntered := make(chan struct{})
			releaseAdmission := make(chan struct{})
			releaseAdmissionFn := releaseSignalT(t, releaseAdmission)
			closeTransitioned := make(chan struct{})
			var admissionOnce sync.Once
			loop.testHooks = &loopTestHooks{
				BeforeCallbackAdmission: func() {
					admissionOnce.Do(func() {
						close(admissionEntered)
						<-releaseAdmission
					})
				},
				AfterCloseStateTerminating: func() { close(closeTransitioned) },
			}

			source, resolveSource, _ := js.NewChainedPromise()
			result := combinator.combine(js, []*ChainedPromise{source})
			resultChannel := result.ToChannel()
			resolveSource("accepted before Close")

			runDone := make(chan error, 1)
			go func() { runDone <- loop.Run(context.Background()) }()
			waitContractSignal(t, admissionEntered, "dequeued Promise reaction admission")
			closeDone := make(chan error, 1)
			go func() { closeDone <- loop.Close() }()
			waitContractSignal(t, closeTransitioned, "Close callback gate transition")
			releaseAdmissionFn()

			if err := waitContractValue(t, closeDone, "Close after dequeued Promise reaction"); err != nil {
				t.Fatalf("Close: %v", err)
			}
			if err := waitContractValue(t, runDone, "Run after dequeued Promise reaction"); err != nil {
				t.Fatalf("Run: %v", err)
			}
			assertTerminalCombinatorRejection(t, result, resultChannel)
			reason := waitContractValue(t, reported, "public terminal combinator diagnostic")
			if err, ok := reason.(error); !ok || !errors.Is(err, ErrLoopTerminated) {
				t.Fatalf("reported rejection = %T %v, want ErrLoopTerminated", reason, reason)
			}
			waitTerminalUnhandledRejectionTrackingDrained(t, js)
			select {
			case orphan := <-reported:
				t.Fatalf("terminal combinator reported a private observer child: %v", orphan)
			default:
			}
		})
	}
}

func TestPromiseCombinatorsAcceptedNotDequeuedImmediateClose(t *testing.T) {
	for _, combinator := range promiseCombinatorTestCases() {
		t.Run(combinator.name, func(t *testing.T) {
			for _, crossLoop := range []bool{false, true} {
				name := "same loop"
				if crossLoop {
					name = "cross loop"
				}
				t.Run(name, func(t *testing.T) {
					reported := make(chan any, 3)
					sourceLoop := New(WithLogger(nil))
					registerLoopCleanupT(t, sourceLoop)
					sourceJS := NewJS(sourceLoop,
						WithUnhandledRejection(func(reason any) { reported <- reason }),
						WithUnhandledRejectionFallback(UnhandledRejectionFallbackIsolated),
					)
					targetLoop, targetJS := sourceLoop, sourceJS
					if crossLoop {
						targetLoop = New(WithLogger(nil))
						registerLoopCleanupT(t, targetLoop)
						targetJS = NewJS(targetLoop,
							WithUnhandledRejection(func(reason any) { reported <- reason }),
							WithUnhandledRejectionFallback(UnhandledRejectionFallbackIsolated),
						)
					}

					source, resolveSource, _ := sourceJS.NewChainedPromise()
					result := combinator.combine(targetJS, []*ChainedPromise{source})
					resultChannel := result.ToChannel()
					resolveSource("accepted without dequeue")
					if got := pendingPromiseReactionCount(sourceLoop); got != 1 {
						t.Fatalf("pending source reactions before Close = %d, want 1", got)
					}
					if err := sourceLoop.Close(); err != nil {
						t.Fatal(err)
					}
					assertTerminalCombinatorRejection(t, result, resultChannel)
					if got := pendingPromiseReactionCount(sourceLoop); got != 0 {
						t.Fatalf("pending source reactions after Close = %d, want 0", got)
					}
					if crossLoop {
						targetLoop.tick()
						waitUnhandledRejectionCheckOwnershipReleased(t, targetJS)
						assertUnhandledRejectionTrackingDrained(t, targetJS)
					} else {
						waitTerminalUnhandledRejectionTrackingDrained(t, targetJS)
					}
					reason := waitContractValue(t, reported, "accepted-not-dequeued aggregate diagnostic")
					if err, ok := reason.(error); !ok || !errors.Is(err, ErrLoopTerminated) {
						t.Fatalf("reported rejection = %T %v, want ErrLoopTerminated", reason, reason)
					}
					select {
					case orphan := <-reported:
						t.Fatalf("accepted-not-dequeued combinator reported a private child: %v", orphan)
					default:
					}
				})
			}
		})
	}
}

func TestPromiseCombinatorsGracefulShutdownDrainsAccepted(t *testing.T) {
	for _, combinator := range promiseCombinatorTestCases() {
		t.Run(combinator.name, func(t *testing.T) {
			reported := make(chan any, 2)
			sourceLoop, sourceJS := newCombinatorTestAdapter(t, reported)
			targetLoop, targetJS := newCombinatorTestAdapter(t, reported)
			source, resolveSource, _ := sourceJS.NewChainedPromise()
			result := combinator.combine(targetJS, []*ChainedPromise{source})
			resultChannel := result.ToChannel()
			resolveSource("graceful source")
			if got := pendingPromiseReactionCount(sourceLoop); got != 1 {
				t.Fatalf("pending source reactions before Shutdown = %d, want 1", got)
			}
			if err := sourceLoop.Shutdown(context.Background()); err != nil {
				t.Fatal(err)
			}
			if sourceLoop.immediateClose.Load() {
				t.Fatal("graceful Shutdown published immediate-Close mode")
			}
			if got := pendingPromiseReactionCount(sourceLoop); got != 0 {
				t.Fatalf("pending source reactions after Shutdown = %d, want 0", got)
			}
			value := waitContractValue(t, resultChannel, "graceful combinator settlement")
			switch combinator.name {
			case "All":
				values, ok := value.([]any)
				if !ok || len(values) != 1 || values[0] != "graceful source" {
					t.Fatalf("graceful All result = %T %#v", value, value)
				}
			case "AllSettled":
				values, ok := value.([]any)
				if !ok || len(values) != 1 {
					t.Fatalf("graceful AllSettled result = %T %#v", value, value)
				}
				outcome, ok := values[0].(map[string]any)
				if !ok || outcome["status"] != "fulfilled" || outcome["value"] != "graceful source" {
					t.Fatalf("graceful AllSettled outcome = %T %#v", values[0], values[0])
				}
			default:
				if value != "graceful source" {
					t.Fatalf("graceful %s result = %#v, want graceful source", combinator.name, value)
				}
			}
			assertPromiseResultChannelClosed(t, resultChannel)
			if state := result.State(); state != Fulfilled {
				t.Fatalf("graceful aggregate state = %v, want Fulfilled", state)
			}
			targetLoop.tick()
			assertUnhandledRejectionTrackingDrained(t, sourceJS)
			assertUnhandledRejectionTrackingDrained(t, targetJS)
			select {
			case reason := <-reported:
				t.Fatalf("graceful combinator reported rejection: %v", reason)
			default:
			}
		})
	}
}
