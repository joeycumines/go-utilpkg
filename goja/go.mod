module github.com/joeycumines/goja

go 1.26

require (
	github.com/Masterminds/semver/v3 v3.5.0
	github.com/dlclark/regexp2/v2 v2.5.2
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible
	github.com/goccy/go-yaml v1.19.2
	github.com/google/pprof v0.0.0-20260402051712-545e8a4df936
	golang.org/x/text v0.37.0
)

require github.com/joeycumines/goja_nodejs v0.0.0-20260508000000-6add961f94bd

replace github.com/joeycumines/goja_nodejs => ../goja_nodejs
