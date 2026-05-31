# MCP Codemode Agent

The MCP Codemode Agent is a lightweight gRPC service that keeps a warm pool of Docker containers available for code execution. It also publishes worker heartbeat and capacity state to Redis so Coderunner can route requests to live workers.

## Production Installation

Use the installer in this directory:

```bash
sudo ./setup.sh
```

The installer does the following:

1. Checks that Linux, `systemd`, and the Docker daemon are available.
2. Downloads the latest GitHub Release asset for this repository.
3. Falls back to `go install github.com/mosteligible/mcp-codemode/agent@latest` if no matching release asset exists.
4. Installs the binary to `/usr/local/bin/mcp-codemode-agent`.
5. Creates a dedicated service account and a `systemd` unit.
6. Enables and starts the background service.

The installer targets these release assets:

- `agent_linux_amd64.tar.gz`
- `agent_linux_arm64.tar.gz`

## Host Requirements

- Linux host with `systemd`
- Docker daemon already installed and running
- `curl` or `wget`
- `tar`
- `go` only if the installer needs to use the fallback path

The agent currently relies on the host Docker runtime configuration. If you require gVisor isolation, configure Docker so the relevant runtime is already available before installing the service.

## Service Layout

- Binary: `/usr/local/bin/mcp-codemode-agent`
- Service: `mcp-codemode-agent`
- Environment file: `/etc/mcp-codemode-agent/agent.env`
- State directory: `/var/lib/mcp-codemode-agent`

Useful commands:

```bash
systemctl status mcp-codemode-agent
journalctl -u mcp-codemode-agent -f
systemctl restart mcp-codemode-agent
```

## Configuration

The installer writes a managed environment file at `/etc/mcp-codemode-agent/agent.env`.

Supported settings:

- `DOCKER_API_VERSION` default: `1.40`
- `DOCKER_IMAGE_NAME` default: `python:3.14-slim`
- `WORKER_PORT` default: `:30031`
- `WORKER_HOST` default: `localhost`
- `WORKER_ADDRESS` default: `WORKER_HOST + WORKER_PORT`
- `MIN_ACTIVE` default: `2`
- `ACTIVE_CONTAINER_CHECK_INTERVAL` default: `30`
- `REDIS_HOST` default: `localhost`
- `REDIS_PORT` default: `6379`
- `REDIS_PASSWORD` default: empty
- `REDIS_DB` default: `0`

`WORKER_ADDRESS` is the address written to Redis and later used by Coderunner as the gRPC target.

Update the environment file and restart the service when you need to change runtime settings.

## Release Automation

The repository publishes agent binaries from GitHub Actions on every push to `main`. Each run creates a new GitHub Release and marks it as the latest release.

The installer uses the latest release by default, which gives you a stable install path while preserving rollback history in GitHub Releases.

## Manual Development Install

If you need a local developer install without `systemd`, you can still use:

```bash
go install github.com/mosteligible/mcp-codemode/agent@latest
```

That path is intended for development, not for long-running production deployment.
