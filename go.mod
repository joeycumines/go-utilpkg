module github.com/joeycumines/go-utilpkg

go 1.27.0

require (
	github.com/BurntSushi/toml v1.6.0 // indirect
	github.com/KimMachineGun/automemlimit v1.0.0 // indirect
	github.com/aclements/go-moremath v0.0.0-20241023150245-c8bbc672ef66 // indirect
	github.com/aws/aws-sdk-go v1.44.259 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dkorunic/betteralign v0.15.0 // indirect
	github.com/google/renameio/v2 v2.0.2 // indirect
	github.com/grailbio/base v0.0.11 // indirect
	github.com/grailbio/grit v0.0.0-20230416231552-d3b81e617b57 // indirect
	github.com/joeycumines/simple-command-output-filter v0.2.1 // indirect
	github.com/pbnjay/memory v0.0.0-20210728143218-7b4eea64cf58 // indirect
	github.com/yuin/goldmark v1.8.5 // indirect
	golang.org/x/exp/typeparams v0.0.0-20260824195058-e88cd73687aa // indirect
	golang.org/x/mod v0.40.0 // indirect
	golang.org/x/perf v0.0.0-20260819171926-ebcb4798430d // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/telemetry v0.0.0-20260824150023-1f5465a7b7fb // indirect
	golang.org/x/tools v0.49.0 // indirect
	golang.org/x/tools/cmd/godoc v0.1.0-deprecated // indirect
	golang.org/x/tools/godoc v0.1.0-deprecated // indirect
	honnef.co/go/tools v0.8.1 // indirect
)

tool (
	github.com/dkorunic/betteralign/cmd/betteralign
	github.com/grailbio/grit
	github.com/joeycumines/simple-command-output-filter
	golang.org/x/perf/cmd/benchstat
	golang.org/x/tools/cmd/deadcode
	golang.org/x/tools/cmd/godoc
	honnef.co/go/tools/cmd/staticcheck
)

replace github.com/grailbio/grit => github.com/joeycumines/grit v0.0.0-20260825210026-67df0bb3a24d
