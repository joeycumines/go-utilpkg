//go:build go1.26 && !go1.27 && arm64 && !js && !tinygo && gc

// NOTICE: This build constraint is strict to mitigate future issues.

#include "textflag.h"

// getGoIDImpl returns the current goroutine's ID.
// On ARM64, the g pseudo-register holds the current goroutine pointer.
// We read g.goid at offset 152 (0x98) from the g pointer.
TEXT ·getGoIDImpl(SB), NOSPLIT, $0-8
	MOVD g, R0         // R0 = g (current goroutine pointer)
	MOVD 152(R0), R0   // R0 = g.goid (offset 152 = 0x98)
	MOVD R0, ret+0(FP) // return goid
	RET
