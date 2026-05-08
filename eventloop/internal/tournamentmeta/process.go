package main

import "os"

type processSpec struct {
	Stdin       *os.File
	Stdout      *os.File
	Stderr      *os.File
	Executable  string
	Directory   string
	Arguments   []string
	Environment []string
}
