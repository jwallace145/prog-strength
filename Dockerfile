# Build stage
FROM golang:1.25-alpine AS builder

# Install build dependencies for CGo (required for go-sqlite3)
RUN apk add --no-cache gcc musl-dev

WORKDIR /build

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# APP_VERSION is injected at build time by the release pipeline (e.g. "v1.2.3").
# Local docker builds without --build-arg get "dev". Surfaces in every API
# response's "version" field via internal/version.Version.
ARG APP_VERSION=dev

# Build the binary.
# CGO_ENABLED=1 is required for go-sqlite3.
# -ldflags injects the version string into internal/version.Version.
RUN CGO_ENABLED=1 GOOS=linux go build \
    -a -installsuffix cgo \
    -ldflags="-X github.com/jwallace145/progressive-overload-fitness-tracker/internal/version.Version=${APP_VERSION}" \
    -o api ./cmd/api

# Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /build/api .

# Create data directory for SQLite database
RUN mkdir -p /data

# Expose port
EXPOSE 8080

# Set environment variables with defaults
ENV DATABASE_URL=/data/app.db
ENV SERVER_ADDR=:8080

# Run the application
CMD ["./api"]
