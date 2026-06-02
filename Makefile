GO ?= go
GOBUILDVCS ?= -buildvcs=false
GOCACHE ?= /tmp/go-build-cache

.PHONY: fmt fix test vet lint check run

fmt:
	gofmt -w .

fix:
	GOCACHE=$(GOCACHE) $(GO) fix ./...

test:
	GOCACHE=$(GOCACHE) $(GO) test $(GOBUILDVCS) -race ./...

vet:
	GOCACHE=$(GOCACHE) $(GO) vet ./...

lint:
	golangci-lint run

check: fix fmt vet test lint

run:
	GOCACHE=$(GOCACHE) $(GO) run $(GOBUILDVCS) ./cmd/intervals-mcp
