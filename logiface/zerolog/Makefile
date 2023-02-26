# Usage: make [target]
#
# Note: You can run from the root like make run.logiface.zerolog (default
# target), or make run-<target>.logiface.zerolog.
#
# Targets:
#   all: Default target, currently just runs bench.
#   clean: Removes all generated files, i.e. benchmarks/, which contains the results.
#   bench: Outputs results for all benchmarks, running them if necessary.
#   bench-<lowercase benchmark name, e.g. disabled>: Similar behavior to bench, for a single benchmark.
#   list: Outputs a list of all targets, will only work on some systems.
#   debug-env: Outputs debug information about the environment.

GO ?= go
BENCHSTAT ?= benchstat
BENCHSTAT_FLAGS ?= -col /variant -row .name
BENCHMARK_FLAGS ?= -count=10 -benchmem
BENCHMARK_DIR ?= benchmarks
ifeq ($(BENCHMARK_DIR),)
$(error BENCHMARK_DIR must not be empty)
endif

BENCHMARK_NAMES = \
	Disabled \
	Info \
	ContextFields \
	ContextAppend \
	LogFields \

# must not be whitespace and must not be contained in BENCHMARK_NAMES, case insensitive
BENCHMARK_NAME_SEPARATOR = =

lc = $(subst A,a,$(subst B,b,$(subst C,c,$(subst D,d,$(subst E,e,$(subst F,f,$(subst G,g,$(subst H,h,$(subst I,i,$(subst J,j,$(subst K,k,$(subst L,l,$(subst M,m,$(subst N,n,$(subst O,o,$(subst P,p,$(subst Q,q,$(subst R,r,$(subst S,s,$(subst T,t,$(subst U,u,$(subst V,v,$(subst W,w,$(subst X,x,$(subst Y,y,$(subst Z,z,$1))))))))))))))))))))))))))

BENCHMARK_TARGETS := $(addprefix bench-,$(call lc,$(BENCHMARK_NAMES)))
BENCHMARK_FILES := $(addprefix $(BENCHMARK_DIR)/,$(addsuffix .txt,$(call lc,$(BENCHMARK_NAMES))))
BENCHMARK_NAME_TO_LC := $(foreach name,$(BENCHMARK_NAMES),$(name)$(BENCHMARK_NAME_SEPARATOR)$(call lc,$(name)))
benchmark_lc_to_name = $(patsubst %$(BENCHMARK_NAME_SEPARATOR)$1,%,$(filter %$(BENCHMARK_NAME_SEPARATOR)$1,$(BENCHMARK_NAME_TO_LC)))

all: bench

clean:
ifeq ($(OS),Windows_NT)
	del /Q /S $(BENCHMARK_DIR)
else
	rm -rf $(BENCHMARK_DIR)
endif

# could have defined BENCHMARK_FILES as prerequisites, didn't to avoid parallelism
.PHONY: bench
bench:
	$(foreach target,$(BENCHMARK_FILES),$(_bench__TEMPLATE))
	$(BENCHSTAT) $(BENCHSTAT_FLAGS) $(BENCHMARK_FILES)
define _bench__TEMPLATE =
	@$(MAKE) --no-print-directory $(target)

endef

.PHONY: $(BENCHMARK_TARGETS)
$(BENCHMARK_TARGETS): bench-%: $(BENCHMARK_DIR)/%.txt
	$(BENCHSTAT) $(BENCHSTAT_FLAGS) $(BENCHMARK_DIR)/$*.txt

$(BENCHMARK_FILES): $(BENCHMARK_DIR)/%.txt: | $(BENCHMARK_DIR)
	$(GO) test $(BENCHMARK_FLAGS) -run=^$$ -bench=^Benchmark$(call benchmark_lc_to_name,$*)$$ >$@

$(BENCHMARK_DIR):
	mkdir $@

.PHONY: list
list:
ifneq ($(OS),Windows_NT)
	@LC_ALL=C $(MAKE) -pRrq -f $(lastword $(MAKEFILE_LIST)) : 2>/dev/null | awk -v RS= -F: '/(^|\n)# Files(\n|$$)/,/(^|\n)# Finished Make data base/ {if ($$1 !~ "^[#.]") {print $$1}}' | sort | egrep -v -e '^[^[:alnum:]]' -e '^$@$$'
endif

.PHONY: debug-env
debug-env:
	@echo BENCHMARK_NAMES = $(BENCHMARK_NAMES)
	@echo BENCHMARK_TARGETS = $(BENCHMARK_TARGETS)
	@echo BENCHMARK_FILES = $(BENCHMARK_FILES)
	@echo BENCHMARK_NAME_TO_LC = $(BENCHMARK_NAME_TO_LC)
	@echo benchmark_lc_to_name = $(foreach name,$(call lc,$(BENCHMARK_NAMES)),$(name)=$(call benchmark_lc_to_name,$(name)))
