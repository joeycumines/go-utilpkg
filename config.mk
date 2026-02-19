# This is an example config.mk file, to support local customizations.

.DEFAULT_GOAL := all

ifndef CUSTOM_TARGETS_DEFINED
CUSTOM_TARGETS_DEFINED := 1
##@ Custom Targets
# IF YOU NEED A CUSTOM TARGET, DEFINE IT BELOW THIS LINE, BEFORE THE `endif`

# ACKNOWLEDGED: Hana directive received. Fast-follow added to blueprint.json. Emergency panic to be removed from slog adapter.

.PHONY: goja-el-delete-junk
goja-el-delete-junk: ## Delete debug junk test files from goja-eventloop
	rm -f $(CURDIR)/goja-eventloop/adapter_debug_test.go
	rm -f $(CURDIR)/goja-eventloop/debug_promise_test.go
	rm -f $(CURDIR)/goja-eventloop/debug_allsettled_test.go
	rm -f $(CURDIR)/goja-eventloop/export_behavior_test.go
	@echo "Deleted 4 debug junk test files"

.PHONY: goja-el-delete-testify
goja-el-delete-testify: ## Delete test files that use forbidden testify packages
	rm -f $(CURDIR)/goja-eventloop/coverage_phase3_test.go
	rm -f $(CURDIR)/goja-eventloop/coverage_phase3b_test.go
	rm -f $(CURDIR)/goja-eventloop/coverage_phase3c_test.go
	rm -f $(CURDIR)/goja-eventloop/adapter_iterator_error_test.go
	@echo "Deleted 4 testify test files"

.PHONY: goja-el-diff-stat
goja-el-diff-stat: ## Show git diff stat for goja-eventloop
	@git diff HEAD --stat -- goja-eventloop/ WIP.md

.PHONY: goja-el-diff-adapter
goja-el-diff-adapter: ## Show git diff for adapter.go
	@git diff HEAD -- goja-eventloop/adapter.go

.PHONY: goja-el-diff-changelog
goja-el-diff-changelog: ## Show git diff for CHANGELOG.md
	@git diff HEAD -- goja-eventloop/CHANGELOG.md

.PHONY: goja-el-diff-other-tests
goja-el-diff-other-tests: ## Show git diff for tests not in the huge deleted files
	@git diff HEAD -- goja-eventloop/abort_test.go goja-eventloop/adapter_coverage_test.go goja-eventloop/adapter_js_combinators_test.go goja-eventloop/adapter_test.go goja-eventloop/array_test.go goja-eventloop/arraybuffer_test.go goja-eventloop/base64_test.go goja-eventloop/builtins_test.go goja-eventloop/collections_test.go goja-eventloop/console_test.go

.PHONY: goja-el-test
goja-el-test: ## Run goja-eventloop tests
	@cd $(CURDIR)/goja-eventloop && go test -timeout=6m ./... 2>&1 | tail -n 10

.PHONY: goja-el-vet
goja-el-vet: ## Vet goja-eventloop
	@cd $(CURDIR)/goja-eventloop && go vet ./... 2>&1

.PHONY: goja-el-tidy
goja-el-tidy: ## Run go mod tidy in goja-eventloop
	@cd $(CURDIR)/goja-eventloop && go mod tidy 2>&1

_CUSTOM_MAKE_ALL_TARGET_MAKE_ARGS := all GO_TEST_FLAGS=-timeout=6m

.PHONY: make-all-with-log
make-all-with-log: ## Run all targets with logging to build.log
make-all-with-log: SHELL := /bin/bash
make-all-with-log:
	@echo "Output limited to avoid context explosion. See $(or $(PROJECT_ROOT),$(error If you are reading this you specified the `file` option when calling `mcp-server-make`. DONT DO THAT.))/build.log for full content."; \
set -o pipefail; \
$(MAKE) $(_CUSTOM_MAKE_ALL_TARGET_MAKE_ARGS) 2>&1 | fold -w 200 | tee $(or $(PROJECT_ROOT),$(error If you are reading this you specified the `file` option when calling `mcp-server-make`. DONT DO THAT.))/build.log | tail -n 15; \
exit $${PIPESTATUS[0]}

