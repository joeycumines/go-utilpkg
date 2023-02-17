package logiface

import (
	"fmt"
	"math"
	"testing"
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

func ExampleLevel_Custom() {
	p := func(level Level) {
		fmt.Printf("%q (%d): %v\n", level, level, level.Custom())
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
	//"emerg" (0): false
	//"alert" (1): false
	//"crit" (2): false
	//"err" (3): false
	//"warning" (4): false
	//"notice" (5): false
	//"info" (6): false
	//"debug" (7): false
	//"trace" (8): false
	//"9" (9): true
	//"127" (127): true
}

func ExampleLevel_Syslog() {
	p := func(level Level) {
		fmt.Printf("%q (%d): %v\n", level, level, level.Syslog())
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
	//"trace" (8): false
	//"9" (9): false
	//"127" (127): false
}

func TestLoggerFactory_LevelDisabled(t *testing.T) {
	if v := (LoggerFactory[*mockEvent]{}).LevelDisabled(); v != LevelDisabled {
		t.Errorf("unexpected value: %v", v)
	}
}

func TestLoggerFactory_LevelEmergency(t *testing.T) {
	if v := (LoggerFactory[*mockEvent]{}).LevelEmergency(); v != LevelEmergency {
		t.Errorf("unexpected value: %v", v)
	}
}

func TestLoggerFactory_LevelAlert(t *testing.T) {
	if v := (LoggerFactory[*mockEvent]{}).LevelAlert(); v != LevelAlert {
		t.Errorf("unexpected value: %v", v)
	}
}

func TestLoggerFactory_LevelCritical(t *testing.T) {
	if v := (LoggerFactory[*mockEvent]{}).LevelCritical(); v != LevelCritical {
		t.Errorf("unexpected value: %v", v)
	}
}

func TestLoggerFactory_LevelError(t *testing.T) {
	if v := (LoggerFactory[*mockEvent]{}).LevelError(); v != LevelError {
		t.Errorf("unexpected value: %v", v)
	}
}

func TestLoggerFactory_LevelWarning(t *testing.T) {
	if v := (LoggerFactory[*mockEvent]{}).LevelWarning(); v != LevelWarning {
		t.Errorf("unexpected value: %v", v)
	}
}

func TestLoggerFactory_LevelNotice(t *testing.T) {
	if v := (LoggerFactory[*mockEvent]{}).LevelNotice(); v != LevelNotice {
		t.Errorf("unexpected value: %v", v)
	}
}

func TestLoggerFactory_LevelInformational(t *testing.T) {
	if v := (LoggerFactory[*mockEvent]{}).LevelInformational(); v != LevelInformational {
		t.Errorf("unexpected value: %v", v)
	}
}

func TestLoggerFactory_LevelDebug(t *testing.T) {
	if v := (LoggerFactory[*mockEvent]{}).LevelDebug(); v != LevelDebug {
		t.Errorf("unexpected value: %v", v)
	}
}

func TestLoggerFactory_LevelTrace(t *testing.T) {
	if v := (LoggerFactory[*mockEvent]{}).LevelTrace(); v != LevelTrace {
		t.Errorf("unexpected value: %v", v)
	}
}
