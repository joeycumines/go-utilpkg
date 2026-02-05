# Custom target for user-requested test summary
test-summary:
	cd /Users/joeyc/dev/go-utilpkg && go test ./eventloop/... 2>&1 | grep -E "^(ok|FAIL|---|\?)" | tail -20

# Race detector tests
race-eventloop:
	cd /Users/joeyc/dev/go-utilpkg && go test -race -timeout=5m ./eventloop/... 2>&1 | tail -50
