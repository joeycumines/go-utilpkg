package logiface

type (
	minimalEventMethods interface {
		Level() Level
		AddField(key string, val any)
	}
)

var (
	// compile time assertions

	_ Event = struct {
		minimalEventMethods
		UnimplementedEvent
	}{}
)
