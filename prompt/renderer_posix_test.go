//go:build unix

package prompt

import (
	"syscall"
	"testing"
)

func TestBreakLineCallback(t *testing.T) {
	var i int
	r := NewRenderer()
	r.out = &PosixWriter{
		fd: syscall.Stdin, // "write" to stdin just so we don't mess with the output of the tests
	}
	r.col = 1
	r.row = 1 // must be > 0, matching the r.row <= 0 guard in BreakLine
	b := NewBuffer()
	r.BreakLine(b, nil)

	if i != 0 {
		t.Errorf("i should initially be 0, before applying a break line callback")
	}

	r.breakLineCallback = func(doc *Document) {
		i++
	}
	r.BreakLine(b, nil)
	r.BreakLine(b, nil)
	r.BreakLine(b, nil)

	if i != 3 {
		t.Errorf("BreakLine callback not called, i should be 3")
	}
}
