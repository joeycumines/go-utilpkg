package testsuite

import (
	"fmt"
	"github.com/joeycumines/logiface"
	"golang.org/x/exp/slices"
	"math"
)

func logLevelCombinations() (combinations map[struct {
	Logger logiface.Level
	Arg    logiface.Level
}]bool) {
	combinations = make(map[struct {
		Logger logiface.Level
		Arg    logiface.Level
	}]bool)
	for logger := math.MinInt8; logger <= math.MaxInt8; logger++ {
		for arg := math.MinInt8; arg <= math.MaxInt8; arg++ {
			a := func(enabled bool) {
				combinations[struct {
					Logger logiface.Level
					Arg    logiface.Level
				}{logiface.Level(logger), logiface.Level(arg)}] = enabled
			}

			switch {
			case arg > 8:
				// if the arg level is a custom one, it's enabled
				a(true)

			case logger < 0 || arg < 0:
				// for non-custom levels, if either are disabled, the log is disabled
				a(false)

			case logger == arg:
				// if they are equal, it's enabled
				a(true)

			default:
				// otherwise, it's just if arg <= logger
				a(arg <= logger)
			}
		}
	}
	return
}

func filterLevelCombinations(fn func(logger, arg logiface.Level, enabled bool) bool) (testCases []struct {
	Name   string
	Logger logiface.Level
	Arg    logiface.Level
}) {
	for v, enabled := range logLevelCombinations() {
		if !fn(v.Logger, v.Arg, enabled) {
			continue
		}
		testCases = append(testCases, struct {
			Name   string
			Logger logiface.Level
			Arg    logiface.Level
		}{
			Name:   fmt.Sprintf(`logger=%s arg=%s`, v.Logger, v.Arg),
			Logger: v.Logger,
			Arg:    v.Arg,
		})
	}
	slices.SortFunc[[]struct {
		Name   string
		Logger logiface.Level
		Arg    logiface.Level
	}](testCases, func(a, b struct {
		Name   string
		Logger logiface.Level
		Arg    logiface.Level
	}) int {
		if a.Name < b.Name {
			return -1
		} else if a.Name > b.Name {
			return 1
		}
		return 0
	})
	return
}

func disabledLevelCombinations() []struct {
	Name   string
	Logger logiface.Level
	Arg    logiface.Level
} {
	return filterLevelCombinations(func(logger, arg logiface.Level, enabled bool) bool { return !enabled })
}

func enabledLevelCombinations() []struct {
	Name   string
	Logger logiface.Level
	Arg    logiface.Level
} {
	return filterLevelCombinations(func(logger, arg logiface.Level, enabled bool) bool { return enabled })
}
