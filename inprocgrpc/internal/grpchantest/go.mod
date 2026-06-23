module github.com/joeycumines/go-inprocgrpc/internal/grpchantest

go 1.26.2

replace github.com/joeycumines/go-inprocgrpc => ../..

replace github.com/joeycumines/go-eventloop => ../../../eventloop

require (
	github.com/fullstorydev/grpchan v1.1.2
	github.com/joeycumines/go-eventloop v0.0.0-20260428025403-c64a0733c558
	github.com/joeycumines/go-inprocgrpc v0.0.0-20260331032414-92dc1790fe75
	google.golang.org/grpc v1.80.0
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/jhump/protoreflect v1.18.0 // indirect
	github.com/jhump/protoreflect/v2 v2.0.0-beta.2 // indirect
	github.com/joeycumines/go-catrate v0.0.0-20260331032335-1bb8bb5ab23b // indirect
	github.com/joeycumines/goroutineid v1.1.0 // indirect
	github.com/joeycumines/logiface v0.5.0 // indirect
	go.opentelemetry.io/otel/metric v1.40.0 // indirect
	go.opentelemetry.io/otel/trace v1.40.0 // indirect
	golang.org/x/exp v0.0.0-20260410095643-746e56fc9e2f // indirect
	golang.org/x/net v0.53.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/text v0.36.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260427160629-7cedc36a6bc4 // indirect
)
