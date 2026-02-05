# Sapply

Minimal, agent-based remote automation utility written in Go. Replaces SSH-based execution with a NATS message bus architecture.

## Features

- **Agent-based**: No SSH dependency; agents run persistently on targets
- **NATS transport**: Request/reply messaging for reliable communication
- **Network security**: NATS URLs restricted to private networks (LAN/CGNAT) by default
- **INI configuration**: Human-editable config using `.stay.ini` extension
- **Systemd-native**: Agents managed by systemd for automatic restart
- **Discovery**: Gather system facts (CPU, memory, disk, IP) from remote nodes
- **Health Checks**: Preflight validation before deployment

## Quick Start

### Prerequisites

- Go 1.21+
- NATS server running (e.g., `apt install nats-server` or via Docker)

### Agent Requirements (Target Nodes)

The `stapply-agent` is a standalone Go binary with **no external NATS package installation required**. The NATS client library is compiled into the binary.

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

- `100.64.0.0/10` (CGNAT/Tailscale)

Use `--allow-public` flag to override (not recommended for production).

-**Payload Encryption:**

- +All command arguments and outputs are encrypted using AES-GCM when a shared secret key is provided.
- +### Key Generation
- +Generate a random 32-byte hex key:
- +`bash
+openssl rand -hex 32
+`
- +### Configuration
- +Set the `STAPPLY_SHARED_KEY` environment variable on both the controller and agent(s):
- +`bash
+export STAPPLY_SHARED_KEY=your-generated-key-here
+`
- +**Note:** If the key is missing or mismatched, secure communication will fail.

  **Deployment**: The standard way to deploy is using the one-line quick install command below. For custom setups, standard manual installation steps are also provided.

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

1. Copy binary: `scp bin/stapply-agent root@host:/usr/local/bin/`
2. Copy config: `scp examples/agent.ini root@host:/etc/stapply/`
3. Copy systemd unit: `scp systemd/stapply-agent.service root@host:/etc/systemd/system/`
4. Enable service: `ssh root@host systemctl enable --now stapply-agent`

### Controller Configuration

Controller configuration files **must** have a `.stay.ini` extension.

### Build

```bash
make build
```

### Run Agent (Terminal 1)

```bash
./bin/stapply-agent -config examples/agent.ini
```

### Ping Agent (Terminal 2)

```bash
./bin/stapply-ctl ping web1
```

### Run Deployment

```bash
./bin/stapply-ctl run -c examples/stapply.stay.ini -e dev
```

### Preflight Check

Validate system health and connectivity before running a deployment:

```bash
./bin/stapply-ctl preflight -c examples/stapply.stay.ini -e dev
```

### Discovery

Gather hardware and network facts from a remote agent:

```bash
./bin/stapply-ctl discover web1
```

### Ad-hoc Command (No Config File Needed)

Run commands on specific agents without a config file:

```bash
# Run shell command on specific agent (uses agent_id as NATS server by default)
./bin/stapply-ctl adhoc -e mini cmd 'w'
./bin/stapply-ctl adhoc -e mini cmd 'ls -la /etc'

# Specify NATS server explicitly
./bin/stapply-ctl adhoc -nats nats.example.com -e web1 cmd 'uname -a'

# Execute across all hosts in environment (requires config file)
./bin/stapply-ctl adhoc -c examples/stapply.stay.ini -e dev cmd 'uname -a'

# Restart a service
./bin/stapply-ctl adhoc -c examples/stapply.stay.ini -e dev systemd restart nginx
```

## Agent Updates

Sapply supports live updates of running agents without SSH access.

### Version Checking

Agents automatically check their version on each ping:

```bash
./bin/stapply-ctl ping web1
# Agent logs: ⚠️ Version mismatch: agent=0.1.202405201030-a1b2c3d, controller=0.1.202405201100-e5f6g7h
```

### Updating Agents

Update a running agent to match controller version:

```bash
# Update agent (NATS defaults to agent hostname)
./bin/stapply-ctl update web1

# Specify NATS server explicitly
./bin/stapply-ctl update -nats nats.example.com web1
```

**How it works:**

1. Controller sends update request with target version and binary URL
2. Agent downloads new binary from repo (`/bin/stapply-agent`)
3. Agent replaces its binary and restarts:
   - **Systemd**: Exits cleanly for systemd restart
   - **Manual/dev**: Uses `execve` to restart in-place

**Requirements:**

- Agent must be running and connected to NATS
- For stopped agents, use SSH or re-run install script

**Compatibility:**

- Agents must be ≤ controller version
- Same MAJOR.MINOR version = compatible

## Configuration

### Controller Config (`stapply.stay.ini`)

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

> **Note:** The INI parser reads files line-by-line. **Multiline values are NOT supported.**
> Long commands or file contents must be on a single line. Turn off "line wrap" in your editor when editing config files.
> For complex file content, use `template_file` with an external file instead of `write_file` with inline content.

### Agent Config (`agent.ini`)

```ini
[agent]
agent_id=web1
nats_server=nats.example.com
nats_creds=/etc/stapply/nats.creds
```

## Actions

| Action            | Status | Description                                              |
| ----------------- | ------ | -------------------------------------------------------- |
| `cmd`             | ✅ M1  | Execute shell command                                    |
| `write_file`      | ✅ M2  | Write content to file with change detection              |
| `template_file`   | ✅ M2  | Render Go template to file                               |
| `systemd`         | ✅ M3  | Systemd unit control (enable/disable/start/stop/restart) |
| `deploy_artifact` | ✅ M4  | Large binary/file distribution (chunked transfer)        |

## Project Structure

```
stapply/
├── cmd/
│   ├── stapply-agent/    # Agent daemon
│   └── stapply-ctl/      # Controller CLI
├── internal/
│   ├── config/          # INI parser (requires .stay.ini)
│   ├── actions/         # Action executors (cmd, file, systemd, artifact)
│   └── protocol/        # NATS message schemas
├── examples/            # Sample configurations
├── systemd/             # Systemd unit files
└── Makefile             # Build and release orchestration
```

## License

AGPL-3.0
