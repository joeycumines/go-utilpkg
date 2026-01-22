#!/bin/bash
# Simple script to copy clean adapter and run tests
cd /Users/joeyc/dev/go-utilpkg/goja-eventloop
cp adapter.go.clean adapter.go
echo "Copied clean adapter to adapter.go"
cd /Users/joeyc/dev/go-utilpkg
make test-goja-eventloop-verbose 2>&1 | tail -n 50
