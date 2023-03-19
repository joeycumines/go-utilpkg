package logiface

import (
	"os"
)

var (
	// OsExit is a variable that can be overridden to change the behavior of
	// fatal errors. It is set to [os.Exit] by default.
	OsExit = os.Exit
)
