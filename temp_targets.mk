.PHONY: test-regression-timer
test-regression-timer: SHELL := /bin/bash
test-regression-timer:
	@echo "=== Running TestRegression_TimerExecution (passing test) ===" && \
	cd eventloop && \
	go test -v -run "TestRegression_TimerExecution" . -timeout 10s 2>&1 | fold -w 200 | tee /Users/joeyc/dev/go-utilpkg/regression_timer.log; \
	exit $${PIPESTATUS[0]}

.PHONY: test-fastpath-timer
test-fastpath-timer: SHELL := /bin/bash
test-fastpath-timer:
	@echo "=== Running TestFastPath_TimerFiresFromFastPath (passing test) ===" && \
	cd eventloop && \
	go test -v -run "TestFastPath_TimerFiresFromFastPath" . -timeout 10s 2>&1 | fold -w 200 | tee /Users/joeyc/dev/go-utilpkg/fastpath_timer.log; \
	exit $${PIPESTATUS[0]}

.PHONY: compare-timing-tests
compare-timing-tests: SHELL := /bin/bash
compare-timing-tests:
	@echo "=== Comparing JS Test vs Loop ScheduleTimer Tests ===" && \
	echo "" && \
	echo "=== JS SetTimeout Test (FAILING) ===" && \
	grep -A 10 "SetTimeout elapsed:" /Users/joeyc/dev/go-utilpkg/settimeout_timing.log 2>/dev/null || echo "No log found" && \
	echo "" && \
	echo "=== Regression Timer Test (PASSING) ===" && \
	grep -A 5 "Timer fired successfully" /Users/joeyc/dev/go-utilpkg/regression_timer.log 2>/dev/null || echo "No log found" && \
	echo "" && \
	echo "=== Test Differences ===" && \
	echo "JS Test uses: js.SetTimeout(func(){...}, 10)" && \
	echo "Loop Test uses: l.ScheduleTimer(10*time.Millisecond, func(){...})" && \
	exit 0
