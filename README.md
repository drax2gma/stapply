# Sapply

Minimal, agent-based remote automation utility written in Go. Replaces SSH-based execution with a NATS message bus architecture.

## Features

- **Agent-based**: No SSH dependency; agents run persistently on targets
- **NATS transport**: Request/reply messaging for reliable communication
- **INI configuration**: Human-editable config with `[env:name]`, `[host:id]`, `[app:name]` sections
- **Systemd-native**: Agents managed by systemd for automatic restart

## Quick Start

### Prerequisites

- Go 1.21+
- NATS server running (e.g., `nats-server` via Homebrew)

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
