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

	_ Event = struct {
		minimalEventMethods
		UnimplementedEvent
	}{}
)

func TestUnimplementedEvent_mustEmbedUnimplementedEvent(t *testing.T) {
	(UnimplementedEvent{}).mustEmbedUnimplementedEvent()
}
