.PHONY: build build-agent build-ctl test clean install-agent

# Build configuration
BINARY_DIR := bin
AGENT_BINARY := $(BINARY_DIR)/sapply-agent
CTL_BINARY := $(BINARY_DIR)/sapply-ctl
GO := go
GOFLAGS := -ldflags="-s -w"

# Default target
all: build

# Build both binaries
build: build-agent build-ctl

# Build agent
build-agent:
	@mkdir -p $(BINARY_DIR)
	$(GO) build $(GOFLAGS) -o $(AGENT_BINARY) ./cmd/sapply-agent
	upx --best --lzma $(AGENT_BINARY)

# Build controller
build-ctl:
	@mkdir -p $(BINARY_DIR)
	$(GO) build $(GOFLAGS) -o $(CTL_BINARY) ./cmd/sapply-ctl
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
	sudo mkdir -p /etc/sapply
	sudo install -m 644 systemd/sapply-agent.service /etc/systemd/system/
	@echo "Agent installed. Run: sudo systemctl daemon-reload && sudo systemctl enable sapply-agent"

# Development helpers
run-agent: build-agent
	./$(AGENT_BINARY) -config examples/agent.ini

ping-local: build-ctl
	./$(CTL_BINARY) ping local

run-dev: build-ctl
	./$(CTL_BINARY) run -c examples/sapply.ini -e dev

adhoc-test: build-ctl
	./$(CTL_BINARY) adhoc -c examples/sapply.ini -e dev cmd 'uname -a'

# Go module maintenance
tidy:
	$(GO) mod tidy

deps:
	$(GO) mod download

# Create release binary for Linux AMD64
release:
	@mkdir -p $(BINARY_DIR)
	@rm -f $(BINARY_DIR)/sapply-agent $(BINARY_DIR)/sapply-ctl
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(BINARY_DIR)/sapply-agent ./cmd/sapply-agent
	upx --best --lzma $(BINARY_DIR)/sapply-agent
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(BINARY_DIR)/sapply-ctl ./cmd/sapply-ctl
	upx --best --lzma $(BINARY_DIR)/sapply-ctl
	@echo "✅ Release binary created: $(BINARY_DIR)/sapply-agent"
	@echo "✅ Release binary created: $(BINARY_DIR)/sapply-ctl"
	@echo "   OS: linux"
	@echo "   Arch: amd64"
	@echo ""
	@echo "Upload this file to GitHub Releases:"
	@echo "https://github.com/drax2gma/stapply/releases/new"
