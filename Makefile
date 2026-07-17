.PHONY: build test vet fmt tidy lint install check ui ui-check

VERSION ?= 0.0.0-dev

ui:
	cd internal/ui/web && npm ci && npm run build

# Fast binary build when web/dist already exists (e.g. after make ui or committed dist).
build:
	@test -f internal/ui/web/dist/index.html || $(MAKE) ui
	go build -ldflags "-X main.version=$(VERSION)" -o bin/camunda ./cmd/camunda

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './.worktrees/*' -not -path './internal/ui/web/node_modules/*')

fmt-check:
	@out="$$(gofmt -l $$(find . -name '*.go' -not -path './.worktrees/*' -not -path './internal/ui/web/node_modules/*'))"; \
	if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

tidy:
	go mod tidy
	git diff --exit-code -- go.mod go.sum

lint:
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not installed — skipping (CI uses vet/fmt)"; exit 0; }
	golangci-lint run ./...

check: fmt-check tidy vet test

ui-check: ui
	@git diff --exit-code internal/ui/web/dist || { \
		echo "internal/ui/web/dist is out of date — run: make ui && commit dist"; exit 1; \
	}

install: build
	install -m 755 bin/camunda "$(HOME)/.local/bin/camunda"
