module github.com/joeycumines/go-inprocgrpc/internal/grpchantest

go 1.26.6

replace github.com/joeycumines/go-inprocgrpc => ../..

require (
	github.com/fullstorydev/grpchan v1.1.2
	github.com/joeycumines/go-eventloop v0.0.0-20260624075642-f9a45542db92
	github.com/joeycumines/go-inprocgrpc v0.0.0-20260624075719-4d4ca2aad3e8
	google.golang.org/grpc v1.83.0
	google.golang.org/protobuf v1.36.12
)

require (
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/jhump/protoreflect v1.18.0 // indirect
	github.com/jhump/protoreflect/v2 v2.0.0-beta.2 // indirect
	github.com/joeycumines/go-catrate v0.0.0-20260429212737-202f4120003b // indirect
	github.com/joeycumines/goroutineid v1.1.0 // indirect
	github.com/joeycumines/logiface v0.5.0 // indirect
	golang.org/x/exp v0.0.0-20260813180055-c1d0aacb2297 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260810153831-ec0a7760b754 // indirect
)

replace github.com/joeycumines/goja => ../../../goja
