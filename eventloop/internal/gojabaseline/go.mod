module github.com/joeycumines/go-eventloop/internal/gojabaseline

go 1.26.6

require (
	github.com/joeycumines/goja v0.0.0-20260807074527-37ac99caa69a
	github.com/joeycumines/goja_nodejs v0.0.0-20260725224646-7b69489f6ce5
)

require (
	github.com/dlclark/regexp2/v2 v2.7.1 // indirect
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible // indirect
	github.com/google/pprof v0.0.0-20260802141513-ef3492d7dac3 // indirect
	golang.org/x/text v0.41.0 // indirect
)

replace github.com/joeycumines/goja => ../../../goja

replace github.com/joeycumines/goja_nodejs => ../../../goja_nodejs
