FROM mcr.microsoft.com/devcontainers/go:1.22-bookworm

# The base image ships a stale Yarn apt source with an expired GPG key.
# Remove it before apt-get update so the build doesn't fail.
RUN rm -f /etc/apt/sources.list.d/yarn.list 2>/dev/null; \
    apt-get update && apt-get install -y --no-install-recommends \
    make \
    jq \
    curl \
    && rm -rf /var/lib/apt/lists/*

RUN go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest \
    && go install github.com/air-verse/air@latest \
    && go install gotest.tools/gotestsum@latest

USER vscode
WORKDIR /workspace
