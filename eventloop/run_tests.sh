#!/bin/bash
cd /Users/joeyc/dev/go-utilpkg/eventloop
go test -v . 2>&1 | fold -w 200 | tee eventloop_test.log
