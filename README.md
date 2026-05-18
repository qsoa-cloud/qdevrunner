# qdevrunner

Full local development runner for qSOA Cloud services. A single binary that provides the complete service runtime environment: service mesh, distributed tracing, metrics, database access, DFS, email, MCP server, and a real-time Web UI.

One qdevrunner instance = one project.

## Quick Start with Claude Code

The fastest way to start a new qSOA Cloud project:

```bash
# One-time setup: register qdevrunner as an MCP server for Claude Code
claude mcp add -s user qdevrunner -- qdevrunner --stdio
```

Then tell Claude: "Create a new qSOA Cloud project for &lt;description&gt;". Claude will automatically start qdevrunner, read the architecture guide, and scaffold everything.

## Quick Start (Manual)

```bash
# Start in any directory — config is created automatically if missing
qdevrunner

# Open the Web UI
open http://127.0.0.1:8090

# See the MCP Setup page for connecting AI assistants
```

No config file required. qdevrunner starts with sensible defaults (project "default", env "dev", webui on :8090) and creates `qdevrunner.ini` automatically.

## Features

- **Service mesh** -- gRPC inter-service discovery and proxying. Services call each other via `qcloud://service-name/` just like production.
- **Auto-build & auto-discover** -- Just point to a Go source directory. qdevrunner builds the binary and discovers transports automatically.
- **DFS** -- Distributed file system backed by local directories.
- **Email** -- QEmail gRPC service proxying to real SMTP/IMAP servers.
- **MySQL** -- DSN discovery from config. Services use `qmysql` driver as in production.
- **Tracing** -- Reads OpenTracing JSON spans from service FIFOs. Stored in-memory, streamed to Web UI in real-time.
- **Metrics** -- Scrapes Prometheus endpoints from services every 5s. Displayed in Web UI.
- **Logs** -- Captures stdout/stderr from managed services. Terminal-like view in Web UI.
- **Auto-rebuild** -- Watches Go source files, rebuilds and restarts on change.
- **MCP server** -- Model Context Protocol server for AI-assisted development (stdio and HTTP transports).
- **Web UI** -- Vue 3 + MDB Vue dashboard with WebSocket real-time updates.
- **Live config** -- Add databases, buckets, mailboxes at runtime via MCP or Web UI without restart.

## Two Modes

### Managed mode (`autostart = true`)
qdevrunner builds the Go binary, discovers transports, starts the service, captures logs, and manages the lifecycle. Start/stop/restart from Web UI or MCP.

### Manual mode (`autostart = false`)
qdevrunner builds the binary (for transport discovery), creates all sockets and FIFOs, but does not start the process. Prints the exact `go run` command for the developer to run (e.g., with a debugger attached). Tracing still works when the service connects.

## Build

```bash
cd qdevrunner

# Build frontend
cd static && npm install && npm run build && cd ..

# Build and install
go generate ./webui/
go install .
```

## Usage

```bash
qdevrunner                      # Uses qdevrunner.ini (created if missing)
qdevrunner -config my.ini       # Custom config path
qdevrunner -addr :9090          # Override Web UI address
qdevrunner --stdio              # Serve MCP over stdin/stdout (for Claude Code)
```

## Configuration

Configuration is optional — qdevrunner creates a default `qdevrunner.ini` on first run. All config can also be managed at runtime via MCP tools or the Web UI.

```ini
[project]
name = myproject
env = dev

[webui]
addr = 127.0.0.1:8090
tracebuffersize = 10000
logbuffersize = 50000

[dfs]
addr = /tmp/qdevrunner/dfs.sock

[bucket "mybucket"]
path = /tmp/dfs-mybucket

[email]
addr = /tmp/qdevrunner/email.sock

[mailbox "dev@localhost"]
smtp = localhost:2587
smtppassword = password
imap = localhost:1993
imappassword = password

[database "example_db"]
type = mysql
dsn = root:pass@tcp(127.0.0.1:3306)/example_db

[envs]
MY_VAR = my_value

[service "api-gateway"]
workdir = /home/user/projects/api-gateway
autostart = true
watch = true
httpport = 8080
```

### Config sections

