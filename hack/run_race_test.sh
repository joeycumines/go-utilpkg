#!/bin/bash
cd "$(dirname "$0")/.."
go test -race -v -run TestRollback ./eventloop/... 2>&1 | tail -n 100
