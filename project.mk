# Extensions to the standard build process for the project.

# TODO update this if the root module gets packages
GO_MODULE_SLUGS_NO_PACKAGES = root
GO_MODULE_SLUGS_NO_UPDATE = sql.export.mysql
GO_MODULE_SLUGS_NO_BETTERALIGN = $(GO_MODULE_SLUGS)
GRIT_SRC ?= https://github.com/joeycumines/go-utilpkg.git
GRIT_DST ?= \
    ./catrate$(MAP_SEPARATOR)https://github.com/joeycumines/go-catrate.git \
    ./fangrpcstream$(MAP_SEPARATOR)https://github.com/joeycumines/go-fangrpcstream.git \
    ./logiface$(MAP_SEPARATOR)https://github.com/joeycumines/logiface.git \
    ./logiface-logrus$(MAP_SEPARATOR)https://github.com/joeycumines/ilogrus.git \
    ./logiface-stumpy$(MAP_SEPARATOR)https://github.com/joeycumines/stumpy.git \
    ./logiface-testsuite$(MAP_SEPARATOR)https://github.com/joeycumines/logiface-testsuite.git \
    ./logiface-zerolog$(MAP_SEPARATOR)https://github.com/joeycumines/izerolog.git \
    ./longpoll$(MAP_SEPARATOR)https://github.com/joeycumines/go-longpoll.git \
    ./microbatch$(MAP_SEPARATOR)https://github.com/joeycumines/go-microbatch.git \
    ./smartpoll$(MAP_SEPARATOR)https://github.com/joeycumines/go-smartpoll.git \
    ./sql$(MAP_SEPARATOR)https://github.com/joeycumines/go-sql.git \
    ./prompt$(MAP_SEPARATOR)https://github.com/joeycumines/go-prompt.git \
    ./grpc-proxy$(MAP_SEPARATOR)https://github.com/joeycumines/grpc-proxy.git \
    ./floater$(MAP_SEPARATOR)https://github.com/joeycumines/floater.git \
    ./eventloop$(MAP_SEPARATOR)https://github.com/joeycumines/go-eventloop.git \
    ./goja$(MAP_SEPARATOR)https://github.com/joeycumines/goja.git \
    ./goja-eventloop$(MAP_SEPARATOR)https://github.com/joeycumines/goja-eventloop.git \
    ./goja-grpc$(MAP_SEPARATOR)https://github.com/joeycumines/goja-grpc.git \
    ./goja-protobuf$(MAP_SEPARATOR)https://github.com/joeycumines/goja-protobuf.git \
    ./goja-protojson$(MAP_SEPARATOR)https://github.com/joeycumines/goja-protojson.git \
    ./goja_nodejs$(MAP_SEPARATOR)https://github.com/joeycumines/goja_nodejs.git \
    ./inprocgrpc$(MAP_SEPARATOR)https://github.com/joeycumines/go-inprocgrpc.git \
    ./goroutineid$(MAP_SEPARATOR)https://github.com/joeycumines/goroutineid.git
# N.B. relative to the go module it applies to
DEADCODE_IGNORE_PATTERNS_FILE = .deadcodeignore
DEADCODE_ERROR_ON_UNIGNORED = true

# Vendored upstream forks, kept byte-close to their origins to minimize
# sync deltas. They are excluded from staticcheck (fork code must not fail
# the gate on upstream style/checks) and from `go fix` (which would rewrite
# fork sources away from upstream). Everything else runs the full default
# staticcheck check set.
GO_MODULE_SLUGS_NO_STATICCHECK = goja goja_nodejs
GO_MODULE_SLUGS_NO_FIX = goja goja_nodejs

# ---

##@ Convenience targets

.PHONY: betteralign-apply
betteralign-apply: ## Apply betteralign lint fixes.
	$(MAKE) betteralign BETTERALIGN_FLAGS=-apply

.PHONY: test.race
test.race: ## Run all Go module tests with the race detector.
	$(MAKE) test GO_TEST_FLAGS="$(strip $(GO_TEST_FLAGS) -race)"

test.race.%: ## Run a Go module's tests with the race detector.
	$(MAKE) test.$* GO_TEST_FLAGS="$(strip $(GO_TEST_FLAGS) -race)"

##@ goja-eventloop oracle targets

GOJA_EVENTLOOP_ORACLE_MANIFEST ?= $(PROJECT_ROOT)/goja-eventloop/testdata/oracle/surface.json
GOJA_EVENTLOOP_ORACLE_NODE_ARCHIVE ?=
GOJA_EVENTLOOP_ORACLE_ROOT ?= $(PROJECT_ROOT)/goja-eventloop
GOJA_EVENTLOOP_ORACLE_GOWORK ?= off

.PHONY: goja-eventloop-oracle-validate
goja-eventloop-oracle-validate: ## Authenticate the finite oracle manifest in the configured resolvable module graph.
	@set -eu; \
	tmpdir="$$(mktemp -d "$${TMPDIR:-/tmp}/goja-eventloop-oracle.XXXXXX")"; \
	trap 'rm -rf "$$tmpdir"' EXIT HUP INT TERM; \
	env GOWORK="$(GOJA_EVENTLOOP_ORACLE_GOWORK)" $(GO) -C "$(GOJA_EVENTLOOP_ORACLE_ROOT)" build -mod=readonly -o "$$tmpdir/oracle" ./cmd/goja-eventloop-oracle; \
	"$$tmpdir/oracle" -validate -manifest "$(GOJA_EVENTLOOP_ORACLE_MANIFEST)"

.PHONY: goja-eventloop-oracle-run
goja-eventloop-oracle-run: ## Run every fixture against Node v26.5.0 and Goja in the configured resolvable module graph.
	@set -eu; \
	tmpdir="$$(mktemp -d "$${TMPDIR:-/tmp}/goja-eventloop-oracle.XXXXXX")"; \
	trap 'rm -rf "$$tmpdir"' EXIT HUP INT TERM; \
	env GOWORK="$(GOJA_EVENTLOOP_ORACLE_GOWORK)" $(GO) -C "$(GOJA_EVENTLOOP_ORACLE_ROOT)" build -mod=readonly -o "$$tmpdir/oracle" ./cmd/goja-eventloop-oracle; \
	archive="$(GOJA_EVENTLOOP_ORACLE_NODE_ARCHIVE)"; \
	if [ -z "$$archive" ]; then \
		platform="$$($(GO) env GOHOSTOS)/$$($(GO) env GOHOSTARCH)"; \
		case "$$platform" in \
			darwin/arm64) file=node-v26.5.0-darwin-arm64.tar.gz ;; \
			darwin/amd64) file=node-v26.5.0-darwin-x64.tar.gz ;; \
			linux/arm64) file=node-v26.5.0-linux-arm64.tar.gz ;; \
			linux/amd64) file=node-v26.5.0-linux-x64.tar.gz ;; \
			*) echo "No authenticated Node v26.5.0 artifact for $$platform" >&2; exit 2 ;; \
		esac; \
		archive="$$tmpdir/$$file"; \
		curl --fail --location --silent --show-error --output "$$archive" "https://nodejs.org/dist/v26.5.0/$$file"; \
	fi; \
	"$$tmpdir/oracle" -manifest "$(GOJA_EVENTLOOP_ORACLE_MANIFEST)" -node-archive "$$archive"

