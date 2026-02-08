# This is an example config.mk file, to support local customizations.

.DEFAULT_GOAL := all

ifndef CUSTOM_TARGETS_DEFINED
CUSTOM_TARGETS_DEFINED := 1
##@ Custom Targets
# IF YOU NEED A CUSTOM TARGET, DEFINE IT BELOW THIS LINE, BEFORE THE `endif`

# ACTIONED: Re-read DIRECTIVE.txt, ran full tournament (macOS/Linux/Windows),
# all 3 platforms pass gmake all, race detector clean, running coverage next.
# ThenWithJS REMOVED (not deprecated). Memory leak analysis pending.

_CUSTOM_MAKE_ALL_TARGET_MAKE_ARGS := all GO_TEST_FLAGS=-timeout=12m

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

.PHONY: session-time
session-time: ## Record/check session time
session-time: SHELL := /bin/bash
session-time:
	@if [ -f $(PROJECT_ROOT)/.session_start ]; then \
		start=$$(cat $(PROJECT_ROOT)/.session_start); \
		now=$$(date +%s); \
		elapsed=$$((now - start)); \
		hours=$$((elapsed / 3600)); \
		mins=$$(( (elapsed % 3600) / 60 )); \
		echo "Session started: $$(date -r $$start 2>/dev/null || date -d @$$start 2>/dev/null || echo $$start)"; \
		echo "Current time:    $$(date)"; \
		echo "Elapsed: $${hours}h $${mins}m ($${elapsed}s)"; \
		remaining=$$(( 9*3600 - elapsed )); \
		if [ $$remaining -gt 0 ]; then \
			echo "Remaining: $$((remaining / 3600))h $$(( (remaining % 3600) / 60 ))m"; \
		else \
			echo "OVERTIME by $$(( (-remaining) / 3600 ))h $$(( ((-remaining) % 3600) / 60 ))m"; \
		fi; \
	else \
		date +%s > $(PROJECT_ROOT)/.session_start; \
		echo "Session started at $$(date)"; \
	fi

.PHONY: record-session-start
record-session-start: ## Force-record session start time NOW
record-session-start: SHELL := /bin/bash
record-session-start:
	@date +%s > $(PROJECT_ROOT)/.session_start && echo "Session start recorded: $$(date)"

.PHONY: git-status
git-status: ## Show git status summary
git-status: SHELL := /bin/bash
git-status:
	@cd $(PROJECT_ROOT) && echo "Branch: $$(git branch --show-current)" && echo "Commits ahead of main:" && git log --oneline main..HEAD 2>/dev/null || echo "(no main branch or same)" && echo "---" && git diff --stat main 2>/dev/null | tail -5 || true

.PHONY: run-on-windows
run-on-windows: ## Run make all on Windows via hack/run-on-windows.sh
run-on-windows: SHELL := /bin/bash
run-on-windows:
	@echo "Output limited to avoid context explosion. See $(PROJECT_ROOT)/build-windows.log for full content."; \
	set -o pipefail; \
	sh $(PROJECT_ROOT)/hack/run-on-windows.sh moo make $(_CUSTOM_MAKE_ALL_TARGET_MAKE_ARGS) 2>&1 | fold -w 200 | tee $(PROJECT_ROOT)/build-windows.log | tail -n 30; \
	exit $${PIPESTATUS[0]}

.PHONY: test-windows-pwd
test-windows-pwd: ## Test actual EXEC_PS template via SSH
test-windows-pwd: SHELL := /bin/bash
test-windows-pwd:
	@echo "=== Test: Full exec template ==="; \
	PS_SCRIPT=$$'$$ErrorActionPreference = \'Stop\';\n$$exitCode = 0;\n$$winPath = \'C:/Users/under/AppData/Local/Temp/test\';\ntry {\n    $$b64Args = \'Z28AdmVyc2lvbgA=\';\n    $$bytes = [System.Convert]::FromBase64String($$b64Args);\n    $$decoded = [System.Text.Encoding]::UTF8.GetString($$bytes);\n    $$allArgs = $$decoded.Split([char]0);\n    Write-Output "args-count: $$($$allArgs.Length)";\n    $$exitCode = 0;\n} catch {\n    Write-Error $$_.Exception.Message;\n    $$exitCode = 1;\n}\nexit $$exitCode;'; \
	B64=$$(printf '%s' "$$PS_SCRIPT" | base64 | tr -d '\n'); \
	echo "B64 len: $${#B64}"; \
	ssh -o ControlPath=none -T moo "powershell -NoProfile -NonInteractive -Command \"\$$e='$$B64';\$$s=[System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String(\$$e));Invoke-Expression \$$s\"" 2>&1; \
	echo "Exit: $$?"

