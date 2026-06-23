module github.com/joeycumines/goja

go 1.25

require (
	github.com/Masterminds/semver/v3 v3.5.0
	github.com/dlclark/regexp2/v2 v2.2.1
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible
	github.com/goccy/go-yaml v1.19.2
	github.com/google/pprof v0.0.0-20240727154555-813a5fbdbec8
	github.com/joeycumines/goja_nodejs v0.0.0-20211022123610-8dd9abb0616d
	golang.org/x/text v0.16.0
)

replace github.com/joeycumines/goja_nodejs => ../goja_nodejs
