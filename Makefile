-include config.mak

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
LIST_TOOLS = [ ! -e tools.go ] || grep -P '^\t_' tools.go | cut -d '"' -f 2
LIST_WORKSPACES = grep -P '^\t\./' go.work | cut -d '	' -f 2

.PHONY: all
all: lint build test

.PHONY: clean
clean:

.PHONY: lint
lint: vet staticcheck

.PHONY: vet
vet: vet-root vet-logiface vet-logiface-zerolog vet-logiface-logrus

.PHONY: staticcheck
staticcheck: staticcheck-root staticcheck-logiface staticcheck-logiface-zerolog staticcheck-logiface-logrus

.PHONY: build
build: build-root build-logiface build-logiface-zerolog build-logiface-logrus

.PHONY: test
test: test-root test-logiface test-logiface-zerolog test-logiface-logrus

.PHONY: fmt
fmt: fmt-root fmt-logiface fmt-logiface-zerolog fmt-logiface-logrus

.PHONY: godoc
godoc:
	@echo 'Running godoc, the default URL is http://localhost:6060/pkg/github.com/joeycumines/go-utilpkg/'
	$(GODOC) $(GODOC_FLAGS)

# use this to run on workspaces
WORK_TARGETS ?=
.PHONY: work
work:
	@for w in $$($(LIST_WORKSPACES)); do \
		for t in $(WORK_TARGETS); do \
			$(MAKE) -f '$(abspath $(lastword $(MAKEFILE_LIST)))' -C "$$w" "$$t"; \
		done; \
    done

# this won't work on all systems
.PHONY: update
update:
	$(GO) get -u -t ./...
	run_command() { echo "$$@" && "$$@"; } && \
		$(LIST_TOOLS) | \
		while read -r line; do run_command $(GO) get -u "$$line" || exit 1; done
	$(GO) mod tidy

# this won't work on all systems
.PHONY: tools
tools:
	export CGO_ENABLED=0 && \
		run_command() { echo "$$@" && "$$@"; } && \
		$(LIST_TOOLS) | \
		while read -r line; do run_command $(GO) install "$$line" || exit 1; done

# ---

.PHONY: vet-root
vet-root:
	#$(GO_VET) ./...

.PHONY: build-root
build-root:
	#$(GO_BUILD) ./...

.PHONY: staticcheck-root
staticcheck-root:
	#$(STATICCHECK) $(STATICCHECK_FLAGS) ./...

.PHONY: test-root
test-root:
	#$(GO_TEST) ./...

.PHONY: fmt-root
fmt-root:
	#$(GO_FMT) ./...

# ---

.PHONY: vet-logiface
vet-logiface:
	$(GO_VET) ./logiface/...

.PHONY: staticcheck-logiface
staticcheck-logiface:
	$(STATICCHECK) $(STATICCHECK_FLAGS) ./logiface/...

.PHONY: build-logiface
build-logiface:
	$(GO_BUILD) ./logiface/...

.PHONY: test-logiface
test-logiface:
	$(GO_TEST) ./logiface/...

.PHONY: fmt-logiface
fmt-logiface:
	cd logiface && $(GO_FMT) ./...

# ---

.PHONY: vet-logiface-zerolog
vet-logiface-zerolog:
	$(GO_VET) ./logiface/zerolog/...

.PHONY: staticcheck-logiface-zerolog
staticcheck-logiface-zerolog:
	$(STATICCHECK) $(STATICCHECK_FLAGS) ./logiface/zerolog/...

.PHONY: build-logiface-zerolog
build-logiface-zerolog:
	$(GO_BUILD) ./logiface/zerolog/...

.PHONY: test-logiface-zerolog
test-logiface-zerolog:
	$(GO_TEST) ./logiface/zerolog/...

.PHONY: fmt-logiface-zerolog
fmt-logiface-zerolog:
	cd logiface/zerolog && $(GO_FMT) ./...

# ---

.PHONY: vet-logiface-logrus
vet-logiface-logrus:
	$(GO_VET) ./logiface/logrus/...

.PHONY: staticcheck-logiface-logrus
staticcheck-logiface-logrus:
	$(STATICCHECK) $(STATICCHECK_FLAGS) ./logiface/logrus/...

.PHONY: build-logiface-logrus
build-logiface-logrus:
	$(GO_BUILD) ./logiface/logrus/...

.PHONY: test-logiface-logrus
test-logiface-logrus:
	$(GO_TEST) ./logiface/logrus/...

.PHONY: fmt-logiface-logrus
fmt-logiface-logrus:
	cd logiface/logrus && $(GO_FMT) ./...

# ---
