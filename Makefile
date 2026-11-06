.DEFAULT_GOAL := list

# List all commands in this file.
.PHONY: list
list:
	@grep '^[^#[:space:].].*:' Makefile

# Run all tests.
.PHONY: test
test:
	go test ./...

# Lint code.
.PHONY: lint
lint:
	go vet ./...
	go tool revive -formatter=stylish -config=revive.toml ./...