.PHONY: make-all-in-container
make-all-in-container: ## Like `make make-all-with-log` inside a linux golang container
make-all-in-container: SHELL := /bin/bash
make-all-in-container:
	@echo "Output limited to avoid context explosion. See $(or $(PROJECT_ROOT),$(error If you are reading this you specified the `file` option when calling `mcp-server-make`. DONT DO THAT.))/build.log for full content."; \
go_version="$$($(GO) -C $(PROJECT_ROOT) mod edit -print | awk '/^go / {print $$2}')"; \
echo "Running in container golang:$${go_version}."; \
set -o pipefail; \
docker run --rm -v $(PROJECT_ROOT):/work -w /work "golang:$${go_version}" bash -lc 'export PATH="/usr/local/go/bin:$$PATH" && export GOFLAGS=-buildvcs=false && { jobs="$$(nproc)" && [ "$$jobs" -gt 0 ] && jobs="-j $${jobs}" || jobs=''; } && set -x && make $${jobs} $(_CUSTOM_MAKE_ALL_TARGET_MAKE_ARGS)' 2>&1 | fold -w 200 | tee build.log | tail -n 15; \
exit $${PIPESTATUS[0]}

.PHONY: make-all-run-windows
make-all-run-windows: ## Run all targets with logging to build.log
make-all-run-windows: SHELL := /bin/bash
make-all-run-windows:
	@echo "Output limited to avoid context explosion. See $(or $(PROJECT_ROOT),$(error If you are reading this you specified the `file` option when calling `mcp-server-make`. DONT DO THAT.))/build.log for full content."; \
set -o pipefail; \
hack/run-on-windows.sh moo make $(_CUSTOM_MAKE_ALL_TARGET_MAKE_ARGS) 2>&1 | fold -w 200 | tee $(or $(PROJECT_ROOT),$(error If you are reading this you specified the `file` option when calling `mcp-server-make`. DONT DO THAT.))/build.log | tail -n 15; \
exit $${PIPESTATUS[0]}

.PHONY: update-blueprint
update-blueprint: ## Move refined blueprint into place
	@cp scratch/blueprint.refined.json blueprint.json && echo "Blueprint updated"

.PHONY: test-slog-units-only
test-slog-units-only: ## Run ONLY logiface-slog unit tests (no testsuite) with 30s timeout
	@cd logiface-slog && go test -v -run='^Test' -timeout=30s ./... 2>&1 | head -200

.PHONY: diagnose-slog-hang
diagnose-slog-hang: ## Run testsuite with verbose output and 30s timeout to identify hanging subtest
	@cd logiface-slog && timeout 30s go test -v -run='Test_TestSuite' 2>&1 | tail -100 || true

.PHONY: test-slog-race-count10
test-slog-race-count10: ## Run logiface-slog race detector tests with 10 iterations
	@cd logiface-slog && go test -race -count=10 ./... 2>&1 | tail -n 100

.PHONY: test-slog-specific-failure
test-slog-specific-failure: ## Run the specific failing test in logiface-slog with verbose output
	@cd logiface-slog && go test -v -run "Test_TestSuite/TestLoggerLogMethod/enabled_levels_without_modifier$$" -timeout=30s ./... 2>&1 | tee test_output.log | head -300

.PHONY: view-slog-test-output
view-slog-test-output: ## View the slog test output file
	@cd logiface-slog && cat test_output.log

.PHONY: session-time-start
session-time-start: ## Start session time tracking
	@echo "SESSION_START_TIMESTAMP: $$(date +%s)" > .session-time.log
	@echo "SESSION_START_READABLE: $$(date)" >> .session-time.log
	@echo "SESSION_DURATION_TARGET: 9 hours" >> .session-time.log
	@echo "SESSION_STATUS: IN_PROGRESS" >> .session-time.log
	@echo "Session started. Timestamp recorded at .session-time.log"

.PHONY: session-time-check
session-time-check: ## Check session time elapsed
	@if [ -f .session-time.log ]; then \
		start_time=$$(grep SESSION_START_TIMESTAMP .session-time.log | cut -d' ' -f2); \
		current_time=$$(date +%s); \
		elapsed=$$((current_time - start_time)); \
		hours=$$((elapsed / 3600)); \
		minutes=$$(((elapsed % 3600) / 60)); \
		seconds=$$((elapsed % 60)); \
		echo "Elapsed time: $$hours hours, $$minutes minutes, $$seconds seconds"; \
		echo "Target: 9 hours (32400 seconds)"; \
		if [ $$elapsed -lt 32400 ]; then \
			remaining=$$((32400 - elapsed)); \
			rem_hours=$$((remaining / 3600)); \
			rem_min=$$(((remaining % 3600) / 60)); \
			echo "Remaining: $$rem_hours hours, $$rem_min minutes"; \
		else \
			echo "9-hour mandate satisfied (elapsed: $$hours hours $$minutes minutes)"; \
		fi; \
	else \
		echo "No .session-time.log found. Start with 'gmake session-time-start'"; \
	fi

