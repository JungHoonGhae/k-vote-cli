BINARY := bin/kvote
VERSION ?= $(shell cat VERSION 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
PKG := github.com/JungHoonGhae/kvote/internal/version
LDFLAGS := -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).Date=$(DATE)

.PHONY: build run test fmt tidy clean install

build:
	mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/kvote

run:
	go run -ldflags "$(LDFLAGS)" ./cmd/kvote

test:
	go test ./...

fmt:
	gofmt -w ./cmd ./internal

tidy:
	go mod tidy

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/kvote

clean:
	rm -rf bin coverage.out
