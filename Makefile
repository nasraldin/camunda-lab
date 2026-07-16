.PHONY: build test vet fmt tidy lint install check

VERSION ?= 0.0.0-dev

build:
	go build -ldflags "-X main.version=$(VERSION)" -o bin/camunda ./cmd/camunda

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './.worktrees/*')

fmt-check:
	@out="$$(gofmt -l $$(find . -name '*.go' -not -path './.worktrees/*'))"; \
	if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

tidy:
	go mod tidy
	git diff --exit-code -- go.mod go.sum

lint:
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not installed — skipping (CI uses vet/fmt)"; exit 0; }
	golangci-lint run ./...

check: fmt-check vet test

install: build
	install -m 755 bin/camunda "$(HOME)/.local/bin/camunda"
