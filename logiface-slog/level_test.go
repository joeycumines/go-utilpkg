package slog

import (
	"log/slog"
	"testing"

	"github.com/joeycumines/logiface"
)

// TestToSlogLevel_Trace tests Trace level mapping
func TestToSlogLevel_Trace(t *testing.T) {
	result := toSlogLevel(logiface.LevelTrace)
	if result != slog.LevelDebug {
		t.Errorf("toSlogLevel(LevelTrace) = %v, expected slog.LevelDebug", result)
	}
}

// TestToSlogLevel_Debug tests Debug level mapping
func TestToSlogLevel_Debug(t *testing.T) {
	result := toSlogLevel(logiface.LevelDebug)
	if result != slog.LevelDebug {
		t.Errorf("toSlogLevel(LevelDebug) = %v, expected slog.LevelDebug", result)
	}
}

// TestToSlogLevel_Informational tests Informational level mapping
func TestToSlogLevel_Informational(t *testing.T) {
	result := toSlogLevel(logiface.LevelInformational)
	if result != slog.LevelInfo {
		t.Errorf("toSlogLevel(LevelInformational) = %v, expected slog.LevelInfo", result)
	}
}

// TestToSlogLevel_Notice tests Notice level mapping
func TestToSlogLevel_Notice(t *testing.T) {
	result := toSlogLevel(logiface.LevelNotice)
	if result != slog.LevelWarn {
		t.Errorf("toSlogLevel(LevelNotice) = %v, expected slog.LevelWarn", result)
	}
}

// TestToSlogLevel_Warning tests Warning level mapping
func TestToSlogLevel_Warning(t *testing.T) {
	result := toSlogLevel(logiface.LevelWarning)
	if result != slog.LevelWarn {
		t.Errorf("toSlogLevel(LevelWarning) = %v, expected slog.LevelWarn", result)
	}
}

// TestToSlogLevel_Error tests Error level mapping
func TestToSlogLevel_Error(t *testing.T) {
	result := toSlogLevel(logiface.LevelError)
	if result != slog.LevelError {
		t.Errorf("toSlogLevel(LevelError) = %v, expected slog.LevelError", result)
	}
}

// TestToSlogLevel_Critical tests Critical level mapping
func TestToSlogLevel_Critical(t *testing.T) {
	result := toSlogLevel(logiface.LevelCritical)
	if result != slog.LevelError {
		t.Errorf("toSlogLevel(LevelCritical) = %v, expected slog.LevelError", result)
	}
}

// TestToSlogLevel_Alert tests Alert level mapping (missed in original coverage)
func TestToSlogLevel_Alert(t *testing.T) {
	result := toSlogLevel(logiface.LevelAlert)
	if result != slog.LevelError {
		t.Errorf("toSlogLevel(LevelAlert) = %v, expected slog.LevelError", result)
	}
}

// TestToSlogLevel_Emergency tests Emergency level mapping (missed in original coverage)
func TestToSlogLevel_Emergency(t *testing.T) {
	result := toSlogLevel(logiface.LevelEmergency)
	if result != slog.LevelError {
		t.Errorf("toSlogLevel(LevelEmergency) = %v, expected slog.LevelError", result)
	}
}

// TestToSlogLevel_CustomPositiveLevel tests custom positive levels (> LevelEmergency)
func TestToSlogLevel_CustomPositiveLevel(t *testing.T) {
	// LevelEmergency is 8, test level 9
	customLevel := logiface.Level(9)
	result := toSlogLevel(customLevel)
	if result != slog.LevelError {
		t.Errorf("toSlogLevel(9) = %v, expected slog.LevelError (default for positive > Emergency)", result)
	}

	// Test level 100
	customLevel = logiface.Level(100)
	result = toSlogLevel(customLevel)
	if result != slog.LevelError {
		t.Errorf("toSlogLevel(100) = %v, expected slog.LevelError (default for high positive)", result)
	}
}

// TestToSlogLevel_NegativeLevel tests negative logiface levels
func TestToSlogLevel_NegativeLevel(t *testing.T) {
	// Negative levels default to Error (safe fallback)
	negativeLevel := logiface.Level(-1)
	result := toSlogLevel(negativeLevel)
	if result != slog.LevelError {
		t.Errorf("toSlogLevel(-1) = %v, expected slog.LevelError (default fallback)", result)
	}

	negativeLevel = logiface.Level(-100)
	result = toSlogLevel(negativeLevel)
	if result != slog.LevelError {
		t.Errorf("toSlogLevel(-100) = %v, expected slog.LevelError (default fallback)", result)
	}
}