| Section | Description |
|---|---|
| `[project]` | Project name and environment. |
| `[webui]` | Web UI address and buffer sizes. |
| `[bucket "name"]` | Maps a DFS bucket to a local directory. |
| `[mailbox "addr"]` | Configures a mailbox with SMTP/IMAP. |
| `[database "name"]` | Maps a database name to a DSN. Supports `type` field (default: `mysql`). |
| `[envs]` | Environment variables injected as `QSOA_*`. |
| `[service "name"]` | Configures a service instance. |

### Service fields

| Field | Description |
|---|---|
| `workdir` | Path to the service source directory (must contain Go code). |
| `autostart` | Start automatically on launch (true/false). |
| `watch` | Auto-rebuild on file change (true/false). |
| `httpport` | TCP port for HTTP proxy to the service's Unix socket. |

qdevrunner automatically builds the Go binary (`go build`) from the workdir and discovers which transports the service uses by inspecting its `-q_*` command-line flags.

## Web UI

Open http://127.0.0.1:8090 (default address).

- **Dashboard** -- Service cards with status, start/stop/restart, HTTP links, run commands for manual mode.
- **Traces** -- Trace list grouped by trace ID. Click to see the span tree with progress bars, tags, and logs.
- **Logs** -- Live stdout/stderr from managed services. Historical backlog loaded on page open.
- **Metrics** -- Prometheus metrics per service.
- **Resources** -- Configured databases, DFS buckets, and mailboxes.
- **MCP Setup** -- Connection URLs and copy-paste configs for Claude Code, Claude Desktop, Cursor, Codex CLI.

All pages receive historical data via WebSocket backlog when opened — no data is missed.

## MCP Server

qdevrunner includes a built-in [Model Context Protocol](https://modelcontextprotocol.io/) server. It lets AI coding assistants observe and manage the local development environment.

### Two Transports

**Stdio** (recommended for Claude Code):
```bash
claude mcp add -s user qdevrunner -- qdevrunner --stdio
```
Claude Code starts qdevrunner automatically. The Web UI and services run in the background.

**HTTP** (for other clients):
The MCP endpoint is `http://<webui-addr>/mcp/` (Streamable HTTP). The MCP Setup page in the Web UI shows copy-paste configs for each client.

### Tools

**Observability:**
- `get_config` -- Project configuration
- `list_services` -- All services with status, mode, PID, transports
- `list_traces` -- Recent distributed traces
- `get_trace` -- All spans for a trace ID
- `get_logs` -- Recent log lines, optionally filtered by service
- `get_metrics` -- Latest Prometheus metrics, optionally filtered by service

**Service management:**
- `start_service`, `stop_service`, `restart_service` -- Lifecycle control
- `get_service_command` -- Shell command to run a service manually

**Config management** (live — changes take effect immediately, persisted to INI):
- `add_database`, `remove_database` -- Manage MySQL database connections
- `add_bucket`, `remove_bucket` -- Manage DFS buckets
- `add_mailbox`, `remove_mailbox` -- Manage email mailboxes
- `add_env`, `remove_env` -- Manage environment variables
- `add_service`, `remove_service`, `update_service` -- Manage services

### Resources

- `qdevrunner://guide` -- Architecture guide, service library reference, and examples
- `qdevrunner://config` -- Live project configuration (JSON)
- `qdevrunner://services` -- Live service status (JSON)

## Architecture

```
qdevrunner binary
├── Instance Manager (os/exec, auto-build, transport discovery)
├── Per-Instance Transports (Unix sockets)
│   ├── gRPC Proxy (grpc_runner.sock ↔ grpc.sock)
│   ├── MySQL DSN Discovery (mysql.sock)
│   ├── DFS (dfs.sock)
│   ├── Email (email.sock)
│   ├── OpenTracing (tracer.fifo)
│   └── Prometheus Scraper (prometheus.sock)
├── HTTP TCP Proxy (per-service, configurable port)
├── In-Memory Stores (ring buffers)
│   ├── Traces, Metrics, Logs
├── MCP Server (stdio + Streamable HTTP)
├── WebSocket Hub (real-time push + backlog on connect)
├── REST API
└── Embedded Vue 3 Frontend
```
