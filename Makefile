.PHONY: build build-agent build-ctl test clean install-agent

# Build configuration
BINARY_DIR := bin
AGENT_BINARY := $(BINARY_DIR)/stapply-agent
CTL_BINARY := $(BINARY_DIR)/stapply-ctl
GO := go

# Dynamic Versioning
VERSION_MAJOR := 0
VERSION_MINOR := 1
BUILD_DATE := $(shell date +%Y%m%d%H%M)
VERSION_FULL := $(VERSION_MAJOR).$(VERSION_MINOR).$(BUILD_DATE)

# Inject version into both main packages
LDFLAGS := -s -w -X main.Version=$(VERSION_FULL)
GOFLAGS := -ldflags="$(LDFLAGS)"

# Default target
all: build

# Build both binaries
build: build-agent build-ctl

# Build agent
build-agent:
	@mkdir -p $(BINARY_DIR)
	$(GO) build $(GOFLAGS) -o $(AGENT_BINARY) ./cmd/stapply-agent
	upx --best --lzma $(AGENT_BINARY)

# Build controller
build-ctl:
	@mkdir -p $(BINARY_DIR)
	$(GO) build $(GOFLAGS) -o $(CTL_BINARY) ./cmd/stapply-ctl
	upx --best --lzma $(CTL_BINARY)

# Run tests
test:
	$(GO) test -v ./...

# Clean build artifacts
clean:
	rm -rf $(BINARY_DIR)
	$(GO) clean

# Install agent (requires sudo)
install-agent: build-agent
	sudo install -m 755 $(AGENT_BINARY) /usr/local/bin/
	sudo mkdir -p /etc/stapply
	sudo install -m 644 systemd/stapply-agent.service /etc/systemd/system/
	@echo "Agent installed. Run: sudo systemctl daemon-reload && sudo systemctl enable stapply-agent"

# Development helpers
run-agent: build-agent
	./$(AGENT_BINARY) -config examples/agent.ini

ping-local: build-ctl
	./$(CTL_BINARY) ping local

run-dev: build-ctl
	./$(CTL_BINARY) run -c examples/stapply.ini -e dev

adhoc-test: build-ctl
	./$(CTL_BINARY) adhoc -c examples/stapply.ini -e dev cmd 'uname -a'

# Go module maintenance
tidy:
	$(GO) mod tidy

deps:
	$(GO) mod download

# Create release binary for Linux AMD64
release:
	@mkdir -p $(BINARY_DIR)
	@rm -f $(BINARY_DIR)/stapply-agent $(BINARY_DIR)/stapply-ctl
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(BINARY_DIR)/stapply-agent ./cmd/stapply-agent
	upx --best --lzma $(BINARY_DIR)/stapply-agent
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(BINARY_DIR)/stapply-ctl ./cmd/stapply-ctl
	upx --best --lzma $(BINARY_DIR)/stapply-ctl
	@echo "✅ Release binary created: $(BINARY_DIR)/stapply-agent"
	@echo "✅ Release binary created: $(BINARY_DIR)/stapply-ctl"
	@echo "   OS: linux"
	@echo "   Arch: amd64"
	@echo ""
	@echo "Upload this file to GitHub Releases:"
	@echo "https://github.com/drax2gma/stapply/releases/new"
