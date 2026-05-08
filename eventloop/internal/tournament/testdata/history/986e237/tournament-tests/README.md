# Replaced immutable-base tournament tests

These are the exact seven tournament test files present at eventloop commit `986e2378c1484aa917a1bb0fd13aef914bdce50f` but absent from the current compiled package. They remain source evidence and are mapped by the manifest to stronger deterministic replacements. They are not recompiled because their sleeps, tolerances, ignored errors, and inaccurate Goja-oriented names would reintroduce weaker contracts.

The historical wake test did not force the check-to-sleep boundary. Current Main proves that boundary through `TestIngressSleepBoundarySkipsNativeWait`; historical variants remain explicitly unproved until they expose equivalent instrumentation.
