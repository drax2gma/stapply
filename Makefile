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

# Build controller
build-ctl:
	@mkdir -p $(BINARY_DIR)
	$(GO) build $(GOFLAGS) -o $(CTL_BINARY) ./cmd/sapply-ctl

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
