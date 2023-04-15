// Package stumpy implements full logiface support, as a JSON logger.
//
// It is intended as the "model" logger for the logiface package, and should be
// the most performant, by virtue of being the most direct. Internally, it
// appends to each event as a byte buffer, in a similar manner as zerolog.
package stumpy
