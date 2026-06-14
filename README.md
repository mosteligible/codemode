# mcp-codemode

This README covers the backend remote code-execution stack in this repository, built around MCP.

The current execution path is:

`client` -> `pyrunner` -> `coderunner` -> `agent` -> Docker/gVisor container

## Components In Scope

- `client`: optional FastAPI chat/orchestration service that runs an LLM agent and connects to MCP.
- `pyrunner`: Python MCP server that exposes tools, including `execute_code`.
- `coderunner`: Go control plane that accepts execution requests over HTTP, uses Redis for worker/session routing, and forwards work to agents over gRPC.
- `agent`: Go worker service that manages sandbox containers and executes code remotely.
- `redis`: worker registry, session routing store, and token storage used by the proxy flow.

This document intentionally focuses on the remote execution backend path and does not describe every app in the repository.

## Architecture

### Code execution flow

1. A user message reaches `client` through `/chat`.
2. The LLM uses MCP tools exposed by `pyrunner`.
3. When the model calls `execute_code`, `pyrunner` sends `POST /run` to `coderunner`.
4. `coderunner` resolves the target worker from Redis, reusing the session's assigned worker when `sessionId` is present.
5. `coderunner` forwards the request over gRPC using `ExecuteCode`.
6. The `agent` validates the language, applies a timeout, and runs the instruction inside a managed container.
7. Stdout, stderr, and exit status return back through `coderunner` to `pyrunner`, then to the MCP client.

### Runtime responsibilities

- `pyrunner` is the MCP and tool gateway. It does not directly execute code locally in the current setup.
- `coderunner` is the transport/control-plane layer. It receives `/run` requests and routes them to workers using Redis state.
- `agent` is the execution tier. It starts and reuses containers, then runs commands with Docker exec.

## Current State

- MCP execution is currently stateless from the Python path unless callers provide a `sessionId`.
- The Go execution API supports `sessionId`, but `pyrunner` does not currently send one.
- `agent` currently validates only `python` and `bash` for remote execution.
- Worker selection in `coderunner` is Redis-backed and prefers workers with more available slots, then lower host load.

## Services And Ports

- `pyrunner`: MCP endpoints on `:8000`
- `client`: FastAPI app on `:8080` by default
- `coderunner`: HTTP API on `:8080` and Go MCP server on `:8081`
- `agent`: gRPC worker on `:30031` by default
- `redis`: `:6379`

Note: `client` and `coderunner` both default to `:8080`, so they are not meant to run on the same host without changing configuration.

## Running The System

### Full remote execution stack

To use MCP with remote code execution, start services in this order:

1. Start `redis`
2. Start one or more `agent` workers
3. Start `coderunner`
4. Start `pyrunner`
5. Optionally start `client`

### 1. Start `redis`

```bash
docker pull redis
docker run -p 6379:6379 --name redis -d redis
```

### 2. Start `agent`

Run the worker on a Linux machine with Docker and gVisor configured:

```bash
cd agent
go run .
```

Important environment variables:

- `WORKER_PORT` default: `:30031`
- `WORKER_HOST` default: `localhost`
- `WORKER_ADDRESS` default: `WORKER_HOST + WORKER_PORT`
- `DOCKER_IMAGE_NAME` default: `python:3.14-slim`
- `MIN_ACTIVE` default: `2`
- `REDIS_HOST` default: `localhost`
- `REDIS_PORT` default: `6379`

### 3. Start `coderunner`

Coderunner discovers live workers from Redis. `REMOTE_HOSTS` is optional and can be used as a static fallback or to pre-create clients. The value is a semicolon-delimited list, for example:

```bash
REMOTE_HOSTS=127.0.0.1:30031
```

Then run:

```bash
make run-coderunner
```

Or:

```bash
cd coderunner
go build -o ../build/coderunner .
../build/coderunner
```

### 4. Start `pyrunner`

Point `pyrunner` at `coderunner`:

```bash
export CODE_EXECUTION_HOST=http://localhost:8080
cd pyrunner
uv run uvicorn main:app
```

This exposes:

- `http://localhost:8000/mcp`
- `http://localhost:8000/mcp-no-code-execute`

### 5. Start `client` (optional)

Start the chat/orchestration service only if you want the built-in FastAPI agent layer:

```bash
cd client
uv run main.py
```

Configure the MCP endpoint in `client/config.py` or with environment variables if needed.

## Quick Start Variants

### MCP only

Use this when another MCP client will call the server directly:

1. Start `agent`
2. Start `coderunner`
3. Start `pyrunner`

### Chat stack

Use this when you want the built-in `/chat` endpoint:

1. Start `redis`
2. Start `agent`
3. Start `coderunner`
4. Start `pyrunner`
5. Start `client`

## Makefile Targets

From the project root:

```bash
make run-pyrunner
make run-client
make build-coderunner
make run-coderunner
```

## Notes

- Start `pyrunner` only after `coderunner` is reachable.
- Start `coderunner` only after Redis is reachable. Start at least one `agent` so Redis contains a live worker heartbeat before sending `/run` requests.
- This root README is intentionally scoped to the remote execution backend architecture.
- Component-specific setup details also live in `pyrunner/README.md`, `coderunner/README.md`, and `agent/README.md`.
