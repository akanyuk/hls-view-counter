BINARY_NAME := hls-view-counter
WIN_BINARY_NAME := $(BINARY_NAME).exe
BUILD_FOLDER := .build

PRINTF_FORMAT := "\033[35m%-18s\033[33m %s\033[0m\n"

.PHONY: all build windows linux vendor test lint clean help

all: build

build: windows linux ## Default: build for windows and linux

windows: vendor ## Build artifacts for windows
	@printf $(PRINTF_FORMAT) BINARY_NAME: $(WIN_BINARY_NAME)
	mkdir -p $(BUILD_FOLDER)/windows
	env GOOS=windows GOARCH=amd64 go build -o $(BUILD_FOLDER)/windows/$(WIN_BINARY_NAME) .

linux: vendor ## Build artifacts for linux
	@printf $(PRINTF_FORMAT) BINARY_NAME: $(BINARY_NAME)
	mkdir -p $(BUILD_FOLDER)/linux
	env GOOS=linux GOARCH=amd64 go build -o $(BUILD_FOLDER)/linux/$(BINARY_NAME) .

vendor: ## Get dependencies according to glide.lock
	env GO111MODULE=auto GOPRIVATE=git.sedmax.ru go mod vendor

test: vendor ## Start unit-tests
	go test ./...

lint: vendor ## Start static code analysis
	hash golangci-lint 2>/dev/null || go get -u github.com/golangci/golangci-lint/cmd/golangci-lint
	golangci-lint run --timeout=5m

clean: ## Remove vendor and artifacts
	rm -rf vendor
	rm -rf $(BUILD_FOLDER)

help: ## Display available commands
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
