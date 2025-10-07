//go:build unix

package prompt

import (
	"syscall"
)

const syscallSIGWINCH = syscall.SIGWINCH
