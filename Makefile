BINARY_NAME = slack-command-relay

.PHONY: all build test vet lint fmt clean

all: build

## build: compile the binary
build:
	go build -o $(BINARY_NAME) .

## test: run all unit tests
test:
	go test ./...

## vet: run go vet
vet:
	go vet ./...

## lint: check formatting and run go vet
lint: vet
	@if [ -n "$$(gofmt -l .)" ]; then \
		echo "The following files have formatting issues (run 'make fmt' to fix):"; \
		gofmt -l .; \
		exit 1; \
	fi

## fmt: format all Go source files
fmt:
	gofmt -w .

## clean: remove build artifacts
clean:
	rm -f $(BINARY_NAME)
