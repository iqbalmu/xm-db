# Stage 1: Build
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /bin/dbm ./cmd/dbm

# Stage 2: Runtime
FROM alpine:3.20

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates

# Create non-root user
RUN adduser -D -H -h /app appuser

# Copy binary from builder
COPY --from=builder /bin/dbm /usr/local/bin/dbm

# Switch to non-root user
USER appuser

ENTRYPOINT ["dbm"]
CMD ["--help"]

