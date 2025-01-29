# Build stage
FROM golang:1.22-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/taskbot ./cmd/taskbot

# Final stage
FROM alpine:3.19

# Add non root user
RUN adduser -D -g '' taskbot

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Copy the binary from builder
COPY --from=builder /app/taskbot /usr/local/bin/
COPY --from=builder /app/config.yaml /etc/taskbot/

# Use non root user
USER taskbot

# Set the entrypoint
ENTRYPOINT ["/usr/local/bin/taskbot"] 