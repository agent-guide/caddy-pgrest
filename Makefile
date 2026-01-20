# Makefile for Caddy Module

.PHONY: all build build-xcaddy clean deps fmt

PACKAGE_NAME=caddy-pgrest
BINARY_NAME=caddy-pgrest

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
XCADDY=xcaddy

all: clean deps build

# Build the binary (recommended method)
build:
	@echo "Building $(BINARY_NAME)..."
	$(GOBUILD) -o $(BINARY_NAME) ./cmd/caddy/main.go

build-xcaddy:
	@echo "Buiding with xcaddy..."
	XCADDY_DEBUG=1 $(XCADDY) build --with github.com/agent-guide/$(PACKAGE_NAME)=$(shell pwd)

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -rf buildenv_*

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

# Format code
fmt:
	@echo "Formatting code..."
	$(GOCMD) fmt ./...
