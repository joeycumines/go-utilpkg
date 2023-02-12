-include config.mak

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

# ---

# recursive wildcard match function, $1 is the directory to search, $2 is the pattern to match
# note 1: $1 requires a trailing slash
# note 2: $2 does not support multiple wildcards
rwildcard = $(wildcard $1$2) $(foreach d,$(wildcard $1*),$(call rwildcard,$d/,$2))

ROOT_MAKEFILE = $(abspath $(lastword $(MAKEFILE_LIST)))

# note 1: paths formatted like ". ./logiface ./logiface/logrus ./logiface/testsuite ./logiface/zerolog"
GO_MODULE_PATHS = $(patsubst %/go.mod,%,$(call rwildcard,./,go.mod))
# example: root logiface logiface.logrus logiface.testsuite logiface.zerolog
GO_MODULE_SLUGS = $(patsubst root.%,%,$(subst /,.,$(subst .,root,$(GO_MODULE_PATHS))))
# the below are used to special-case tools which fail if they find no packages (e.g. go vet)
# TODO update this if the root module gets packages
GO_MODULE_SLUGS_NO_PACKAGES = root
GO_MODULE_SLUGS_EXCL_NO_PACKAGES = $(filter-out $(GO_MODULE_SLUGS_NO_PACKAGES),$(GO_MODULE_SLUGS))
# resolves a go module path from a slog, e.g. logiface.logrus -> logiface/logrus, with special case for root
go_module_path = $(if $(filter root,$1),.,./$(subst .,/,$(filter-out root,$1)))

ifeq ($(OS),Windows_NT)
	LIST_TOOLS := if exist tools.go (for /f tokens^=2^ delims^=^" %%a in ('findstr /r "^[\t ]*_ " tools.go') do echo %%a)
else
	LIST_TOOLS ?= [ ! -e tools.go ] || grep -E '^[	 ]*_' tools.go | cut -d '"' -f 2
endif

# ---

# module pattern rules

# all: builds, then lints and tests in parallel (all modules in parallel)

ALL_TARGETS = $(addprefix all.,$(GO_MODULE_SLUGS))

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

LINT_TARGETS = $(addprefix lint.,$(GO_MODULE_SLUGS))

.PHONY: lint
lint: $(LINT_TARGETS)

.PHONY: $(LINT_TARGETS)
$(LINT_TARGETS): lint.%: vet.% staticcheck.%

# staticcheck: runs the staticcheck tool

STATICCHECK_TARGETS = $(addprefix staticcheck.,$(GO_MODULE_SLUGS))

.PHONY: staticcheck
staticcheck: $(STATICCHECK_TARGETS)

.PHONY: $(STATICCHECK_TARGETS)
$(STATICCHECK_TARGETS): staticcheck.%:
	$(STATICCHECK) $(STATICCHECK_FLAGS) $(call go_module_path,$*)/...

# vet: runs the go vet tool

VET_TARGETS = $(addprefix vet.,$(GO_MODULE_SLUGS))

.PHONY: vet
vet: $(VET_TARGETS)

.PHONY: $(addprefix vet.,$(GO_MODULE_SLUGS_EXCL_NO_PACKAGES))
$(addprefix vet.,$(GO_MODULE_SLUGS_EXCL_NO_PACKAGES)): vet.%:
	$(GO_VET) $(call go_module_path,$*)/...

.PHONY: $(addprefix vet.,$(GO_MODULE_SLUGS_NO_PACKAGES))
$(addprefix vet.,$(GO_MODULE_SLUGS_NO_PACKAGES)): vet.%:

# build: runs the go build tool

BUILD_TARGETS = $(addprefix build.,$(GO_MODULE_SLUGS))

.PHONY: build
build: $(BUILD_TARGETS)

.PHONY: $(BUILD_TARGETS)
$(BUILD_TARGETS): build.%:
	$(GO_BUILD) $(call go_module_path,$*)/...

# test: runs the go test tool

TEST_TARGETS = $(addprefix test.,$(GO_MODULE_SLUGS))

.PHONY: test
test: $(TEST_TARGETS)

.PHONY: $(addprefix test.,$(GO_MODULE_SLUGS_EXCL_NO_PACKAGES))
$(addprefix test.,$(GO_MODULE_SLUGS_EXCL_NO_PACKAGES)): test.%:
	$(GO_TEST) $(call go_module_path,$*)/...

.PHONY: $(addprefix test.,$(GO_MODULE_SLUGS_NO_PACKAGES))
$(addprefix test.,$(GO_MODULE_SLUGS_NO_PACKAGES)): test.%:

# fmt: runs go fmt on the module

FMT_TARGETS = $(addprefix fmt.,$(GO_MODULE_SLUGS))

.PHONY: fmt
fmt: $(FMT_TARGETS)

.PHONY: $(FMT_TARGETS)
$(FMT_TARGETS): fmt.%:
	$(MAKE) -s -C $(call go_module_path,$*) -f $(ROOT_MAKEFILE) _fmt

.PHONY: _fmt
_fmt:
	$(GO_FMT) ./...

# update: runs go get -u -t ./... and go get -u on all tools

UPDATE_TARGETS = $(addprefix update.,$(GO_MODULE_SLUGS))

.PHONY: update
update: $(UPDATE_TARGETS)

.PHONY: $(UPDATE_TARGETS)
$(UPDATE_TARGETS): update.%:
	@$(MAKE) -C $(call go_module_path,$*) -f $(ROOT_MAKEFILE) _update

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

TOOLS_TARGETS = $(addprefix tools.,$(GO_MODULE_SLUGS))

.PHONY: tools
tools: $(TOOLS_TARGETS)

.PHONY: $(TOOLS_TARGETS)
$(TOOLS_TARGETS): tools.%:
	$(MAKE) --no-print-directory -C $(call go_module_path,$*) -f $(ROOT_MAKEFILE) _tools

.PHONY: _tools
_tools: GO_TOOLS := $(shell $(LIST_TOOLS))
_tools:
	$(foreach tool,$(GO_TOOLS),$(_tools_TEMPLATE))
define _tools_TEMPLATE =
	$(GO) install $(tool)

endef

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
	@echo go_module_path = $(foreach d,$(GO_MODULE_SLUGS),$d=$(call go_module_path,$d))
