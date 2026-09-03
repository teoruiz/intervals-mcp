GO ?= go
GOBUILDVCS ?= -buildvcs=false
GOCACHE ?= /tmp/go-build-cache
GOLANGCI_LINT_CACHE ?= /tmp/golangci-lint-cache
PREK ?= prek
PREK_HOME ?= /tmp/prek-cache

.PHONY: fmt fmt-check fix test vet lint check run hooks precommit worker-install worker-test dev-worker deploy docker-build

fmt:
	gofmt -w .

fmt-check:
	@files="$$(gofmt -l .)"; \
	if [ -n "$$files" ]; then \
		echo "$$files"; \
		exit 1; \
	fi

fix:
	GOCACHE=$(GOCACHE) $(GO) fix ./...

test:
	GOCACHE=$(GOCACHE) $(GO) test $(GOBUILDVCS) -race ./...

vet:
	GOCACHE=$(GOCACHE) $(GO) vet ./...

lint:
	GOCACHE=$(GOCACHE) GOLANGCI_LINT_CACHE=$(GOLANGCI_LINT_CACHE) golangci-lint run

check: fmt-check vet test lint

hooks:
	PREK_HOME=$(PREK_HOME) $(PREK) install

precommit:
	PREK_HOME=$(PREK_HOME) $(PREK) run --all-files

run:
	GOCACHE=$(GOCACHE) $(GO) run $(GOBUILDVCS) ./cmd/intervals-mcp

worker-install:
	npm ci

worker-test:
	npm run typecheck
	npm run test:worker

dev-worker:
	npm run dev

deploy:
	npm run deploy

docker-build:
	docker build --platform linux/amd64 -t intervals-mcp:local .
