# Custom target for user-requested test summary
test-summary:
	cd /Users/joeyc/dev/go-utilpkg && go test ./eventloop/... 2>&1 | grep -E "^(ok|FAIL|---|\?)" | tail -20
