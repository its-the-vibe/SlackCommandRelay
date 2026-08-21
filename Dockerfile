# Build stage
FROM --platform=$BUILDPLATFORM golang:1.27.0-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY *.go ./

# Build the application
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o slack-command-relay

# Final stage (distroless)
FROM gcr.io/distroless/static-debian13:nonroot

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /app/slack-command-relay .

# Expose port (default 8080, can be overridden)
EXPOSE 8080

USER nonroot:nonroot

# Run the application
ENTRYPOINT ["./slack-command-relay"]
