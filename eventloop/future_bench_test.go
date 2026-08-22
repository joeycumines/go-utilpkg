package eventloop

import (
	"strconv"
	"testing"
)

const futureTournamentBatchSize = 256
const futureTournamentFanOutBudget = 4096

var benchmarkFutureStateSink PromiseState
var benchmarkFutureResultSink any

func BenchmarkFutureRawCreate(b *testing.B) {
	for _, implementation := range futureTournamentImplementations() {
		b.Run(implementation.id, func(b *testing.B) {
			instances := make([]futureTournamentInstance, futureTournamentBatchSize)
			b.ReportAllocs()
			b.ResetTimer()
			b.StopTimer()
			for completed := 0; completed < b.N; {
				count := futureTournamentBatchCount(b.N-completed, futureTournamentBatchSize)
				b.StartTimer()
				for index := range count {
					instances[index] = implementation.new()
				}
				b.StopTimer()
				for index := range count {
					instance := instances[index]
					if instance.future == nil || instance.resolve == nil || instance.reject == nil || instance.future.State() != Pending {
						b.Fatalf("factory returned invalid instance %d", index)
					}
					instance.resolve(nil)
				}
				completed += count
			}
		})
	}
}

func BenchmarkFutureRegisteredCreate(b *testing.B) {
	b.Run(futureTournamentDirectSend.id, func(b *testing.B) {
		registry := newRegistry()
		promises := make([]*promise, futureTournamentBatchSize)
		b.ReportAllocs()
		b.ResetTimer()
		b.StopTimer()
		for completed := 0; completed < b.N; {
			count := futureTournamentBatchCount(b.N-completed, futureTournamentBatchSize)
			b.StartTimer()
			for index := range count {
				promises[index] = registry.NewPromise()
			}
			b.StopTimer()
			for index := range count {
				promises[index].resolve(nil)
			}
			registry.Scavenge(len(registry.ring))
			completed += count
		}
		registry.mu.RLock()
		remaining := len(registry.data)
		registry.mu.RUnlock()
		if remaining != 0 {
			b.Fatalf("settled registry entries = %d, want 0", remaining)
		}
	})
}

func BenchmarkFutureSubscribePending(b *testing.B) {
	for _, implementation := range futureTournamentImplementations() {
		b.Run(implementation.id, func(b *testing.B) {
			instances := make([]futureTournamentInstance, futureTournamentBatchSize)
			channels := make([]<-chan any, futureTournamentBatchSize)
			b.ReportAllocs()
			b.ResetTimer()
			b.StopTimer()
			for completed := 0; completed < b.N; {
				count := futureTournamentBatchCount(b.N-completed, futureTournamentBatchSize)
				for index := range count {
					instances[index] = implementation.new()
				}
				b.StartTimer()
				for index := range count {
					channels[index] = instances[index].future.ToChannel()
				}
				b.StopTimer()
				for index := range count {
					instances[index].resolve(42)
					assertFutureTournamentChannel(b, channels[index], 42)
				}
				completed += count
			}
		})
	}
}

func BenchmarkFutureSubscribeSettled(b *testing.B) {
	for _, implementation := range futureTournamentImplementations() {
		b.Run(implementation.id, func(b *testing.B) {
			for _, settlement := range futureTournamentSettlements() {
				b.Run(settlement.name, func(b *testing.B) {
					instances := make([]futureTournamentInstance, futureTournamentBatchSize)
					channels := make([]<-chan any, futureTournamentBatchSize)
					b.ReportAllocs()
					b.ResetTimer()
					b.StopTimer()
					for completed := 0; completed < b.N; {
						count := futureTournamentBatchCount(b.N-completed, futureTournamentBatchSize)
						for index := range count {
							instances[index] = implementation.new()
							settlement.apply(instances[index])
						}
						b.StartTimer()
						for index := range count {
							channels[index] = instances[index].future.ToChannel()
						}
						b.StopTimer()
						for index := range count {
							assertFutureTournamentChannel(b, channels[index], settlement.want)
						}
						completed += count
					}
				})
			}
		})
	}
}

