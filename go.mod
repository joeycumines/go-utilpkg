module github.com/joeycumines/go-utilpkg

go 1.20

// Update via:
//   go mod edit -replace github.com/grailbio/grit=github.com/joeycumines/grit@latest && go mod tidy
replace github.com/grailbio/grit => github.com/joeycumines/grit v0.0.0-20230416222730-bf226e10ae38

require (
	github.com/grailbio/grit v0.0.0-20200605233837-2ad5ef8ce918
	golang.org/x/perf v0.0.0-20230227161431-f7320a6d63e8
	golang.org/x/tools v0.8.0
	honnef.co/go/tools v0.4.3
)

require (
	github.com/BurntSushi/toml v1.2.1 // indirect
	github.com/aclements/go-moremath v0.0.0-20210112150236-f10218a38794 // indirect
	github.com/grailbio/base v0.0.10 // indirect
	github.com/yuin/goldmark v1.5.4 // indirect
	golang.org/x/exp/typeparams v0.0.0-20230321023759-10a507213a29 // indirect
	golang.org/x/mod v0.10.0 // indirect
	golang.org/x/sys v0.7.0 // indirect
)
