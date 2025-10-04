# Makefile for Service Router Operator

# Define the Go version and the image name
GO_VERSION := 1.18
IMAGE_NAME := service-router-operator

# Define the directories
SRC_DIR := ./cmd
BUILD_DIR := ./bin
DOCKER_DIR := .

# Define the default target
.PHONY: all
all: build

# Build the operator binary
.PHONY: build
build:
	@echo "Building the operator..."
	go build -o $(BUILD_DIR)/$(IMAGE_NAME) $(SRC_DIR)/main.go

# Run tests
.PHONY: test
test:
	@echo "Running unit tests..."
	go test ./test/unit -v
	@echo "Running integration tests..."
	go test ./test/integration -v
	@echo "Running end-to-end tests..."
	go test ./test/e2e -v

# Build the Docker image
.PHONY: docker
docker:
	@echo "Building Docker image..."
	docker build -t $(IMAGE_NAME) $(DOCKER_DIR)

# Push the Docker image to ACR
.PHONY: push
push:
	@echo "Pushing Docker image to ACR..."
	docker push $(IMAGE_NAME)

# Clean up build artifacts
.PHONY: clean
clean:
	@echo "Cleaning up..."
	rm -rf $(BUILD_DIR)/*

# Install dependencies
.PHONY: install
install:
	@echo "Installing dependencies..."
	go mod tidy

# Run the operator locally
.PHONY: run
run: build
	@echo "Running the operator..."
	$(BUILD_DIR)/$(IMAGE_NAME)

# Help command
.PHONY: help
help:
	@echo "Makefile commands:"
	@echo "  all        - Build the operator"
	@echo "  build      - Build the operator binary"
	@echo "  test       - Run all tests"
	@echo "  docker     - Build the Docker image"
	@echo "  push       - Push the Docker image to ACR"
	@echo "  clean      - Clean up build artifacts"
	@echo "  install    - Install dependencies"
	@echo "  run        - Run the operator locally"
	@echo "  help       - Show this help message"