.PHONY: test-windows-ssh
test-windows-ssh: ## Test basic SSH connectivity to Windows
test-windows-ssh: SHELL := /bin/bash
test-windows-ssh:
	@echo "Testing SSH..."; \
	ssh -T moo "echo hello from windows && where go && go version" 2>&1 | tail -n 10

.PHONY: test-windows-wsl
test-windows-wsl: ## Test WSL availability on Windows
test-windows-wsl: SHELL := /bin/bash
test-windows-wsl:
	@echo "=== Test 1: WSL tar --help check for no-absolute-names ==="; \
	ssh -T moo 'C:\Windows\System32\wsl.exe -e bash -c "tar --help 2>&1 | grep absolute"' 2>&1; \
	echo "=== Test 2: Direct tar --no-absolute-names test ==="; \
	ssh -T moo "C:\\Windows\\System32\\wsl.exe -e bash -c \"echo test | tar --no-absolute-names -xzf - 2>&1 || echo EXIT:$$?\"" 2>&1; \
	echo "=== Test 3: What gets passed through ==="; \
	ssh -T moo "C:\\Windows\\System32\\wsl.exe -e bash -c \"echo ARGS_RECEIVED\"" 2>&1

.PHONY: test-windows-echo
test-windows-echo: ## Test run-on-windows.sh with 'echo hello'
test-windows-echo: SHELL := /bin/bash
test-windows-echo:
	@$(PROJECT_ROOT)/hack/run-on-windows.sh moo echo hello 2>&1

.PHONY: test-windows-go-version
test-windows-go-version: ## Test run-on-windows.sh with 'go version'
test-windows-go-version: SHELL := /bin/bash
test-windows-go-version:
	@$(PROJECT_ROOT)/hack/run-on-windows.sh moo go version 2>&1

.PHONY: tournament-promise-macos
tournament-promise-macos: ## Re-run just promise tournament benchmarks on macOS
tournament-promise-macos: SHELL := /bin/bash
tournament-promise-macos:
	@echo "=== macOS Promise Tournament Benchmarks ===" | tee $(PROJECT_ROOT)/tournament-promise-macos.log; \
	cd $(PROJECT_ROOT)/eventloop && go test -C internal/promisetournament -bench=. -benchmem -benchtime=2s -timeout=15m -count=3 ./... 2>&1 | tee -a $(PROJECT_ROOT)/tournament-promise-macos.log; \
	echo "=== DONE ===" | tee -a $(PROJECT_ROOT)/tournament-promise-macos.log; \
	tail -5 $(PROJECT_ROOT)/tournament-promise-macos.log

.PHONY: test-conservation
test-conservation: ## Run conservation test for AlternateOne
test-conservation: SHELL := /bin/bash
test-conservation:
	@cd $(PROJECT_ROOT)/eventloop && go test -C internal/tournament -run 'TestShutdownConservation' -v -count=3 -timeout=2m ./... 2>&1 | tee $(PROJECT_ROOT)/build.log | tail -n 20

.PHONY: test-promise-all-settled
test-promise-all-settled: ## Run Promise.All already-settled test
test-promise-all-settled: SHELL := /bin/bash
test-promise-all-settled:
	@cd $(PROJECT_ROOT)/eventloop && go test -run TestPromiseAll_AlreadySettledPromises -v -timeout=30s 2>&1 | tail -30

