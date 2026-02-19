# This is an example config.mk file, to support local customizations.

.DEFAULT_GOAL := all

ifndef CUSTOM_TARGETS_DEFINED
CUSTOM_TARGETS_DEFINED := 1
##@ Custom Targets
# IF YOU NEED A CUSTOM TARGET, DEFINE IT BELOW THIS LINE, BEFORE THE `endif`

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

.PHONY: stage-testify-batch
stage-testify-batch: ## Stage testify removal batch for commit
	cd $(CURDIR) && git add catrate/ prompt/ grpc-proxy/ goja-grpc/coverage_test.go config.mk && git add -f scratch/

.PHONY: diff-stat-staged
diff-stat-staged: ## Show staged diff stat
	@git diff --staged --stat

.PHONY: commit-testify-batch
commit-testify-batch: ## Commit testify removal batch using message file
	git commit -F $(CURDIR)/scratch/commit-msg.txt

.PHONY: commit-log
commit-log: ## Show last 5 commits
	@git log --oneline -5

# IF YOU NEED A CUSTOM TARGET, DEFINE IT ABOVE THIS LINE, AFTER THE `##@ Custom Targets`
endif