.PHONY: goja-eventloop-oracle
goja-eventloop-oracle: goja-eventloop-oracle-run ## Alias for the exact goja-eventloop oracle run.

##@ goroutineid targets

.PHONY: bench-goroutineid
bench-goroutineid: ## Run goroutineid benchmarks.
	$(GO) -C $(or $(PROJECT_ROOT),$(error If you are reading this you specified the `file` option when calling `mcp-server-make`. DONT DO THAT.))/goroutineid test -run='^$$' -bench='.' -benchmem

##@ eventloop targets

EVENTLOOP_LIVE_PACKAGES ?= . ./examples/...
GO_PACKAGES.eventloop ?= $(EVENTLOOP_LIVE_PACKAGES)

.PHONY: eventloop-live-fmt
eventloop-live-fmt: ## Format only the live eventloop product and examples.
	$(GO) -C $(PROJECT_ROOT)/eventloop fmt $(EVENTLOOP_LIVE_PACKAGES)

.PHONY: eventloop-live-test
eventloop-live-test: ## Test only the live eventloop product and examples.
	$(GO) -C $(PROJECT_ROOT)/eventloop test $(GO_FLAGS) $(GO_TEST_FLAGS) $(EVENTLOOP_LIVE_PACKAGES)

.PHONY: eventloop-live-race
eventloop-live-race: ## Race-test only the live eventloop product and examples.
	$(MAKE) eventloop-live-test GO_TEST_FLAGS="$(strip $(GO_TEST_FLAGS) -race)"

EVENTLOOP_LIVE_LINUX_SOURCE ?= $(PROJECT_ROOT)
EVENTLOOP_LIVE_LINUX_GO_TEST_FLAGS ?= -count=1 -timeout=10m
EVENTLOOP_LIVE_LINUX_TARGET ?= eventloop-live-test

.PHONY: eventloop-live-linux-test
eventloop-live-linux-test: ## Test a copied live eventloop source tree in its declared Linux Go image.
	@set -eu; \
	source='$(EVENTLOOP_LIVE_LINUX_SOURCE)'; \
	go_version="$$(awk '$$1 == "go" { print $$2; exit }' "$$source/eventloop/go.mod")"; \
	test -n "$$go_version"; \
	staging="$$(mktemp -d "$${TMPDIR:-/tmp}/eventloop-live-linux.XXXXXX")"; \
	container=''; \
	trap 'test -z "$$container" || docker rm -f "$$container" >/dev/null 2>&1 || true; rm -rf "$$staging"' EXIT HUP INT TERM; \
	container="$$(docker create --workdir /workspace --env GOFLAGS=-buildvcs=false "golang:$$go_version" \
		make '$(EVENTLOOP_LIVE_LINUX_TARGET)' 'GO_TEST_FLAGS=$(EVENTLOOP_LIVE_LINUX_GO_TEST_FLAGS)')"; \
	mkdir -p "$$staging/eventloop/internal"; \
	cp "$$source/Makefile" "$$source/project.mk" "$$staging/"; \
	find "$$source/eventloop" -maxdepth 1 -type f \( -name '*.go' -o -name go.mod -o -name go.sum \) \
		-exec cp {} "$$staging/eventloop/" \;; \
	cp -R "$$source/eventloop/examples" "$$staging/eventloop/examples"; \
	cp -R "$$source/eventloop/internal/eventlooptest" "$$staging/eventloop/internal/eventlooptest"; \
	cp -R "$$source/eventloop/testdata" "$$staging/eventloop/testdata"; \
	docker cp "$$staging/." "$$container:/workspace"; \
	docker start --attach "$$container"

.PHONY: eventloop-live-linux-race
eventloop-live-linux-race: ## Race-test a copied live eventloop source tree in its declared Linux Go image.
	$(MAKE) eventloop-live-linux-test EVENTLOOP_LIVE_LINUX_TARGET=eventloop-live-race

.PHONY: eventloop-live-js-test
eventloop-live-js-test: ## Run the complete live eventloop root suite under js/wasm.
	@set -eu; \
	runner="$$($(GO) env GOROOT)/lib/wasm/go_js_wasm_exec"; \
	test -x "$$runner"; \
	tmpdir="$$(mktemp -d "$${TMPDIR:-/tmp}/eventloop-live-js.XXXXXX")"; \
	trap 'rm -rf "$$tmpdir"' EXIT HUP INT TERM; \
	GOOS=js GOARCH=wasm $(GO) -C $(PROJECT_ROOT)/eventloop test -c -o "$$tmpdir/eventloop.test" .; \
	env -i HOME="$(HOME)" PATH="$(PATH)" "$$runner" "$$tmpdir/eventloop.test" \
		-test.count=1 -test.timeout=8m

EVENTLOOP_LIVE_CROSS_GOOS ?= aix android darwin dragonfly freebsd illumos ios js linux netbsd openbsd plan9 solaris wasip1 windows
EVENTLOOP_LIVE_CROSS_TARGETS ?=
EVENTLOOP_LIVE_CROSS_FLAGS ?=
EVENTLOOP_LIVE_CROSS_REQUIRED_GOOS = $(if $(strip $(EVENTLOOP_LIVE_CROSS_TARGETS)),,$(EVENTLOOP_LIVE_CROSS_GOOS))
EVENTLOOP_LIVE_CROSS_REQUIRED_TARGETS = $(if $(strip $(EVENTLOOP_LIVE_CROSS_TARGETS)),,$(if $(filter js,$(EVENTLOOP_LIVE_CROSS_GOOS)),js/wasm) $(if $(filter wasip1,$(EVENTLOOP_LIVE_CROSS_GOOS)),wasip1/wasm))