.PHONY: test-slog-direct
test-slog-direct: ## Run tests directly and capture output
	@cd logiface-slog && go test -v -run "^Test_TestSuite/TestLoggerLogMethod/disabled_levels/logger=warning_arg=info$$" -timeout=30s ./...

.PHONY: get-current-timestamp
get-current-timestamp: ## Get current Unix timestamp
	@go run temp_timestamp.go

.PHONY: test-slog-examples
test-slog-examples: ## Run logiface-slog Example tests to check for missing Output comments
	$(GO) -C logiface-slog test -run=Example -v

.PHONY: test-slog-examples-list
test-slog-examples-list: ## List all Example tests in logiface-slog
	$(GO) -C logiface-slog test -list=Example -v

.PHONY: test-slog-examples-all
test-slog-examples-all: ## Run all tests and list examples
	@echo "=== Running all tests in pkg islog ===" && \
	$(GO) -C logiface-slog test -v -list ".*" 2>&1 | grep -i example || echo "No examples found" && \
	echo "=== Now running Example-specific tests ===" && \
	$(GO) -C logiface-slog test -run="^Example" -v

.PHONY: test-slog-examples-debug
test-slog-examples-debug: ## Debug - show what go sees
	@echo "=== Listing tests in islog package ===" && \
	$(GO) -C logiface-slog test -v -list ".*" | grep -i example || echo "No example matches" && \
	$(GO) -C logiface-slog test -v -list ".*" | head -20 && \
	echo "=== Direct test of ExampleLogger ===" && \
	$(GO) -C logiface-slog test -run "^ExampleLogger$$" -v

.PHONY: remove-slog-capture-example
remove-slog-capture-example: ## Delete capture_example_output_test.go
	@rm -f /Users/joeyc/dev/go-utilpkg/logiface-slog/capture_example_output_test.go && echo "Deleted capture_example_output_test.go"

.PHONY: test-slog-after-delete
test-slog-after-delete: ## Run logiface-slog tests after deleting capture_example_output_test.go
	@cd /Users/joeyc/dev/go-utilpkg/logiface-slog && go test ./... 2>&1

.PHONY: test-slog-direct-simplified
test-slog-direct-simplified: ## Run simple Go test directly on logiface-slog
	@cd /Users/joeyc/dev/go-utilpkg/logiface-slog && go test -v -run "^TestLoggerLevelFiltering$$" -timeout 30s ./... 2>&1 | head -50

.PHONY: check-slog-inlining-and-alignment
check-slog-inlining-and-alignment: ## Run inlining and alignment checks for logiface-slog
	@echo "=== Check 1: Inlining analysis ===" && \
	cd /Users/joeyc/dev/go-utilpkg/logiface-slog && \
	go build -gcflags=-m ./... 2>&1 | grep -E "(Level|AddField|toSlogLevel)" | head -20 || echo "No matches found"
	@echo ""
	@echo "=== Check 2: Struct alignment ===" && \
	if command -v betteralign &>/dev/null; then \
		cd /Users/joeyc/dev/go-utilpkg/logiface-slog && betteralign ./...; \
	elif command -v fieldalignment &>/dev/null; then \
		cd /Users/joeyc/dev/go-utilpkg/logiface-slog && fieldalignment -fix=false ./...; \
	else \
		echo "No alignment tool found - skipping"; \
	fi

.PHONY: check-slog-struct-alignment
check-slog-struct-alignment: ## Check struct sizes and alignment for logiface-slog
	@cd /Users/joeyc/dev/go-utilpkg/logiface-slog && \
	go run check_alignment_manual.go 2>&1 || true

.PHONY: check-islog-fmt
check-islog-fmt: ## Check gofmt on islog package files only (excludes package main tools)
	@echo "=== Checking gofmt on islog package files ===" && \
	cd logiface-slog && \
	gofmt -l slog.go doc.go slog_test.go example_test.go smoke_test.go debug_level_test.go simple_filter_test.go check_alignment_test.go

