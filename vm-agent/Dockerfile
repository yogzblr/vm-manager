# Build stage
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk --no-cache add ca-certificates git

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build with optimizations
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w \
    -X github.com/yourorg/vm-agent/internal/version.Version=${VERSION} \
    -X github.com/yourorg/vm-agent/internal/version.GitCommit=${GIT_COMMIT} \
    -X github.com/yourorg/vm-agent/internal/version.BuildDate=${BUILD_DATE}" \
    -o vm-agent cmd/agent/main.go

# Final stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/vm-agent /app/vm-agent

# Create non-root user
RUN addgroup -g 1000 vm-agent && \
    adduser -u 1000 -G vm-agent -s /bin/sh -D vm-agent && \
    mkdir -p /var/lib/vm-agent /etc/vm-agent && \
    chown -R vm-agent:vm-agent /var/lib/vm-agent /etc/vm-agent

# Switch to non-root user
USER vm-agent

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD ["/app/vm-agent", "status"]

ENTRYPOINT ["/app/vm-agent"]
CMD ["run"]
