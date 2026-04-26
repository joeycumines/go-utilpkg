//go:build go1.26 && !go1.27 && amd64 && !js && !tinygo && gc

// NOTICE: This build constraint is strict to mitigate future issues.

#include "textflag.h"

// getGoIDImpl returns the current goroutine's ID.
// On AMD64, the R14 register holds the current goroutine pointer (g).
// We read g.goid at offset 152 (0x98) from the g pointer.
TEXT ·getGoIDImpl(SB), NOSPLIT, $0-8
	MOVQ 0x98(R14), AX      // AX = g.goid (offset 152 = 0x98)
	MOVQ AX, ret+0(FP)      // return goid
	RET