.PHONY: eventloop-live-cross
eventloop-live-cross: ## Check the live root on every declared pair; link tests where the configured toolchain permits.
	@set -eu; \
	targets='$(strip $(EVENTLOOP_LIVE_CROSS_TARGETS))'; \
	hostos="$$($(GO) env GOHOSTOS)"; \
	goroot="$$($(GO) env GOROOT)"; \
	if [ -z "$$targets" ]; then \
		for target in $$($(GO) tool dist list); do \
			goos="$${target%/*}"; \
			case " $(EVENTLOOP_LIVE_CROSS_GOOS) " in \
				*" $$goos "*) targets="$$targets $$target" ;; \
			esac; \
		done; \
	fi; \
	for required in $(EVENTLOOP_LIVE_CROSS_REQUIRED_GOOS); do \
		case " $$targets " in *" $$required/"*) ;; *) echo "eventloop live cross: no target for $$required" >&2; exit 1 ;; esac; \
	done; \
	for required in $(EVENTLOOP_LIVE_CROSS_REQUIRED_TARGETS); do \
		case " $$targets " in *" $$required "*) ;; *) echo "eventloop live cross: missing $$required" >&2; exit 1 ;; esac; \
	done; \
	for target in $$targets; do \
		goos="$${target%/*}"; \
		goarch="$${target#*/}"; \
		echo "eventloop live cross: $$target"; \
		case "$$goos" in \
			android) \
				echo "eventloop live cross: $$target root production compile only (tests/examples need an Android linker)"; \
				env GOOS="$$goos" GOARCH="$$goarch" CGO_ENABLED=0 GOWORK=off \
					$(GO) -C $(PROJECT_ROOT)/eventloop list $(GO_FLAGS) $(EVENTLOOP_LIVE_CROSS_FLAGS) -export . >/dev/null; \
				;; \
			ios) \
				if [ "$$hostos" = darwin ] && command -v xcrun >/dev/null 2>&1; then \
					env GOOS="$$goos" GOARCH="$$goarch" CGO_ENABLED=1 GOWORK=off CC="$$goroot/misc/ios/clangwrap.sh" \
						$(GO) -C $(PROJECT_ROOT)/eventloop test $(GO_FLAGS) $(EVENTLOOP_LIVE_CROSS_FLAGS) \
						-run='^$$' -exec=true $(EVENTLOOP_LIVE_PACKAGES); \
				else \
					echo "eventloop live cross: $$target root production compile only (tests/examples need an iOS linker)"; \
					env GOOS="$$goos" GOARCH="$$goarch" CGO_ENABLED=0 GOWORK=off \
						$(GO) -C $(PROJECT_ROOT)/eventloop list $(GO_FLAGS) $(EVENTLOOP_LIVE_CROSS_FLAGS) -export . >/dev/null; \
				fi; \
				;; \
			*) \
				env GOOS="$$goos" GOARCH="$$goarch" CGO_ENABLED=0 GOWORK=off \
					$(GO) -C $(PROJECT_ROOT)/eventloop test $(GO_FLAGS) $(EVENTLOOP_LIVE_CROSS_FLAGS) \
					-run='^$$' -exec=true $(EVENTLOOP_LIVE_PACKAGES); \
				;; \
		esac; \
	done

GOJA_EVENTLOOP_LIVE_PACKAGES ?= ./...
GOJA_EVENTLOOP_LIVE_LINKERLESS_PACKAGES ?= . ./internal/oracle
GOJA_EVENTLOOP_LIVE_ROOT ?= $(PROJECT_ROOT)/goja-eventloop
GOJA_EVENTLOOP_LIVE_GOWORK ?= off
GOJA_EVENTLOOP_LIVE_CROSS_GOOS ?= $(EVENTLOOP_LIVE_CROSS_GOOS)
GOJA_EVENTLOOP_LIVE_CROSS_TARGETS ?=
GOJA_EVENTLOOP_LIVE_CROSS_FLAGS ?= -mod=readonly
GOJA_EVENTLOOP_LIVE_CROSS_REQUIRED_GOOS = $(GOJA_EVENTLOOP_LIVE_CROSS_GOOS)
GOJA_EVENTLOOP_LIVE_CROSS_REQUIRED_TARGETS = $(if $(filter js,$(GOJA_EVENTLOOP_LIVE_CROSS_GOOS)),js/wasm) $(if $(filter wasip1,$(GOJA_EVENTLOOP_LIVE_CROSS_GOOS)),wasip1/wasm)

.PHONY: goja-eventloop-live-cross
goja-eventloop-live-cross: ## Compile the Goja adapter on every declared pair; link tests where the configured toolchain permits.
	@set -eu; \
	targets='$(strip $(GOJA_EVENTLOOP_LIVE_CROSS_TARGETS))'; \
	hostos="$$($(GO) env GOHOSTOS)"; \
	goroot="$$($(GO) env GOROOT)"; \
	if [ -z "$$targets" ]; then \
		for target in $$($(GO) tool dist list); do \
			goos="$${target%/*}"; \
			case " $(GOJA_EVENTLOOP_LIVE_CROSS_GOOS) " in \
				*" $$goos "*) targets="$$targets $$target" ;; \
			esac; \
		done; \
	fi; \
	for required in $(GOJA_EVENTLOOP_LIVE_CROSS_REQUIRED_GOOS); do \
		case " $$targets " in *" $$required/"*) ;; *) echo "goja-eventloop live cross: no target for $$required" >&2; exit 1 ;; esac; \
	done; \
	for required in $(GOJA_EVENTLOOP_LIVE_CROSS_REQUIRED_TARGETS); do \
		case " $$targets " in *" $$required "*) ;; *) echo "goja-eventloop live cross: missing $$required" >&2; exit 1 ;; esac; \
	done; \
	for target in $$targets; do \
		goos="$${target%/*}"; \
		goarch="$${target#*/}"; \
		echo "goja-eventloop live cross: $$target"; \
		case "$$goos" in \
			android) \
				if [ "$$goarch" = arm64 ]; then \
				env GOWORK="$(GOJA_EVENTLOOP_LIVE_GOWORK)" GOOS="$$goos" GOARCH="$$goarch" CGO_ENABLED=0 \
					$(GO) -C "$(GOJA_EVENTLOOP_LIVE_ROOT)" test $(GO_FLAGS) $(GOJA_EVENTLOOP_LIVE_CROSS_FLAGS) \
						-run='^$$' -exec=true $(GOJA_EVENTLOOP_LIVE_PACKAGES); \
				else \
					echo "goja-eventloop live cross: $$target adapter-library compile only (tests and oracle command need an Android linker)"; \
				env GOWORK="$(GOJA_EVENTLOOP_LIVE_GOWORK)" GOOS="$$goos" GOARCH="$$goarch" CGO_ENABLED=0 \
					$(GO) -C "$(GOJA_EVENTLOOP_LIVE_ROOT)" list $(GO_FLAGS) $(GOJA_EVENTLOOP_LIVE_CROSS_FLAGS) -export $(GOJA_EVENTLOOP_LIVE_LINKERLESS_PACKAGES) >/dev/null; \
				fi; \
				;; \
			ios) \
				if [ "$$hostos" = darwin ] && command -v xcrun >/dev/null 2>&1; then \
				env GOWORK="$(GOJA_EVENTLOOP_LIVE_GOWORK)" GOOS="$$goos" GOARCH="$$goarch" CGO_ENABLED=1 CC="$$goroot/misc/ios/clangwrap.sh" \
					$(GO) -C "$(GOJA_EVENTLOOP_LIVE_ROOT)" test $(GO_FLAGS) $(GOJA_EVENTLOOP_LIVE_CROSS_FLAGS) \
						-run='^$$' -exec=true $(GOJA_EVENTLOOP_LIVE_PACKAGES); \
				else \
					echo "goja-eventloop live cross: $$target adapter-library compile only (tests and oracle command need an iOS linker)"; \
				env GOWORK="$(GOJA_EVENTLOOP_LIVE_GOWORK)" GOOS="$$goos" GOARCH="$$goarch" CGO_ENABLED=0 \
					$(GO) -C "$(GOJA_EVENTLOOP_LIVE_ROOT)" list $(GO_FLAGS) $(GOJA_EVENTLOOP_LIVE_CROSS_FLAGS) -export $(GOJA_EVENTLOOP_LIVE_LINKERLESS_PACKAGES) >/dev/null; \
				fi; \
				;; \
			*) \
			env GOWORK="$(GOJA_EVENTLOOP_LIVE_GOWORK)" GOOS="$$goos" GOARCH="$$goarch" CGO_ENABLED=0 \
				$(GO) -C "$(GOJA_EVENTLOOP_LIVE_ROOT)" test $(GO_FLAGS) $(GOJA_EVENTLOOP_LIVE_CROSS_FLAGS) \
					-run='^$$' -exec=true $(GOJA_EVENTLOOP_LIVE_PACKAGES); \
				;; \
		esac; \
	done

