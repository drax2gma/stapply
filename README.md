# Sapply

Minimal, agent-based remote automation utility written in Go. Replaces SSH-based execution with a NATS message bus architecture.

## Features

- **Agent-based**: No SSH dependency; agents run persistently on targets
- **NATS transport**: Request/reply messaging for reliable communication
- **Network security**: NATS URLs restricted to private networks (LAN/CGNAT) by default
- **INI configuration**: Human-editable config with `[env:name]`, `[host:id]`, `[app:name]` sections
- **Systemd-native**: Agents managed by systemd for automatic restart

## Quick Start

### Prerequisites

- Go 1.21+
- NATS server running (e.g., `nats-server` via Homebrew)

### Agent Requirements (Target Nodes)

The `sapply-agent` is a standalone Go binary with **no external NATS package installation required**. The NATS client library is compiled into the binary.

**Minimum requirements for Linux 64-bit target nodes:**

| Component           | Requirement   | Notes                                                      |
| ------------------- | ------------- | ---------------------------------------------------------- |
| **Kernel**          | Linux 2.6.32+ | RHEL/CentOS 6+ era                                         |
| **libc**            | glibc 2.3.2+  | Standard on distros from last 20 years                     |
| **systemd**         | Any version   | Standard since ~2015 (Ubuntu 15.04+, Debian 8+, CentOS 7+) |
| **ca-certificates** | Installed     | For TLS/SSL connections                                    |

**Distro compatibility (last 10 years):**

- ✅ **Ubuntu 16.04+, Debian 8+, CentOS 7+, RHEL 7+**: Fully supported
- ⚠️ **Alpine Linux**: Requires `libc6-compat` or static build (`CGO_ENABLED=0`)

**Network security:**

By default, NATS connections are restricted to private networks only:

- `127.0.0.0/8` (localhost)
- `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16` (private LAN)
- `100.64.0.0/10` (CGNAT/Tailscale)

Use `--allow-public` flag to override (not recommended for production).

**Deployment**: Just copy the `sapply-agent` binary and systemd unit file. No package installation needed.

## Installation

### Quick Install (One-liner)

Install agent on remote node:

```bash
curl -fsSL https://raw.githubusercontent.com/drax2gma/stapply/main/install.sh | \
  sudo bash -s -- --agent-id web1 --nats-url nats://nats.example.com:4222
```

**Options:**

- `--agent-id <id>` (required): Unique agent identifier
- `--nats-url <url>`: NATS server URL (default: `nats://localhost:4222`)
- `--nats-creds <path>`: Path to NATS credentials file (optional)

### Manual Install

1. Copy binary: `scp bin/sapply-agent root@host:/usr/local/bin/`
2. Copy config: `scp examples/agent.ini root@host:/etc/sapply/`
3. Copy systemd unit: `scp systemd/sapply-agent.service root@host:/etc/systemd/system/`
4. Enable service: `ssh root@host systemctl enable --now sapply-agent`

### Build

```bash
make build
```

### Run Agent (Terminal 1)

```bash
./bin/sapply-agent -config examples/agent.ini
```

### Ping Agent (Terminal 2)

```bash
./bin/sapply-ctl ping local
```

### Run Deployment

```bash
./bin/sapply-ctl run -c examples/sapply.ini -e dev
```

### Ad-hoc Command (No Config File Needed)

```bash
# Execute single command across all hosts in environment
./bin/sapply-ctl adhoc -c examples/sapply.ini -e dev cmd 'uname -a'

# Restart a service
./bin/sapply-ctl adhoc -c examples/sapply.ini -e dev systemd restart nginx
```

## Agent Updates

Sapply supports live updates of running agents without SSH access.

### Version Checking

Agents automatically check their version on each ping:

```bash
./bin/sapply-ctl ping web1
# Agent logs: ⚠️ Version mismatch: agent=0.0.9, controller=0.1.0
```

### Updating Agents

Update a running agent to match controller version:

```bash
# Update agent (NATS defaults to agent hostname)
./bin/sapply-ctl update web1

# Specify NATS server explicitly
./bin/sapply-ctl update -nats nats.example.com web1
```

**How it works:**

1. Controller sends update request with target version and binary URL
2. Agent downloads new binary from repo (`/bin/sapply-agent`)
3. Agent replaces its binary and restarts:
   - **Systemd**: Exits cleanly for systemd restart
   - **Manual/dev**: Uses `execve` to restart in-place

**Requirements:**

- Agent must be running and connected to NATS
- For stopped agents, use SSH or re-run install script

**Compatibility:**

- Agents must be ≤ controller version
- Same MAJOR version = compatible

## Configuration

### Controller Config (`sapply.ini`)

```ini
[env:prod]
hosts=web1,web2
apps=nginx
concurrency=10

[host:web1]
agent_id=web1
tags=edge

[app:nginx]
step1=cmd:apt-get install -y nginx
step2=cmd:systemctl enable nginx
step3=cmd:systemctl start nginx
```

### Agent Config (`agent.ini`)

```ini
[agent]
agent_id=web1
nats_url=nats://nats.example.com:4222
nats_creds=/etc/sapply/nats.creds
```

## Actions

| Action          | Status | Description                                              |
| --------------- | ------ | -------------------------------------------------------- |
| `cmd`           | ✅ M1  | Execute shell command                                    |
| `write_file`    | ✅ M2  | Write content to file with change detection              |
| `template_file` | ✅ M2  | Render Go template to file                               |
| `systemd`       | ✅ M3  | Systemd unit control (enable/disable/start/stop/restart) |

## Project Structure

```
stapply/
├── cmd/
│   ├── sapply-agent/    # Agent daemon
│   └── sapply-ctl/      # Controller CLI
├── internal/
│   ├── config/          # INI parser
│   ├── actions/         # Action executors
│   └── protocol/        # NATS message schemas
├── examples/            # Sample configurations
├── systemd/             # Systemd unit files
└── PLAN.md              # High-level design
```

## License

AGPL-3.0
