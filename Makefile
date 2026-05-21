# S3lytics Makefile

BINARY_NAME=s3lytics
BUILD_DIR=build
MAIN_PATH=./cmd/s3lytics
GO_FLAGS=-ldflags="-s -w"

.PHONY: all build run clean test lint fmt vet

all: build

build:
	@mkdir -p $(BUILD_DIR)
	go build $(GO_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)

run:
	go run $(MAIN_PATH)

clean:
	rm -rf $(BUILD_DIR)
	rm -rf s3lytics-data/

test:
	go test ./... -v -count=1

lint:
	golangci-lint run ./... 2>/dev/null || echo "golangci-lint not installed, skipping"

fmt:
	go fmt ./...

vet:
	go vet ./...
