module github.com/joeycumines/go-inprocgrpc/internal/grpchantest

go 1.26.1

replace github.com/joeycumines/go-inprocgrpc => ../..

replace github.com/joeycumines/go-eventloop => ../../../eventloop

require (
	github.com/fullstorydev/grpchan v1.1.2
	github.com/joeycumines/go-eventloop v0.0.0-20260306044146-d41d75c38083
	github.com/joeycumines/go-inprocgrpc v0.0.0-20260306044219-a3095125631d
	google.golang.org/grpc v1.79.3
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/jhump/protoreflect v1.18.0 // indirect
	github.com/jhump/protoreflect/v2 v2.0.0-beta.2 // indirect
	github.com/joeycumines/go-catrate v0.0.0-20260306044141-5afeab37e342 // indirect
	github.com/joeycumines/logiface v0.5.0 // indirect
	go.opentelemetry.io/otel/metric v1.40.0 // indirect
	go.opentelemetry.io/otel/trace v1.40.0 // indirect
	golang.org/x/exp v0.0.0-20260312153236-7ab1446f8b90 // indirect
	golang.org/x/net v0.52.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.35.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260330182312-d5a96adf58d8 // indirect
)
