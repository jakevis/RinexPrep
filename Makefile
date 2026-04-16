APP_NAME := rinexprep
GO := go
GOFLAGS := -v
LDFLAGS := -s -w
BUILD_DIR := bin

.PHONY: all build test lint clean docker run fmt vet

all: lint test build

build:
	$(GO) build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME) ./cmd/rinexprep

test:
	$(GO) test $(GOFLAGS) -race -coverprofile=coverage.out ./...

test-short:
	$(GO) test -short ./...

lint:
	golangci-lint run ./...

fmt:
	$(GO) fmt ./...
	goimports -w .

vet:
	$(GO) vet ./...

clean:
	rm -rf $(BUILD_DIR) coverage.out coverage.html

coverage: test
	$(GO) tool cover -html=coverage.out -o coverage.html

docker-build:
	docker build -t $(APP_NAME):latest .

docker-run: docker-build
	docker run --rm -p 8080:8080 $(APP_NAME):latest

run:
	$(GO) run ./cmd/rinexprep $(ARGS)

# Development with hot reload
dev:
	air -c .air.toml
