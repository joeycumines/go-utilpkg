//go:build tools
// +build tools

package tools

import (
	_ "golang.org/x/perf/cmd/benchstat"
	_ "golang.org/x/tools/cmd/godoc"
	_ "honnef.co/go/tools/cmd/staticcheck"
)
