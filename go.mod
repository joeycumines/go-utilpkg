module github.com/joeycumines/go-utilpkg

go 1.27.1

require (
	github.com/BurntSushi/toml v1.6.0 // indirect
	github.com/KimMachineGun/automemlimit v1.0.0 // indirect
	github.com/aclements/go-moremath v0.0.0-20241023150245-c8bbc672ef66 // indirect
	github.com/aws/aws-sdk-go v1.44.259 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dkorunic/betteralign v0.15.0 // indirect
	github.com/google/renameio/v2 v2.0.2 // indirect
	github.com/grailbio/base v0.0.11 // indirect
	github.com/joeycumines/grit v0.0.0-20260905053727-b5d3cd22ac16 // indirect
	github.com/joeycumines/simple-command-output-filter v0.2.1 // indirect
	github.com/pbnjay/memory v0.0.0-20210728143218-7b4eea64cf58 // indirect
	github.com/yuin/goldmark v1.8.6 // indirect
	golang.org/x/exp/typeparams v0.0.0-20260824195058-e88cd73687aa // indirect
	golang.org/x/mod v0.40.0 // indirect
	golang.org/x/perf v0.0.0-20260825160852-19be9d8e6c70 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/telemetry v0.0.0-20260902144106-3ef544be8421 // indirect
	golang.org/x/tools v0.49.0 // indirect
	golang.org/x/tools/cmd/godoc v0.1.0-deprecated // indirect
	golang.org/x/tools/godoc v0.1.0-deprecated // indirect
	honnef.co/go/tools v0.8.1 // indirect
)

replace github.com/joeycumines/grit => /Users/joeyc/dev/grit

tool (
	github.com/dkorunic/betteralign/cmd/betteralign
	github.com/joeycumines/grit
	github.com/joeycumines/simple-command-output-filter
	golang.org/x/perf/cmd/benchstat
	golang.org/x/tools/cmd/deadcode
	golang.org/x/tools/cmd/godoc
	honnef.co/go/tools/cmd/staticcheck
)
