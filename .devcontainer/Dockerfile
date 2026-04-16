FROM mcr.microsoft.com/devcontainers/go:1.22-bookworm

RUN apt-get update && apt-get install -y --no-install-recommends \
    make \
    protobuf-compiler \
    jq \
    curl \
    && rm -rf /var/lib/apt/lists/*

RUN go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest \
    && go install github.com/air-verse/air@latest \
    && go install gotest.tools/gotestsum@latest

USER vscode
WORKDIR /workspace
