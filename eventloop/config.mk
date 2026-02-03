# Custom test target for quick functionality verification
quick-test:
	@go test -run "TestNew|TestClose|TestSubmit" -short -timeout 30s .

# Staticcheck with latest version
run-staticcheck:
	@go run honnef.co/go/tools/cmd/staticcheck@latest ./... 2>&1
