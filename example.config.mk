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

.PHONY: eventloop-tournament
eventloop-tournament: eventloop-tournament-darwin eventloop-tournament-linux eventloop-tournament-windows ## Run full 3-platform eventloop tournament

.PHONY: eventloop-tournament-darwin
eventloop-tournament-darwin: ## Run eventloop benchmarks on Darwin (local macOS)
eventloop-tournament-darwin: SHELL := /bin/bash
eventloop-tournament-darwin:
	@echo "Running eventloop benchmarks on Darwin..."; \
	set -o pipefail; \
	$(MAKE) -C $(PROJECT_ROOT) eventloop-tournament-bench 2>&1 | tee $(PROJECT_ROOT)/eventloop-tournament-darwin.log; \
	exit $${PIPESTATUS[0]}

.PHONY: eventloop-tournament-linux
eventloop-tournament-linux: ## Run eventloop benchmarks on Linux (Docker)
eventloop-tournament-linux: SHELL := /bin/bash
eventloop-tournament-linux:
	@echo "Running eventloop benchmarks on Linux..."; \
	go_version="$$($(GO) -C $(PROJECT_ROOT) mod edit -print | awk '/^go / {print $$2}')"; \
	set -o pipefail; \
	docker run --rm -v $(PROJECT_ROOT):/work -w /work "golang:$${go_version}" bash -lc 'export PATH="/usr/local/go/bin:$$PATH" && export GOFLAGS=-buildvcs=false && $(MAKE) eventloop-tournament-bench' 2>&1 | tee $(PROJECT_ROOT)/eventloop-tournament-linux.log; \
	exit $${PIPESTATUS[0]}

.PHONY: eventloop-tournament-windows
eventloop-tournament-windows: ## Run eventloop benchmarks on Windows
eventloop-tournament-windows: SHELL := /bin/bash
eventloop-tournament-windows:
	@echo "Running eventloop benchmarks on Windows..."; \
	set -o pipefail; \
	hack/run-on-windows.sh moo make eventloop-tournament-bench 2>&1 | tee $(PROJECT_ROOT)/eventloop-tournament-windows.log; \
	exit $${PIPESTATUS[0]}

# IF YOU NEED A CUSTOM TARGET, DEFINE IT ABOVE THIS LINE, AFTER THE `##@ Custom Targets`
endif