GOJA_EVENTLOOP_PRODUCT_ROOT ?= $(PROJECT_ROOT)/goja-eventloop
GOJA_EVENTLOOP_PRODUCT_GOWORK ?= off
GOJA_EVENTLOOP_PRODUCT_LIFECYCLE_RE ?= ^Test(TerminateBenchmarkLoopBoundsEveryOperation|PromiseJobBenchmarkIterationTailWaitsJobReturn|ProcessBeforeExitTimerBenchmarkFixtureLifecycle)$$
GOJA_EVENTLOOP_PRODUCT_BENCH_RE ?= ^Benchmark(AdapterAsyncAwaitResolve|AdapterPromiseThenChain|ProcessBeforeExitTimerEndToEnd)$$
GOJA_EVENTLOOP_PRODUCT_LIFECYCLE_COUNT ?= 20
GOJA_EVENTLOOP_PRODUCT_COUNT ?= 5
GOJA_EVENTLOOP_PRODUCT_BENCHTIME ?= 1s
GOJA_EVENTLOOP_PRODUCT_TIMEOUT ?= 30m

.PHONY: goja-eventloop-product-bench
goja-eventloop-product-bench: ## Check benchmark fixture lifecycle, then measure only GEL production paths.
	env GOWORK="$(GOJA_EVENTLOOP_PRODUCT_GOWORK)" $(GO) -C "$(GOJA_EVENTLOOP_PRODUCT_ROOT)" test $(GO_FLAGS) -mod=readonly \
		-run='$(GOJA_EVENTLOOP_PRODUCT_LIFECYCLE_RE)' \
		-count=$(GOJA_EVENTLOOP_PRODUCT_LIFECYCLE_COUNT) -timeout=$(GOJA_EVENTLOOP_PRODUCT_TIMEOUT) .
	env GOWORK="$(GOJA_EVENTLOOP_PRODUCT_GOWORK)" $(GO) -C "$(GOJA_EVENTLOOP_PRODUCT_ROOT)" test $(GO_FLAGS) -mod=readonly \
		-run='^$$' -bench='$(GOJA_EVENTLOOP_PRODUCT_BENCH_RE)' -benchmem \
		-count=$(GOJA_EVENTLOOP_PRODUCT_COUNT) -benchtime=$(GOJA_EVENTLOOP_PRODUCT_BENCHTIME) \
		-timeout=$(GOJA_EVENTLOOP_PRODUCT_TIMEOUT) .

# Focused final-product eventloop suite. This live-product lane remains
# independently runnable from the parked longitudinal comparison corpus.
EVENTLOOP_PRODUCT_BENCH_RE ?= ^Benchmark(AutoExitImmediate|AutoExitImmediateCancelableContext|ConcurrentSubmissionWakeIntegration|EventTargetDispatchEmptyReusedEvent|EventTargetDispatchFreshEvent|EventTargetDispatchParallelDistinctEvents|EventTargetDispatchParallelDistinctEventsControl|EventTargetDispatchReusedEvent|EventTargetListenerConstruction|EventTargetListenerRegistrationWithLiveSet|FDReadinessDispatchHighCount|FDReadinessDispatchSingle|IdleWakeRecoveryTurnCost|JSAdapterRegistrationLiveSet|LargeDeadlineList|MetricsCollection|MetricsHotPath|MetricsHotPathSnapshot|MicrotaskLatency|MicrotaskScheduleExternal|MicrotaskScheduleLoopThread|MixedWorkload|NextTickRecursiveDrain|NextTickScheduleExternal|NextTickScheduleLoopThread|NoMetrics|PromiseAllFixedArityEndToEnd|PromiseAllSettledFixedArityEndToEnd|PromiseAnyFixedArityEndToEnd|PromiseCreation|PromiseDepthThreeEndToEnd|PromiseRaceFixedArityEndToEnd|PromiseReactionEndToEnd|PromiseSettleNoHandler|PromisifyCompletionEndToEnd|RetentionBoundary|RetentionPostBurstSteady|RetentionPublicSubmissionTerminal|RetentionWarmedSteady|SchedulerInternalExternalBurst|SchedulerPriorityLatency|SetImmediateBurst|SetIntervalSteadyTicks|SparseFDRegistration|SubmitInternalChainHandoff|SubmitLatency|TimerCancelScale|TimerLatency|TimerScheduleRandomDeadlines|TimerScheduleSameDeadline100K)$$
EVENTLOOP_PRODUCT_SMOKE_BENCH_RE ?= ^Benchmark(MicrotaskLatency|NextTickRecursiveDrain|TimerLatency)$$
EVENTLOOP_SCHEDULER_TOURNAMENT_BENCH_RE ?= ^Benchmark(GCPressure|GCPressure_Allocations|MultiProducer|MultiProducerContention|PingPong|PingPongLatency|BurstSubmit|MicroBatchBudget_Throughput|MicroBatchBudget_Latency|MicroBatchBudget_Continuous|MicroBatchBudget_Mixed|MicroCASContention|MicroCASContention_Latency|MicrotaskRoundTrip|MicroWakeupSyscall_Running|MicroWakeupSyscall_Sleeping|MicroWakeupSyscall_Burst|MicroWakeupSyscall_RapidSubmit|PromisesV2|ReadinessRoundTrip|SchedulerPriorityLatency|SubmitInternalChainHandoff|SchedulerInternalExternalBurst)$$
EVENTLOOP_PROMISE_TOURNAMENT_BENCH_RE ?= ^Benchmark(Tournament|ChainDepth)$$
EVENTLOOP_LIBUV_TOURNAMENT_BENCH_RE ?= ^BenchmarkLibuv_
EVENTLOOP_LIBUV_LEGACY_BENCH_RE ?= ^BenchmarkLibuv_(AsyncSend|TimerScheduleAndFire|TimerRepeat|TimerCrossThread)$$
EVENTLOOP_LIBUV_V2_BENCH_RE ?= ^BenchmarkLibuv_(AsyncSendV2|TimerScheduleAndFireV2|TimerBatchOneShot100V2|TimerCrossThreadV2)$$
EVENTLOOP_LIBUV_TEST_TIMEOUT ?= 5m
EVENTLOOP_PRODUCT_COUNT ?= 5
EVENTLOOP_PRODUCT_BENCHTIME ?= 1s
EVENTLOOP_PRODUCT_TIMEOUT ?= 30m
EVENTLOOP_PRODUCT_LABEL ?= eventloop: lane=product
EVENTLOOP_PRODUCT_FLAGS = -benchmem -count=$(EVENTLOOP_PRODUCT_COUNT) -run='^$$' -benchtime=$(EVENTLOOP_PRODUCT_BENCHTIME) -timeout=$(EVENTLOOP_PRODUCT_TIMEOUT)
EVENTLOOP_TOURNAMENT_COUNT ?= 5
EVENTLOOP_TOURNAMENT_BENCHTIME ?= 1s
EVENTLOOP_TOURNAMENT_TIMEOUT ?= 30m
EVENTLOOP_TOURNAMENT_FLAGS = -benchmem -count=$(EVENTLOOP_TOURNAMENT_COUNT) -run='^$$' -benchtime=$(EVENTLOOP_TOURNAMENT_BENCHTIME) -timeout=$(EVENTLOOP_TOURNAMENT_TIMEOUT)

