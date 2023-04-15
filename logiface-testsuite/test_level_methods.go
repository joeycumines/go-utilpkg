package testsuite

import (
	"github.com/joeycumines/go-utilpkg/logiface"
	"math"
	"strconv"
	"testing"
	"time"
)

// TestLevelMethods is part of TestSuite, it tests the level methods on the logger.
func TestLevelMethods[E logiface.Event](t *testing.T, cfg Config[E]) {
	t.Run(`sequential simple events`, func(t *testing.T) {
		t.Parallel()
		cfg.RunTest(TestRequest[E]{
			Level: logiface.LevelTrace,
		}, func(tr TestResponse[E]) {
			expectedIfEnabled := func(lvl logiface.Level, msg string) {
				t.Helper()
				lvl = tr.LevelMapping(lvl)
				if !lvl.Enabled() {
					return
				}
				timer := time.NewTimer(tr.ReceiveTimeout)
				defer timer.Stop()
				select {
				case <-timer.C:
					t.Fatal(`timeout`)
				case event, ok := <-tr.Events:
					if !ok {
						t.Fatal(`expected message`)
					} else if event.Level != lvl || event.Message == nil || *event.Message != msg {
						if event.Message != nil {
							msg = *event.Message
						} else {
							msg = `nil`
						}
						t.Fatalf("unexpected event: level=%s, message=%q\n%s", event.Level, msg, msg)
					} else {
						t.Logf(`log event ok: level=%s, message=%q`, event.Level, *event.Message)
					}
				}
			}

			tr.Logger.Trace().Log(`msg 1`)
			expectedIfEnabled(logiface.LevelTrace, `msg 1`)

			tr.Logger.Debug().Log(`msg 2`)
			expectedIfEnabled(logiface.LevelDebug, `msg 2`)

			tr.Logger.Info().Log(`msg 3`)
			expectedIfEnabled(logiface.LevelInformational, `msg 3`)

			tr.Logger.Notice().Log(`msg 4`)
			expectedIfEnabled(logiface.LevelNotice, `msg 4`)

			tr.Logger.Warning().Log(`msg 5`)
			expectedIfEnabled(logiface.LevelWarning, `msg 5`)

			tr.Logger.Err().Log(`msg 6`)
			expectedIfEnabled(logiface.LevelError, `msg 6`)

			tr.Logger.Crit().Log(`msg 7`)
			expectedIfEnabled(logiface.LevelCritical, `msg 7`)

			if !cfg.AlertCallsOsExit {
				tr.Logger.Alert().Log(`msg 8`)
				expectedIfEnabled(logiface.LevelAlert, `msg 8`)
			}

			cfg.HandleEmergencyPanic(t, func() {
				tr.Logger.Emerg().Log(`msg 9`)
			})
			expectedIfEnabled(logiface.LevelEmergency, `msg 9`)

			// all the custom levels
			for level := int(logiface.LevelTrace) + 1; level <= math.MaxInt8; level++ {
				lvl := logiface.Level(level)
				msg := `msg ` + strconv.Itoa(level+1)
				tr.Logger.Build(lvl).Log(msg)
				expectedIfEnabled(lvl, msg)
			}

			tr.SendEOFExpectNoEvents(t)
		})
	})
	t.Run(`disabled log levels`, func(t *testing.T) {
		t.Parallel()
		for _, tc := range disabledLevelCombinations() {
			tc := tc
			t.Run(tc.Name, func(t *testing.T) {
				t.Parallel()
				cfg.RunTest(TestRequest[E]{
					Level: tc.Logger,
				}, func(tr TestResponse[E]) {
					if tr.Logger.Build(tc.Arg) != nil {
						t.Error(`expected nil builder as the level should have been disabled`)
					}
					tr.SendEOFExpectNoEvents(t)
				})
			})
		}
	})
}
