# VM Agent

Multi-Tenant VM Management Agent - a lightweight agent for remote VM management.

## Features

- **Remote Execution**: Execute workflows remotely via Piko tunneling
- **Health Monitoring**: Continuous health monitoring and reporting
- **Self-Upgrade**: Automatic self-upgrade with rollback capabilities
- **Multi-Tenant**: Full tenant isolation and authentication
- **Cross-Platform**: Supports Linux (systemd) and Windows (Service)

## Installation

### Prerequisites

- Linux (systemd) or Windows
- Network access to Piko server and Control Plane

### Quick Install

```bash
# Download the agent
curl -LO https://releases.example.com/vm-agent/latest/vm-agent-linux-amd64

# Make executable
chmod +x vm-agent-linux-amd64

# Install as service
sudo ./vm-agent-linux-amd64 install \
  --tenant-id "your-tenant" \
  --key "your-installation-key" \
  --piko-url "https://piko.example.com" \
  --control-plane-url "https://control-plane.example.com"
```

## Commands

### run
Start the agent.

```bash
vm-agent run --config /etc/vm-agent/config.yaml
```

### install
Install the agent as a system service.

```bash
vm-agent install \
  --tenant-id "acme" \
  --key "install-key-123" \
  --piko-url "https://piko.example.com" \
  --control-plane-url "https://cp.example.com"
```

### configure
Update agent configuration.

```bash
vm-agent configure
```

### repair
Diagnose and repair agent issues.

```bash
# Diagnose only
vm-agent repair --diagnose

# Diagnose and repair
vm-agent repair
```

### upgrade
Upgrade the agent to a new version.

```bash
vm-agent upgrade \
  --version "1.2.0" \
  --url "https://releases.example.com/vm-agent/1.2.0/vm-agent-linux-amd64" \
  --checksum "sha256:abc123..."
```

### uninstall
Uninstall the agent.

```bash
# Standard uninstall
vm-agent uninstall

# Keep data and logs
vm-agent uninstall --keep-data --keep-logs

# Complete removal
vm-agent uninstall --purge
```

### status
Show agent status and health information.

```bash
vm-agent status
```

### version
Display version information.

```bash
vm-agent version
```

## Configuration

Configuration file: `/etc/vm-agent/config.yaml`

```yaml
agent:
  id: "server-001"
  tenant_id: "acme"
  control_plane_url: "https://control-plane.example.com"

piko:
  server_url: "https://piko.example.com"
  endpoint: "tenant-acme/server-001"
  reconnect:
    initial_delay: 1s
    max_delay: 60s
    multiplier: 2.0

webhook:
  listen_addr: "0.0.0.0"
  port: 9999
  tls_enabled: false

probe:
  work_dir: "/var/lib/vm-agent/work"
  default_timeout: 300s
  max_concurrent: 5

health:
  check_interval: 30s
  report_interval: 300s
```

## Building

```bash
# Build for current platform
make build

# Build for all platforms
make build-all

# Run tests
make test

# Build Docker image
make docker-build VERSION=1.0.0
```

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                      VM Agent                            │
├─────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐     │
│  │   Manager   │  │    Piko     │  │   Webhook   │     │
│  │             │  │   Client    │  │   Server    │     │
│  └─────────────┘  └─────────────┘  └─────────────┘     │
│         │                │                │             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐     │
│  │   Config    │  │   Probe     │  │   Health    │     │
│  │   Loader    │  │  Executor   │  │   Monitor   │     │
│  └─────────────┘  └─────────────┘  └─────────────┘     │
│         │                │                │             │
│  ┌─────────────────────────────────────────────────┐   │
│  │                  Lifecycle                        │   │
│  │  (Install, Configure, Repair, Upgrade, Uninstall)│   │
│  └─────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
```

## License

Copyright (c) 2024. All rights reserved.
