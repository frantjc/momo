GO ?= go
GIT ?= git
GOLANGCI-LINT ?= golangci-lint

all: fmt lint

fmt generate test:
	@$(GO) $@ ./...

lint:
	@$(GOLANGCI-LINT) run --fix

SEMVER ?= 0.1.0

release:
	@$(GIT) tag v$(SEMVER)
	@$(GIT) push --tags

.PHONY: all fmt generate lint release
