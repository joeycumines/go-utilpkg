module github.com/joeycumines/goja-protojson

go 1.26.2

require (
	github.com/joeycumines/goja v0.0.0-20260508000000-cf4c54bba590
	github.com/joeycumines/goja-protobuf v0.0.0-20260331032401-b5c5be7a30d3
	github.com/joeycumines/goja_nodejs v0.0.0-20260508000000-6add961f94bd
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/dlclark/regexp2 v1.11.5 // indirect
	github.com/dlclark/regexp2/v2 v2.5.2 // indirect
	github.com/dop251/goja v0.0.0-20260311135729-065cd970411c // indirect
	github.com/dop251/goja_nodejs v0.0.0-20260212111938-1f56ff5bcf14 // indirect
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible // indirect
	github.com/google/pprof v0.0.0-20260402051712-545e8a4df936 // indirect
	golang.org/x/text v0.37.0 // indirect
)

replace github.com/joeycumines/goja => ../goja

replace github.com/joeycumines/goja_nodejs => ../goja_nodejs
