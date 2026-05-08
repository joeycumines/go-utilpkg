package gojaeventloop

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

func BenchmarkProcessBeforeExitSchedulesTimer(b *testing.B) {
	b.ReportAllocs()
	watchdog := time.AfterFunc(30*time.Minute, func() { panic("BenchmarkProcessBeforeExitSchedulesTimer timed out") })
	defer watchdog.Stop()
	b.ResetTimer()
	b.StopTimer()
	for i := 0; i < b.N; i++ {
		loop := goeventloop.New(goeventloop.WithAutoExit(true))
		runtime := goja.New()
		adapter, err := New(loop, runtime)
		if err != nil {
			b.Fatalf("New adapter: %v", err)
		}
		if err := adapter.Bind(); err != nil {
			b.Fatalf("Bind: %v", err)
		}
		_, err = runtime.RunString(`
			globalThis.events = [];
			let scheduled = false;
			let count = 0;
			process.on("beforeExit", function() {
				count += 1;
				events.push("beforeExit" + count);
				if (!scheduled) {
					scheduled = true;
					setTimeout(function() { events.push("timer"); }, 0);
				}
			});
		`)
		if err != nil {
			b.Fatalf("RunString: %v", err)
		}
		b.StartTimer()
		err = loop.Run(context.Background())
		b.StopTimer()
		if err != nil {
			b.Fatalf("Run: %v", err)
		}
		value, err := runtime.RunString(`events.join(",")`)
		if err != nil {
			b.Fatalf("read events: %v", err)
		}
		if got, want := value.String(), "beforeExit1,timer,beforeExit2"; got != want {
			b.Fatalf("beforeExit timer path = %q, want %q", got, want)
		}
	}
}

const processBeforeExitBatchSize = 32

const processBeforeExitTimerScript = `
	globalThis.events = [];
	let scheduled = false;
	let count = 0;
	process.on("beforeExit", function() {
		count += 1;
		events.push("beforeExit" + count);
		if (!scheduled) {
			scheduled = true;
			setTimeout(function() { events.push("timer"); }, 0);
		}
	});
`

type processBeforeExitFixture struct {
	firstClose error
	loop       *goeventloop.Loop
	runtime    *goja.Runtime
	state      goeventloop.LoopState
}

// BenchmarkProcessBeforeExitTimerEndToEnd measures bounded batches of the
// complete adapter construction, binding, script evaluation, beforeExit timer
// extension, and joined auto-exit lifecycle. Result, state, and repeated-close
// checks are untimed.
func BenchmarkProcessBeforeExitTimerEndToEnd(b *testing.B) {
	b.ReportAllocs()
	watchdog := time.AfterFunc(30*time.Minute, func() { panic("BenchmarkProcessBeforeExitTimerEndToEnd timed out") })
	defer watchdog.Stop()
	b.ResetTimer()
	b.StopTimer()
	completed := 0
	for completed < b.N {
		batchSize := min(processBeforeExitBatchSize, b.N-completed)
		fixtures := make([]processBeforeExitFixture, 0, batchSize)
		b.StartTimer()
		for range batchSize {
			fixture, err := runProcessBeforeExitFixture(context.Background())
			if err != nil {
				b.StopTimer()
				b.Fatalf("run process beforeExit fixture: %v", err)
			}
			fixtures = append(fixtures, fixture)
		}
		b.StopTimer()
		for _, fixture := range fixtures {
			if err := verifyProcessBeforeExitFixture(fixture); err != nil {
				b.Fatal(err)
			}
		}
		completed += batchSize
	}
}

func runProcessBeforeExitFixture(ctx context.Context) (processBeforeExitFixture, error) {
	loop := goeventloop.New(goeventloop.WithAutoExit(true))
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		return processBeforeExitFixture{}, errors.Join(fmt.Errorf("New adapter: %w", err), loop.Close())
	}
	if err := adapter.Bind(); err != nil {
		return processBeforeExitFixture{}, errors.Join(fmt.Errorf("Bind: %w", err), loop.Close())
	}
	if _, err := runtime.RunString(processBeforeExitTimerScript); err != nil {
		return processBeforeExitFixture{}, errors.Join(fmt.Errorf("RunString: %w", err), loop.Close())
	}
	if err := loop.Run(ctx); err != nil {
		return processBeforeExitFixture{}, errors.Join(fmt.Errorf("Run: %w", err), loop.Close())
	}
	state := loop.State()
	firstClose := loop.Close()
	return processBeforeExitFixture{firstClose: firstClose, loop: loop, runtime: runtime, state: state}, nil
}

func verifyProcessBeforeExitFixture(fixture processBeforeExitFixture) error {
	secondClose := fixture.loop.Close()
	value, err := fixture.runtime.RunString(`events.join(",")`)
	if err != nil {
		return fmt.Errorf("read events: %w", err)
	}
	if got, want := value.String(), "beforeExit1,timer,beforeExit2"; got != want {
		return fmt.Errorf("beforeExit timer path = %q, want %q", got, want)
	}
	if fixture.state != goeventloop.StateTerminated {
		return fmt.Errorf("loop state = %v, want %v", fixture.state, goeventloop.StateTerminated)
	}
	if fixture.firstClose != nil && !errors.Is(fixture.firstClose, goeventloop.ErrLoopTerminated) {
		return fmt.Errorf("first Close after Run = %w", fixture.firstClose)
	}
	if !errors.Is(secondClose, goeventloop.ErrLoopTerminated) {
		return fmt.Errorf("repeated Close after terminal completion = %v, want ErrLoopTerminated", secondClose)
	}
	return nil
}

func TestProcessBeforeExitTimerBenchmarkFixtureLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	fixture, err := runProcessBeforeExitFixture(ctx)
	if err != nil {
		t.Fatalf("run process beforeExit fixture: %v", err)
	}
	if err := verifyProcessBeforeExitFixture(fixture); err != nil {
		t.Fatal(err)
	}
}
