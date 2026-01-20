package eventloop

import (
	"fmt"
	"testing"
	"unsafe"
)

// Analyze intervalState alignment
func TestIntervalStateAlign(t *testing.T) {
	s := &intervalState{}
	_ = s // Use s to avoid staticcheck warning

	fmt.Printf("=== intervalState ===\n")
	fmt.Printf("canceled: offset=%d, size=%d\n", unsafe.Offsetof(s.canceled), unsafe.Sizeof(s.canceled))
	fmt.Printf("delayMs: offset=%d, size=%d\n", unsafe.Offsetof(s.delayMs), unsafe.Sizeof(s.delayMs))
	fmt.Printf("currentLoopTimerID: offset=%d, size=%d\n", unsafe.Offsetof(s.currentLoopTimerID), unsafe.Sizeof(s.currentLoopTimerID))
	fmt.Printf("fn: offset=%d, size=%d\n", unsafe.Offsetof(s.fn), unsafe.Sizeof(s.fn))
	fmt.Printf("wrapper: offset=%d, size=%d\n", unsafe.Offsetof(s.wrapper), unsafe.Sizeof(s.wrapper))
	fmt.Printf("js: offset=%d, size=%d\n", unsafe.Offsetof(s.js), unsafe.Sizeof(s.js))
	fmt.Printf("m: offset=%d, size=%d\n", unsafe.Offsetof(s.m), unsafe.Sizeof(s.m))
	fmt.Printf("Total: %d bytes\n", unsafe.Sizeof(s))
	fmt.Printf("\n")
}

// Analyze JS struct alignment
func TestJSAlign(t *testing.T) {
	s := &JS{}
	_ = s // Use s to avoid staticcheck warning

	fmt.Printf("=== JS ===\n")
	fmt.Printf("nextTimerID: offset=%d, size=%d\n", unsafe.Offsetof(s.nextTimerID), unsafe.Sizeof(s.nextTimerID))
	fmt.Printf("loop: offset=%d, size=%d\n", unsafe.Offsetof(s.loop), unsafe.Sizeof(s.loop))
	fmt.Printf("unhandledCallback: offset=%d, size=%d\n", unsafe.Offsetof(s.unhandledCallback), unsafe.Sizeof(s.unhandledCallback))
	fmt.Printf("timers: offset=%d, size=%d\n", unsafe.Offsetof(s.timers), unsafe.Sizeof(s.timers))
	fmt.Printf("intervals: offset=%d, size=%d\n", unsafe.Offsetof(s.intervals), unsafe.Sizeof(s.intervals))
	fmt.Printf("unhandledRejections: offset=%d, size=%d\n", unsafe.Offsetof(s.unhandledRejections), unsafe.Sizeof(s.unhandledRejections))
	fmt.Printf("promiseHandlers: offset=%d, size=%d\n", unsafe.Offsetof(s.promiseHandlers), unsafe.Sizeof(s.promiseHandlers))
	fmt.Printf("mu: offset=%d, size=%d\n", unsafe.Offsetof(s.mu), unsafe.Sizeof(s.mu))
	fmt.Printf("Total: %d bytes\n", unsafe.Sizeof(*s))
	fmt.Printf("\n")
}

// Analyze Loop struct alignment (partial - key fields)
func TestLoopAlign(t *testing.T) {
	s := &Loop{}
	_ = s // Use s to avoid staticcheck warning

	fmt.Printf("=== Loop (key fields) ===\n")
	fmt.Printf("nextTimerID: offset=%d, size=%d\n", unsafe.Offsetof(s.nextTimerID), unsafe.Sizeof(s.nextTimerID))
	fmt.Printf("tickElapsedTime: offset=%d, size=%d\n", unsafe.Offsetof(s.tickElapsedTime), unsafe.Sizeof(s.tickElapsedTime))
	fmt.Printf("loopGoroutineID: offset=%d, size=%d\n", unsafe.Offsetof(s.loopGoroutineID), unsafe.Sizeof(s.loopGoroutineID))
	fmt.Printf("userIOFDCount: offset=%d, size=%d\n", unsafe.Offsetof(s.userIOFDCount), unsafe.Sizeof(s.userIOFDCount))
	fmt.Printf("wakeUpSignalPending: offset=%d, size=%d\n", unsafe.Offsetof(s.wakeUpSignalPending), unsafe.Sizeof(s.wakeUpSignalPending))
	fmt.Printf("forceNonBlockingPoll: offset=%d, size=%d\n", unsafe.Offsetof(s.forceNonBlockingPoll), unsafe.Sizeof(s.forceNonBlockingPoll))
	fmt.Printf("StrictMicrotaskOrdering: offset=%d, size=%d\n", unsafe.Offsetof(s.StrictMicrotaskOrdering), unsafe.Sizeof(s.StrictMicrotaskOrdering))
	fmt.Printf("Total: %d bytes\n", unsafe.Sizeof(*s))
	fmt.Printf("\n")
}

// Analyze ChainedPromise alignment
func TestChainedPromiseAlign(t *testing.T) {
	s := &ChainedPromise{}
	_ = s // Use s to avoid staticcheck warning

	fmt.Printf("=== ChainedPromise ===\n")
	fmt.Printf("id: offset=%d, size=%d\n", unsafe.Offsetof(s.id), unsafe.Sizeof(s.id))
	fmt.Printf("js: offset=%d, size=%d\n", unsafe.Offsetof(s.js), unsafe.Sizeof(s.js))
	fmt.Printf("state: offset=%d, size=%d\n", unsafe.Offsetof(s.state), unsafe.Sizeof(s.state))
	fmt.Printf("mu: offset=%d, size=%d\n", unsafe.Offsetof(s.mu), unsafe.Sizeof(s.mu))
	fmt.Printf("value: offset=%d, size=%d\n", unsafe.Offsetof(s.value), unsafe.Sizeof(s.value))
	fmt.Printf("reason: offset=%d, size=%d\n", unsafe.Offsetof(s.reason), unsafe.Sizeof(s.reason))
	fmt.Printf("handlers: offset=%d, size=%d\n", unsafe.Offsetof(s.handlers), unsafe.Sizeof(s.handlers))
	fmt.Printf("Total: %d bytes\n", unsafe.Sizeof(s))
	fmt.Printf("\n")
}
