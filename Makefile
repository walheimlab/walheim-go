.PHONY: all build test test-integration lint lint-fix coverage clean install-hooks

BINARY_NAME=whctl

all: lint test

build:
	go build -o $(BINARY_NAME) ./cmd/whctl

test:
	go test -v ./...

test-integration:
	go test -v -tags=integration ./...

coverage:
	go test -v -tags=integration -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out

lint:
	golangci-lint run

lint-fix:
	golangci-lint run --fix

install-hooks:
	chmod +x .githooks/pre-commit
	mkdir -p .git/hooks
	ln -sf ../../.githooks/pre-commit .git/hooks/pre-commit

clean:
	rm -f $(BINARY_NAME) coverage.out
