module github.com/joeycumines/go-fangrpcstream

go 1.27.0

require (
	github.com/joeycumines/go-bigbuff v1.21.1
	google.golang.org/grpc v1.83.1
	google.golang.org/protobuf v1.36.12
)

require (
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260819154853-08b0e4226688 // indirect
	google.golang.org/grpc/cmd/protoc-gen-go-grpc v1.6.2 // indirect
)

tool (
	google.golang.org/grpc/cmd/protoc-gen-go-grpc
	google.golang.org/protobuf/cmd/protoc-gen-go
)
