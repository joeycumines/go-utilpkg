package testsuite

import (
	"github.com/joeycumines/go-utilpkg/logiface"
	"testing"
	"time"
)

// TestLoggerLogMethod is part of TestSuite, it tests logiface.Logger.Log.
func TestLoggerLogMethod[E logiface.Event](t *testing.T, cfg Config[E]) {
	t.Run(`disabled levels`, func(t *testing.T) {
		t.Parallel()
		for _, tc := range disabledLevelCombinations() {
			tc := tc
			t.Run(tc.Name, func(t *testing.T) {
				t.Parallel()
				cfg.RunTest(TestRequest[E]{
					Level: tc.Logger,
				}, func(tr TestResponse[E]) {
					if err := tr.Logger.Log(tc.Arg, nil); err != logiface.ErrDisabled {
						t.Errorf(`expected logiface.ErrDisabled, got %v`, err)
					}
					tr.SendEOFExpectNoEvents(t)
				})
			})
		}
	})
	t.Run(`enabled levels without modifier`, func(t *testing.T) {
		t.Parallel()
		for _, tc := range enabledLevelCombinations() {
			tc := tc
			t.Run(tc.Name, func(t *testing.T) {
				t.Parallel()
				cfg.RunTest(TestRequest[E]{
					Level: tc.Logger,
				}, func(tr TestResponse[E]) {
					var err error
					switch tc.Arg {
					case logiface.LevelEmergency:
						cfg.HandleEmergencyPanic(t, func() {
							err = tr.Logger.Log(tc.Arg, nil)
						})
					case logiface.LevelAlert:
						if cfg.AlertCallsOsExit {
							tr.SendEOFExpectNoEvents(t)
							return
						}
						fallthrough
					default:
						err = tr.Logger.Log(tc.Arg, nil)
					}

					mapped := tr.LevelMapping(tc.Arg)
					if !mapped.Enabled() {
						if err != logiface.ErrDisabled {
							t.Errorf(`expected logiface.ErrDisabled, got %v`, err)
						}
						tr.SendEOFExpectNoEvents(t)
						return
					}
					if err != nil {
						t.Errorf(`expected no error, got %v`, err)
					}

					if ev, ok := tr.ReceiveEvent(); !ok {
						t.Error(`expected event`)
					} else if ev.Level != mapped || cfg.LogsEmptyMessage != (ev.Message != nil) {
						t.Errorf(`unexepected event: %s`, ev)
					}

					tr.SendEOFExpectNoEvents(t)
				})
			})
		}
	})
	t.Run(`log lines with modifiers`, func(t *testing.T) {
		t.Parallel()
		cfg.RunTest(TestRequest[E]{
			Level: logiface.LevelTrace,
		}, func(tr TestResponse[E]) {
			useLogLevel := logiface.LevelCritical
			if !tr.LevelMapping(useLogLevel).Enabled() {
				useLogLevel = logiface.LevelInformational
				if !tr.LevelMapping(useLogLevel).Enabled() {
					t.Fatal(`logger should at least support info logging`)
				}
			}

			var expected []Event
			for _, template := range eventTemplates {
				template := template
				if err := tr.Logger.Log(useLogLevel, logiface.ModifierFunc[E](func(in E) error {
					ev := template(in)
					if ev.Level != useLogLevel {
						t.Error(`unexpected level`, ev.Level)
					}
					expected = append(expected, normalizeEvent(cfg, tr, ev))
					return nil
				})); err != nil {
					t.Fatalf(`expected no error, got %v`, err)
				}
			}

			var actual []Event
			for i := 0; i < len(eventTemplates); i++ {
				ev, ok := tr.ReceiveEvent()
				if !ok {
					break
				}
				actual = append(actual, ev)
			}
			tr.SendEOF()
			timer := time.NewTimer(tr.ReceiveTimeout)
			defer timer.Stop()
		ReceiveLoop:
			for {
				select {
				case <-timer.C:
					t.Fatal(`timeout`)
				case ev, ok := <-tr.Events:
					if !ok {
						break ReceiveLoop
					}
					actual = append(actual, ev)
				}
			}

			if !EventsEqual(expected, actual) {
				t.Errorf("unexpected events:\n%s", EventsDiff(expected, actual))
			}
		})
	})
}
