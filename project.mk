# Extensions to the standard build process for the project.

# TODO update this if the root module gets packages
GO_MODULE_SLUGS_NO_PACKAGES = root
GO_MODULE_SLUGS_NO_UPDATE = sql.export.mysql
GO_MODULE_SLUGS_NO_BETTERALIGN = prompt
GRIT_SRC ?= https://github.com/joeycumines/go-utilpkg.git
GRIT_DST ?= \
    catrate$(MAP_SEPARATOR)https://github.com/joeycumines/go-catrate.git \
    fangrpcstream$(MAP_SEPARATOR)https://github.com/joeycumines/go-fangrpcstream.git \
    logiface$(MAP_SEPARATOR)https://github.com/joeycumines/logiface.git \
    logiface-logrus$(MAP_SEPARATOR)https://github.com/joeycumines/ilogrus.git \
    logiface-stumpy$(MAP_SEPARATOR)https://github.com/joeycumines/stumpy.git \
    logiface-testsuite$(MAP_SEPARATOR)https://github.com/joeycumines/logiface-testsuite.git \
    logiface-zerolog$(MAP_SEPARATOR)https://github.com/joeycumines/izerolog.git \
    longpoll$(MAP_SEPARATOR)https://github.com/joeycumines/go-longpoll.git \
    microbatch$(MAP_SEPARATOR)https://github.com/joeycumines/go-microbatch.git \
    smartpoll$(MAP_SEPARATOR)https://github.com/joeycumines/go-smartpoll.git \
    sql$(MAP_SEPARATOR)https://github.com/joeycumines/go-sql.git \
    prompt$(MAP_SEPARATOR)https://github.com/joeycumines/go-prompt.git \
    grpc-proxy$(MAP_SEPARATOR)https://github.com/joeycumines/grpc-proxy.git \
    floater$(MAP_SEPARATOR)https://github.com/joeycumines/floater.git \
    goja-eventloop$(MAP_SEPARATOR)https://github.com/joeycumines/goja-eventloop.git
# N.B. relative to the go module it applies to
DEADCODE_IGNORE_PATTERNS_FILE = .deadcodeignore
DEADCODE_ERROR_ON_UNIGNORED = true

.PHONY: betteralign-apply
betteralign-apply:
	$(MAKE) betteralign BETTERALIGN_FLAGS=-apply

# Run specific promise regression test 5 times
.PHONY: test-promise-race-concurrent
test-promise-race-concurrent:
	cd $(PROJECT_ROOT)/eventloop && go test -v -run TestPromiseRace_ConcurrentThenReject_HandlersCalled -count=5 2>&1 | tail -100

# Race detector for eventloop
.PHONY: race-eventloop
race-eventloop:
	cd $(PROJECT_ROOT) && go test -race -timeout=5m ./eventloop/... 2>&1 | tail -50

# Race detector for eventloop - detailed failures
.PHONY: race-eventloop-failures
race-eventloop-failures:
	cd $(PROJECT_ROOT) && go test -race -timeout=5m ./eventloop/... 2>&1 | tee /tmp/race_test.log | grep -E "(^=== RUN|---.*FAIL|race detected|DATA RACE|^FAIL|^panic|panic:|Warning)" | head -100; echo "Exit: $$?"

# Race test with full output
.PHONY: race-eventloop-full
race-eventloop-full:
	cd $(PROJECT_ROOT) && go test -race -timeout=6m ./eventloop/... 2>&1 | tee /tmp/race_test.log; echo "Test exit: $$?"

# EXPAND-049: Cleanup leftover *_fixed.go files
.PHONY: cleanup-fixed-files
cleanup-fixed-files:
	rm -f $(PROJECT_ROOT)/goja-eventloop/base64_test_fixed.go
	rm -f $(PROJECT_ROOT)/goja-eventloop/crypto_test_fixed.go
	rm -f $(PROJECT_ROOT)/goja-eventloop/nexttick_test_fixed.go
	@echo "Cleaned up *_fixed.go files"
