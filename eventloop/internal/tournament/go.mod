module github.com/joeycumines/go-eventloop/internal/tournament

go 1.26.6

replace github.com/joeycumines/go-eventloop => ../..

replace github.com/joeycumines/go-eventloop/internal/gojabaseline => ../gojabaseline

require (
	github.com/joeycumines/go-eventloop v0.0.0-20260624075642-f9a45542db92
	github.com/joeycumines/go-eventloop/internal/gojabaseline v0.0.0-20260624075642-f9a45542db92
	github.com/joeycumines/goroutineid v1.1.0
	golang.org/x/sys v0.47.0
)

require (
	github.com/dlclark/regexp2/v2 v2.7.1 // indirect
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible // indirect
	github.com/google/pprof v0.0.0-20260802141513-ef3492d7dac3 // indirect
	github.com/joeycumines/go-catrate v0.0.0-20260429212737-202f4120003b // indirect
	github.com/joeycumines/goja v0.0.0-20260807074527-37ac99caa69a // indirect
	github.com/joeycumines/goja_nodejs v0.0.0-20260725224646-7b69489f6ce5 // indirect
	github.com/joeycumines/logiface v0.5.0 // indirect
	golang.org/x/exp v0.0.0-20260813180055-c1d0aacb2297 // indirect
	golang.org/x/text v0.41.0 // indirect
)

replace github.com/joeycumines/goja => ../../../goja

replace github.com/joeycumines/goja_nodejs => ../../../goja_nodejs
