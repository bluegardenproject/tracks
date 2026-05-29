.PHONY: build build-all release clean test fmt vet tidy install

BINARY_NAME := tracks
VERSION     := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILD_TIME  := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS     := -s -w -X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)

build:
	@echo "Building $(BINARY_NAME) $(VERSION)..."
	@go build -ldflags="$(LDFLAGS)" -o $(BINARY_NAME) .
	@echo "Built ./$(BINARY_NAME)"

# Cross-compile to dist/ for the 5 release targets. CGO is off so the
# resulting binaries are fully static and don't depend on libc on the
# host they're installed to.
build-all:
	@echo "Building $(BINARY_NAME) $(VERSION) for all platforms..."
	@mkdir -p dist
	@CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o dist/$(BINARY_NAME)-linux-amd64 .
	@CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o dist/$(BINARY_NAME)-linux-arm64 .
	@CGO_ENABLED=0 GOOS=darwin  GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o dist/$(BINARY_NAME)-darwin-amd64 .
	@CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o dist/$(BINARY_NAME)-darwin-arm64 .
	@CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o dist/$(BINARY_NAME)-windows-amd64.exe .
	@echo "Built all platform binaries in dist/"

release: clean build-all
	@echo "Release artifacts ready in dist/:"
	@ls -la dist/

clean:
	@rm -rf dist/ $(BINARY_NAME)
	@echo "Cleaned build artifacts"

test:
	go test ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

tidy:
	go mod tidy

# `make install` builds and copies the binary into the user's PATH at
# ~/bin/tracks. Override DESTDIR to install elsewhere.
DESTDIR ?= $(HOME)/bin
install: build
	@mkdir -p $(DESTDIR)
	@cp -f $(BINARY_NAME) $(DESTDIR)/$(BINARY_NAME)
	@echo "Installed $(DESTDIR)/$(BINARY_NAME)"
