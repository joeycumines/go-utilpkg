#!/bin/bash
# Run individual tests to see which pass/fail

cd /Users/joeyc/dev/go-utilpkg/goja-eventloop

echo "=== Running goja-eventloop tests individually ==="
echo ""

echo "1. TestNewAdapter"
go test -v -run TestNewAdapter 2>&1 | grep -E "(PASS|FAIL|RUN)"
echo ""

echo "2. TestSetTimeout"
go test -v -run TestSetTimeout 2>&1 | grep -E "(PASS|FAIL|RUN)"
echo ""

echo "3. TestClearTimeout"
go test -v -run TestClearTimeout 2>&1 | grep -E "(PASS|FAIL|RUN)"
echo ""

echo "4. TestSetInterval"
go test -v -run TestSetInterval 2>&1 | grep -E "(PASS|FAIL|RUN)"
echo ""

echo "5. TestClearInterval"
go test -v -run TestClearInterval 2>&1 | grep -E "(PASS|FAIL|RUN)"
echo ""

echo "6. TestQueueMicrotask"
go test -v -run TestQueueMicrotask 2>&1 | grep -E "(PASS|FAIL|RUN)"
echo ""

echo "7. TestPromiseThen"
go test -v -run TestPromiseThen 2>&1 | grep -E "(PASS|FAIL|RUN)"
echo ""

echo "8. TestPromiseCatch"
go test -v -run TestPromiseCatch 2>&1 | grep -E "(PASS|FAIL|RUN)"
echo ""

echo "9. TestPromiseFinally"
go test -v -run TestPromiseFinally 2>&1 | grep -E "(PASS|FAIL|RUN)"
echo ""

echo "10. TestPromiseChain"
go test -v -run TestPromiseChain 2>&1 | grep -E "(PASS|FAIL|RUN)"
echo ""

echo "11. TestMixedTimersAndPromises"
go test -v -run TestMixedTimersAndPromises 2>&1 | grep -E "(PASS|FAIL|RUN)"
echo ""

echo "12. TestContextCancellation"
go test -v -run TestContextCancellation 2>&1 | grep -E "(PASS|FAIL|RUN)"
echo ""

echo "13. TestConcurrentJSOperations"
go test -v -run TestConcurrentJSOperations 2>&1 | grep -E "(PASS|FAIL|RUN)"
echo ""

echo "14. Promise Combinators Tests"
go test -v -run "Promise.*All|Promise.*Race|Promise.*Any|Promise.*AllSettled" 2>&1 | grep -E "(PASS|FAIL|RUN)"
echo ""
