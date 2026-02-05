# Build stage
FROM golang:1.26rc3-alpine AS builder

# Install CA certificates for HTTPS
RUN apk add --no-cache ca-certificates git

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY *.go ./

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o slack-command-relay

# Final stage
FROM scratch

WORKDIR /app

# Copy CA certificates from builder
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the binary from builder
COPY --from=builder /app/slack-command-relay .

# Expose port (default 8080, can be overridden)
EXPOSE 8080

# Run the application
CMD ["./slack-command-relay"]
