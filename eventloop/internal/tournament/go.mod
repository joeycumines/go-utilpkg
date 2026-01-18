module github.com/joeycumines/go-eventloop/internal/tournament

go 1.25.6

replace github.com/joeycumines/go-eventloop => ../..

replace github.com/joeycumines/go-eventloop/internal/gojabaseline => ../gojabaseline

require (
	github.com/joeycumines/go-eventloop v0.0.0-00010101000000-000000000000
	github.com/joeycumines/go-eventloop/internal/gojabaseline v0.0.0-00010101000000-000000000000
)

require (
	github.com/dlclark/regexp2 v1.11.5 // indirect
	github.com/dop251/goja v0.0.0-20260106131823-651366fbe6e3 // indirect
	github.com/dop251/goja_nodejs v0.0.0-20251015164255-5e94316bedaf // indirect
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible // indirect
	github.com/google/pprof v0.0.0-20260115054156-294ebfa9ad83 // indirect
	golang.org/x/sys v0.40.0 // indirect
	golang.org/x/text v0.33.0 // indirect
)
