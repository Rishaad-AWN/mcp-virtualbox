BINARY := mcp-virtualbox
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

.PHONY: build test check vet fmt clean install run bump version-check

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) .

install:
	go install -ldflags "$(LDFLAGS)" .

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

# CI gate: formatting, vet, and tests must all pass.
check:
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed on:"; gofmt -l .; exit 1)
	go vet ./...
	go test ./...

# Stamp a new version across main.go + plugin/marketplace manifests, e.g.:
#   make bump v=0.4.0
bump:
	@test -n "$(v)" || (echo "usage: make bump v=X.Y.Z"; exit 1)
	sh scripts/version.sh set "$(v)"

# Verify all manifests agree with a version (defaults to the current git tag).
version-check:
	sh scripts/version.sh check "$(shell git describe --tags --abbrev=0 2>/dev/null || echo 0.0.0)"

run: build
	./$(BINARY)

clean:
	rm -f $(BINARY)
