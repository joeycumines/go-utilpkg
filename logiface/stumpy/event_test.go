package stumpy

import (
	"github.com/joeycumines/go-utilpkg/logiface"
)

var (
	// compile time assertions

	_ logiface.Event = (*Event)(nil)
)