.PHONY: tournament-macos
tournament-macos: ## Run full tournament benchmarks on macOS (event loop + promise)
tournament-macos: SHELL := /bin/bash
tournament-macos:
	@echo "=== macOS Tournament: Event Loop Benchmarks ===" | tee $(PROJECT_ROOT)/tournament-macos.log; \
	cd $(PROJECT_ROOT)/eventloop && go test -C internal/tournament -bench=. -benchmem -benchtime=2s -timeout=15m -count=3 ./... 2>&1 | tee -a $(PROJECT_ROOT)/tournament-macos.log; \
	echo "" | tee -a $(PROJECT_ROOT)/tournament-macos.log; \
	echo "=== macOS Tournament: Promise Benchmarks ===" | tee -a $(PROJECT_ROOT)/tournament-macos.log; \
	cd $(PROJECT_ROOT)/eventloop && go test -C internal/promisetournament -bench=. -benchmem -benchtime=2s -timeout=15m -count=3 ./... 2>&1 | tee -a $(PROJECT_ROOT)/tournament-macos.log; \
	echo "" | tee -a $(PROJECT_ROOT)/tournament-macos.log; \
	echo "=== macOS Tournament: Correctness Tests ===" | tee -a $(PROJECT_ROOT)/tournament-macos.log; \
	cd $(PROJECT_ROOT)/eventloop && go test -C internal/tournament -run='Test' -v -timeout=5m ./... 2>&1 | tee -a $(PROJECT_ROOT)/tournament-macos.log; \
	echo "" | tee -a $(PROJECT_ROOT)/tournament-macos.log; \
	echo "=== macOS Tournament: Race Detector ===" | tee -a $(PROJECT_ROOT)/tournament-macos.log; \
	cd $(PROJECT_ROOT)/eventloop && go test -C internal/tournament -run='Test' -race -timeout=10m ./... 2>&1 | tee -a $(PROJECT_ROOT)/tournament-macos.log; \
	echo "=== DONE ===" | tee -a $(PROJECT_ROOT)/tournament-macos.log; \
	tail -5 $(PROJECT_ROOT)/tournament-macos.log

.PHONY: tournament-linux
tournament-linux: ## Run full tournament benchmarks in Linux container
tournament-linux: SHELL := /bin/bash
tournament-linux:
	@echo "Running tournament in Linux container..." | tee $(PROJECT_ROOT)/tournament-linux.log; \
	go_version="$$($(GO) -C $(PROJECT_ROOT) mod edit -print | awk '/^go / {print $$2}')"; \
	echo "Using golang:$${go_version}" | tee -a $(PROJECT_ROOT)/tournament-linux.log; \
	docker run --rm -v $(PROJECT_ROOT):/work -w /work/eventloop "golang:$${go_version}" bash -lc ' \
		export PATH="/usr/local/go/bin:$$PATH" && export GOFLAGS=-buildvcs=false && \
		echo "=== Linux Tournament: Event Loop Benchmarks ===" && \
		go test -C internal/tournament -bench=. -benchmem -benchtime=2s -timeout=30m -count=3 ./... && \
		echo "" && \
		echo "=== Linux Tournament: Promise Benchmarks ===" && \
		go test -C internal/promisetournament -bench=. -benchmem -benchtime=2s -timeout=30m -count=3 ./... && \
		echo "" && \
		echo "=== Linux Tournament: Correctness Tests ===" && \
		go test -C internal/tournament -run="Test" -v -timeout=5m ./... && \
		echo "" && \
		echo "=== Linux Tournament: Race Detector ===" && \
		go test -C internal/tournament -run="Test" -race -timeout=10m ./... && \
		echo "=== DONE ===" \
	' 2>&1 | tee -a $(PROJECT_ROOT)/tournament-linux.log | tail -5

.PHONY: tournament-windows
tournament-windows: ## Run full tournament benchmarks on Windows via SSH
tournament-windows: SHELL := /bin/bash
tournament-windows:
	@echo "Running tournament on Windows (event loop + promise + correctness)..." | tee $(PROJECT_ROOT)/tournament-windows.log; \
	echo "=== Windows Tournament: Event Loop Benchmarks ===" | tee -a $(PROJECT_ROOT)/tournament-windows.log; \
	sh $(PROJECT_ROOT)/hack/run-on-windows.sh moo go test -C eventloop/internal/tournament -bench=. -benchmem -benchtime=2s -timeout=30m -count=3 ./... 2>&1 | tee -a $(PROJECT_ROOT)/tournament-windows.log; \
	echo "" | tee -a $(PROJECT_ROOT)/tournament-windows.log; \
	echo "=== Windows Tournament: Promise Benchmarks ===" | tee -a $(PROJECT_ROOT)/tournament-windows.log; \
	sh $(PROJECT_ROOT)/hack/run-on-windows.sh moo go test -C eventloop/internal/promisetournament -bench=. -benchmem -benchtime=2s -timeout=30m -count=3 ./... 2>&1 | tee -a $(PROJECT_ROOT)/tournament-windows.log; \
	echo "" | tee -a $(PROJECT_ROOT)/tournament-windows.log; \
	echo "=== Windows Tournament: Correctness Tests ===" | tee -a $(PROJECT_ROOT)/tournament-windows.log; \
	sh $(PROJECT_ROOT)/hack/run-on-windows.sh moo go test -C eventloop/internal/tournament -run=Test -v -timeout=5m ./... 2>&1 | tee -a $(PROJECT_ROOT)/tournament-windows.log; \
	echo "" | tee -a $(PROJECT_ROOT)/tournament-windows.log; \
	echo "=== Windows Tournament: Race Detector ===" | tee -a $(PROJECT_ROOT)/tournament-windows.log; \
	sh $(PROJECT_ROOT)/hack/run-on-windows.sh moo go test -C eventloop/internal/tournament -run=Test -race -timeout=10m ./... 2>&1 | tee -a $(PROJECT_ROOT)/tournament-windows.log; \
	echo "=== DONE ===" | tee -a $(PROJECT_ROOT)/tournament-windows.log; \
	tail -5 $(PROJECT_ROOT)/tournament-windows.log

