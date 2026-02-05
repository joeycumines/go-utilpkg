# Temporary Makefile for Promise.Race tests
.PHONY: test-wakeup-integration test-promise-all test-promise-race test-promise-race-all test-promise-race-stats test-promise-race-terminated test-promise-race-comprehensive

test-wakeup-integration:
	cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -v -timeout=30s -run "TestWakeUpPipe_Integration" ./...
test-promise-all:
	cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -v -race -timeout=60s -run "TestPromiseAll_" 2>&1 | grep -E "(=== RUN|--- PASS|--- FAIL)" | head -50

test-promise-all-verbose:
	cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -v -race -timeout=60s -run "TestPromiseAll_" 2>&1 | grep -E "(=== RUN|--- PASS|--- FAIL|FAIL\s)"

test-promise-all-full:
	cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -v -race -timeout=60s -run "TestPromiseAll_" 2>&1

test-promise-all-combinators:
	cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -v -race -timeout=60s -run "TestPromiseAll_EmptyArray|TestPromiseAll_SingleElement|TestPromiseAll_MultipleValues_AllResolve|TestPromiseAll_AnyRejection_RejectsImmediately" 2>&1

test-promise-all-all:
	cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -v -race -timeout=60s -run "^TestPromiseAll" 2>&1

test-promise-other-combinators:
	cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -v -race -timeout=30s -run "TestPromise(AllSettled|Any|ToChannel)" 2>&1

test-promise-other-combinators-summary:
	@echo "=== TestPromiseAllSettled, TestPromiseAny, TestPromiseToChannel Summary ===" && \
	cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -v -race -timeout=30s -run "TestPromise(AllSettled|Any|ToChannel)" 2>&1 | grep -E "(=== RUN|--- PASS|--- FAIL|PASS|FAIL)"

test-promise-all-coverage:
	cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -v -race -timeout=60s -run "TestPromiseAll$$" -run ./coverage_gap_tests.go 2>&1 || true
	cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -v -race -timeout=60s -run "TestPromiseAll" ./coverage_gap_tests.go 2>&1
test-promise-race:
	cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -v -race -timeout=60s -run "TestPromiseRace" 2>&1 | tee /tmp/race_test.log

test-promise-race-all:
	cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -v -race -timeout=120s -run "TestPromiseRace" 2>&1 | grep -E "(=== RUN|--- PASS|--- FAIL|FAIL\s)"

test-promise-race-stats:
	@echo "=== TestPromiseRace Summary ==="
	@cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -v -race -timeout=120s -run "TestPromiseRace" 2>&1 | grep "^=== RUN" | wc -l | xargs echo "Total tests:"
	@cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -v -race -timeout=120s -run "TestPromiseRace" 2>&1 | grep "^--- PASS" | wc -l | xargs echo "Passed:"
	@cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -v -race -timeout=120s -run "TestPromiseRace" 2>&1 | grep "^--- FAIL" | wc -l | xargs echo "Failed:"
	@cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -v -race -timeout=120s -run "TestPromiseRace" 2>&1 | grep "^--- FAIL" -A 5

test-promise-race-terminated:
	cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -v -race -timeout=30s -run "TestPromiseRace_TerminatedLoop" 2>&1

test-promise-race-comprehensive:
	@echo "=========================================="
	@echo "COMPREHENSIVE TestPromiseRace ANALYSIS"
	@echo "=========================================="
	@echo ""
	@echo "Step 1: Listing all TestPromiseRace functions..."
	@grep -r "func.*TestPromiseRace" /Users/joeyc/dev/go-utilpkg/eventloop/*.go | grep -v "^Binary" | wc -l | xargs echo "Total TestPromiseRace functions found:"
	@echo ""
	@echo "Step 2: Running all tests with detailed output..."
	@cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -v -race -timeout=120s -run "TestPromiseRace" 2>&1 | grep -E "(=== RUN|--- PASS|--- FAIL|PASS|FAIL)"
	@echo ""
	@echo "Step 3: Test statistics..."
	@cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -v -race -timeout=120s -run "TestPromiseRace" 2>&1 | grep "^=== RUN" | wc -l | xargs echo "Total tests executed:"
	@cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -v -race -timeout=120s -run "TestPromiseRace" 2>&1 | grep "^--- PASS" | wc -l | xargs echo "Tests passed:"
	@cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -v -race -timeout=120s -run "TestPromiseRace" 2>&1 | grep "^--- FAIL" | wc -l | xargs echo "Tests failed:"
	@echo ""
	@echo "Step 4: Failed test details..."
	@cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -v -race -timeout=120s -run "TestPromiseRace" 2>&1 | grep -A 20 "FAIL\t"

# Custom target for specific handlepollerror and fd tests
test-specific-fd:
	cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -v -timeout=60s -run "TestHandlePollError_Logging|TestDrainWakeUpPipe_Basic|TestDrainWakeUpPipe_Idempotent|TestCreateWakeFd_ValidPipe|TestCreateWakeFd_VariousParams|TestHandlePollError_MultipleCycles|TestHandlePollError_ConcurrentWithOtherOps" 2>&1

# TestDrainWakeUpPipe coverage tests
test-drain-wakeup-pipe:
	cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -v -timeout=30s -run "TestDrainWakeUpPipe" ./... 2>&1

# Coverage analysis for specific functions
coverage-analysis:
	cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -coverprofile=coverage.out -timeout=120s ./...
	@echo "=== Coverage for handlePollError, drainWakeUpPipe, createWakeFd ===" && cd /Users/joeyc/dev/go-utilpkg/eventloop && go tool cover -func=coverage.out | grep -E "(handlePollError|drainWakeUpPipe|createWakeFd)"
	@echo "=== Full function details ===" && cd /Users/joeyc/dev/go-utilpkg/eventloop && go tool cover -func=coverage.out | grep -E "^(.*handlePollError.*|.*drainWakeUpPipe.*|.*createWakeFd.*)"

# Wakeup coverage tests - specific test execution
test-wakeup-coverage:
	cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -v -timeout=30s -run "TestDrainWakeUpPipe_NegativeWakePipe|TestWakeUpPipe_Integration" ./... 2>&1

# TestWakeup_HighContention specific test
test-wakeup-high-contention:
	cd /Users/joeyc/dev/go-utilpkg/eventloop && go test -v -timeout=30s -run "TestWakeup_HighContention" 2>&1
