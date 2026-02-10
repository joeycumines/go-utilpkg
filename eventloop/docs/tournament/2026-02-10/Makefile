#!/usr/bin/make -f
# Benchmark Tournament 2026-02-10 — Build & Analysis Targets

SHELL := /bin/bash

TOURNAMENT_DIR := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
PROJECT_ROOT   := $(abspath $(TOURNAMENT_DIR)/../../../..)

PYTHON3 := python3

# JSON data files
DARWIN_JSON  := $(TOURNAMENT_DIR)darwin.json
LINUX_JSON   := $(TOURNAMENT_DIR)linux.json
WINDOWS_JSON := $(TOURNAMENT_DIR)windows.json

# Raw benchmark logs (at project root)
DARWIN_LOG   := $(PROJECT_ROOT)/build.benchmark.darwin.log
LINUX_LOG    := $(PROJECT_ROOT)/build.benchmark.linux.log
WINDOWS_LOG  := $(PROJECT_ROOT)/build.benchmark.windows.log

# Generated markdown files
GENERATED_MD := \
	$(TOURNAMENT_DIR)comparison.md \
	$(TOURNAMENT_DIR)comparison-3platform.md

.PHONY: all
all: parse analyze ## Run parse then analyze (full pipeline)

.PHONY: analyze
analyze: ## Run analyze_3platform.py to generate comparison-3platform.md
	cd $(TOURNAMENT_DIR) && $(PYTHON3) analyze_3platform.py

.PHONY: analyze-2platform
analyze-2platform: ## Run analyze_2platform.py to generate comparison.md
	cd $(TOURNAMENT_DIR) && $(PYTHON3) analyze_2platform.py

.PHONY: parse
parse: ## Run parse_benchmarks.py for all platforms with available logs
	@if [ -f "$(DARWIN_LOG)" ]; then \
		echo "Parsing Darwin benchmarks..."; \
		$(PYTHON3) $(TOURNAMENT_DIR)parse_benchmarks.py "$(DARWIN_LOG)" darwin darwin arm64 > "$(DARWIN_JSON)"; \
		echo "  -> darwin.json ($$(wc -l < "$(DARWIN_JSON)") lines)"; \
	else \
		echo "Skipping Darwin (no log at $(DARWIN_LOG))"; \
	fi
	@if [ -f "$(LINUX_LOG)" ]; then \
		echo "Parsing Linux benchmarks..."; \
		$(PYTHON3) $(TOURNAMENT_DIR)parse_benchmarks.py "$(LINUX_LOG)" linux linux arm64 > "$(LINUX_JSON)"; \
		echo "  -> linux.json ($$(wc -l < "$(LINUX_JSON)") lines)"; \
	else \
		echo "Skipping Linux (no log at $(LINUX_LOG))"; \
	fi
	@if [ -f "$(WINDOWS_LOG)" ]; then \
		echo "Parsing Windows benchmarks..."; \
		$(PYTHON3) $(TOURNAMENT_DIR)parse_benchmarks.py "$(WINDOWS_LOG)" windows windows amd64 > "$(WINDOWS_JSON)"; \
		echo "  -> windows.json ($$(wc -l < "$(WINDOWS_JSON)") lines)"; \
	else \
		echo "Skipping Windows (no log at $(WINDOWS_LOG))"; \
	fi

.PHONY: clean
clean: ## Remove generated .md files (preserves JSON, .py, and raw data)
	rm -f $(GENERATED_MD)
	@echo "Cleaned generated markdown files."

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'
