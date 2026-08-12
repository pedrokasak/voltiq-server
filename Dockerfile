# Stage 1: Build
FROM golang:1.25-alpine AS builder

WORKDIR /build

# Install dependencies
RUN apk add --no-cache git ca-certificates

# Copy go.mod and go.sum
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /api ./cmd/api

# Stage 2: Runtime
FROM alpine:3.19

WORKDIR /app

# Install ca-certificates for HTTPS and timezone data
RUN apk --no-cache add ca-certificates tzdata

# Copy binary from builder
COPY --from=builder /api /app/api

# Copy migrations
COPY --from=builder /build/migrations /app/migrations

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget -q --spider http://localhost:8080/health || exit 1

# Run
CMD ["/app/api"]
