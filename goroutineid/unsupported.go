//go:build !((go1.26 && !go1.27 && amd64 && !js && !tinygo && gc) || (go1.26 && !go1.27 && arm64 && !js && !tinygo && gc))

// NOTICE: This build constraint is the exact inverse of supported.go's.

package goroutineid

func getGoIDImpl() int64 {
	return -1
}
