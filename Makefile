.PHONY: build test lint install
VERSION ?= 0.0.0-dev

build:
	go build -ldflags "-X main.version=$(VERSION)" -o bin/camunda ./cmd/camunda

test:
	go test ./...

lint:
	golangci-lint run ./...

install: build
	install -m 755 bin/camunda "$(HOME)/.local/bin/camunda"
