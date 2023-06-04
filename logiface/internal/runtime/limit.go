package runtime

import (
	"path/filepath"
	"runtime"
)

type (
	Caller struct {
		Function string
		File     string
		Entry    uintptr
		Line     int
	}
)

func CallerSkipPackage(pkgPath string, i int) Caller {
	const size = 1 << 4
	var (
		callers = make([]uintptr, size)
		frames  *runtime.Frames
		frame   runtime.Frame
		ok      bool
	)
CallerLoop:
	for i += 2; i > 0; i += size {
		callers = callers[:runtime.Callers(i, callers[:])]
		frames = runtime.CallersFrames(callers)
		for frame, ok = frames.Next(); ok; frame, ok = frames.Next() {
			if pkgPath == `` || filepath.Dir(frame.File) != pkgPath {
				break CallerLoop
			}
		}
		if len(callers) != size {
			break
		}
	}
	if ok {
		return Caller{
			Function: frame.Function,
			File:     frame.File,
			Entry:    frame.Entry,
			Line:     frame.Line,
		}
	}
	return Caller{}
}
