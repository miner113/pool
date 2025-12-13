# Build stage
FROM golang:1.21-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the stratum daemon
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o stratumd ./cmd/stratumd

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/stratumd /app/stratumd

# Copy default config (will be overridden by mount)
COPY config.example.yaml /app/config.yaml

# Create non-root user
RUN adduser -D -u 1000 pool
USER pool

# Stratum port
EXPOSE 4444
# Metrics port
EXPOSE 9100

ENTRYPOINT ["/app/stratumd"]
