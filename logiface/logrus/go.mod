module github.com/joeycumines/go-utilpkg/logiface/logrus

go 1.20

replace github.com/joeycumines/go-utilpkg/logiface => ./..

require (
	github.com/joeycumines/go-utilpkg/logiface v0.0.0-20230211055231-49637a1d9603
	github.com/sirupsen/logrus v1.9.0
	golang.org/x/exp v0.0.0-20230210204819-062eb4c674ab
)

require golang.org/x/sys v0.5.0 // indirect
