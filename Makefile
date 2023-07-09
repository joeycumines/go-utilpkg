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
# Example: https://github.com/joeycumines/go-utilpkg

# Example tools.go:
# ---
#//go:build tools
#// +build tools
#
#package tools
#
#import (
#	_ "github.com/dkorunic/betteralign/cmd/betteralign"
#	_ "github.com/grailbio/grit"
#	_ "golang.org/x/perf/cmd/benchstat"
#	_ "golang.org/x/tools/cmd/godoc"
#	_ "honnef.co/go/tools/cmd/staticcheck"
#)

# windows gnu make seems to append includes to the end of MAKEFILE_LIST
# hence the simple variable assignment, prior to any includes
ROOT_MAKEFILE := $(abspath $(lastword $(MAKEFILE_LIST)))

-include config.mak
-include project.mak

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
GO_COVERAGE_MODULE_FILE ?= coverage.out
GO_COVERAGE_ALL_MODULES_FILE ?= coverage-all.out
GO_TOOL_COVER ?= $(GO) tool cover
GODOC ?= godoc
GODOC_FLAGS ?= -http=:6060
GRIT ?= grit
GRIT_FLAGS ?= -push
GRIT_BRANCH ?= main
GRIT_SRC ?=
GRIT_DST ?=
STATICCHECK ?= staticcheck
STATICCHECK_FLAGS ?=
BETTERALIGN ?= betteralign
BETTERALIGN_FLAGS ?=
ifeq ($(OS),Windows_NT)
LIST_TOOLS ?= if exist tools.go (for /f tokens^=2^ delims^=^" %%a in ('findstr /r "^[\t ]*_" tools.go') do echo %%a)
else
LIST_TOOLS ?= [ ! -e tools.go ] || grep -E '^[	 ]*_' tools.go | cut -d '"' -f 2
endif
# used to special-case modules for tools which fail if they find no packages (e.g. go vet)
GO_MODULE_SLUGS_NO_PACKAGES ?=
# used to exclude modules from the update* targets
GO_MODULE_SLUGS_NO_UPDATE ?=

# configurable, but unlikely to need to be configured

# separates keys and values, see also the map_* functions
MAP_SEPARATOR ?= :
# path separator (/ replacement) for slugs
SLUG_SEPARATOR ?= .

# ---

# recursive wildcard match function, $1 is the directory to search, $2 is the pattern to match
# note 1: $1 requires a trailing slash
# note 2: $2 does not support multiple wildcards
rwildcard = $(wildcard $1$2) $(foreach d,$(wildcard $1*),$(call rwildcard,$d/,$2))

# looks up a value in a map, $1 is the map, $2 is the key associated with the value
map_value_by_key = $(patsubst $2$(MAP_SEPARATOR)%,%,$(filter $2$(MAP_SEPARATOR)%,$1))
# looks up a key in a map, $1 is the map, $2 is the value associated with the key
map_key_by_value = $(patsubst %$(MAP_SEPARATOR)$2,%,$(filter %$(MAP_SEPARATOR)$2,$1))
# builds a new map, from a set of keys, using a transform function to build values from the keys
# $1 are the keys, $2 is the transform function
map_transform_keys = $(foreach v,$1,$v$(MAP_SEPARATOR)$(call $2,$v))
# extracts only the keys from a map variable, $1 is the map variable
map_keys = $(foreach v,$1,$(word 1,$(subst $(MAP_SEPARATOR), ,$v)))

# convert a path to a slug, e.g. ./logiface/logrus -> logiface.logrus, with special case for root
slug_transform = $(if $(filter .,$1),root,$(subst /,$(SLUG_SEPARATOR),$(patsubst ./%,%,$1)))

go_module_path_to_slug = $(call map_value_by_key,$(_GO_MODULE_MAP),$1)
go_module_slug_to_path = $(call map_key_by_value,$(_GO_MODULE_MAP),$1)

subdir_makefile_path_to_slug = $(call map_value_by_key,$(_SUBDIR_MAKEFILE_MAP),$1)
subdir_makefile_slug_to_path = $(call map_key_by_value,$(_SUBDIR_MAKEFILE_MAP),$1)

go_module_slug_to_grit_src = $(GRIT_SRC),$(patsubst ./%,%,$(call go_module_slug_to_path,$1))/,$(GRIT_BRANCH)
go_module_slug_to_grit_dst = $(call map_value_by_key,$(GRIT_DST),$1),,$(GRIT_BRANCH)

# paths formatted like ". ./logiface ./logiface/logrus ./logiface/testsuite ./logiface/zerolog"
GO_MODULE_PATHS := $(patsubst %/go.mod,%,$(call rwildcard,./,go.mod))
# used by go_module_path_to_slug and go_module_slug_to_path to lookup an associated path/slug
_GO_MODULE_MAP := $(call map_transform_keys,$(GO_MODULE_PATHS),slug_transform)
# example: root logiface logiface.logrus logiface.testsuite logiface.zerolog
GO_MODULE_SLUGS := $(foreach d,$(GO_MODULE_PATHS),$(call go_module_path_to_slug,$d))
# sanity check the path and slug lookups
ifneq ($(GO_MODULE_PATHS),$(foreach d,$(GO_MODULE_SLUGS),$(call go_module_slug_to_path,$d)))
$(error GO_MODULE_PATHS contains unsupported paths)
endif
ifneq ($(GO_MODULE_SLUGS),$(foreach d,$(GO_MODULE_PATHS),$(call go_module_path_to_slug,$d)))
$(error GO_MODULE_SLUGS contains unsupported paths)
endif
GO_MODULE_SLUGS_EXCL_NO_PACKAGES := $(filter-out $(GO_MODULE_SLUGS_NO_PACKAGES),$(GO_MODULE_SLUGS))
GO_MODULE_SLUGS_EXCL_NO_UPDATE := $(filter-out $(GO_MODULE_SLUGS_NO_UPDATE),$(GO_MODULE_SLUGS))
GO_MODULE_SLUGS_GRIT_DST := $(filter $(call map_keys,$(GRIT_DST)),$(GO_MODULE_SLUGS))
GO_MODULE_SLUGS_EXCL_GRIT_DST := $(filter-out $(GO_MODULE_SLUGS_GRIT_DST),$(GO_MODULE_SLUGS))

# subdirectories which contain a file named "Makefile", formatted with a leading ".", and no trailing slash
# note that the root Makefile (this file) is excluded
SUBDIR_MAKEFILE_PATHS := $(filter-out .,$(patsubst %/Makefile,%,$(call rwildcard,./,Makefile)))
# used by subdir_makefile_path_to_slug and subdir_makefile_slug_to_path to lookup an associated path/slug
_SUBDIR_MAKEFILE_MAP := $(call map_transform_keys,$(SUBDIR_MAKEFILE_PATHS),slug_transform)
# slugs for subdirectories, without a leading ./, / replaced with ., and the path . mapped to root
SUBDIR_MAKEFILE_SLUGS := $(foreach d,$(SUBDIR_MAKEFILE_PATHS),$(call subdir_makefile_path_to_slug,$d))
# sanity check the path and slug lookups
ifneq ($(SUBDIR_MAKEFILE_PATHS),$(foreach d,$(SUBDIR_MAKEFILE_SLUGS),$(call subdir_makefile_slug_to_path,$d)))
$(error SUBDIR_MAKEFILE_PATHS contains unsupported paths)
endif
ifneq ($(SUBDIR_MAKEFILE_SLUGS),$(foreach d,$(SUBDIR_MAKEFILE_PATHS),$(call subdir_makefile_path_to_slug,$d)))
$(error SUBDIR_MAKEFILE_SLUGS contains unsupported paths)
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
$(LINT_TARGETS): lint.%: vet.% staticcheck.% betteralign.%

# staticcheck: runs the staticcheck tool

STATICCHECK_TARGETS := $(addprefix staticcheck.,$(GO_MODULE_SLUGS))

.PHONY: staticcheck
staticcheck: $(STATICCHECK_TARGETS)

.PHONY: $(STATICCHECK_TARGETS)
$(STATICCHECK_TARGETS): staticcheck.%:
	$(STATICCHECK) $(STATICCHECK_FLAGS) $(call go_module_slug_to_path,$*)/...

# vet: runs the go vet tool

VET_TARGETS := $(addprefix vet.,$(GO_MODULE_SLUGS))

.PHONY: vet
vet: $(VET_TARGETS)

.PHONY: $(addprefix vet.,$(GO_MODULE_SLUGS_EXCL_NO_PACKAGES))
$(addprefix vet.,$(GO_MODULE_SLUGS_EXCL_NO_PACKAGES)): vet.%:
	$(GO_VET) $(call go_module_slug_to_path,$*)/...

.PHONY: $(addprefix vet.,$(GO_MODULE_SLUGS_NO_PACKAGES))
$(addprefix vet.,$(GO_MODULE_SLUGS_NO_PACKAGES)): vet.%:

# betteralign: runs the betteralign tool

BETTERALIGN_TARGETS := $(addprefix betteralign.,$(GO_MODULE_SLUGS))

.PHONY: betteralign
betteralign: $(BETTERALIGN_TARGETS)

.PHONY: $(addprefix betteralign.,$(GO_MODULE_SLUGS_EXCL_NO_PACKAGES))
$(addprefix betteralign.,$(GO_MODULE_SLUGS_EXCL_NO_PACKAGES)): betteralign.%:
	$(MAKE) -s -C $(call go_module_slug_to_path,$*) -f $(ROOT_MAKEFILE) _betteralign BETTERALIGN_FLAGS='$(BETTERALIGN_FLAGS)'

.PHONY: $(addprefix betteralign.,$(GO_MODULE_SLUGS_NO_PACKAGES))
$(addprefix betteralign.,$(GO_MODULE_SLUGS_NO_PACKAGES)): betteralign.%:

.PHONY: _betteralign
_betteralign:
	$(BETTERALIGN) $(BETTERALIGN_FLAGS) ./...

# build: runs the go build tool

BUILD_TARGETS := $(addprefix build.,$(GO_MODULE_SLUGS))

.PHONY: build
build: $(BUILD_TARGETS)

.PHONY: $(BUILD_TARGETS)
$(BUILD_TARGETS): build.%:
	$(GO_BUILD) $(call go_module_slug_to_path,$*)/...

# test: runs the go test tool

TEST_TARGETS := $(addprefix test.,$(GO_MODULE_SLUGS))

.PHONY: test
test: $(TEST_TARGETS)

.PHONY: $(addprefix test.,$(GO_MODULE_SLUGS_EXCL_NO_PACKAGES))
$(addprefix test.,$(GO_MODULE_SLUGS_EXCL_NO_PACKAGES)): test.%:
	$(GO_TEST) $(call go_module_slug_to_path,$*)/...

.PHONY: $(addprefix test.,$(GO_MODULE_SLUGS_NO_PACKAGES))
$(addprefix test.,$(GO_MODULE_SLUGS_NO_PACKAGES)): test.%:

# cover: runs the go test tool with -covermode=count and generates a coverage report

COVER_TARGETS := $(addprefix cover.,$(GO_MODULE_SLUGS))

.PHONY: cover
cover: $(COVER_TARGETS)
	echo mode: count >$(GO_COVERAGE_ALL_MODULES_FILE)
	$(foreach d,$(GO_MODULE_SLUGS_EXCL_NO_PACKAGES),$(cover__TEMPLATE))
	$(GO_TOOL_COVER) -html=$(GO_COVERAGE_ALL_MODULES_FILE)
ifeq ($(OS),Windows_NT)
define cover__TEMPLATE =
type $(subst /,\,$(call go_module_slug_to_path,$d)/$(GO_COVERAGE_MODULE_FILE)) | more +1 | findstr /v /r "^$$" >>$(GO_COVERAGE_ALL_MODULES_FILE)

endef
else
define cover__TEMPLATE =
tail -n +2 $(call go_module_slug_to_path,$d)/$(GO_COVERAGE_MODULE_FILE) >>$(GO_COVERAGE_ALL_MODULES_FILE)

endef
endif

.PHONY: $(addprefix cover.,$(GO_MODULE_SLUGS_EXCL_NO_PACKAGES))
$(addprefix cover.,$(GO_MODULE_SLUGS_EXCL_NO_PACKAGES)): cover.%:
	$(GO_TEST) -coverprofile=$(call go_module_slug_to_path,$*)/$(GO_COVERAGE_MODULE_FILE) -covermode=count $(call go_module_slug_to_path,$*)/...

.PHONY: $(addprefix cover.,$(GO_MODULE_SLUGS_NO_PACKAGES))
$(addprefix cover.,$(GO_MODULE_SLUGS_NO_PACKAGES)): cover.%:

# fmt: runs go fmt on the module

FMT_TARGETS := $(addprefix fmt.,$(GO_MODULE_SLUGS))

.PHONY: fmt
fmt: $(FMT_TARGETS)

.PHONY: $(FMT_TARGETS)
$(FMT_TARGETS): fmt.%:
	$(MAKE) -s -C $(call go_module_slug_to_path,$*) -f $(ROOT_MAKEFILE) _fmt

.PHONY: _fmt
_fmt:
	$(GO_FMT) ./...

# update: runs go get -u -t ./... and go get -u on all tools

UPDATE_TARGETS := $(addprefix update.,$(GO_MODULE_SLUGS))

.PHONY: update
update: $(UPDATE_TARGETS)

.PHONY: $(addprefix update.,$(GO_MODULE_SLUGS_EXCL_NO_UPDATE))
$(addprefix update.,$(GO_MODULE_SLUGS_EXCL_NO_UPDATE)): update.%:
	@$(MAKE) -C $(call go_module_slug_to_path,$*) -f $(ROOT_MAKEFILE) _update

.PHONY: $(addprefix update.,$(GO_MODULE_SLUGS_NO_UPDATE))
$(addprefix update.,$(GO_MODULE_SLUGS_NO_UPDATE)): update.%: tidy.%

.PHONY: _update
_update: GO_TOOLS := $(shell $(LIST_TOOLS))
_update:
	$(GO) get -u -t ./...
	$(foreach tool,$(GO_TOOLS),$(_update_TEMPLATE))
	$(GO) mod tidy
define _update_TEMPLATE =
$(GO) get -u $(tool)

endef

# tidy: runs go mod tidy

TIDY_TARGETS := $(addprefix tidy.,$(GO_MODULE_SLUGS))

.PHONY: tidy
tidy: $(TIDY_TARGETS)

.PHONY: $(TIDY_TARGETS)
$(TIDY_TARGETS): tidy.%:
	@$(MAKE) -C $(call go_module_slug_to_path,$*) -f $(ROOT_MAKEFILE) _tidy

.PHONY: _tidy
_tidy:
	$(GO) mod tidy

# tools: runs go install on all tools

TOOLS_TARGETS := $(addprefix tools.,$(GO_MODULE_SLUGS))

.PHONY: tools
tools: $(TOOLS_TARGETS)

.PHONY: $(TOOLS_TARGETS)
$(TOOLS_TARGETS): tools.%:
	$(MAKE) --no-print-directory -C $(call go_module_slug_to_path,$*) -f $(ROOT_MAKEFILE) _tools

.PHONY: _tools
_tools: GO_TOOLS := $(shell $(LIST_TOOLS))
_tools:
	$(foreach tool,$(GO_TOOLS),$(_tools_TEMPLATE))
define _tools_TEMPLATE =
$(GO) install $(tool)

endef

# grit: runs grit to sync modules to defined target repositories

GRIT_TARGETS := $(addprefix grit.,$(GO_MODULE_SLUGS))

.PHONY: grit
grit: $(GRIT_TARGETS)

.PHONY: $(addprefix grit.,$(GO_MODULE_SLUGS_GRIT_DST))
$(addprefix grit.,$(GO_MODULE_SLUGS_GRIT_DST)): grit.%:
	$(GRIT) $(GRIT_FLAGS) $(call go_module_slug_to_grit_src,$*) $(call go_module_slug_to_grit_dst,$*)

.PHONY: $(addprefix grit.,$(GO_MODULE_SLUGS_EXCL_GRIT_DST))
$(addprefix grit.,$(GO_MODULE_SLUGS_EXCL_GRIT_DST)): grit.%:

# ---

# makefile pattern rules

# run.<./**/Makefile path as slug>: runs make at the given path

SUBDIR_MAKEFILE_TARGETS := $(addprefix run.,$(SUBDIR_MAKEFILE_SLUGS))

.PHONY: $(SUBDIR_MAKEFILE_TARGETS)
$(SUBDIR_MAKEFILE_TARGETS): run.%:
	@$(MAKE) -C $(call subdir_makefile_slug_to_path,$*) $(RUN_FLAGS)

# makefile implicit rules

# run-%.<./**/Makefile path as slug>: runs make target at the given path
# note that eval is necessary to make this work properly (a pattern rule can only be used once)
# additionally, note the FORCE target is to support .PHONY when using pattern rules to implement implicit rules
define _run_TEMPLATE =
run-%.$2: FORCE
	@$$(MAKE) -C $1 $(RUN_FLAGS) $$*

endef
# warning: simply-expanded
$(foreach d,$(SUBDIR_MAKEFILE_PATHS),$(eval $(call _run_TEMPLATE,$d,$(call subdir_makefile_path_to_slug,$d))))

# ---

.PHONY: clean
clean:

.PHONY: godoc
godoc:
	@echo 'Running godoc, the default URL is http://localhost:6060/pkg/'
	$(GODOC) $(GODOC_FLAGS)

.PHONY: debug-env
debug-env:
	@echo GO_MODULE_PATHS = $(GO_MODULE_PATHS)
	@echo GO_MODULE_SLUGS = $(GO_MODULE_SLUGS)
	@echo GO_MODULE_SLUGS_NO_PACKAGES = $(GO_MODULE_SLUGS_NO_PACKAGES)
	@echo GO_MODULE_SLUGS_EXCL_NO_PACKAGES = $(GO_MODULE_SLUGS_EXCL_NO_PACKAGES)
	@echo GO_MODULE_SLUGS_NO_UPDATE = $(GO_MODULE_SLUGS_NO_UPDATE)
	@echo GO_MODULE_SLUGS_EXCL_NO_UPDATE = $(GO_MODULE_SLUGS_EXCL_NO_UPDATE)
	@echo GO_MODULE_SLUGS_GRIT_DST = $(GO_MODULE_SLUGS_GRIT_DST)
	@echo GO_MODULE_SLUGS_EXCL_GRIT_DST = $(GO_MODULE_SLUGS_EXCL_GRIT_DST)
	@echo SUBDIR_MAKEFILE_PATHS = $(SUBDIR_MAKEFILE_PATHS)
	@echo SUBDIR_MAKEFILE_SLUGS = $(SUBDIR_MAKEFILE_SLUGS)

# we use .PHONY, but there's an edge case requiring this pattern
.PHONY: FORCE
FORCE:

.PHONY: clean
clean: CLEAN_PATHS := $(GO_COVERAGE_ALL_MODULES_FILE) $(addsuffix /$(GO_COVERAGE_MODULE_FILE),$(GO_MODULE_PATHS))
clean:
ifeq ($(OS),Windows_NT)
	del /Q /S $(subst /,\,$(CLEAN_PATHS))
else
	rm -rf $(CLEAN_PATHS)
endif
