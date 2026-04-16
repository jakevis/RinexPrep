# Build stage
FROM golang:1.22-bookworm AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /rinexprep ./cmd/rinexprep

# Runtime stage
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /rinexprep /rinexprep

USER nonroot:nonroot
EXPOSE 8080

ENTRYPOINT ["/rinexprep"]
CMD ["serve"]
