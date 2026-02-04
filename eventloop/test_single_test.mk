# Test target for the specific failing test
test-promise-first-rejection:
	cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -v -race -timeout=30s -run "TestPromiseAll_FirstRejectionWins" 2>&1
