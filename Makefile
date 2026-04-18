.PHONY: build run test test-short vet fmt fmt-check lint check clean cover tidy

BIN_DIR := bin
BIN     := $(BIN_DIR)/surql
PKG     := ./...

build:
	go build -o $(BIN) ./cmd/surql

run:
	go run ./cmd/surql

test:
	go test $(PKG) -race -v

test-short:
	go test $(PKG) -short

vet:
	go vet $(PKG)

fmt:
	gofmt -w .

fmt-check:
	@diff -u /dev/null <(gofmt -l .)

lint:
	golangci-lint run $(PKG)

check: fmt-check vet test
	@echo "All checks passed."

tidy:
	go mod tidy

cover:
	go test $(PKG) -race -coverprofile=coverage.out -covermode=atomic
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

clean:
	rm -rf $(BIN_DIR) coverage.out coverage.html