.PHONY: check-islog-imports
check-islog-imports: ## Check unused imports in islog package files using goimports
	@echo "=== Checking unused imports with goimports ===" && \
	if command -v goimports &>/dev/null; then \
		cd logiface-slog && \
		goimports -l slog.go doc.go slog_test.go example_test.go smoke_test.go debug_level_test.go simple_filter_test.go check_alignment_test.go; \
	else \
		echo "goimports not available - skipping import check"; \
	fi

.PHONY: cleanup-slog-debug-files
cleanup-slog-debug-files: ## Fix gofmt doc.go and remove debug/test files from logiface-slog
	@echo "=== Fixing doc.go formatting ===" && \
	gofmt -w $(CURDIR)/logiface-slog/doc.go && \
	echo "=== Removing debug files ===" && \
	rm -f $(CURDIR)/logiface-slog/check_alignment_manual.go $(CURDIR)/logiface-slog/check_alignment_test.go $(CURDIR)/logiface-slog/temp_show_uncovered.sh $(CURDIR)/logiface-slog/run_examples_test.sh $(CURDIR)/logiface-slog/check_aligment_direct.sh && \
	echo "=== Running unit tests ===" && \
	cd $(CURDIR)/logiface-slog && go test -v -run='^Test' -timeout=30s ./... 2>&1 | head -200

.PHONY: check-golangci-lint
check-golangci-lint: ## Check if golangci-lint is installed and run on logiface-slog
	@echo "=== Checking golangci-lint installation ===" && \
	if command -v golangci-lint &>/dev/null; then \
		echo "golangci-lint is installed:" && \
		golangci-lint --version 2>&1 | head -1 && \
		echo "" && \
		echo "=== Running golangci-lint on logiface-slog ===" && \
		cd $(CURDIR)/logiface-slog && \
		golangci-lint run ./... 2>&1; \
	else \
		echo "golangci-lint is NOT installed on this system"; \
	fi

.PHONY: fuzz-eventaddfield
fuzz-eventaddfield: ## Run FuzzEventAddField fuzz test for 30 seconds and capture summary
	@echo "=== Running FuzzEventAddField fuzz test for 30 seconds ===" && \
	cd $(CURDIR)/logiface-slog && \
	go test -run='^$$' -fuzz=FuzzEventAddField -fuzztime=30s 2>&1 | tail -n 20

.PHONY: test-slog-integration
test-slog-integration: ## Run integration tests in logiface-slog
	@echo "=== Running integration tests in logiface-slog ===" && \
	cd $(CURDIR)/logiface-slog && \
	go test -v -run=TestIntegration -timeout=30s ./... 2>&1

.PHONY: test-slog-shuffle
test-slog-shuffle: ## Run logiface-slog tests with random shuffle, 3 iterations to verify test independence
	@echo "=== Running logiface-slog tests with shuffle (3 iterations) ===" && \
	cd $(CURDIR)/logiface-slog && \
	set -o pipefail; \
	go test -shuffle=on -count=3 -timeout=15m ./... 2>&1 | tee /tmp/test_shuffle_$$(date +%s).log | tail -n 100; \
	exit $${PIPESTATUS[0]}

.PHONY: test-slog-units-shuffle
test-slog-units-shuffle: ## Run ONLY unit tests (not testsuite) with shuffle to verify test independence
	@echo "=== Running logiface-slog UNIT TESTS ONLY with shuffle (3 iterations) ===" && \
	cd $(CURDIR)/logiface-slog && \
	set -o pipefail; \
	go test -shuffle=on -count=3 -run='^Test[^T]' -timeout=5m ./... 2>&1 | tee /tmp/test_unit_shuffle_$$(date +%s).log | tail -n 50; \
	exit $${PIPESTATUS[0]}

