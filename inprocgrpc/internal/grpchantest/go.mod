module github.com/joeycumines/go-inprocgrpc/internal/grpchantest

go 1.26.0

replace github.com/joeycumines/go-inprocgrpc => ../..

replace github.com/joeycumines/go-eventloop => ../../../eventloop

require (
	github.com/fullstorydev/grpchan v1.1.2
	github.com/joeycumines/go-eventloop v0.0.0-20260213164852-99e8a33a69b7
	github.com/joeycumines/go-inprocgrpc v0.0.0-20260213164927-0dc92b109371
	google.golang.org/grpc v1.79.1
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/jhump/protoreflect v1.18.0 // indirect
	github.com/jhump/protoreflect/v2 v2.0.0-beta.2 // indirect
	github.com/joeycumines/go-catrate v0.0.0-20260213164847-3d7ee3241422 // indirect
	github.com/joeycumines/logiface v0.5.0 // indirect
	go.opentelemetry.io/otel/metric v1.40.0 // indirect
	go.opentelemetry.io/otel/trace v1.40.0 // indirect
	golang.org/x/exp v0.0.0-20260212183809-81e46e3db34a // indirect
	golang.org/x/net v0.50.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260217215200-42d3e9bedb6d // indirect
)
