# Temporary config for validation
.PHONY: validate-json
validate-json:
	@echo "Validating blueprint.json..."
	@cd $(dir $(lastword $(MAKEFILE_LIST))) && go run validate_json.go 2>&1 | tail -n 5
