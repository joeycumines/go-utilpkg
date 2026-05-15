module github.com/joeycumines/goja-grpc

go 1.26.2

require (
	github.com/joeycumines/go-eventloop v0.0.0-20260428025403-c64a0733c558
	github.com/joeycumines/go-inprocgrpc v0.0.0-20260331032414-92dc1790fe75
	github.com/joeycumines/goja v0.0.0-20260508000000-cf4c54bba590
	github.com/joeycumines/goja-eventloop v0.0.0-20260331032353-b381e124657b
	github.com/joeycumines/goja-protobuf v0.0.0-20260331032401-b5c5be7a30d3
	github.com/joeycumines/goja-protojson v0.0.0-20260331032406-6db2ea2c9a56
	github.com/joeycumines/goja_nodejs v0.0.0-20260508000000-6add961f94bd
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260427160629-7cedc36a6bc4
	google.golang.org/grpc v1.82.1
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/dlclark/regexp2 v1.11.5 // indirect
	github.com/dlclark/regexp2/v2 v2.5.2 // indirect
	github.com/dop251/goja v0.0.0-20260311135729-065cd970411c // indirect
	github.com/dop251/goja_nodejs v0.0.0-20260212111938-1f56ff5bcf14 // indirect
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible // indirect
	github.com/google/pprof v0.0.0-20260402051712-545e8a4df936 // indirect
	github.com/joeycumines/go-catrate v0.0.0-20260429212737-202f4120003b // indirect
	github.com/joeycumines/goroutineid v1.1.0 // indirect
	github.com/joeycumines/logiface v0.5.0 // indirect
	golang.org/x/exp v0.0.0-20260410095643-746e56fc9e2f // indirect
	golang.org/x/net v0.54.0 // indirect
	golang.org/x/sys v0.44.0 // indirect
	golang.org/x/text v0.37.0 // indirect
)

replace github.com/joeycumines/goja => ../goja

replace github.com/joeycumines/goja_nodejs => ../goja_nodejs
