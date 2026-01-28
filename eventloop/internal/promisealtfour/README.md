# PromiseAltFour

This package contains a "frozen snapshot" of the Main `eventloop.ChainedPromise` implementation as of Jan 2026.

## Purpose

The purpose of this package is to serve as a baseline for tournament comparisons. As the Main `eventloop` promise implementation evolves (e.g. adopting optimizations from `promisealtone`), this package will serve as a faithful representation of the "original" implementation to ensure we can accurately measure the impact of changes.

## Taxonomy

- **Main**: `eventloop.ChainedPromise` (The moving target)
- **AltOne**: `promisealtone` (Highly optimized, experimental variant)
- **AltTwo**: `promisealttwo` (Lock-free experiment)
- **AltThree**: `promisealtthree` (Another variant)
- **AltFour**: `promisealtfour` (Baseline snapshot of Main)

## Differences from Main

Although this package is a direct port of `eventloop.ChainedPromise`, there are necessary adaptation differences due to package visibility rules:

1.  **Direct Function Calls**: Constructors and combinators are standalone functions (e.g., `New(js)`) rather than methods on `*eventloop.JS`.
2.  **No Unhandled Rejection Tracking**: This implementation DOES NOT track unhandled rejections in the global `eventloop.JS` map because those fields are private. It strictly handles Promise/A+ logic without the global error reporting side effects.
3.  **Public Microtask API**: It uses `js.QueueMicrotask` exclusively, rather than internal hooks.

Aside from these structural adaptations, the core logic (struct fields, locking strategy, handler management, resolution algorithm) is identical to the Main implementation at the time of snapshotting.
