# MIT License
#
# Copyright (c) 2023 Joseph Cumines
#
# Permission is hereby granted, free of charge, to any person obtaining a copy
# of this software and associated documentation files (the "Software"), to deal
# in the Software without restriction, including without limitation the rights
# to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
# copies of the Software, and to permit persons to whom the Software is
# furnished to do so, subject to the following conditions:
#
# The above copyright notice and this permission notice shall be included in all
# copies or substantial portions of the Software.
#
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
# AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
# LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
# OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
# SOFTWARE.
#
# Source: https://gist.github.com/joeycumines/3352c393c1bf43df72b120ae9134168d

-include config.mak

# ---

# intended to be provided on the command line, for certain targets

# additional make flags to be used by the pattern targets like run.%, and implicit targets like run-%.<path>
# (used to run subdir makefiles)
RUN_FLAGS ?=

# ---

# intended to be configurable via config.mak

GO ?= go
GO_FLAGS ?=
GO_TEST_FLAGS ?=
GO_TEST ?= $(GO) test $(GO_FLAGS) $(GO_TEST_FLAGS)
GO_BUILD ?= $(GO) build $(GO_FLAGS)
GO_VET ?= $(GO) vet $(GO_FLAGS)
GO_FMT ?= $(GO) fmt
GODOC ?= godoc
GODOC_FLAGS ?= -http=:6060
STATICCHECK ?= staticcheck
STATICCHECK_FLAGS ?=
ifeq ($(OS),Windows_NT)
LIST_TOOLS ?= if exist tools.go (for /f tokens^=2^ delims^=^" %%a in ('findstr /r "^[\t ]*_" tools.go') do echo %%a)
else
LIST_TOOLS ?= [ ! -e tools.go ] || grep -E '^[	 ]*_' tools.go | cut -d '"' -f 2
endif

# ---

# recursive wildcard match function, $1 is the directory to search, $2 is the pattern to match
# note 1: $1 requires a trailing slash
# note 2: $2 does not support multiple wildcards
rwildcard = $(wildcard $1$2) $(foreach d,$(wildcard $1*),$(call rwildcard,$d/,$2))

# convert a path to a slug, e.g. ./logiface/logrus -> logiface.logrus, with special case for root
path_to_slug = $(if $(filter .,$1),root,$(subst /,.,$(patsubst ./%,%,$1)))

# convert a slug to a path, e.g. logiface.logrus -> ./logiface/logrus, with special case for root
slug_to_path = $(if $(filter root,$1),.,./$(subst .,/,$(filter-out root,$1)))

ROOT_MAKEFILE := $(abspath $(lastword $(MAKEFILE_LIST)))

# paths formatted like ". ./logiface ./logiface/logrus ./logiface/testsuite ./logiface/zerolog"
GO_MODULE_PATHS := $(patsubst %/go.mod,%,$(call rwildcard,./,go.mod))
# example: root logiface logiface.logrus logiface.testsuite logiface.zerolog
GO_MODULE_SLUGS := $(foreach d,$(GO_MODULE_PATHS),$(call path_to_slug,$d))
# guard to ensure that GO_MODULE_PATHS doesn't contain any unsupported paths (e.g. containing .)
ifneq ($(GO_MODULE_PATHS),$(foreach d,$(GO_MODULE_SLUGS),$(call slug_to_path,$d)))
$(error GO_MODULE_PATHS contains unsupported paths)
endif
# the below are used to special-case tools which fail if they find no packages (e.g. go vet)
# TODO update this if the root module gets packages
GO_MODULE_SLUGS_NO_PACKAGES := root
GO_MODULE_SLUGS_EXCL_NO_PACKAGES := $(filter-out $(GO_MODULE_SLUGS_NO_PACKAGES),$(GO_MODULE_SLUGS))

# subdirectories which contain a file named "Makefile", formatted with a leading ".", and no trailing slash
# note that the root Makefile (this file) is excluded
SUBDIR_MAKEFILE_PATHS := $(filter-out .,$(patsubst %/Makefile,%,$(call rwildcard,./,Makefile)))
# slugs for subdirectories, without a leading ./, / replaced with ., and the path . mapped to root
SUBDIR_MAKEFILE_SLUGS := $(foreach d,$(SUBDIR_MAKEFILE_PATHS),$(call path_to_slug,$d))
# guard to ensure that SUBDIR_MAKEFILE_PATHS doesn't contain any unsupported paths (e.g. containing .)
ifneq ($(SUBDIR_MAKEFILE_PATHS),$(foreach d,$(SUBDIR_MAKEFILE_SLUGS),$(call slug_to_path,$d)))
$(error SUBDIR_MAKEFILE_PATHS contains unsupported paths)
endif

# ---

# module pattern rules

# all: builds, then lints and tests in parallel (all modules in parallel)

ALL_TARGETS := $(addprefix all.,$(GO_MODULE_SLUGS))

.PHONY: all
all: $(ALL_TARGETS)

.PHONY: $(ALL_TARGETS)
$(ALL_TARGETS): all.%: _all__build.% _all__lint.% _all__test.%

.PHONY: $(addprefix _all__build.,$(GO_MODULE_SLUGS))
$(addprefix _all__build.,$(GO_MODULE_SLUGS)): _all__build.%:
	@$(MAKE) --no-print-directory build.$*

.PHONY: $(addprefix _all__lint.,$(GO_MODULE_SLUGS))
$(addprefix _all__lint.,$(GO_MODULE_SLUGS)): _all__lint.%: _all__build.%
	@$(MAKE) --no-print-directory lint.$*

.PHONY: $(addprefix _all__test.,$(GO_MODULE_SLUGS))
$(addprefix _all__test.,$(GO_MODULE_SLUGS)): _all__test.%: _all__build.%
	@$(MAKE) --no-print-directory test.$*

# lint: runs the vet and staticcheck targets

LINT_TARGETS := $(addprefix lint.,$(GO_MODULE_SLUGS))

.PHONY: lint
lint: $(LINT_TARGETS)

.PHONY: $(LINT_TARGETS)
$(LINT_TARGETS): lint.%: vet.% staticcheck.%

# staticcheck: runs the staticcheck tool

STATICCHECK_TARGETS := $(addprefix staticcheck.,$(GO_MODULE_SLUGS))

.PHONY: staticcheck
staticcheck: $(STATICCHECK_TARGETS)

.PHONY: $(STATICCHECK_TARGETS)
$(STATICCHECK_TARGETS): staticcheck.%:
	$(STATICCHECK) $(STATICCHECK_FLAGS) $(call slug_to_path,$*)/...

# vet: runs the go vet tool

VET_TARGETS := $(addprefix vet.,$(GO_MODULE_SLUGS))

.PHONY: vet
vet: $(VET_TARGETS)

.PHONY: $(addprefix vet.,$(GO_MODULE_SLUGS_EXCL_NO_PACKAGES))
$(addprefix vet.,$(GO_MODULE_SLUGS_EXCL_NO_PACKAGES)): vet.%:
	$(GO_VET) $(call slug_to_path,$*)/...

.PHONY: $(addprefix vet.,$(GO_MODULE_SLUGS_NO_PACKAGES))
$(addprefix vet.,$(GO_MODULE_SLUGS_NO_PACKAGES)): vet.%:

# build: runs the go build tool

BUILD_TARGETS := $(addprefix build.,$(GO_MODULE_SLUGS))

.PHONY: build
build: $(BUILD_TARGETS)

.PHONY: $(BUILD_TARGETS)
$(BUILD_TARGETS): build.%:
	$(GO_BUILD) $(call slug_to_path,$*)/...

# test: runs the go test tool

TEST_TARGETS := $(addprefix test.,$(GO_MODULE_SLUGS))

.PHONY: test
test: $(TEST_TARGETS)

.PHONY: $(addprefix test.,$(GO_MODULE_SLUGS_EXCL_NO_PACKAGES))
$(addprefix test.,$(GO_MODULE_SLUGS_EXCL_NO_PACKAGES)): test.%:
	$(GO_TEST) $(call slug_to_path,$*)/...

.PHONY: $(addprefix test.,$(GO_MODULE_SLUGS_NO_PACKAGES))
$(addprefix test.,$(GO_MODULE_SLUGS_NO_PACKAGES)): test.%:

# fmt: runs go fmt on the module

FMT_TARGETS := $(addprefix fmt.,$(GO_MODULE_SLUGS))

.PHONY: fmt
fmt: $(FMT_TARGETS)

.PHONY: $(FMT_TARGETS)
$(FMT_TARGETS): fmt.%:
	$(MAKE) -s -C $(call slug_to_path,$*) -f $(ROOT_MAKEFILE) _fmt

.PHONY: _fmt
_fmt:
	$(GO_FMT) ./...

# update: runs go get -u -t ./... and go get -u on all tools

UPDATE_TARGETS := $(addprefix update.,$(GO_MODULE_SLUGS))

.PHONY: update
update: $(UPDATE_TARGETS)

.PHONY: $(UPDATE_TARGETS)
$(UPDATE_TARGETS): update.%:
	@$(MAKE) -C $(call slug_to_path,$*) -f $(ROOT_MAKEFILE) _update

.PHONY: _update
_update: GO_TOOLS := $(shell $(LIST_TOOLS))
_update:
	$(GO) get -u -t ./...
	$(foreach tool,$(GO_TOOLS),$(_update_TEMPLATE))
	$(GO) mod tidy
define _update_TEMPLATE =
	$(GO) get -u $(tool)

endef

# tools: runs go install on all tools

TOOLS_TARGETS := $(addprefix tools.,$(GO_MODULE_SLUGS))

.PHONY: tools
tools: $(TOOLS_TARGETS)

.PHONY: $(TOOLS_TARGETS)
$(TOOLS_TARGETS): tools.%:
	$(MAKE) --no-print-directory -C $(call slug_to_path,$*) -f $(ROOT_MAKEFILE) _tools

.PHONY: _tools
_tools: GO_TOOLS := $(shell $(LIST_TOOLS))
_tools:
	$(foreach tool,$(GO_TOOLS),$(_tools_TEMPLATE))
define _tools_TEMPLATE =
	$(GO) install $(tool)

endef

# ---

# makefile pattern rules

# run.<./**/Makefile path as slug>: runs make at the given path

SUBDIR_MAKEFILE_TARGETS := $(addprefix run.,$(SUBDIR_MAKEFILE_SLUGS))

.PHONY: $(SUBDIR_MAKEFILE_TARGETS)
$(SUBDIR_MAKEFILE_TARGETS): run.%:
	@$(MAKE) -C $(call slug_to_path,$*) $(RUN_FLAGS)

# makefile implicit rules

# run-%.<./**/Makefile path as slug>: runs make target at the given path
# note that eval is necessary to make this work properly (a pattern rule can only be used once)
# additionally, note the FORCE target is to support .PHONY when using pattern rules to implement implicit rules
define _run_TEMPLATE =
run-%.$2: FORCE
	@$$(MAKE) -C $1 $(RUN_FLAGS) $$*

endef
# warning: simply-expanded
$(foreach d,$(SUBDIR_MAKEFILE_PATHS),$(eval $(call _run_TEMPLATE,$d,$(call path_to_slug,$d))))

# ---

.PHONY: clean
clean:

.PHONY: godoc
godoc:
	@echo 'Running godoc, the default URL is http://localhost:6060/pkg/github.com/joeycumines/go-utilpkg/'
	$(GODOC) $(GODOC_FLAGS)

.PHONY: debug-env
debug-env:
	@echo GO_MODULE_PATHS = $(GO_MODULE_PATHS)
	@echo GO_MODULE_SLUGS = $(GO_MODULE_SLUGS)
	@echo SUBDIR_MAKEFILE_PATHS = $(SUBDIR_MAKEFILE_PATHS)
	@echo SUBDIR_MAKEFILE_SLUGS = $(SUBDIR_MAKEFILE_SLUGS)

# we use .PHONY, but there's an edge case requiring this pattern
.PHONY: FORCE
FORCE:

.PHONY: list
list:
ifneq ($(OS),Windows_NT)
	@LC_ALL=C $(MAKE) -pRrq -f $(lastword $(MAKEFILE_LIST)) : 2>/dev/null | awk -v RS= -F: '/(^|\n)# Files(\n|$$)/,/(^|\n)# Finished Make data base/ {if ($$1 !~ "^[#.]") {print $$1}}' | sort | egrep -v -e '^[^[:alnum:]]' -e '^$@$$'
endif