.PHONY: slog-cleanup-delete-slop
slog-cleanup-delete-slop: ## Delete AI slop files from logiface-slog
	@echo "=== Deleting AI slop files ===" && \
	rm -f $(CURDIR)/logiface-slog/doc.go && echo "Deleted doc.go" && \
	rm -f $(CURDIR)/logiface-slog/PERFORMANCE.md && echo "Deleted PERFORMANCE.md" && \
	rm -f $(CURDIR)/logiface-slog/TESTING.md && echo "Deleted TESTING.md" && \
	rm -f $(CURDIR)/logiface-slog/smoke_test.go && echo "Deleted smoke_test.go" && \
	rm -f $(CURDIR)/logiface-slog/debug_level_test.go && echo "Deleted debug_level_test.go" && \
	echo "=== Running tests to verify ===" && \
	cd $(CURDIR)/logiface-slog && go test -short -timeout=30s ./... 2>&1 | tail -n 10

.PHONY: slog-test-quick
slog-test-quick: ## Quick slog test (short mode)
	@cd $(CURDIR)/logiface-slog && go test -short -timeout=30s ./... 2>&1 | tail -n 20

.PHONY: slog-vet
slog-vet: ## Vet logiface-slog
	@cd $(CURDIR)/logiface-slog && go vet ./... 2>&1

.PHONY: slog-go-mod-tidy
slog-go-mod-tidy: ## Run go mod tidy in logiface-slog
	@cd $(CURDIR)/logiface-slog && go mod tidy 2>&1

.PHONY: slog-test-full
slog-test-full: ## Run full logiface-slog test suite with 6m timeout
	@cd $(CURDIR)/logiface-slog && go test -timeout=6m ./... 2>&1 | tail -n 5

.PHONY: slog-test-race
slog-stage-commit:
	git add logiface-slog/slog.go logiface-slog/slog_test.go logiface-slog/example_test.go logiface-slog/README.md logiface-slog/CHANGELOG.md logiface-slog/go.mod logiface-slog/go.sum
	git add logiface-slog/doc.go logiface-slog/PERFORMANCE.md logiface-slog/TESTING.md logiface-slog/smoke_test.go logiface-slog/debug_level_test.go
	git add go.work.sum

slog-commit-exec:
	git commit -m "Clean logiface-slog adapter, fix Emergency write order" -m "Remove AI-generated bloat from the slog adapter:" -m "- Delete doc.go, PERFORMANCE.md, TESTING.md, smoke_test.go, debug_level_test.go" -m "- Strip excessive doc comments from slog.go to match zerolog/logrus style" -m "- Trim example_test.go from 10 to 4 examples" -m "- Rewrite README.md to 2-line terse format" -m "- Clean CHANGELOG.md (remove duplicates, non-standard sections)" -m "" -m "Fix critical bugs:" -m "- Write() now writes event before panicking for Emergency level" -m "- Fix testSuiteLevelMapping for slog 4-level system (Custom→Debug," -m "  Trace→Debug, Notice→Warning, Critical/Alert/Emergency→Error)" -m "- Remove AlertCallsOsExit (slog has no fatal/os.Exit behavior)" -m "- Remove dead _level parsing code from testSuiteParseEvent" -m "" -m "Clean dependencies:" -m "- Remove phantom OTel requires (otel, otel/trace, auto/sdk, metric)" -m "- Remove transitive deps (cespare/xxhash, go-logr/logr, go-logr/stdr)"

slog-stage-and-commit-emergency:
	git add logiface-slog/slog.go logiface-slog/slog_test.go
	git commit -m "fix(logiface-slog): remove Emergency panic from adapter" -m "The adapter should not panic on LevelEmergency. That is handled by" -m "logiface's Builder.send() via builderModePanic when Panic() is called." -m "slog has no panic level, so the adapter just writes at slog.LevelError." -m "" -m "- Remove panic(logiface.LevelEmergency) from Write()" -m "- Set EmergencyPanics: false in test config"

slog-test-race: ## Run logiface-slog tests with race detector
	@cd $(CURDIR)/logiface-slog && go test -race -timeout=6m ./... 2>&1 | tail -n 5

.PHONY: slog-test-verbose-fail
slog-test-verbose-fail: ## Run testsuite verbose, show failures
	@cd $(CURDIR)/logiface-slog && go test -v -timeout=6m -run='Test_TestSuite' ./... 2>&1 | grep -E 'FAIL|ErrDisabled|unexpected event' | head -30

.PHONY: goja-commit-stage
goja-commit-stage: ## Stage goja-eventloop changes plus blueprint.json and WIP.md
	cd $(CURDIR) && git add goja-eventloop/ blueprint.json WIP.md

.PHONY: goja-commit-diff-stat
goja-commit-diff-stat: ## Show staged diff stat
	@git diff --staged --stat