.PHONY: test-race-eventloop
test-race-eventloop: ## Run eventloop tests with race detector
test-race-eventloop: SHELL := /bin/bash
test-race-eventloop:
	@echo "Running eventloop race detector tests..."; \
	cd $(PROJECT_ROOT)/eventloop && go test -race -timeout=12m -count=1 ./... 2>&1 | tail -30

.PHONY: test-windows-race
test-windows-race: ## Test if race detector works at all on Windows
test-windows-race: SHELL := /bin/bash
test-windows-race:
	@sh $(PROJECT_ROOT)/hack/run-on-windows.sh moo go test -C eventloop -race -run TestExampleLifecycle -timeout=1m ./... 2>&1 | tail -20

.PHONY: build-check
build-check: ## Compile check a specific package
build-check: SHELL := /bin/bash
build-check:
	@echo "Compiling eventloop promise tournament..."; \
	cd $(PROJECT_ROOT)/eventloop && go test -C internal/promisetournament -run '^$$' -c -o /dev/null ./... 2>&1; \
	echo "Exit: $$?"

.PHONY: coverage-eventloop
coverage-eventloop: ## Run coverage for eventloop and report on promise.go
coverage-eventloop: SHELL := /bin/bash
coverage-eventloop:
	@cd $(PROJECT_ROOT)/eventloop && \
	echo "=== Running tests with coverage ===" && \
	go test -coverprofile=coverage.out -timeout=12m ./... 2>&1 | tail -30 && \
	echo "" && \
	echo "=== promise.go functions (main package) ===" && \
	go tool cover -func=coverage.out | grep 'go-eventloop/promise.go' | head -60 && \
	echo "" && \
	echo "=== Main package low coverage (< 80%) ===" && \
	go tool cover -func=coverage.out | grep 'go-eventloop/' | grep -v '/internal/' | grep -v '/examples/' | awk '{ pct=$$NF; gsub(/%/, "", pct); if (pct+0 < 80 && pct+0 > 0) print }' | head -40 && \
	echo "" && \
	echo "=== Main package overall ===" && \
	grep '^github.com/joeycumines/go-eventloop\b' $(PROJECT_ROOT)/eventloop/coverage.out | head -3 && \
	echo "" && \
	echo "=== Overall coverage ===" && \
	go tool cover -func=coverage.out | tail -1

.PHONY: test-promisify-terminating
test-promisify-terminating: ## Run TestPromisify_InTerminatingState 5x
test-promisify-terminating: SHELL := /bin/bash
test-promisify-terminating:
	@cd $(PROJECT_ROOT)/eventloop && go test -run 'TestPromisify_InTerminatingState' -v -timeout=30s -count=5 ./... 2>&1 | tail -80

.PHONY: test-eventloop-full
test-eventloop-full: ## Run full eventloop test suite
test-eventloop-full: SHELL := /bin/bash
test-eventloop-full:
	@cd $(PROJECT_ROOT)/eventloop && go test -count=1 -timeout=5m ./... 2>&1 | tail -20

# IF YOU NEED A CUSTOM TARGET, DEFINE IT ABOVE THIS LINE, AFTER THE `##@ Custom Targets`

endif
