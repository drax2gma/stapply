#!/usr/bin/env bash
# Sapply Agent Installer
# Usage: curl -fsSL https://raw.githubusercontent.com/drax2gma/stapply/main/install.sh | sudo bash -s -- --nats-server <fqdn>

set -e

# Default values
AGENT_ID="$(hostname)"
NATS_URL="nats://localhost:4222"
NATS_CREDS=""
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/stapply"
SYSTEMD_DIR="/etc/systemd/system"
BINARY_URL="https://raw.githubusercontent.com/drax2gma/stapply/main/bin/stapply-agent"

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --agent-id)
            AGENT_ID="$2"
            shift 2
            ;;
        --nats-server)
            NATS_URL="$2"
            shift 2
            ;;
        --nats-creds)
            NATS_CREDS="$2"
            shift 2
            ;;
        --binary-url)
            BINARY_URL="$2"
            shift 2
            ;;
        *)
            echo "Unknown option: $1"
            echo "Usage: $0 [--agent-id <id>] [--nats-server <fqdn>] [--nats-creds <path>]"
            exit 1
            ;;
    esac
done

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo "Error: This script must be run as root"
    exit 1
fi

echo "🚀 Installing Sapply Agent..."
echo "   Agent ID: $AGENT_ID"
echo "   NATS URL: $NATS_URL"

# Create directories
echo "📁 Creating directories..."
mkdir -p "$CONFIG_DIR"
mkdir -p "$INSTALL_DIR"

# Stop service if running (to avoid "Text file busy" error)
if systemctl is-active --quiet stapply-agent 2>/dev/null; then
    echo "🛑 Stopping existing stapply-agent service..."
    systemctl stop stapply-agent
    SERVICE_WAS_RUNNING=true
else
    SERVICE_WAS_RUNNING=false
fi

# Download binary
echo "⬇️  Downloading stapply-agent..."
if command -v wget >/dev/null 2>&1; then
    wget -q -O "$INSTALL_DIR/stapply-agent" "$BINARY_URL"
elif command -v curl >/dev/null 2>&1; then
    curl -fsSL -o "$INSTALL_DIR/stapply-agent" "$BINARY_URL"
else
    echo "Error: Neither wget nor curl found. Please install one."
    exit 1
fi

chmod +x "$INSTALL_DIR/stapply-agent"

# Create agent config
echo "📝 Creating agent configuration..."
cat > "$CONFIG_DIR/agent.ini" <<EOF
[agent]
agent_id=$AGENT_ID
nats_server=$NATS_URL
EOF

# Add nats_creds if provided
if [ -n "$NATS_CREDS" ]; then
    echo "nats_creds=$NATS_CREDS" >> "$CONFIG_DIR/agent.ini"
fi

# Create systemd unit
echo "🔧 Installing systemd service..."
cat > "$SYSTEMD_DIR/stapply-agent.service" <<'EOF'
[Unit]
Description=Sapply Agent
Documentation=https://github.com/drax2gma/stapply
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/stapply-agent -config /etc/stapply/agent.ini
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=stapply-agent

# Security settings for automation agent
# Note: Relaxed restrictions to allow package installation,
# config management, and systemd service control
NoNewPrivileges=true
PrivateTmp=true

# Environment file (optional, for overrides)
EnvironmentFile=-/etc/stapply/agent.env

[Install]
WantedBy=multi-user.target
EOF

# Reload systemd
echo "🔄 Reloading systemd..."
systemctl daemon-reload

# Enable and start service
echo "✅ Enabling and starting stapply-agent..."
systemctl enable stapply-agent
systemctl start stapply-agent

# Check status
sleep 2
if systemctl is-active --quiet stapply-agent; then
    echo "✅ Sapply Agent installed and running!"
    echo ""
    echo "Agent ID: $AGENT_ID"
    echo "NATS URL: $NATS_URL"
    echo ""
    echo "Check status: systemctl status stapply-agent"
    echo "View logs: journalctl -u stapply-agent -f"
else
    echo "⚠️  Agent installed but failed to start"
    echo "Check logs: journalctl -u stapply-agent -n 50"
    exit 1
fi
