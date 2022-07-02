package log

type (
	// Logger is the logging interface used by this module.
	// It's a subset of logrus.FieldLogger.
	Logger interface {
		WithField(key string, value any) Logger
		WithFields(fields map[string]any) Logger
		WithError(err error) Logger
		Debug(args ...any)
		Info(args ...any)
		Warn(args ...any)
		Error(args ...any)
	}

	// Discard implements a Logger that does nothing.
	Discard struct{}
)

var (
	_ Logger = Discard{}
)

func (Discard) WithField(string, any) Logger     { return Discard{} }
func (Discard) WithFields(map[string]any) Logger { return Discard{} }
func (Discard) WithError(error) Logger           { return Discard{} }
func (Discard) Debug(...any)                     {}
func (Discard) Info(...any)                      {}
func (Discard) Warn(...any)                      {}
func (Discard) Error(...any)                     {}
