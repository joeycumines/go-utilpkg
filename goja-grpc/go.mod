module github.com/joeycumines/goja-grpc

go 1.25.7

replace (
	github.com/joeycumines/go-eventloop => ../eventloop
	github.com/joeycumines/go-inprocgrpc => ../inprocgrpc
	github.com/joeycumines/goja-eventloop => ../goja-eventloop
	github.com/joeycumines/goja-protobuf => ../goja-protobuf
	github.com/joeycumines/goja-protojson => ../goja-protojson
)

require (
	github.com/dop251/goja v0.0.0-20260106131823-651366fbe6e3
	github.com/dop251/goja_nodejs v0.0.0-20251015164255-5e94316bedaf
	github.com/joeycumines/go-eventloop v0.0.0
	github.com/joeycumines/go-inprocgrpc v0.0.0-00010101000000-000000000000
	github.com/joeycumines/goja-eventloop v0.0.0-00010101000000-000000000000
	github.com/joeycumines/goja-protobuf v0.0.0-00010101000000-000000000000
	github.com/joeycumines/goja-protojson v0.0.0-00010101000000-000000000000
	github.com/stretchr/testify v1.11.1
	google.golang.org/grpc v1.78.0
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dlclark/regexp2 v1.11.5 // indirect
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible // indirect
	github.com/google/pprof v0.0.0-20260202012954-cb029daf43ef // indirect
	github.com/joeycumines/go-catrate v0.0.0-20251223235052-5947b7adba56 // indirect
	github.com/joeycumines/logiface v0.5.0 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	golang.org/x/exp v0.0.0-20260112195511-716be5621a96 // indirect
	golang.org/x/net v0.49.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
	golang.org/x/text v0.33.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260203192932-546029d2fa20 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
