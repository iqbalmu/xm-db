# Stage 1: Build
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Inject the version; plain `docker build` without the arg shows "dev".
#   docker build --build-arg VERSION=v1.2.3 -t xm-db .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build \
	-ldflags "-s -w -X github.com/iqbalmu/xm-db/internal/commands.Version=${VERSION}" \
	-o /bin/xmdb ./cmd/xmdb

# Stage 2: Runtime
FROM alpine:3.20

# ca-certificates for the GitHub release API (xmdb update)
RUN apk --no-cache add ca-certificates

# Runs as root on purpose: it talks to the mounted docker.sock and stores
# state in /root/.xmdb, which the docker-run target mounts.
COPY --from=builder /bin/xmdb /usr/local/bin/xmdb

ENTRYPOINT ["xmdb"]
CMD ["--help"]