.PHONY: goja-commit-exec
goja-commit-exec: ## Execute the goja-eventloop commit using message file
	git commit -F $(CURDIR)/scratch/goja-commit-msg.txt

.PHONY: goja-commit-log
goja-commit-log: ## Show last 3 commits
	@git log --oneline -3

.PHONY: goja-pb-delete-testify-tests
goja-pb-delete-testify-tests: ## Delete testify test files from goja-protobuf for rewriting
	rm -f $(CURDIR)/goja-protobuf/testhelpers_test.go
	rm -f $(CURDIR)/goja-protobuf/types_test.go
	rm -f $(CURDIR)/goja-protobuf/oneof_test.go
	rm -f $(CURDIR)/goja-protobuf/map_test.go
	rm -f $(CURDIR)/goja-protobuf/repeated_test.go
	rm -f $(CURDIR)/goja-protobuf/serialize_test.go
	rm -f $(CURDIR)/goja-protobuf/integration_test.go
	rm -f $(CURDIR)/goja-protobuf/coverage_phase2_test.go
	rm -f $(CURDIR)/goja-protobuf/descriptors_test.go
	rm -f $(CURDIR)/goja-protobuf/coverage_gap_test.go
	rm -f $(CURDIR)/goja-protobuf/conversion_test.go
	rm -f $(CURDIR)/goja-protobuf/options_test.go
	rm -f $(CURDIR)/goja-protobuf/fuzz_test.go
	rm -f $(CURDIR)/goja-protobuf/message_test.go
	rm -f $(CURDIR)/goja-protobuf/module_test.go
	rm -f $(CURDIR)/goja-protobuf/register_test.go
	@echo "Deleted 16 test files"

.PHONY: goja-pb-restore-from-git
goja-pb-restore-from-git: ## Restore remaining deleted test files from git for editing
	cd $(CURDIR) && git checkout HEAD -- \
		goja-protobuf/integration_test.go \
		goja-protobuf/coverage_phase2_test.go \
		goja-protobuf/descriptors_test.go \
		goja-protobuf/coverage_gap_test.go \
		goja-protobuf/conversion_test.go \
		goja-protobuf/options_test.go \
		goja-protobuf/fuzz_test.go \
		goja-protobuf/message_test.go \
		goja-protobuf/module_test.go \
		goja-protobuf/register_test.go
	@echo "Restored 10 test files from git"

.PHONY: goja-pb-delete-restored
goja-pb-delete-restored: ## Delete the 10 restored test files for recreation
	rm -f $(CURDIR)/goja-protobuf/integration_test.go \
		$(CURDIR)/goja-protobuf/coverage_phase2_test.go \
		$(CURDIR)/goja-protobuf/descriptors_test.go \
		$(CURDIR)/goja-protobuf/conversion_test.go \
		$(CURDIR)/goja-protobuf/options_test.go \
		$(CURDIR)/goja-protobuf/fuzz_test.go \
		$(CURDIR)/goja-protobuf/message_test.go \
		$(CURDIR)/goja-protobuf/module_test.go \
		$(CURDIR)/goja-protobuf/register_test.go
	@echo "Deleted 9 smaller test files (kept coverage_gap_test.go for editing)"