.PHONY: eventloop-product-bench
eventloop-product-bench: ## Run the current eventloop product benchmark lane.
	@echo '$(EVENTLOOP_PRODUCT_LABEL)'
	$(GO) -C $(PROJECT_ROOT)/eventloop test -bench='$(EVENTLOOP_PRODUCT_BENCH_RE)' $(EVENTLOOP_PRODUCT_FLAGS) .

.PHONY: eventloop-product-bench-smoke
eventloop-product-bench-smoke: ## Smoke-test the live scheduling benchmark entry points once.
	@echo 'eventloop: lane=product-smoke'
	$(GO) -C $(PROJECT_ROOT)/eventloop test -bench='$(EVENTLOOP_PRODUCT_SMOKE_BENCH_RE)' -benchmem -count=1 -run='^$$' -benchtime=1x -timeout=5m .

.PHONY: eventloop-tournament-scheduler-bench
eventloop-tournament-scheduler-bench: ## Compare current, historical, and Goja-baseline schedulers.
	@echo 'tournament: lane=scheduler'
	$(GO) -C $(PROJECT_ROOT)/eventloop/internal/tournament test -bench='$(EVENTLOOP_SCHEDULER_TOURNAMENT_BENCH_RE)' $(EVENTLOOP_TOURNAMENT_FLAGS) .

.PHONY: eventloop-tournament-test
eventloop-tournament-test: eventloop-tournament-component-cross eventloop-tournament-tool-test ## Verify every restored implementation before measuring it.
	$(GO) -C $(PROJECT_ROOT)/eventloop test -count=1 -timeout=$(EVENTLOOP_TOURNAMENT_TIMEOUT) ./internal/...
	$(GO) -C $(PROJECT_ROOT)/eventloop/internal/tournament test -count=1 -timeout=$(EVENTLOOP_TOURNAMENT_TIMEOUT) ./...

.PHONY: eventloop-tournament-component-test
eventloop-tournament-component-test: ## Verify isolated FD-table and timer variants, policy, and provenance without benchmarks.
	$(GO) -C $(PROJECT_ROOT)/eventloop/internal/tournament test $(GO_TEST_FLAGS) -count=1 -run='^Test(FDTable|Timer)' .
	$(GO) -C $(PROJECT_ROOT)/eventloop/internal/tournament test $(GO_TEST_FLAGS) -count=1 ./component/...

.PHONY: eventloop-tournament-component-cross
eventloop-tournament-component-cross: ## Compile FD-table and timer registries and component tests for every supported and fallback target.
	@set -eu; \
	output="$$(mktemp -d "$${TMPDIR:-/tmp}/eventloop-tournament-component-cross.XXXXXX")"; \
	trap 'rm -rf "$$output"' EXIT HUP INT TERM; \
	component_packages="$$(GOWORK=off $(GO) -C $(PROJECT_ROOT)/eventloop/internal/tournament list ./component/...)"; \
	for target in darwin/arm64 darwin/amd64 linux/arm64 linux/amd64 windows/arm64 windows/amd64 js/wasm wasip1/wasm plan9/amd64; do \
		goos="$${target%/*}"; goarch="$${target#*/}"; package_index=0; \
		case "$$goos" in darwin|linux|windows) packages=". $$component_packages" ;; *) packages="$$component_packages" ;; esac; \
		echo "Compiling tournament components for $$goos/$$goarch"; \
		for package in $$packages; do \
			package_index=$$((package_index + 1)); \
			env GOOS="$$goos" GOARCH="$$goarch" CGO_ENABLED=0 GOWORK=off $(GO) -C $(PROJECT_ROOT)/eventloop/internal/tournament test -c -o "$$output/$${goos}-$${goarch}-$${package_index}.test" "$$package"; \
		done; \
	done

.PHONY: eventloop-tournament-retention-test
EVENTLOOP_TOURNAMENT_RETENTION_TEST_RE ?= ^(TestActiveBenchmarkRootsGovernedOrDisposed|TestBenchmarkRootProjectionSchema5.*|TestHarnessIndexCandidateReconstructsExactAuthority|TestHistoricalTournamentSourcesRemainExact|TestImmutableBaseRootBenchmarksRemainExact|TestLineageRetainsEveryKnownSemanticFamily|TestManifestCoverageContract|TestManifestRetainsImmutableTournamentCatalog|TestRestoredHarnessArchiveReconstructsExactAuthority|TestTimerIndexCandidateReconstructsExactAuthority|TestTournamentCandidateTrackedPathCensus|TestTournamentCandidateTreeReconstructsTrackedScope|TestTournamentHistoricalDirectoriesRemainPhysical|TestTournamentWorkspaceEntriesRemainRegistered|TestTrackedTournamentRawEvidenceExact|TestUnreachableCommitPayloadsRemainExact|TestUnreachablePatchesReconstructExactTrees)$$
eventloop-tournament-retention-test: ## Verify exact retained tournament source and revision reconstruction.
	$(GO) -C $(PROJECT_ROOT)/eventloop/internal/tournament test $(GO_TEST_FLAGS) -count=1 -run='$(EVENTLOOP_TOURNAMENT_RETENTION_TEST_RE)' .

.PHONY: eventloop-tournament-manifest-test
eventloop-tournament-manifest-test: ## Verify the live tournament manifest independently of active-root admission.
	$(GO) -C $(PROJECT_ROOT)/eventloop/internal/tournament test $(GO_TEST_FLAGS) -run='^TestManifest' .

.PHONY: eventloop-tournament-tool-test
eventloop-tournament-tool-test: ## Verify manifest, parser, comparison, and fingerprint tooling.
	$(GO) -C $(PROJECT_ROOT)/eventloop test $(GO_TEST_FLAGS) ./internal/tournamentmeta
	PYTHONDONTWRITEBYTECODE=1 python3 -m unittest \
		eventloop.docs.tournament.parse_benchmarks_test \
		eventloop.docs.tournament.protocol_identity_test \
		eventloop.docs.tournament.timer_workload_digest_test

