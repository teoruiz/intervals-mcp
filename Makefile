GO ?= go
GOBUILDVCS ?= -buildvcs=false
GOCACHE ?= /tmp/go-build-cache
GOLANGCI_LINT_CACHE ?= /tmp/golangci-lint-cache
PREK ?= prek
PREK_HOME ?= /tmp/prek-cache

.PHONY: fmt fix test vet lint check run run-local hooks precommit

fmt:
	gofmt -w .

fix:
	GOCACHE=$(GOCACHE) $(GO) fix ./...

test:
	GOCACHE=$(GOCACHE) $(GO) test $(GOBUILDVCS) -race ./...

vet:
	GOCACHE=$(GOCACHE) $(GO) vet ./...

lint:
	GOCACHE=$(GOCACHE) GOLANGCI_LINT_CACHE=$(GOLANGCI_LINT_CACHE) golangci-lint run

check: fix fmt vet test lint

hooks:
	PREK_HOME=$(PREK_HOME) $(PREK) install

precommit:
	PREK_HOME=$(PREK_HOME) $(PREK) run --all-files

run:
	GOCACHE=$(GOCACHE) $(GO) run $(GOBUILDVCS) ./cmd/intervals-mcp

run-local:
	GOCACHE=$(GOCACHE) $(GO) run $(GOBUILDVCS) ./cmd/intervals-mcp --local