// TestToSlogLevel_ZeroLevel tests zero level (LevelDisabled)
func TestToSlogLevel_ZeroLevel(t *testing.T) {
	result := toSlogLevel(logiface.LevelDisabled)
	if result != slog.LevelError {
		t.Errorf("toSlogLevel(LevelDisabled/0) = %v, expected slog.LevelError (default fallback)", result)
	}
}

// TestToLogifaceLevel_Debug tests slog.LevelDebug mapping
func TestToLogifaceLevel_Debug(t *testing.T) {
	result := toLogifaceLevel(slog.LevelDebug)
	if result != logiface.LevelDebug {
		t.Errorf("toLogifaceLevel(slog.LevelDebug) = %v, expected logiface.LevelDebug", result)
	}
}

// TestToLogifaceLevel_Info tests slog.LevelInfo mapping
func TestToLogifaceLevel_Info(t *testing.T) {
	result := toLogifaceLevel(slog.LevelInfo)
	if result != logiface.LevelInformational {
		t.Errorf("toLogifaceLevel(slog.LevelInfo) = %v, expected logiface.LevelInformational", result)
	}
}

// TestToLogifaceLevel_Warn tests slog.LevelWarn mapping
func TestToLogifaceLevel_Warn(t *testing.T) {
	result := toLogifaceLevel(slog.LevelWarn)
	if result != logiface.LevelNotice {
		t.Errorf("toLogifaceLevel(slog.LevelWarn) = %v, expected logiface.LevelNotice", result)
	}
}

// TestToLogifaceLevel_Error tests slog.LevelError mapping
func TestToLogifaceLevel_Error(t *testing.T) {
	result := toLogifaceLevel(slog.LevelError)
	if result != logiface.LevelError {
		t.Errorf("toLogifaceLevel(slog.LevelError) = %v, expected logiface.LevelError", result)
	}
}

// TestToLogifaceLevel_DynamicLevelNegativeOne tests dynamic level -1
func TestToLogifaceLevel_DynamicLevelNegativeOne(t *testing.T) {
	result := toLogifaceLevel(-1)
	if result != logiface.LevelDebug {
		t.Errorf("toLogifaceLevel(-1) = %v, expected logiface.LevelDebug (dynamic level)", result)
	}
}

// TestToLogifaceLevel_DynamicLevelNegativeMore tests other negative dynamic levels
func TestToLogifaceLevel_DynamicLevelNegativeMore(t *testing.T) {
	result := toLogifaceLevel(-5)
	if result != logiface.LevelDebug {
		t.Errorf("toLogifaceLevel(-5) = %v, expected logiface.LevelDebug (negative dynamic)", result)
	}

	result = toLogifaceLevel(-10)
	if result != logiface.LevelDebug {
		t.Errorf("toLogifaceLevel(-10) = %v, expected logiface.LevelDebug (negative dynamic)", result)
	}
}

// TestToLogifaceLevel_BetweenDebugAndInfo tests level between Debug (-4/7) and Info (0/6)
func TestToLogifaceLevel_BetweenDebugAndInfo(t *testing.T) {
	// slog.Level(-4) maps to logiface.LevelDebug (7)
	// slog.Level(0) maps to logiface.LevelInformational (6)
	// Levels in default case: int(lvl) <= int(LevelDebug=7) returns LevelDebug
	for i := -3; i < 0; i++ {
		result := toLogifaceLevel(slog.Level(i))
		if result != logiface.LevelDebug {
			t.Errorf("toLogifaceLevel(%d) = %v, expected logiface.LevelDebug", i, result)
		}
	}
}

// TestToLogifaceLevel_BetweenInfoAndWarn tests level between Info (0/6) and Warn (4/5)
func TestToLogifaceLevel_BetweenInfoAndWarn(t *testing.T) {
	// slog.Level(0) = slog.LevelInfo maps to logiface.LevelInformational (6)
	// slog.Level(4) = slog.LevelWarn maps to logiface.LevelNotice (5)
	// In the default case:
	//   - lvl=1,2,3: int(lvl) <= int(LevelDebug=7) returns LevelDebug

	result := toLogifaceLevel(slog.Level(1))
	expected := logiface.LevelDebug
	if result != expected {
		t.Errorf("toLogifaceLevel(1) = %v, expected %v (Debug)", result, expected)
	}

	result = toLogifaceLevel(slog.Level(2))
	if result != expected {
		t.Errorf("toLogifaceLevel(2) = %v, expected %v (Debug)", result, expected)
	}

	result = toLogifaceLevel(slog.Level(3))
	if result != expected {
		t.Errorf("toLogifaceLevel(3) = %v, expected %v (Debug)", result, expected)
	}
}

