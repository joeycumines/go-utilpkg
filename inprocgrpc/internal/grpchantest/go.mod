module github.com/joeycumines/go-inprocgrpc/internal/grpchantest

go 1.26.0

replace github.com/joeycumines/go-inprocgrpc => ../..

replace github.com/joeycumines/go-eventloop => ../../../eventloop

require (
	github.com/fullstorydev/grpchan v1.1.2
	github.com/joeycumines/go-eventloop v0.0.0-20260228170514-ce9591fd940c
	github.com/joeycumines/go-inprocgrpc v0.0.0-20260228154239-f7983abaf819
	google.golang.org/grpc v1.79.1
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/jhump/protoreflect v1.18.0 // indirect
	github.com/jhump/protoreflect/v2 v2.0.0-beta.2 // indirect
	github.com/joeycumines/go-catrate v0.0.0-20260228071149-ca3b62cde775 // indirect
	github.com/joeycumines/logiface v0.5.0 // indirect
	go.opentelemetry.io/otel/metric v1.40.0 // indirect
	go.opentelemetry.io/otel/trace v1.40.0 // indirect
	golang.org/x/exp v0.0.0-20260218203240-3dfff04db8fa // indirect
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260226221140-a57be14db171 // indirect
)
