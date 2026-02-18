package testsuite

import (
	"github.com/joeycumines/logiface"
	"golang.org/x/exp/slices"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
)

// TestParallel tests the integrity of parallel writes.
func TestParallel[E logiface.Event](t *testing.T, cfg Config[E]) {
	cfg.RunTest(TestRequest[E]{
		Level: logiface.LevelTrace,
	}, func(tr TestResponse[E]) {
		const (
			maxMessages = 5_000
			numWorkers  = 25
		)

		// going to log messages with identifiers ranging from 1 to maxMessages
		var monotoniclyIncreasingID int64
		nextID := func() (int64, bool) {
			ID := atomic.AddInt64(&monotoniclyIncreasingID, 1)
			return ID, ID <= maxMessages
		}
		const messageIDField = "TestParallel_message_id"
		allMessages := make([]Event, 0, maxMessages)
		var variants []func(ID int64) Event
		log := func(ID int64) Event {
			return variants[ID%int64(len(variants))](ID)
		}

		// critical -> trace, and all custom levels (if supported)
		for i := 2; i <= math.MaxInt8; i++ {
			level := logiface.Level(i)
			if !tr.LevelMapping(level).Enabled() {
				continue
			}

			// variants using eventTemplates + logiface.Logger.Log
			for _, template := range eventTemplates {
				template := template
				var once sync.Once
				variants = append(variants, func(ID int64) (ev Event) {
					if err := tr.Logger.Log(level, logiface.ModifierFunc[E](func(event E) error {
						ev = normalizeEvent(cfg, tr, template(event))
						event.AddField(messageIDField, float64(ID))
						ev.Fields[messageIDField] = float64(ID)
						once.Do(func() { t.Logf(`logged (expected) message: %s`, ev) })
						return nil
					})); err != nil {
						t.Errorf(`expected no error, got %v`, err)
						panic(err)
					}
					if ev.Fields[messageIDField] != float64(ID) {
						const bad = `didnt log for some reason?`
						t.Error(bad)
						panic(bad)
					}
					return
				})
			}

			// variants using eventTemplates + logiface.Logger.Build
			for _, template := range eventTemplates {
				template := template
				var once sync.Once
				variants = append(variants, func(ID int64) (ev Event) {
					modifier := logiface.ModifierFunc[E](func(event E) error {
						ev = normalizeEvent(cfg, tr, template(event))
						event.AddField(messageIDField, float64(ID))
						ev.Fields[messageIDField] = float64(ID)
						once.Do(func() { t.Logf(`logged (expected) message: %s`, ev) })
						return nil
					})
					tr.Logger.Build(level).
						Call(func(b *logiface.Builder[E]) { _ = modifier.Modify(b.Event) }).
						Log(``)
					if ev.Fields[messageIDField] != float64(ID) {
						const bad = `didnt log for some reason?`
						t.Error(bad)
						panic(bad)
					}
					return
				})
			}
		}

		// shuffle the variants
		rand.Shuffle(len(variants), func(i, j int) {
			variants[i], variants[j] = variants[j], variants[i]
		})

		// run the workers
		stop := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(numWorkers)
		for i := 0; i < numWorkers; i++ {
			go func() {
				defer wg.Done()
				for {
					select {
					case <-stop:
						return
					default:
						if ID, ok := nextID(); ok {
							log(ID)
						}
					}
				}
			}()
		}
		defer func() {
			close(stop)
			go func() {
				for range tr.Events {
				}
			}()
			wg.Wait()
			tr.SendEOF()
		}()

		// receive all messages
		for i := 0; i < maxMessages; i++ {
			ev, ok := tr.ReceiveEvent()
			if !ok {
				break
			}
			allMessages = append(allMessages, ev)
		}

		if len(allMessages) != maxMessages {
			t.Errorf("expected len(allMessages) == maxMessages, got %d != %d", len(allMessages), maxMessages)
		}
		slices.SortFunc(allMessages, func(a, b Event) int {
			af, _ := a.Fields[messageIDField].(float64)
			bf, _ := b.Fields[messageIDField].(float64)
			if af < bf {
				return -1
			} else if af > bf {
				return 1
			}
			return 0
		})
		for i, msg := range allMessages {
			ID := int64(i) + 1
			if msg.Fields[messageIDField] != float64(ID) {
				t.Fatalf("message[%d]: unexpected id: %s", i, msg)
			}
			// log the message again, so we can check it aligns with what we expect
			expected := log(ID)
			if !msg.Equal(expected) {
				t.Fatalf("message[%d]: unexpected message:\nexpected: %s\nactual: %s", i, expected, msg)
			}
			var ok bool
			expected, ok = tr.ReceiveEvent()
			if !ok {
				t.Fatalf("message[%d]: expected check message, got none", i)
			}
			if !msg.Equal(expected) {
				t.Fatalf("message[%d]: unexpected check message:\nexpected: %s\nactual: %s", i, expected, msg)
			}
		}

		tr.SendEOFExpectNoEvents(t)
	})
}