// TestToLogifaceLevel_BetweenWarnAndError tests level between Warn (4/5) and Error (8/3)
func TestToLogifaceLevel_BetweenWarnAndError(t *testing.T) {
	// slog.Level(4) = slog.LevelWarn maps to logiface.LevelNotice (5)
	// slog.Level(8) = slog.LevelError maps to logiface.LevelError (3)
	// In the default case:
	//   - lvl=5,6,7: int(lvl) <= int(LevelDebug=7) returns LevelDebug

	for i := 5; i <= 7; i++ {
		result := toLogifaceLevel(slog.Level(i))
		expected := logiface.LevelDebug
		if result != expected {
			t.Errorf("toLogifaceLevel(%d) = %v, expected %v (Debug)", i, result, expected)
		}
	}
}

// TestToLogifaceLevel_AboveError tests levels above Error (8)
func TestToLogifaceLevel_AboveError(t *testing.T) {
	// slog.LevelError is 8, logiface.LevelError is 7
	// Levels above 8 should map to LevelError
	result := toLogifaceLevel(slog.Level(9))
	if result != logiface.LevelError {
		t.Errorf("toLogifaceLevel(9) = %v, expected logiface.LevelError", result)
	}

	result = toLogifaceLevel(slog.Level(10))
	if result != logiface.LevelError {
		t.Errorf("toLogifaceLevel(10) = %v, expected logiface.LevelError", result)
	}

	result = toLogifaceLevel(slog.Level(100))
	if result != logiface.LevelError {
		t.Errorf("toLogifaceLevel(100) = %v, expected logiface.LevelError", result)
	}
}

// TestToLogifaceLevel_Zero tests zero level (slog.LevelInfo)
func TestToLogifaceLevel_Zero(t *testing.T) {
	result := toLogifaceLevel(slog.Level(0))

	// slog.Level(0) equals slog.LevelInfo, which maps to LevelInformational
	if result != logiface.LevelInformational {
		t.Errorf("toLogifaceLevel(0) = %v, expected logiface.LevelInformational", result)
	}
}

// TestLevelMapping_RoundTrip_Debug tests round-trip for Debug level
func TestLevelMapping_RoundTrip_Debug(t *testing.T) {
	// logiface LevelDebug -> slog LevelDebug -> logiface LevelDebug
	slogLevel := toSlogLevel(logiface.LevelDebug)
	back := toLogifaceLevel(slogLevel)
	if back != logiface.LevelDebug {
		t.Errorf("Round trip LevelDebug: logiface.LevelDebug -> %v -> %v, expected logiface.LevelDebug",
			slogLevel, back)
	}
}

// TestLevelMapping_RoundTrip_Informational tests round-trip for Informational level
func TestLevelMapping_RoundTrip_Informational(t *testing.T) {
	// logiface LevelInformational -> slog LevelInfo -> logiface LevelInformational
	slogLevel := toSlogLevel(logiface.LevelInformational)
	back := toLogifaceLevel(slogLevel)
	if back != logiface.LevelInformational {
		t.Errorf("Round trip LevelInformational: logiface.LevelInformational -> %v -> %v, expected logiface.LevelInformational",
			slogLevel, back)
	}
}

// TestLevelMapping_RoundTrip_Warning tests round-trip for Warning level
func TestLevelMapping_RoundTrip_Warning(t *testing.T) {
	// logiface LevelWarning -> slog LevelWarn -> logiface LevelNotice
	// Note: Warning maps to Warn, which maps back to Notice (precision loss)
	slogLevel := toSlogLevel(logiface.LevelWarning)
	back := toLogifaceLevel(slogLevel)
	if back != logiface.LevelNotice {
		t.Errorf("Round trip LevelWarning: logiface.LevelWarning -> %v -> %v, expected logiface.LevelNotice (precision loss expected)",
			slogLevel, back)
	}
}

// TestLevelMapping_RoundTrip_Error tests round-trip for Error level
func TestLevelMapping_RoundTrip_Error(t *testing.T) {
	// logiface LevelError -> slog LevelError -> logiface LevelError
	slogLevel := toSlogLevel(logiface.LevelError)
	back := toLogifaceLevel(slogLevel)
	if back != logiface.LevelError {
		t.Errorf("Round trip LevelError: logiface.LevelError -> %v -> %v, expected logiface.LevelError",
			slogLevel, back)
	}
}

// TestLevelMapping_AlertToErrorNotice tests Alert maps to Error, which maps back to Warning (not Notice)
func TestLevelMapping_AlertToErrorNotice(t *testing.T) {
	// logiface LevelAlert -> slog LevelError -> logiface LevelError
	// This is correct - Alert is above Error in severity
	slogLevel := toSlogLevel(logiface.LevelAlert)
	back := toLogifaceLevel(slogLevel)
	if back != logiface.LevelError {
		t.Errorf("LevelAlert mapping: %v -> %v -> %v, expected LevelError", logiface.LevelAlert, slogLevel, back)
	}
}
