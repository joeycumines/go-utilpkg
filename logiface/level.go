package logiface

import (
	"strconv"
)

const (
	LevelDisabled Level = iota - 1
	LevelEmergency
	LevelAlert
	LevelCritical
	LevelError
	LevelWarning
	LevelNotice
	LevelInformational
	LevelDebug
	LevelTrace
)

type (
	// Level models the severity level of a log message.
	//
	// Valid Level values include all the syslog log levels, as defined in
	// RFC 5424, with the addition of a "trace" level (LevelTrace), which is
	// expected to use abnormal output mechanisms (e.g. a separate log file).
	// Negative values are treated as disabled, see also LevelDisabled.
	//
	// Severity level
	// The list of severities is also described by the standard:
	//
	// Value	Severity	Keyword	Deprecated keywords	Description	Condition
	// 0	Emergency	emerg	panic[9]	System is unusable	A panic condition.[10]
	// 1	Alert	alert		Action must be taken immediately	A condition that should be corrected immediately, such as a corrupted system database.[10]
	// 2	Critical	crit		Critical conditions	Hard device errors.[10]
	// 3	Error	err	error[9]	Error conditions
	// 4	Warning	warning	warn[9]	Warning conditions
	// 5	Notice	notice		Normal but significant conditions	Conditions that are not error conditions, but that may require special handling.[10]
	// 6	Informational	info		Informational messages	Confirmation that the program is working as expected.
	// 7	Debug	debug		Debug-level messages	Messages that contain information normally of use only when debugging a program.[10]
	//
	// [9] https://linux.die.net/man/5/syslog.conf
	// [10] https://pubs.opengroup.org/onlinepubs/009695399/functions/syslog.html
	Level int8
)

// String implements fmt.Stringer, note that it uses the short keyword (for the actual syslog levels).
func (x Level) String() string {
	switch x {
	case LevelDisabled:
		return "disabled"
	case LevelEmergency:
		return "emerg"
	case LevelAlert:
		return "alert"
	case LevelCritical:
		return "crit"
	case LevelError:
		return "err"
	case LevelWarning:
		return "warning"
	case LevelNotice:
		return "notice"
	case LevelInformational:
		return "info"
	case LevelDebug:
		return "debug"
	case LevelTrace:
		return "trace"
	default:
		return strconv.FormatInt(int64(x), 10)
	}
}

// Enabled returns true if the Level is enabled (greater than or equal to 0).
func (x Level) Enabled() bool { return x > LevelDisabled }

// for convenience, expose the level enums as methods on LoggerFactory

// LevelDisabled returns LevelDisabled, and is provided as a convenience for
// implementation packages, so end users don't have to import logiface.
func (LoggerFactory[E]) LevelDisabled() Level { return LevelDisabled }

// LevelEmergency returns LevelEmergency, and is provided as a convenience for
// implementation packages, so end users don't have to import logiface.
func (LoggerFactory[E]) LevelEmergency() Level { return LevelEmergency }

// LevelAlert returns LevelAlert, and is provided as a convenience for
// implementation packages, so end users don't have to import logiface.
func (LoggerFactory[E]) LevelAlert() Level { return LevelAlert }

// LevelCritical returns LevelCritical, and is provided as a convenience for
// implementation packages, so end users don't have to import logiface.
func (LoggerFactory[E]) LevelCritical() Level { return LevelCritical }

// LevelError returns LevelError, and is provided as a convenience for
// implementation packages, so end users don't have to import logiface.
func (LoggerFactory[E]) LevelError() Level { return LevelError }

// LevelWarning returns LevelWarning, and is provided as a convenience for
// implementation packages, so end users don't have to import logiface.
func (LoggerFactory[E]) LevelWarning() Level { return LevelWarning }

// LevelNotice returns LevelNotice, and is provided as a convenience for
// implementation packages, so end users don't have to import logiface.
func (LoggerFactory[E]) LevelNotice() Level { return LevelNotice }

// LevelInformational returns LevelInformational, and is provided as a convenience for
// implementation packages, so end users don't have to import logiface.
func (LoggerFactory[E]) LevelInformational() Level { return LevelInformational }

// LevelDebug returns LevelDebug, and is provided as a convenience for
// implementation packages, so end users don't have to import logiface.
func (LoggerFactory[E]) LevelDebug() Level { return LevelDebug }

// LevelTrace returns LevelTrace, and is provided as a convenience for
// implementation packages, so end users don't have to import logiface.
func (LoggerFactory[E]) LevelTrace() Level { return LevelTrace }
