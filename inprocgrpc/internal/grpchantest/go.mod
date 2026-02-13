module github.com/joeycumines/go-inprocgrpc/internal/grpchantest

go 1.25.7

replace github.com/joeycumines/go-inprocgrpc => ../..

replace github.com/joeycumines/go-eventloop => ../../../eventloop

require (
	github.com/fullstorydev/grpchan v1.1.2
	github.com/joeycumines/go-eventloop v0.0.0-00010101000000-000000000000
	github.com/joeycumines/go-inprocgrpc v0.0.0-00010101000000-000000000000
	google.golang.org/grpc v1.78.0
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/bufbuild/protocompile v0.9.0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/jhump/protoreflect v1.15.6 // indirect
	github.com/joeycumines/go-catrate v0.0.0-20251223235052-5947b7adba56 // indirect
	github.com/joeycumines/logiface v0.5.0 // indirect
	golang.org/x/exp v0.0.0-20260112195511-716be5621a96 // indirect
	golang.org/x/net v0.49.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
	golang.org/x/text v0.33.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260203192932-546029d2fa20 // indirect
)
