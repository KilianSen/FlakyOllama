# Build stage
FROM golang:1.26-alpine AS builder

# Install build dependencies
RUN apk add --no-cache gcc musl-dev

WORKDIR /app

# Copy and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=1 GOOS=linux go build -o flakyollama main.go

# Final stage
FROM alpine:latest

# Install runtime dependencies (like nvidia-smi if needed, though usually it's passed from host)
RUN apk add --no-cache ca-certificates sqlite

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
