// Package gojabaseline provides a tournament adapter for the goja_nodejs eventloop.
//
// This adapter wraps github.com/dop251/goja_nodejs/eventloop to allow it to
// compete in the event loop tournament as the "Baseline" implementation.
// It serves as a reference implementation that our custom implementations
// must outperform to be considered viable.
package gojabaseline
