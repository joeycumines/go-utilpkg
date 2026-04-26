//go:build (go1.26 && !go1.27 && amd64 && !js && !tinygo && gc) || (go1.26 && !go1.27 && arm64 && !js && !tinygo && gc)

// NOTICE: This build constraint is logically "fast_amd64.s OR fast_arm64.s".

package goroutineid

func getGoIDImpl() int64