.PHONY: eventloop-tournament-helper-test
eventloop-tournament-helper-test: ## Verify the frozen-source and process-containment helper only.
	$(GO) -C $(PROJECT_ROOT)/eventloop test $(GO_TEST_FLAGS) ./internal/tournamentmeta

.PHONY: eventloop-tournament-helper-cross
eventloop-tournament-helper-cross: ## Compile the helper tests for every governed source target.
	@set -eu; \
	output="$$(mktemp -d "$${TMPDIR:-/tmp}/eventloop-tournament-helper-cross.XXXXXX")"; \
	trap 'rm -rf "$$output"' EXIT HUP INT TERM; \
	for target in darwin/arm64 darwin/amd64 linux/arm64 linux/amd64 windows/arm64 windows/amd64 js/wasm wasip1/wasm plan9/amd64; do \
		goos="$${target%/*}"; goarch="$${target#*/}"; \
		echo "Compiling tournament helper for $$goos/$$goarch"; \
		env GOOS="$$goos" GOARCH="$$goarch" CGO_ENABLED=0 GOWORK=off $(GO) -C $(PROJECT_ROOT)/eventloop test -c -o "$$output/$${goos}-$${goarch}.test" ./internal/tournamentmeta; \
	done

EVENTLOOP_TOURNAMENT_HISTORY_OUTPUT ?= $(PROJECT_ROOT)/eventloop/internal/tournament/source_history.json
EVENTLOOP_TOURNAMENT_HISTORY_FLOOR_DIRECTORY ?= $(PROJECT_ROOT)/eventloop/internal/tournament/historyfloors
EVENTLOOP_TOURNAMENT_HISTORY_FLOOR_OUTPUT ?= $(EVENTLOOP_TOURNAMENT_HISTORY_FLOOR_DIRECTORY)/000002.json
EVENTLOOP_TOURNAMENT_LINEAGE_OUTPUT ?= $(PROJECT_ROOT)/eventloop/internal/tournament/lineage.json
EVENTLOOP_TOURNAMENT_LINEAGE_FLOOR_DIRECTORY ?= $(PROJECT_ROOT)/eventloop/internal/tournament/lineagefloors
EVENTLOOP_TOURNAMENT_LINEAGE_FLOOR_OUTPUT ?= $(EVENTLOOP_TOURNAMENT_LINEAGE_FLOOR_DIRECTORY)/000003.json

.PHONY: eventloop-tournament-history-generate
eventloop-tournament-history-generate: ## Generate a new exhaustive source-history registry.
	@set -eu; \
	output="$(EVENTLOOP_TOURNAMENT_HISTORY_OUTPUT)"; \
	test -n "$$output"; \
	helper_dir="$$(mktemp -d "$${TMPDIR:-/tmp}/eventloop-tournament-history.XXXXXX")"; \
	trap 'rm -rf "$$helper_dir"' EXIT HUP INT TERM; \
	GOWORK=off $(GO) -C $(PROJECT_ROOT)/eventloop build -o "$$helper_dir/tournamentmeta" ./internal/tournamentmeta; \
	git_path="$$(command -v git)"; \
	"$$helper_dir/tournamentmeta" history generate -git "$$git_path" -repository "$(PROJECT_ROOT)" -output "$$output"

.PHONY: eventloop-tournament-history-floor-generate
eventloop-tournament-history-floor-generate: ## Append a new immutable source-history record floor.
	@set -eu; \
	floor_directory="$(EVENTLOOP_TOURNAMENT_HISTORY_FLOOR_DIRECTORY)"; \
	output="$(EVENTLOOP_TOURNAMENT_HISTORY_FLOOR_OUTPUT)"; \
	test -n "$$floor_directory"; \
	test -n "$$output"; \
	mkdir -p "$$floor_directory"; \
	helper_dir="$$(mktemp -d "$${TMPDIR:-/tmp}/eventloop-tournament-history.XXXXXX")"; \
	trap 'rm -rf "$$helper_dir"' EXIT HUP INT TERM; \
	GOWORK=off $(GO) -C $(PROJECT_ROOT)/eventloop build -o "$$helper_dir/tournamentmeta" ./internal/tournamentmeta; \
	"$$helper_dir/tournamentmeta" history floor-generate \
		-inventory "$(PROJECT_ROOT)/eventloop/internal/tournament/source_history.json" \
		-floor-directory "$$floor_directory" \
		-output "$$output"

.PHONY: eventloop-tournament-history-verify
eventloop-tournament-history-verify: ## Verify static and live exhaustive source-history closure.
	@set -eu; \
	helper_dir="$$(mktemp -d "$${TMPDIR:-/tmp}/eventloop-tournament-history.XXXXXX")"; \
	trap 'rm -rf "$$helper_dir"' EXIT HUP INT TERM; \
	GOWORK=off $(GO) -C $(PROJECT_ROOT)/eventloop build -o "$$helper_dir/tournamentmeta" ./internal/tournamentmeta; \
	git_path="$$(command -v git)"; \
	"$$helper_dir/tournamentmeta" history verify -inventory "$(PROJECT_ROOT)/eventloop/internal/tournament/source_history.json"; \
	"$$helper_dir/tournamentmeta" history audit-live -git "$$git_path" -repository "$(PROJECT_ROOT)" -inventory "$(PROJECT_ROOT)/eventloop/internal/tournament/source_history.json"

.PHONY: eventloop-tournament-lineage-floor-generate
eventloop-tournament-lineage-floor-generate: ## Append a new immutable tournament-lineage record floor.
	@set -eu; \
	floor_directory="$(EVENTLOOP_TOURNAMENT_LINEAGE_FLOOR_DIRECTORY)"; \
	output="$(EVENTLOOP_TOURNAMENT_LINEAGE_FLOOR_OUTPUT)"; \
	test -n "$$floor_directory"; \
	test -n "$$output"; \
	mkdir -p "$$floor_directory"; \
	helper_dir="$$(mktemp -d "$${TMPDIR:-/tmp}/eventloop-tournament-lineage.XXXXXX")"; \
	trap 'rm -rf "$$helper_dir"' EXIT HUP INT TERM; \
	GOWORK=off $(GO) -C $(PROJECT_ROOT)/eventloop build -o "$$helper_dir/tournamentmeta" ./internal/tournamentmeta; \
	"$$helper_dir/tournamentmeta" lineage floor-generate \
		-inventory "$(EVENTLOOP_TOURNAMENT_LINEAGE_OUTPUT)" \
		-source-history "$(PROJECT_ROOT)/eventloop/internal/tournament/source_history.json" \
		-floor-directory "$$floor_directory" \
		-output "$$output"

.PHONY: eventloop-tournament-lineage-verify
eventloop-tournament-lineage-verify: ## Verify the append-only tournament-lineage authority.
	@set -eu; \
	helper_dir="$$(mktemp -d "$${TMPDIR:-/tmp}/eventloop-tournament-lineage.XXXXXX")"; \
	trap 'rm -rf "$$helper_dir"' EXIT HUP INT TERM; \
	GOWORK=off $(GO) -C $(PROJECT_ROOT)/eventloop build -o "$$helper_dir/tournamentmeta" ./internal/tournamentmeta; \
	"$$helper_dir/tournamentmeta" lineage verify \
		-inventory "$(EVENTLOOP_TOURNAMENT_LINEAGE_OUTPUT)" \
		-source-history "$(PROJECT_ROOT)/eventloop/internal/tournament/source_history.json"

