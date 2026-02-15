module github.com/joeycumines/goja-grpc

go 1.26

replace (
	github.com/joeycumines/go-eventloop => ../eventloop
	github.com/joeycumines/go-inprocgrpc => ../inprocgrpc
	github.com/joeycumines/goja-eventloop => ../goja-eventloop
	github.com/joeycumines/goja-protobuf => ../goja-protobuf
	github.com/joeycumines/goja-protojson => ../goja-protojson
)

require (
	github.com/dop251/goja v0.0.0-20260106131823-651366fbe6e3
	github.com/dop251/goja_nodejs v0.0.0-20260212111938-1f56ff5bcf14
	github.com/joeycumines/go-eventloop v0.0.0
	github.com/joeycumines/go-inprocgrpc v0.0.0-20260213164927-0dc92b109371
	github.com/joeycumines/goja-eventloop v0.0.0-20260213164906-b3f83ceccca4
	github.com/joeycumines/goja-protobuf v0.0.0-20260213164915-e7601209bd26
	github.com/joeycumines/goja-protojson v0.0.0-20260213164919-c434665d6fbf
	github.com/stretchr/testify v1.11.1
	google.golang.org/grpc v1.79.1
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dlclark/regexp2 v1.11.5 // indirect
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible // indirect
	github.com/google/pprof v0.0.0-20260202012954-cb029daf43ef // indirect
	github.com/joeycumines/go-catrate v0.0.0-20260213164847-3d7ee3241422 // indirect
	github.com/joeycumines/logiface v0.5.0 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	golang.org/x/exp v0.0.0-20260212183809-81e46e3db34a // indirect
	golang.org/x/net v0.50.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260209200024-4cfbd4190f57 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
