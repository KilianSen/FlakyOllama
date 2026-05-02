# Build stage
FROM golang:1.26-bookworm AS builder

# Install build dependencies
RUN apt-get update && apt-get install -y gcc libc6-dev

WORKDIR /app

# Copy and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=1 GOOS=linux go build -o flakyollama main.go --ldflags="-s -w"

# Final stage
FROM debian:bookworm-slim

# Install runtime dependencies
RUN apt-get update && apt-get install -y ca-certificates sqlite3 && rm -rf /var/lib/apt/lists/*

WORKDIR /root/

# Copy the binary from the builder
COPY --from=builder /app/flakyollama .

# Default environment variables
ENV ROLE=balancer
ENV BALANCER_ADDR=0.0.0.0:8080
ENV AGENT_ADDR=0.0.0.0:8081
ENV BALANCER_URL=http://localhost:8080
ENV OLLAMA_URL=http://localhost:11434
ENV DB_PATH=/data/flakyollama.db

# Create data directory
RUN mkdir /data

# Expose ports
EXPOSE 8080 8081

# Run the application
CMD ["./flakyollama"]
