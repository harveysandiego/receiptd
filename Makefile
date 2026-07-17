.PHONY: build test test-race bench cover lint fmt vet tidy clean install-tools ci

BIN_DIR := bin

build:
	go build -o $(BIN_DIR)/receiptd ./cmd/receiptd
	go build -o $(BIN_DIR)/receipt ./cmd/receipt

test:
	go test ./...

test-race:
	go test -race ./...

# No benchmarks exist yet — this is a no-op until one is added, and is
# never part of `make ci` (benchmark numbers are noise in CI; run and
# compare locally instead). See CONTRIBUTING.md "Benchmarking" for when a
# change warrants a benchmark.
bench:
	go test -bench=. -benchmem -run='^$$' ./...

cover:
	go test -race -coverprofile=coverage.txt -covermode=atomic ./...
	go tool cover -func=coverage.txt

fmt:
	gofmt -l -w .
	goimports -l -w .

vet:
	go vet ./...

lint:
	golangci-lint run ./...

tidy:
	go mod tidy

clean:
	rm -rf $(BIN_DIR) dist coverage.txt coverage.html

install-tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	go install golang.org/x/tools/cmd/goimports@latest

# Mirrors what CI runs, in order, so failures surface locally first.
ci: fmt vet lint test-race cover