.PHONY: eventloop-tournament-source-fingerprint
eventloop-tournament-source-fingerprint: ## Print the governed source fingerprint, excluding dated evidence.
	@set -eu; \
	fingerprint_dir="$$(mktemp -d "$${TMPDIR:-/tmp}/go-utilpkg-eventloop-fingerprint.XXXXXX")"; \
	fingerprint_dir="$$(cd "$$fingerprint_dir" && pwd -P)"; \
	trap 'rm -rf "$$fingerprint_dir"' EXIT HUP INT TERM; \
	go_path="$$(command -v $(GO))"; \
	test -n "$$go_path"; \
	module_cache="$$(GOWORK=off "$$go_path" env GOMODCACHE)"; \
	test -d "$$module_cache"; \
	mkdir -p "$$fingerprint_dir/build-cache" "$$fingerprint_dir/scratch"; \
	GOWORK=off "$$go_path" -C $(PROJECT_ROOT)/eventloop build -o "$$fingerprint_dir/tournamentmeta" ./internal/tournamentmeta; \
	"$$fingerprint_dir/tournamentmeta" source-fingerprint \
		-root "$(PROJECT_ROOT)" \
		-go "$$go_path" \
		-gomodcache "$$module_cache" \
		-gocache "$$fingerprint_dir/build-cache" \
		-go-scratch "$$fingerprint_dir/scratch"

.PHONY: eventloop-tournament-source-identity
eventloop-tournament-source-identity: _EVENTLOOP_SOURCE_IDENTITY_FORMAT := json
eventloop-tournament-source-identity: ## Print platform-neutral and capture-specific governed source identities.

.PHONY: eventloop-tournament-source-identity-raw-v2
eventloop-tournament-source-identity-raw-v2: _EVENTLOOP_SOURCE_IDENTITY_FORMAT := raw-v2
eventloop-tournament-source-identity-raw-v2: ## Print one raw-v2 identity marker set without running benchmarks.

eventloop-tournament-source-identity eventloop-tournament-source-identity-raw-v2:
	@set -eu; \
	identity_dir="$$(mktemp -d "$${TMPDIR:-/tmp}/go-utilpkg-eventloop-identity.XXXXXX")"; \
	identity_dir="$$(cd "$$identity_dir" && pwd -P)"; \
	trap 'rm -rf "$$identity_dir"' EXIT HUP INT TERM; \
	go_path="$$(command -v $(GO))"; \
	test -n "$$go_path"; \
	module_cache="$$(GOWORK=off "$$go_path" env GOMODCACHE)"; \
	test -d "$$module_cache"; \
	mkdir -p "$$identity_dir/build-cache" "$$identity_dir/scratch"; \
	GOWORK=off "$$go_path" -C $(PROJECT_ROOT)/eventloop build -o "$$identity_dir/tournamentmeta" ./internal/tournamentmeta; \
	"$$identity_dir/tournamentmeta" source-identity \
		-format "$(_EVENTLOOP_SOURCE_IDENTITY_FORMAT)" \
		-root "$(PROJECT_ROOT)" \
		-go "$$go_path" \
		-gomodcache "$$module_cache" \
		-gocache "$$identity_dir/build-cache" \
		-go-scratch "$$identity_dir/scratch"

.PHONY: eventloop-tournament-promise-bench
eventloop-tournament-promise-bench: ## Compare every implemented Promise design.
	@echo 'tournament: lane=promise'
	$(GO) -C $(PROJECT_ROOT)/eventloop test -bench='$(EVENTLOOP_PROMISE_TOURNAMENT_BENCH_RE)' $(EVENTLOOP_TOURNAMENT_FLAGS) ./internal/promisetournament

.PHONY: eventloop-tournament-libuv-test
eventloop-tournament-libuv-test: ## Verify the native libuv baselines without measuring them.
	@pkg-config --exists libuv || { echo 'libuv is unavailable through pkg-config' >&2; exit 2; }
	$(GO) -C $(PROJECT_ROOT)/eventloop test -tags=libuv -count=1 -timeout=$(EVENTLOOP_LIBUV_TEST_TIMEOUT) $(GO_TEST_FLAGS) ./internal/libuvbaseline

.PHONY: eventloop-tournament-libuv-bench
eventloop-tournament-libuv-bench: eventloop-tournament-libuv-test ## Run the native libuv baseline (requires pkg-config libuv).
	@echo 'tournament: lane=libuv'
	@echo 'tournament: libuv-phase=v2'
	$(GO) -C $(PROJECT_ROOT)/eventloop test -tags=libuv -bench='$(EVENTLOOP_LIBUV_V2_BENCH_RE)' $(EVENTLOOP_TOURNAMENT_FLAGS) ./internal/libuvbaseline
	@echo 'tournament: libuv-phase=legacy'
	$(GO) -C $(PROJECT_ROOT)/eventloop test -tags=libuv -bench='$(EVENTLOOP_LIBUV_LEGACY_BENCH_RE)' $(EVENTLOOP_TOURNAMENT_FLAGS) ./internal/libuvbaseline

.PHONY: eventloop-tournament-mod-download
eventloop-tournament-mod-download: ## Pre-populate the module cache for hermetic source fingerprinting.
	GOPRIVATE= GONOSUMDB=* GONOSUMCHECK=* GOWORK=off $(GO) -C $(PROJECT_ROOT)/goja-eventloop mod download
	GOPRIVATE= GONOSUMDB=* GONOSUMCHECK=* GOWORK=off $(GO) -C $(PROJECT_ROOT)/goja-grpc mod download
	GOPRIVATE= GONOSUMDB=* GONOSUMCHECK=* GOWORK=off $(GO) -C $(PROJECT_ROOT)/goja-protobuf mod download
	GOPRIVATE= GONOSUMDB=* GONOSUMCHECK=* GOWORK=off $(GO) -C $(PROJECT_ROOT)/goja-protojson mod download
	GOPRIVATE= GONOSUMDB=* GONOSUMCHECK=* GOWORK=off $(GO) -C $(PROJECT_ROOT)/eventloop mod download
	GOPRIVATE= GONOSUMDB=* GONOSUMCHECK=* GOWORK=off $(GO) -C $(PROJECT_ROOT)/eventloop/internal/tournament mod download
	GOPRIVATE= GONOSUMDB=* GONOSUMCHECK=* GOWORK=off $(GO) -C $(PROJECT_ROOT)/eventloop/internal/gojabaseline mod download