func BenchmarkFutureSettleNoSubscriber(b *testing.B) {
	for _, implementation := range futureTournamentImplementations() {
		b.Run(implementation.id, func(b *testing.B) {
			for _, settlement := range futureTournamentSettlements() {
				b.Run(settlement.name, func(b *testing.B) {
					instances := make([]futureTournamentInstance, futureTournamentBatchSize)
					b.ReportAllocs()
					b.ResetTimer()
					b.StopTimer()
					for completed := 0; completed < b.N; {
						count := futureTournamentBatchCount(b.N-completed, futureTournamentBatchSize)
						for index := range count {
							instances[index] = implementation.new()
						}
						b.StartTimer()
						for index := range count {
							settlement.apply(instances[index])
						}
						b.StopTimer()
						for index := range count {
							if got := instances[index].future.State(); got != settlement.state {
								b.Fatalf("State() = %v, want %v", got, settlement.state)
							}
							if got := instances[index].future.Result(); got != settlement.want {
								b.Fatalf("Result() = %v, want %v", got, settlement.want)
							}
						}
						completed += count
					}
				})
			}
		})
	}
}

func BenchmarkFutureSettleFanOut(b *testing.B) {
	for _, implementation := range futureTournamentImplementations() {
		b.Run(implementation.id, func(b *testing.B) {
			for _, settlement := range futureTournamentSettlements() {
				b.Run(settlement.name, func(b *testing.B) {
					for _, subscribers := range []int{1, 8, 64, 1024} {
						b.Run(strconv.Itoa(subscribers), func(b *testing.B) {
							batchLimit := max(1, min(futureTournamentBatchSize, futureTournamentFanOutBudget/subscribers))
							instances := make([]futureTournamentInstance, batchLimit)
							channels := make([]<-chan any, batchLimit*subscribers)
							b.ReportAllocs()
							b.ResetTimer()
							b.StopTimer()
							for completed := 0; completed < b.N; {
								count := futureTournamentBatchCount(b.N-completed, batchLimit)
								for index := range count {
									instances[index] = implementation.new()
									base := index * subscribers
									for subscriber := range subscribers {
										channels[base+subscriber] = instances[index].future.ToChannel()
									}
								}
								b.StartTimer()
								for index := range count {
									settlement.apply(instances[index])
								}
								b.StopTimer()
								for index := range count {
									base := index * subscribers
									for subscriber := range subscribers {
										assertFutureTournamentChannel(b, channels[base+subscriber], settlement.want)
									}
								}
								completed += count
							}
						})
					}
				})
			}
		})
	}
}

func BenchmarkFutureState(b *testing.B) {
	for _, implementation := range futureTournamentImplementations() {
		b.Run(implementation.id, func(b *testing.B) {
			for _, state := range futureTournamentStates() {
				b.Run(state.name, func(b *testing.B) {
					instance := implementation.new()
					state.apply(instance)
					if got := instance.future.State(); got != state.state {
						b.Fatalf("State() = %v, want %v", got, state.state)
					}
					b.ReportAllocs()
					b.ResetTimer()
					for range b.N {
						benchmarkFutureStateSink = instance.future.State()
					}
				})
			}
		})
	}
}

func BenchmarkFutureResult(b *testing.B) {
	for _, implementation := range futureTournamentImplementations() {
		b.Run(implementation.id, func(b *testing.B) {
			for _, state := range futureTournamentStates() {
				b.Run(state.name, func(b *testing.B) {
					instance := implementation.new()
					state.apply(instance)
					if got := instance.future.State(); got != state.state {
						b.Fatalf("State() = %v, want %v", got, state.state)
					}
					if got := instance.future.Result(); got != state.want {
						b.Fatalf("Result() = %v, want %v", got, state.want)
					}
					b.ReportAllocs()
					b.ResetTimer()
					for range b.N {
						benchmarkFutureResultSink = instance.future.Result()
					}
				})
			}
		})
	}
}

type futureTournamentSettlement struct {
	name  string
	state PromiseState
	want  any
	apply func(futureTournamentInstance)
}

func futureTournamentSettlements() []futureTournamentSettlement {
	return []futureTournamentSettlement{
		{name: "Fulfilled", state: Fulfilled, want: 42, apply: func(instance futureTournamentInstance) { instance.resolve(42) }},
		{name: "Rejected", state: Rejected, want: errFutureTournament, apply: func(instance futureTournamentInstance) { instance.reject(errFutureTournament) }},
	}
}

type futureTournamentState struct {
	name  string
	state PromiseState
	want  any
	apply func(futureTournamentInstance)
}

func futureTournamentStates() []futureTournamentState {
	return []futureTournamentState{
		{name: "Pending", state: Pending, want: nil, apply: func(futureTournamentInstance) {}},
		{name: "Fulfilled", state: Fulfilled, want: 42, apply: func(instance futureTournamentInstance) { instance.resolve(42) }},
		{name: "Rejected", state: Rejected, want: errFutureTournament, apply: func(instance futureTournamentInstance) { instance.reject(errFutureTournament) }},
	}
}

func futureTournamentBatchCount(remaining, limit int) int {
	return min(remaining, limit)
}
