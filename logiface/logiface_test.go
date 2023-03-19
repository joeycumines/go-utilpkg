package logiface

import (
	"testing"
)

type (
	minimalEventMethods interface {
		Level() Level
		AddField(key string, val any)
	}
)

var (
	// compile time assertions

	_ EventFactory[Event]  = EventFactoryFunc[Event](nil)
	_ EventReleaser[Event] = EventReleaserFunc[Event](nil)
	_ Modifier[Event]      = ModifierFunc[Event](nil)
	_ Writer[Event]        = WriterFunc[Event](nil)
	_ Modifier[Event]      = ModifierSlice[Event](nil)
	_ Writer[Event]        = WriterSlice[Event](nil)
	_ Event                = struct {
		minimalEventMethods
		UnimplementedEvent
	}{}
)

func TestUnimplementedEvent_mustEmbedUnimplementedEvent(t *testing.T) {
	(UnimplementedEvent{}).mustEmbedUnimplementedEvent()
}
