#!/bin/bash
# Test runner for Promise tests with proper logging

cd /Users/joeyc/dev/go-utilpkg/eventloop

echo "=== Running Promise Tests: TestChainedPromise ==="
echo "Command: go test -v -run 'TestChainedPromise' -timeout 60s"
echo ""

START_TIME=$SECONDS
go test -v -run "TestChainedPromise" -timeout 60s 2>&1 | fold -w 200 | tee /Users/joeyc/dev/go-utilpkg/promise_test.log
EXIT_CODE=$?
ELAPSED=$(($SECONDS - START_TIME))

echo ""
echo "=== Test Execution Summary ==="
echo "Exit Code: $EXIT_CODE"
echo "Total Execution Time: $ELAPSED seconds"
echo ""
echo "=== Test Results Analysis ==="

# Count total tests run
TOTAL_TESTS=$(grep -c "^=== RUN" /Users/joeyc/dev/go-utilpkg/promise_test.log || echo 0)
echo "Total tests run: $TOTAL_TESTS"

# Count passed tests
PASSED_TESTS=$(grep -c "^--- PASS:" /Users/joeyc/dev/go-utilpkg/promise_test.log || echo 0)
echo "Tests passed: $PASSED_TESTS"

# Count failed tests
FAILED_TESTS=$(grep -c "^--- FAIL:" /Users/joeyc/dev/go-utilpkg/promise_test.log || echo 0)
echo "Tests failed: $FAILED_TESTS"

# Show any failures with details
if [ $FAILED_TESTS -gt 0 ]; then
    echo ""
    echo "=== Failed Tests Details ==="
    grep -A 20 "^--- FAIL:" /Users/joeyc/dev/go-utilpkg/promise_test.log || true
fi

exit $EXIT_CODE
