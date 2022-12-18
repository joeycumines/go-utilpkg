package logiface

import (
	"fmt"
	"math"
)

func ExampleLevel_Enabled() {
	p := func(level Level) {
		fmt.Printf("%q (%d): %v\n", level, level, level.Enabled())
	}
	p(math.MinInt8)
	p(-2)
	p(LevelDisabled)
	p(LevelEmergency)
	p(LevelAlert)
	p(LevelCritical)
	p(LevelError)
	p(LevelWarning)
	p(LevelNotice)
	p(LevelInformational)
	p(LevelDebug)
	p(LevelTrace)
	p(9)
	p(math.MaxInt8)
	//output:
	//"-128" (-128): false
	//"-2" (-2): false
	//"disabled" (-1): false
	//"emerg" (0): true
	//"alert" (1): true
	//"crit" (2): true
	//"err" (3): true
	//"warning" (4): true
	//"notice" (5): true
	//"info" (6): true
	//"debug" (7): true
	//"trace" (8): true
	//"9" (9): true
	//"127" (127): true
}