.PHONY: goja-pb-git-grep-new
goja-pb-git-grep-new: ## Search git for func new helper
	@cd $(CURDIR) && git show HEAD -- goja-protobuf/testhelpers_test.go | grep -n 'func new' || echo "not in testhelpers"
	@cd $(CURDIR) && for f in goja-protobuf/*_test.go; do \
		result=$$(git show HEAD -- "$$f" 2>/dev/null | grep 'func new(' || true); \
		if [ -n "$$result" ]; then echo "$$f: $$result"; fi; \
	done
	@echo "=== checking all deleted files for func new ==="
	@cd $(CURDIR) && git log --all --diff-filter=D --name-only --pretty=format: -- goja-protobuf/*_test.go | sort -u | while read f; do \
		if [ -n "$$f" ]; then \
			result=$$(git show HEAD -- "$$f" 2>/dev/null | grep 'func new(' || true); \
			if [ -n "$$result" ]; then echo "$$f: $$result"; fi; \
		fi; \
	done || true

.PHONY: goja-pb-tidy
goja-pb-tidy: ## Run go mod tidy in goja-protobuf
	@cd $(CURDIR)/goja-protobuf && go mod tidy 2>&1

.PHONY: goja-pb-test
goja-pb-test: ## Run goja-protobuf tests
	@cd $(CURDIR)/goja-protobuf && go test -timeout=2m ./... 2>&1

.PHONY: goja-pb-test-verbose
goja-pb-test-verbose: ## Run goja-protobuf tests verbose
	@cd $(CURDIR)/goja-protobuf && go test -v -timeout=2m ./... 2>&1

.PHONY: goja-pb-vet-quick
goja-pb-vet-quick: ## Quick vet check on goja-protobuf
	@cd $(CURDIR)/goja-protobuf && go vet ./... 2>&1 | head -30

.PHONY: goja-pb-delete-coverage-gap
goja-pb-delete-coverage-gap: ## Delete coverage_gap_test.go for recreation
	rm -f $(CURDIR)/goja-protobuf/coverage_gap_test.go
	@echo "Deleted coverage_gap_test.go"

.PHONY: el-delete-junk
el-delete-junk: ## Delete testify + debug junk test files from eventloop
	@echo "=== File sizes before deletion ===" && \
	ls -la $(CURDIR)/eventloop/coverage_phase3_test.go $(CURDIR)/eventloop/coverage_phase3b_test.go $(CURDIR)/eventloop/fastpath_debug_test.go $(CURDIR)/eventloop/wake_debug_test.go 2>&1 && \
	echo "" && \
	echo "=== Deleting files ===" && \
	rm -v $(CURDIR)/eventloop/coverage_phase3_test.go && \
	rm -v $(CURDIR)/eventloop/coverage_phase3b_test.go && \
	rm -v $(CURDIR)/eventloop/fastpath_debug_test.go && \
	rm -v $(CURDIR)/eventloop/wake_debug_test.go && \
	echo "" && \
	echo "=== Running go mod tidy ===" && \
	cd $(CURDIR)/eventloop && go mod tidy 2>&1 && \
	echo "" && \
	echo "=== Running go vet ===" && \
	cd $(CURDIR)/eventloop && go vet ./... 2>&1 && \
	echo "" && \
	echo "=== Running go test ===" && \
	cd $(CURDIR)/eventloop && go test -timeout=6m ./... 2>&1 | tail -5

.PHONY: goja-grpc-tidy
goja-grpc-tidy: ## Run go mod tidy in goja-grpc
	@cd $(CURDIR)/goja-grpc && go mod tidy 2>&1

.PHONY: goja-grpc-vet
goja-grpc-vet: ## Run go vet on goja-grpc
	@cd $(CURDIR)/goja-grpc && go vet ./... 2>&1

.PHONY: goja-grpc-test
goja-grpc-test: ## Run goja-grpc tests
	@cd $(CURDIR)/goja-grpc && go test -timeout=4m ./... 2>&1

.PHONY: goja-grpc-test-tail
goja-grpc-test-tail: ## Run goja-grpc tests, tail output
	@cd $(CURDIR)/goja-grpc && go test -timeout=4m ./... 2>&1 | tail -30

.PHONY: goja-grpc-build
goja-grpc-build: ## Build goja-grpc to check compilation
	@cd $(CURDIR)/goja-grpc && go build ./... 2>&1

.PHONY: goja-grpc-grep-testify
goja-grpc-grep-testify: ## Check for remaining testify references in goja-grpc
	@cd $(CURDIR)/goja-grpc && grep -rn 'testify\|assert\.\|require\.' *_test.go 2>/dev/null | head -30 || echo "No testify references found!"

.PHONY: logiface-commit-stage
logiface-commit-stage: ## Stage logiface core changes
	cd $(CURDIR) && git add logiface/ && git add -f config.mk scratch/ blueprint.json WIP.md

.PHONY: batch-commit-diff-stat
batch-commit-diff-stat: ## Show staged diff stat for batch commit
	@git diff --staged --stat

.PHONY: batch-commit-exec
batch-commit-exec: ## Execute the batch commit using message file
	git commit -F $(CURDIR)/scratch/commit-msg.txt

.PHONY: batch-commit-log
batch-commit-log: ## Show last 5 commits
	@git log --oneline -5

# IF YOU NEED A CUSTOM TARGET, DEFINE IT ABOVE THIS LINE, AFTER THE `##@ Custom Targets`
endif
