module github.com/joeycumines/goja_nodejs

go 1.25

require (
	github.com/dop251/base64dec v0.0.0-20231022112746-c6c9f9a96217
	github.com/joeycumines/goja v0.0.0-20250309171923-bcd7cc6bf64c
	go.uber.org/goleak v1.3.0
	golang.org/x/net v0.27.0
	golang.org/x/text v0.16.0
)

require (
	github.com/dlclark/regexp2/v2 v2.2.1 // indirect
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible // indirect
	github.com/google/pprof v0.0.0-20240727154555-813a5fbdbec8 // indirect
)

replace github.com/joeycumines/goja => ../goja
