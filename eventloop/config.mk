# Diagnostic targets for finding hanging tests

# Quick diagnostic to find hanging tests
diagnose-hanging-tests:
	cd /Users/joeyc/dev/go-utilpkg/eventloop && timeout 30 go test -v -timeout=20s ./... 2>&1 | grep -E "(RUN|PASS|FAIL|SKIP|panic|timeout|Hang)" | tail -50

# Run individual problematic test files with short timeout
test-promisify-coverage:
	cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -v -timeout=30s -run TestPromisify_ ./...

test-wakeup-coverage:
	cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -v -timeout=30s -run TestCreateWakeFd ./...

test-coverage-gaps:
	cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -v -timeout=30s -run "TestPromisify_Terminating|TestPromisify_ShutdownWaits" ./...

# Comprehensive coverage analysis targets
coverage-full:
	cd /Users/joeyc/dev/go-utilpkg/eventloop && go clean -testcache && go test -coverprofile=final_coverage.out -timeout=5m ./...

coverage-overall:
	cd /Users/joeyc/dev/go-utilpkg/eventloop && go tool cover -func=final_coverage.out | tail -10

coverage-promisealtthree:
	cd /Users/joeyc/dev/go-utilpkg/eventloop && go tool cover -func=final_coverage.out | grep promisealtthree

coverage-analyze: coverage-full coverage-overall coverage-promisealtthree
	@echo "Coverage analysis complete. Check final_coverage.out for details."

# Run poll_error_simple_test.go with timeout
test-poll-error-simple:
	cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -v -timeout=30s -run Test_PollError_Path 2>&1 | tee /tmp/poll_error_test.log | tail -n 50

# Full test suite re-run with logging
test-rerun:
	cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -v -timeout=60s ./... 2>&1 | tee /tmp/test_rerun.log | tail -n 150

# Coverage analysis targets
coverage-detailed:
	go tool cover -func=final_coverage.out | grep -v "^total" | sort -t: -k3 -rn | head -50

coverage-files:
	go tool cover -func=final_coverage.out | grep -v "^github" | tail -1

coverage-zero:
	go tool cover -func=final_coverage.out | grep "0.0%"

coverage-files:
	go tool cover -func=final_coverage.out | grep -E "^[a-z]" | tail -1

coverage-by-file:
	@echo "File-level coverage analysis:" && \
	go tool cover -func=final_coverage.out | grep -E "^github" | cut -d: -f1 | uniq | while read file; do \
		percentage=$$(go tool cover -func=final_coverage.out | grep "^$$file:" | tail -1 | awk '{print $$NF}'); \
		echo "$$file: $$percentage"; \
	done | sort -t: -k2 -rn

# Test individual promise regression - run 5 times
test-promise-race-concurrent:
	cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -v -run TestPromiseRace_ConcurrentThenReject_HandlersCalled -count=5 2>&1 | tail -80

# Custom target for specific poll error and wakeup tests
test-poll-error-wakeup:
	cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -v -timeout=60s -run "TestHandlePollError_NonSleepingState|TestHandlePollError_MultipleErrors|TestDrainWakeUpPipe|TestCreateWakeFd|TestHandlePollError_NilError" 2>&1 | tail -40

# Run Promise.All, Promise.Any, Promise.Race, Promise.ToChannel tests multiple times to check for intermittent failures
test-promise-intermittent:
	cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -v -race -timeout=30s -run "TestPromise(AllSettled|Any|Race|ToChannel)" -count=3 2>&1 | tee /tmp/promise_intermittent_test.log | tail -n 100

# Run the three fixed panic handling tests
test-panic-fixes:
	cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -v -timeout=30s -run "TestJSIntegration_PanicInCallback|TestJSIntegration_PanicInCatch|TestJSIntegration_ErrorPropagationThroughChain" 2>&1 | fold -w 200 | tee /tmp/panic_fixes_test.log | tail -n 50

# Coverage analysis for specific functions
coverage-analysis:
	cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -coverprofile=coverage.out -timeout=120s ./...
	@echo "=== Coverage for handlePollError, drainWakeUpPipe, createWakeFd ===" && cd /Users/joeyc/dev/go-utilpkg/eventloop && go tool cover -func=coverage.out | grep -E "(handlePollError|drainWakeUpPipe|createWakeFd)"
	@echo "=== Relevant functions and percentages ===" && cd /Users/joeyc/dev/go-utilpkg/eventloop && go tool cover -func=coverage.out | grep -E "^(.*handlePollError.*|.*drainWakeUpPipe.*|.*createWakeFd.*|[0-9]+\.[0-9]+%)"

