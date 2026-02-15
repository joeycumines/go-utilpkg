package slog

import (
	"log/slog"

	"github.com/joeycumines/logiface"
)

// toSlogLevel converts logiface.Level to slog.Level.
//
// Mapping is lossy because slog has only 4 standard levels:
//   - logiface.LevelDebug → slog.LevelDebug
//   - logiface.LevelInformational → slog.LevelInfo
//   - logiface.LevelNotice, Warning → slog.LevelWarn
//   - logiface.LevelError, Critical, Alert, Emergency → slog.LevelError
//   - logiface.LevelTrace → slog.LevelDebug (extra verbose)
func toSlogLevel(lvl logiface.Level) slog.Level {
	switch lvl {
	case logiface.LevelTrace, logiface.LevelDebug:
		return slog.LevelDebug
	case logiface.LevelInformational:
		return slog.LevelInfo
	case logiface.LevelNotice, logiface.LevelWarning:
		return slog.LevelWarn
	case logiface.LevelError, logiface.LevelCritical,
		logiface.LevelAlert, logiface.LevelEmergency:
		return slog.LevelError
	default:
		// Handle custom levels (positive values > LevelEmergency)
		// Map everything else to Error as a safe default
		return slog.LevelError
	}
}

// toLogifaceLevel converts slog.Level to logiface.Level.
//
// Mapping attempts to find closest equivalent:
//   - slog.LevelDebug → logiface.LevelDebug
//   - slog.LevelInfo → logiface.LevelInformational
//   - slog.LevelWarn → logiface.LevelNotice
//   - slog.LevelError → logiface.LevelError
//   - slog.Level(-1) for dynamic → logiface.LevelDebug
func toLogifaceLevel(lvl slog.Level) logiface.Level {
	switch lvl {
	case slog.LevelDebug:
		return logiface.LevelDebug
	case slog.LevelInfo:
		return logiface.LevelInformational
	case slog.LevelWarn:
		return logiface.LevelNotice
	case slog.LevelError:
		return logiface.LevelError
	default:
		// Handle dynamic level (-1) and other values
		if lvl < 0 {
			return logiface.LevelDebug
		}
		// For positive levels, map to appropriate syslog level
		if int(lvl) <= int(logiface.LevelDebug) {
			return logiface.LevelDebug
		}
		if int(lvl) <= int(logiface.LevelInformational) {
			return logiface.LevelInformational
		}
		if int(lvl) <= int(logiface.LevelWarning) {
			return logiface.LevelWarning
		}
		return logiface.LevelError
	}
}
