# Stage 1: Build React frontend
FROM node:22-slim AS frontend
WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 2: Build Go backend (embeds frontend via frontend/embed.go)
FROM golang:1.22-bookworm AS builder
WORKDIR /build
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
COPY --from=frontend /app/web/dist ./frontend/dist
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /rinexprep ./cmd/rinexprep

# Stage 3: Minimal runtime
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /rinexprep /rinexprep
# Create temp directory for job files
VOLUME /data
ENV RINEXPREP_DATA_DIR=/data
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/rinexprep"]
CMD ["serve"]
