GO ?= go
BIN := bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

.PHONY: all frontend build arm64 test clean

all: frontend build

frontend:
	cd web && npm install && npm run build

build:
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BIN)/sprinklerd ./cmd/sprinklerd

# Raspberry Pi, 64-bit OS
arm64:
	GOOS=linux GOARCH=arm64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BIN)/sprinklerd-arm64 ./cmd/sprinklerd

test:
	$(GO) vet ./...
	$(GO) test ./...

clean:
	rm -rf $(BIN) web/dist
