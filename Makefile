.PHONY: build run install clean test docker-build docker-run

# Binary name
BINARY_NAME=dbm
DOCKER_IMAGE=dbm:latest

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	go build -o bin/$(BINARY_NAME) ./cmd/dbm

# Run the application
run: build
	@./bin/$(BINARY_NAME)

# Install to system
install: build
	@echo "Installing $(BINARY_NAME) to /usr/local/bin..."
	sudo cp bin/$(BINARY_NAME) /usr/local/bin/
	@echo "Done! Run '$(BINARY_NAME)' to use"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf bin/
	go clean

# Run tests
test:
	go test -v ./...

# Build Docker image
docker-build:
	@echo "Building Docker image $(DOCKER_IMAGE)..."
	docker build -t $(DOCKER_IMAGE) .

# Run in Docker
docker-run: docker-build
	docker run --rm -it \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v ${HOME}/.dbm:/root/.dbm \
		$(DOCKER_IMAGE)

# Development mode
dev:
	go run ./cmd/dbm

# Format code
fmt:
	go fmt ./...

# Tidy dependencies
tidy:
	go mod tidy

# Download dependencies
deps:
	go mod download

# Test a release build locally (no upload; requires goreleaser)
release-snapshot:
	goreleaser release --snapshot --clean

# Check the release configuration
release-check:
	goreleaser check

