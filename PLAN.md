# Stapply - High-Level Design

## Overview

Stapply is a minimal, agent-based remote automation utility written in Go that replaces SSH-based execution with a NATS message bus architecture. The controller reads environment/host/app definitions from `.ini` files and sends typed action requests to agents running under systemd supervision on target machines. Agents execute actions locally (commands, file operations, systemd control) and return structured results via NATS request/reply. [docs.nats](https://docs.nats.io/nats-concepts/core-nats/reqreply)

## Design Principles

- **Agent-based**: no SSH dependency; agents run persistently on targets and communicate via NATS [redhat](https://www.redhat.com/en/topics/automation/learning-ansible-tutorial)
- **Minimal scope**: focus on core automation primitives (cmd, file writes, templates, systemd) without reproducing Ansible's module ecosystem [docs.ansible](https://docs.ansible.com/ansible/latest/playbook_guide/playbooks_intro.html)
- **INI configuration**: human-editable `.ini` files with `#` comments and `key=val1,val2,...` list syntax instead of YAML/JSON [quickref](https://quickref.me/ini.html)
- **Go stdlib + NATS**: leverage Go standard library where possible, use NATS client library for message transport [pkg.go](https://pkg.go.dev/text/template)
- **Systemd-native**: agents managed by systemd for automatic restart and logging integration [redhat](https://www.redhat.com/en/blog/systemd-automate-recovery)

## Architecture

### Components

**Controller** (`cmd/ansigo-ctl`)

- CLI that parses `.ini` configuration files
- Expands environments into host lists and app definitions
- Sends NATS requests to agents and aggregates results
- Reports execution status (ok/changed/failed/timeout) per host/app

**Agent** (`cmd/ansigo-agent`)

- Long-running daemon managed by systemd
- Subscribes to NATS subjects: `ansigo.ping.<agent_id>`, `ansigo.run.<agent_id>` [docs.nats](https://docs.nats.io/nats-concepts/subjects)
- Executes actions locally on the target machine
- Returns structured JSON responses via NATS reply subjects [docs.nats](https://docs.nats.io/using-nats/developer/sending/replyto)

**Libraries** (`internal/`)

- `config`: INI parser with support for `[section]`, `key=value`, `key_list=a,b,c`, and `#` comments [w3schools](https://www.w3schools.io/file/ini-comments-syntax/)
- `actions`: typed action executors (cmd, write_file, template_file, systemd)
- `protocol`: JSON message schemas for requests/responses
- `executor`: controller execution engine (concurrency control, timeout handling)
- `reporter`: result aggregation and display

### NATS Message Bus

**Subjects** [docs.nats](https://docs.nats.io/using-nats/developer/receiving/wildcards)

- `ansigo.ping.<agent_id>`: health check request/reply
- `ansigo.run.<agent_id>`: action execution request/reply

**Request/Reply Pattern** [docs.nats](https://docs.nats.io/nats-concepts/core-nats/reqreply)

- Controller publishes request with auto-generated inbox reply subject
- Agent processes request and publishes response to reply subject
- Controller waits for response or times out

**Permissions** [github](https://github.com/nats-io/nats-server/discussions/6382)

- Agents need: subscribe to `ansigo.*.<agent_id>`, publish to `_INBOX.>`
- Controller needs: publish to `ansigo.*`, subscribe to `_INBOX.>`

## Configuration Model

### INI Dialect [en.wikipedia](https://en.wikipedia.org/wiki/INI_file)

Strict rules:

- Sections: `[env:<name>]`, `[host:<id>]`, `[app:<name>]`
- Comments: full-line `#` only (no inline comments)
- Lists: `key=val1,val2,val3` (comma-separated, whitespace trimmed)
- Keys: unique within section, case-sensitive

### Schema

**Environment Section** `[env:<name>]`

```ini
[env:prod]
hosts=web1,web2,db1       # List of host IDs
apps=nginx,api            # List of app names to deploy
concurrency=10            # Max parallel agents
```

**Host Section** `[host:<id>]`

```ini
[host:web1]
agent_id=web1             # Must match NATS subject agent_id
tags=edge,az1             # Optional metadata
# Future: vars for templating
```

**App Section** `[app:<name>]`

```ini
[app:nginx]
step1=cmd:apt-get install -y nginx
step2=systemd:enable nginx
step3=write_file:/etc/nginx/nginx.conf mode=0644
step4=systemd:restart nginx
```

Steps are executed sequentially per agent in numeric order.

## Action Types

### 1. `cmd` - Execute Command [redhat](https://www.redhat.com/en/topics/automation/learning-ansible-tutorial)

Runs a shell command on the target via local execution.

**Request args:**

- `command` (string): shell command to execute
- `creates` (optional string): path; skip if exists (idempotency guard)

**Response:**

- `stdout`, `stderr` (string)
- `exit_code` (int)
- `changed` (bool): always true unless `creates` guard triggered

### 2. `write_file` - Write File

Writes content to a file path with optional mode/owner.

**Request args:**

- `path` (string): target file path
- `content` (string): file content (or base64 for binary)
- `mode` (optional string): octal mode (e.g., "0644")
- `owner` (optional string): user:group

**Response:**

- `changed` (bool): true if content/mode/owner differ from existing

### 3. `template_file` - Render Go Template [pkg.go](https://pkg.go.dev/text/template)

Renders a Go `text/template` on the agent and writes result.

**Request args:**

- `path` (string): target file path
- `template` (string): Go template text
- `vars` (map): variables for template context
- `mode` (optional string)

**Response:**

- `changed` (bool): true if rendered output differs

### 4. `systemd` - Systemd Control [wiki.archlinux](https://wiki.archlinux.org/title/Systemd)

Wrapper around `systemctl` commands.

**Request args:**

- `action` (string): `enable|disable|start|stop|restart|daemon-reload`
- `unit` (string): systemd unit name (e.g., "nginx.service")

**Response:**

- `changed` (bool): state changed
- `stdout`, `stderr`, `exit_code`

## Protocol (JSON Wire Format)

### Ping Request

```json
{
  "request_id": "uuid",
  "type": "ping"
}
```

### Ping Response

```json
{
  "request_id": "uuid",
  "agent_id": "web1",
  "version": "0.1.0",
  "uptime_seconds": 3600
}
```

### Run Request [docs.nats](https://docs.nats.io/nats-concepts/core-nats/reqreply)

```json
{
  "request_id": "uuid",
  "type": "run",
  "timeout_ms": 30000,
  "action": "cmd",
  "args": {
    "command": "apt-get install -y nginx"
  }
}
```

### Run Response

```json
{
  "request_id": "uuid",
  "status": "ok",
  "changed": true,
  "exit_code": 0,
  "stdout": "...",
  "stderr": "",
  "duration_ms": 1234
}
```

Status values: `ok`, `failed`, `timeout`, `error`

## Workflows

### Bootstrap (One-Time Setup) [redhat](https://www.redhat.com/en/blog/systemd-automate-recovery)

1. Distribute agent binary + systemd unit via HTTPS link/package
2. Install script writes `/etc/ansigo/agent.ini`:
   ```ini
   [agent]
   agent_id=web1
   nats_url=nats://nats.example.com:4222
   nats_creds=/etc/ansigo/nats.creds
   ```
3. Enable and start systemd unit: `systemctl enable --now ansigo-agent`
4. Agent connects to NATS and subscribes to `ansigo.*.<agent_id>` [docs.nats](https://docs.nats.io/nats-concepts/subjects)

### Deploy Flow (Controller) [docs.nats](https://docs.nats.io/nats-concepts/core-nats/reqreply)

1. **Parse config**: load `ansigo.ini`, validate sections
2. **Expand environment**: resolve `[env:prod]` → hosts list → apps list
3. **Build execution plan**: for each host × app, create ordered step list
4. **Execute with concurrency**:
   - Worker pool (size = `concurrency` setting)
   - Per-agent: run steps sequentially
   - Across agents: run in parallel
5. **Send NATS requests**: `ansigo.run.<agent_id>` for each step [docs.nats](https://docs.nats.io/nats-concepts/core-nats/reqreply)
6. **Aggregate results**: collect responses, detect timeouts/failures
7. **Report**: display summary (ok/changed/failed counts per host/app)

### Agent Execution Loop [redhat](https://www.redhat.com/en/blog/systemd-automate-recovery)

1. Connect to NATS server with credentials
2. Subscribe to `ansigo.ping.<agent_id>` and `ansigo.run.<agent_id>` [docs.nats](https://docs.nats.io/nats-concepts/subjects)
3. On request:
   - Validate JSON schema
   - Execute action type handler
   - Capture result (stdout/stderr/changed/exit_code)
   - Publish response to reply subject [docs.nats](https://docs.nats.io/using-nats/developer/sending/replyto)
4. On disconnect: systemd restarts agent automatically [redhat](https://www.redhat.com/en/blog/systemd-automate-recovery)

## Concurrency Model

- **Per-agent**: steps execute **sequentially** (step1 → step2 → step3)
- **Across agents**: execute in **parallel** up to `concurrency` limit
- **Timeout handling**: controller abandons request after `timeout_ms`, marks as failed

Example: `concurrency=10` with 50 hosts means 10 agents run simultaneously, each executing its steps in order.

## Directory Structure

```
ansigo/
├── cmd/
│   ├── ansigo-ctl/          # Controller CLI
│   │   └── main.go
│   └── ansigo-agent/        # Agent daemon
│       └── main.go
├── internal/
│   ├── config/              # INI parser
│   │   ├── parser.go
│   │   └── schema.go
│   ├── actions/             # Action executors
│   │   ├── cmd.go
│   │   ├── file.go
│   │   ├── template.go
│   │   └── systemd.go
│   ├── protocol/            # NATS message schemas
│   │   ├── request.go
│   │   └── response.go
│   ├── executor/            # Controller execution engine
│   │   ├── planner.go
│   │   └── worker.go
│   └── reporter/            # Result aggregation
│       └── report.go
├── examples/
│   └── ansigo.ini           # Sample configuration
├── systemd/
│   └── ansigo-agent.service # Systemd unit file
├── go.mod
├── go.sum
├── HLD.md                   # This document
└── README.md
```

## Dependencies

- `github.com/nats-io/nats.go`: NATS client library [docs.nats](https://docs.nats.io/nats-concepts/core-nats/reqreply)
- Go stdlib: `text/template`, `encoding/json`, `os/exec`, `net/http`, `sync`

## Milestones

### M1: Core Agent/Controller + `cmd` Action

- INI parser (env/host/app sections)
- NATS request/reply for `ping` and `cmd` action
- Basic systemd unit and bootstrap script
- Controller: single-app sequential execution

### M2: File and Template Actions [pkg.go](https://pkg.go.dev/text/template)

- `write_file` with change detection (hash comparison)
- `template_file` using Go `text/template`
- Controller: multi-app support with per-agent sequencing

### M3: Systemd + Concurrency

- `systemd` action (enable/start/restart)
- Controller: parallel execution across agents (worker pool)
- Timeout and error handling

### M4: Polish

- Result reporting (table format, JSON output)
- Ad-hoc mode: `ansigo-ctl adhoc -e prod cmd 'uname -a'`
- Agent health monitoring dashboard (optional)

## Security Considerations

- **NATS authentication**: use NATS creds/JWT or TLS client certs [docs.nats](https://docs.nats.io/nats-concepts/core-nats/reqreply)
- **Agent isolation**: each agent only subscribes to its own `<agent_id>` subject [docs.nats](https://docs.nats.io/nats-concepts/subjects)
- **File permissions**: `write_file` respects umask; agent runs as specific user (not root by default)
- **Command injection**: no shell interpolation in controller; commands sent as-is to agent

## Limitations (by Design)

- No SSH transport (agents must be pre-installed) [redhat](https://www.redhat.com/en/topics/automation/learning-ansible-tutorial)
- No YAML/Jinja2 compatibility (INI + Go templates only) [docs.ansible](https://docs.ansible.com/ansible/latest/playbook_guide/playbooks_templating.html)
- No inventory groups (flat environment → hosts mapping) [docs.ansible](https://docs.ansible.com/ansible/latest/plugins/inventory.html)
- No module ecosystem (4 core actions only)
- No implicit gathering of facts (agents are stateless per-request)

## Future Enhancements (Out of Scope for v0.1)

- Per-host variables for template rendering
- Conditional execution (`when:` equivalent)
- Parallel step execution within an agent
- Pull-based agent mode (agent polls for work)
- Encrypted secret storage for sensitive variables
- Audit logging (request/response history)

---

**Document Version**: 0.1  
**Last Updated**: 2026-02-04