.PHONY: eventloop-tournament-bench
eventloop-tournament-bench: ## Run the complete longitudinal eventloop tournament.
	@echo 'tournament: schema=1'
	@echo 'tournament: meta=head='"$$([ -n "$(EVENTLOOP_TOURNAMENT_HEAD)" ] && printf '%s' "$(EVENTLOOP_TOURNAMENT_HEAD)" || git -C $(PROJECT_ROOT) rev-parse HEAD 2>/dev/null)"
	@echo 'tournament: meta=source-state='"$$([ -n "$(EVENTLOOP_TOURNAMENT_SOURCE_STATE)" ] && printf '%s' "$(EVENTLOOP_TOURNAMENT_SOURCE_STATE)" || { status="$$(git -C $(PROJECT_ROOT) status --porcelain=v1 --untracked-files=all -- eventloop go.work project.mk 2>/dev/null)"; test $$? -eq 0 || { echo unknown; exit; }; test -z "$$status" && echo clean || echo dirty; })"
	@echo 'tournament: meta=go-version='"$$($(GO) version)"
	@echo 'tournament: meta=sample-count=$(EVENTLOOP_TOURNAMENT_COUNT)'
	@echo 'tournament: meta=manifest-git-blob='"$$(git -C $(PROJECT_ROOT) hash-object eventloop/internal/tournament/manifest.json)"
	@echo 'tournament: meta=goja-fork-version='"$$($(GO) -C $(PROJECT_ROOT)/eventloop/internal/tournament list -m -f '{{.Version}}' github.com/joeycumines/goja)"
	@echo 'tournament: meta=goja-nodejs-version='"$$($(GO) -C $(PROJECT_ROOT)/eventloop/internal/tournament list -m -f '{{.Version}}' github.com/joeycumines/goja_nodejs)"
	@$(MAKE) --no-print-directory eventloop-tournament-mod-download
	@source_fingerprint="$$($(MAKE) --no-print-directory eventloop-tournament-source-fingerprint)"; \
		echo 'tournament: meta=source-fingerprint='"$$source_fingerprint"
	@$(MAKE) --no-print-directory eventloop-tournament-test
	@$(MAKE) --no-print-directory eventloop-product-bench
	@$(MAKE) --no-print-directory eventloop-tournament-scheduler-bench
	@$(MAKE) --no-print-directory eventloop-tournament-promise-bench
	@if command -v pkg-config >/dev/null 2>&1 && pkg-config --exists libuv; then \
		echo 'tournament: meta=libuv-version='"$$(pkg-config --modversion libuv)"; \
		$(MAKE) --no-print-directory eventloop-tournament-libuv-bench; \
	else \
		echo 'tournament: skip=libuv:pkg-config-libuv-unavailable'; \
	fi
	@echo 'tournament: complete'

.PHONY: eventloop-tournament-compare
eventloop-tournament-compare: ## Compare two controlled raw tournament logs with pinned benchstat.
	@test -n "$(OLD_LOG)" || { echo 'Set OLD_LOG to the previous raw tournament log.' >&2; exit 2; }
	@test -n "$(NEW_LOG)" || { echo 'Set NEW_LOG to the current raw tournament log.' >&2; exit 2; }
	@test -f "$(OLD_LOG)" || { echo 'OLD_LOG does not exist: $(OLD_LOG)' >&2; exit 2; }
	@test -f "$(NEW_LOG)" || { echo 'NEW_LOG does not exist: $(NEW_LOG)' >&2; exit 2; }
	$(GO) -C $(PROJECT_ROOT) tool benchstat -ignore tournament,tournament-revision "$(OLD_LOG)" "$(NEW_LOG)"

.PHONY: eventloop-tournament-revision-bench
eventloop-tournament-revision-bench: ## Run every benchmark at a governed revision ID or commit.
	@test -n "$(REVISION)" || { echo 'Set REVISION to a revision ID or commit from the tournament manifest.' >&2; exit 2; }
	@set -eu; \
	revision_record="$$(python3 -c 'import json, sys; manifest = json.load(open(sys.argv[1], encoding="utf-8")); requested = sys.argv[2]; match = next((item for item in manifest["revision_variants"] if requested in (item["id"], item["commit"])), None); match is not None or sys.exit("REVISION is not governed by the tournament manifest: " + requested); print(match["id"] + "\t" + match["commit"])' "$(PROJECT_ROOT)/eventloop/internal/tournament/manifest.json" "$(REVISION)")"; \
	revision_id="$$(printf '%s\n' "$$revision_record" | cut -f1)"; \
	revision_ref="$$(printf '%s\n' "$$revision_record" | cut -f2)"; \
	if test "$$revision_ref" = current; then \
		echo 'tournament-revision: id='"$$revision_id"; \
		echo 'tournament-revision: source=live-governed-worktree'; \
		$(MAKE) --no-print-directory eventloop-tournament-bench; \
		echo 'tournament-revision: complete'; \
		exit 0; \
	fi; \
	revision_commit="$$(git -C $(PROJECT_ROOT) rev-parse --verify "$${revision_ref}^{commit}")"; \
	revision_tree="$$(git -C $(PROJECT_ROOT) rev-parse --verify "$${revision_commit}^{tree}")"; \
	revision_dir="$$(mktemp -d "$${TMPDIR:-/tmp}/go-utilpkg-eventloop-revision.XXXXXX")"; \
	revision_dir="$$(cd "$$revision_dir" && pwd -P)"; \
	trap 'rm -rf "$$revision_dir"' EXIT HUP INT TERM; \
	echo 'tournament-revision: id='"$$revision_id"; \
	echo 'tournament-revision: commit='"$$revision_commit"; \
	echo 'tournament-revision: tree='"$$revision_tree"; \
	echo 'tournament-revision: go-version='"$$($(GO) version)"; \
	git -C $(PROJECT_ROOT) archive -o "$$revision_dir/source.tar" "$$revision_commit"; \
	tar -xf "$$revision_dir/source.tar" -C "$$revision_dir"; \
	rm "$$revision_dir/source.tar"; \
	revision_work=off; \
	if test -f "$$revision_dir/go.work"; then revision_work="$$revision_dir/go.work"; fi; \
	echo 'tournament-revision: workspace='"$$revision_work"; \
	GOWORK="$$revision_work" GOFLAGS=-buildvcs=false $(GO) -C "$$revision_dir/eventloop" test -bench='.' $(EVENTLOOP_TOURNAMENT_FLAGS) ./...; \
	if test -f "$$revision_dir/eventloop/internal/tournament/go.mod"; then \
		GOWORK="$$revision_work" GOFLAGS=-buildvcs=false $(GO) -C "$$revision_dir/eventloop/internal/tournament" test -bench='.' $(EVENTLOOP_TOURNAMENT_FLAGS) .; \
	fi; \
	if command -v pkg-config >/dev/null 2>&1 && pkg-config --exists libuv && test -d "$$revision_dir/eventloop/internal/libuvbaseline"; then \
		GOWORK="$$revision_work" GOFLAGS=-buildvcs=false $(GO) -C "$$revision_dir/eventloop" test -tags=libuv -bench='$(EVENTLOOP_LIBUV_TOURNAMENT_BENCH_RE)' $(EVENTLOOP_TOURNAMENT_FLAGS) ./internal/libuvbaseline; \
	fi; \
	echo 'tournament-revision: complete'

# ---
