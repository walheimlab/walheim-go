.PHONY: all build test test-integration lint coverage clean

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

clean:
	rm -f $(BINARY_NAME) coverage.out
