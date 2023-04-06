package stumpy

import (
	"github.com/joeycumines/go-utilpkg/logiface"
)

var (
	// compile time assertions

	_ logiface.EventFactory[*Event]                = (*Logger)(nil)
	_ logiface.Writer[*Event]                      = (*Logger)(nil)
	_ logiface.EventReleaser[*Event]               = (*Logger)(nil)
	_ logiface.JSONSupport[*Event, *Event, *Event] = (*Logger)(nil)
)
