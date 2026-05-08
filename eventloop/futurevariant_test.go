package eventloop

import (
	"errors"
	"log"
	"slices"
	"sync"
	"testing"
)

type futureTournament interface {
	State() PromiseState
	Result() any
	ToChannel() <-chan any
}

type futureTournamentInstance struct {
	future  futureTournament
	resolve func(any)
	reject  func(error)
}

type futureTournamentImplementation struct {
	id   string
	name string
	new  func() futureTournamentInstance
}

var futureTournamentDirectSend = futureTournamentImplementation{
	id:   "future.basic.direct-send-channel-fanout",
	name: "DirectSendChannelFanOut",
	new: func() futureTournamentInstance {
		future := &promise{state: Pending}
		return futureTournamentInstance{future: future, resolve: future.resolve, reject: future.reject}
	},
}

var futureTournamentTrySendLog = futureTournamentImplementation{
	id:   "future.basic.try-send-log-channel-fanout",
	name: "TrySendLogChannelFanOut",
	new: func() futureTournamentInstance {
		future := &trySendLogFutureTournament{state: Pending}
		return futureTournamentInstance{future: future, resolve: future.Resolve, reject: future.Reject}
	},
}

func futureTournamentImplementations() []futureTournamentImplementation {
	return []futureTournamentImplementation{
		futureTournamentDirectSend,
		futureTournamentTrySendLog,
	}
}

// trySendLogFutureTournament preserves the Future algorithm introduced in the
// 506d664/c0f1445e source snapshot. Its fallback is unreachable while every
// subscriber remains a unique, empty, capacity-one channel, but the historical
// hot path remains independently executable for longitudinal measurement.
type trySendLogFutureTournament struct {
	result      any
	subscribers []chan any
	state       PromiseState
	mu          sync.Mutex
}

func (p *trySendLogFutureTournament) State() PromiseState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

func (p *trySendLogFutureTournament) Result() any {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.result
}

func (p *trySendLogFutureTournament) ToChannel() <-chan any {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state != Pending {
		channel := make(chan any, 1)
		channel <- p.result
		close(channel)
		return channel
	}
	channel := make(chan any, 1)
	p.subscribers = append(p.subscribers, channel)
	return channel
}

func (p *trySendLogFutureTournament) Resolve(value any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state != Pending {
		return
	}
	p.state = Fulfilled
	p.result = value
	p.fanOut()
}

func (p *trySendLogFutureTournament) Reject(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state != Pending {
		return
	}
	p.state = Rejected
	p.result = err
	p.fanOut()
}

func (p *trySendLogFutureTournament) fanOut() {
	for _, channel := range p.subscribers {
		select {
		case channel <- p.result:
		default:
			log.Printf("WARNING: eventloop: dropped promise result, channel full")
		}
		close(channel)
	}
	p.subscribers = nil
}

var errFutureTournament = errors.New("future tournament rejection")

func TestFutureTournamentImplementations(t *testing.T) {
	implementations := futureTournamentImplementations()
	if len(implementations) != 2 {
		t.Fatalf("implementation count = %d, want 2", len(implementations))
	}
	if !slices.IsSortedFunc(implementations, func(left, right futureTournamentImplementation) int {
		return compareFutureTournamentStrings(left.id, right.id)
	}) {
		t.Fatal("Future tournament implementations are not sorted by stable ID")
	}
	seen := make(map[string]struct{}, len(implementations))
	for _, implementation := range implementations {
		if implementation.id == "" || implementation.name == "" || implementation.new == nil {
			t.Errorf("incomplete implementation: %+v", implementation)
		}
		if _, duplicate := seen[implementation.id]; duplicate {
			t.Errorf("duplicate implementation ID %q", implementation.id)
		}
		seen[implementation.id] = struct{}{}
		t.Run(implementation.name, func(t *testing.T) {
			testFutureTournamentImplementation(t, implementation)
		})
	}
}

func testFutureTournamentImplementation(t *testing.T, implementation futureTournamentImplementation) {
	for _, test := range []struct {
		name      string
		settle    func(futureTournamentInstance)
		wantState PromiseState
		want      any
	}{
		{name: "Resolve", settle: func(instance futureTournamentInstance) { instance.resolve(42) }, wantState: Fulfilled, want: 42},
		{name: "Reject", settle: func(instance futureTournamentInstance) { instance.reject(errFutureTournament) }, wantState: Rejected, want: errFutureTournament},
		{name: "ResolveNil", settle: func(instance futureTournamentInstance) { instance.resolve(nil) }, wantState: Fulfilled, want: nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			instance := implementation.new()
			if instance.future == nil || instance.resolve == nil || instance.reject == nil {
				t.Fatal("factory returned an incomplete instance")
			}
			pending := instance.future.ToChannel()
			test.settle(instance)
			instance.resolve("ignored resolve")
			instance.reject(errors.New("ignored reject"))
			if got := instance.future.State(); got != test.wantState {
				t.Fatalf("State() = %v, want %v", got, test.wantState)
			}
			if got := instance.future.Result(); got != test.want {
				t.Fatalf("Result() = %v, want %v", got, test.want)
			}
			assertFutureTournamentChannel(t, pending, test.want)
			assertFutureTournamentChannel(t, instance.future.ToChannel(), test.want)
		})
	}

	t.Run("SubscribeSettleRace", func(t *testing.T) {
		for range 128 {
			instance := implementation.new()
			start := make(chan struct{})
			channelReady := make(chan (<-chan any), 1)
			var group sync.WaitGroup
			group.Add(2)
			go func() {
				defer group.Done()
				<-start
				channelReady <- instance.future.ToChannel()
			}()
			go func() {
				defer group.Done()
				<-start
				instance.resolve(42)
			}()
			close(start)
			group.Wait()
			assertFutureTournamentChannel(t, <-channelReady, 42)
		}
	})
}

func assertFutureTournamentChannel(t testing.TB, channel <-chan any, want any) {
	t.Helper()
	select {
	case got, ok := <-channel:
		if !ok || got != want {
			t.Fatalf("subscription = (%v, %t), want (%v, true)", got, ok, want)
		}
	default:
		t.Fatal("subscription did not synchronously publish its settled value")
	}
	select {
	case _, ok := <-channel:
		if ok {
			t.Fatal("subscription channel remained open after its settled value")
		}
	default:
		t.Fatal("subscription channel was not synchronously closed")
	}
}

func compareFutureTournamentStrings(left, right string) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}
