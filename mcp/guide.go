package mcp

const guideText = `# qdevrunner — Local Development Runner for qSOA Cloud

## What is qdevrunner?

qdevrunner is a local development tool that provides the full qSOA Cloud service runtime:
service mesh (gRPC inter-service discovery), MySQL DSN discovery, DFS (distributed file system),
email, distributed tracing, Prometheus metrics, and auto-rebuild on code changes.

One qdevrunner instance = one project. All services within the project discover and call each other
through qdevrunner's service mesh, just like in production.

## qSOA Cloud Architecture

A **project** is a collection of microservices that work together. Each service is a standalone Go
binary built with the qSOA service library. The runner (qdevrunner locally, qc-runner in production)
manages service lifecycles, provides the service mesh, and proxies all infrastructure:

- **gRPC Service Mesh** — services call each other via ` + "`" + `qcloud://service-name/` + "`" + ` URIs. The runner
  proxies gRPC calls between services, handles discovery, and injects trace context.
- **MySQL** — services open databases by name via ` + "`" + `sql.Open("qmysql", "db_name")` + "`" + `. The runner
  resolves the name to a DSN from its config.
- **DFS (Distributed File System)** — services access file buckets via ` + "`" + `qdfs.GetFs("bucket")` + "`" + `.
  The runner serves files from local directories (dev) or the DFS cluster (production).
- **Email** — services send/receive email via ` + "`" + `qemail.GetMailbox("addr")` + "`" + `. The runner proxies
  to SMTP/IMAP servers.
- **Distributed Tracing** — all gRPC and HTTP calls are automatically traced (OpenTracing).
  Traces flow through the runner and are visible in the Web UI.
- **Prometheus Metrics** — automatically exposed by the service library and scraped by the runner.

Services do NOT connect to infrastructure directly. All connections go through the runner's
Unix sockets, which are passed as command-line flags (` + "`" + `-q_*` + "`" + ` flags, managed by qdevrunner).

## Building Services — qSOA Service Library

**All services MUST be built using the qSOA service library.** This library handles flag parsing,
transport setup, tracing, metrics, graceful shutdown, and integration with the runner.

Module: ` + "`" + `gopkg.qsoa.cloud/service` + "`" + `

### Core Package (gopkg.qsoa.cloud/service)

` + "```" + `go
import "gopkg.qsoa.cloud/service"

// Register services, then call Run() — it never returns.
service.Run()

// Access runtime context:
service.GetProject()  // project name
service.GetEnv()      // environment name
service.GetService()  // service name
service.GetVersion()  // service version
` + "```" + `

### qgrpc — gRPC Server & Client (gopkg.qsoa.cloud/service/qgrpc)

**Server** — register gRPC service implementations:
` + "```" + `go
import "gopkg.qsoa.cloud/service/qgrpc"
import "myservice/pb"

pb.RegisterMyServiceServer(qgrpc.GetServer(), &myImpl{})
` + "```" + `

**Client** — call other services in the mesh:
` + "```" + `go
conn, err := qgrpc.Dial("qcloud://other-service/")
client := pb.NewOtherServiceClient(conn)
` + "```" + `

gRPC reflection is enabled. Unary and streaming interceptors inject distributed tracing automatically.

**Proto generation** — use standard Go protobuf and gRPC plugins:
` + "```" + `bash
protoc --go_out=. --go-grpc_out=. myservice.proto
` + "```" + `

### qhttp — HTTP Server (gopkg.qsoa.cloud/service/qhttp)

` + "```" + `go
import "gopkg.qsoa.cloud/service/qhttp"

qhttp.Handle("/api/", myHandler)
` + "```" + `

Standard http.Handler interface. Tracing is injected automatically for incoming requests.
If httpport is set in qdevrunner config, the runner creates a TCP proxy to the service's HTTP socket.

### qmysql — MySQL Database (gopkg.qsoa.cloud/service/qmysql)

` + "```" + `go
import "database/sql"
import _ "gopkg.qsoa.cloud/service/qmysql"  // blank import registers the driver

db, err := sql.Open("qmysql", "mydb")  // "mydb" is the database name from qdevrunner config
` + "```" + `

Uses the standard database/sql interface. The driver contacts the runner to resolve the database name
to a DSN, then delegates to the standard MySQL driver. Connection pooling works normally.

**Gotcha:** the driver does NOT set ` + "`" + `parseTime=true` + "`" + ` on the DSN. Scanning a MySQL ` + "`" + `TIMESTAMP` + "`" + ` /
` + "`" + `DATETIME` + "`" + ` column directly into ` + "`" + `*time.Time` + "`" + ` fails with ` + "`" + `unsupported Scan, storing
driver.Value type []uint8 into type *time.Time` + "`" + `. Workarounds: scan into ` + "`" + `string` + "`" + ` and parse, or
` + "`" + `SELECT UNIX_TIMESTAMP(col)` + "`" + ` into ` + "`" + `int64` + "`" + `.

### qdfs — Distributed File System (gopkg.qsoa.cloud/service/qdfs)

` + "```" + `go
import "gopkg.qsoa.cloud/service/qdfs"

fs, err := qdfs.GetFs("mybucket")  // bucket name from qdevrunner config
` + "```" + `

**Dfs methods:**
- ` + "`" + `OpenFile(ctx, name, flag) (*DfsFile, error)` + "`" + ` — open file (os.O_RDONLY, os.O_WRONLY|os.O_CREATE, etc.)
- ` + "`" + `Mkdir(ctx, name) error` + "`" + ` — create directory
- ` + "`" + `Stat(ctx, name) (os.FileInfo, error)` + "`" + ` — file info
- ` + "`" + `RemoveAll(ctx, name) error` + "`" + ` — remove file or directory tree
- ` + "`" + `Rename(ctx, oldName, newName) error` + "`" + ` — rename/move

**DfsFile** implements io.ReadWriter with Read, Write, Seek, Close, Readdir, Stat.

### qemail — Email Service (gopkg.qsoa.cloud/service/qemail)

` + "```" + `go
import "gopkg.qsoa.cloud/service/qemail"

mb, err := qemail.GetMailbox("noreply@example.com")  // address from qdevrunner config
` + "```" + `

**Mailbox methods:**
- ` + "`" + `Send(ctx, Message) (messageID string, err)` + "`" + ` — send email
- ` + "`" + `ListMessages(ctx, folder, offset, limit) ([]MessageSummary, total, err)` + "`" + ` — list inbox
- ` + "`" + `GetMessage(ctx, uid, folder) (*FullMessage, err)` + "`" + ` — get full message
- ` + "`" + `DeleteMessage(ctx, uid, folder) error` + "`" + ` — delete message
- ` + "`" + `MoveMessage(ctx, uid, toFolder) error` + "`" + ` — move between folders

**Message struct:** FromName, To[], Cc[], Bcc[], Subject, TextBody, HtmlBody, Attachments[], Headers[]

### Complete Service Example

` + "```" + `go
package main

import (
    "database/sql"
    "log"
    "net/http"

    "gopkg.qsoa.cloud/service"
    "gopkg.qsoa.cloud/service/qgrpc"
    "gopkg.qsoa.cloud/service/qhttp"
    _ "gopkg.qsoa.cloud/service/qmysql"

    "myservice/pb"
)

func main() {
    // Connect to another service via gRPC mesh
    conn, err := qgrpc.Dial("qcloud://users/")
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()
    usersClient := pb.NewUsersClient(conn)

    // Open database
    db, err := sql.Open("qmysql", "mydb")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    // Register HTTP handler
    qhttp.Handle("/", &myHandler{users: usersClient, db: db})

    // Register gRPC server
    pb.RegisterMyServiceServer(qgrpc.GetServer(), &myGrpcServer{db: db})

    // Run (blocks forever, handles graceful shutdown)
    service.Run()
}

type myHandler struct {
    users pb.UsersClient
    db    *sql.DB
}

func (h *myHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte("OK"))
}

type myGrpcServer struct {
    pb.UnimplementedMyServiceServer
    db *sql.DB
}
` + "```" + `

### Project Layout

` + "```" + `
myservice/
├── main.go           # Entry point: register services, call service.Run()
├── go.mod            # module myservice
├── pb/
│   ├── myservice.proto
│   ├── myservice.pb.go
│   └── myservice_grpc.pb.go
├── handler/          # HTTP handlers (optional)
│   └── handler.go
└── server/           # gRPC server implementation (optional)
    └── server.go
` + "```" + `

The go.mod must include:
` + "```" + `
require gopkg.qsoa.cloud/service v1.0.0
` + "```" + `

Set GOPRIVATE=gopkg.qsoa.cloud/* when running go mod tidy.

## Quick Start from Scratch

### With Claude Code (recommended)

One-time setup:
  claude mcp add -s user qdevrunner -- qdevrunner --stdio

Then just ask Claude to create a new qSOA Cloud project. Claude will have full access
to this guide, all MCP tools, and can scaffold the entire project.

### Manual setup

1. Run qdevrunner in your project directory (no config file needed):
  qdevrunner

2. Open http://127.0.0.1:8090 for the Web UI.

3. Add resources and services using MCP tools or the Web UI.

A default qdevrunner.ini is created automatically with sensible defaults
(project "default", env "dev", webui on 127.0.0.1:8090).

## Adding Resources

### Databases
Use add_database tool with name and DSN:
  add_database(name="mydb", dsn="root:password@tcp(127.0.0.1:3306)/mydb")

Services access databases via the qmysql driver:
  db, _ := sql.Open("qmysql", "mydb")

### DFS Buckets
Use add_bucket tool with name and local directory path:
  add_bucket(name="uploads", path="/tmp/dfs-uploads")

The directory is created automatically. Services access buckets via:
  fs, _ := qdfs.GetFs("uploads")

### Mailboxes
Use add_mailbox tool with address and SMTP/IMAP credentials:
  add_mailbox(address="dev@localhost", smtp="localhost:2587", smtp_password="pass",
              imap="localhost:1993", imap_password="pass")

Services access mailboxes via:
  mb, _ := qemail.GetMailbox("dev@localhost")

### Environment Variables
Use add_env tool. Variables are injected with QSOA_ prefix:
  add_env(name="API_KEY", value="secret123")
  → Service sees QSOA_API_KEY=secret123

## Adding Services

Use add_service tool — only the service directory is needed, qdevrunner handles the rest:
  add_service(name="api-gateway", workdir="/home/user/projects/api-gateway",
              autostart=true, watch=true, httpport=8080)

qdevrunner automatically:
1. Builds the Go binary (go build) from the workdir
2. Discovers which transports the service uses by inspecting its -q_* flags
3. Creates the appropriate Unix sockets and starts the transport proxies

### Service Modes

**Managed mode** (autostart=true): qdevrunner builds, starts, stops, and restarts the service.
Stdout/stderr are captured and visible in the Logs page and via get_logs tool.
Auto-rebuild happens when Go source files change (if watch=true).

**Manual mode** (autostart=false): qdevrunner builds the binary (for transport discovery),
creates all sockets and FIFOs, but does NOT start the process. Use get_service_command to get
the exact command to run manually (e.g., with a debugger or IDE).

### Transports (Auto-Discovered)

Transports are discovered automatically from the built binary. Each transport creates a Unix
socket that the service connects to:

- **grpc** — Service mesh. Services call other services via qcloud://service-name/.
  Creates grpc_runner.sock (outbound proxy) and grpc.sock (inbound listener).
  Discovered when service imports gopkg.qsoa.cloud/service/qgrpc.

- **http** — HTTP server. Service listens on http.sock.
  If httpport is set, qdevrunner creates a TCP proxy (e.g., 127.0.0.1:8080 → http.sock).

- **mysql** — MySQL DSN discovery. Service calls sql.Open("qmysql", "db_name").
  Discovered when service imports gopkg.qsoa.cloud/service/qmysql.

- **dfs** — Distributed file system. Service calls qdfs.GetFs("bucket").
  Discovered when service imports gopkg.qsoa.cloud/service/qdfs.

- **email** — Email service. Service calls qemail.GetMailbox("addr").
  Discovered when service imports gopkg.qsoa.cloud/service/qemail.

- **tracer** — OpenTracing spans. Always available when using the service library.

- **prometheus** — Prometheus metrics. Always available when using the service library.

### Socket Layout

Per-service sockets are created at:
  /tmp/qdevrunner/<project>/<service>/misc/  — runner-side sockets (grpc_runner.sock, mysql.sock, dfs.sock, email.sock, tracer.fifo)
  /tmp/qdevrunner/<project>/<service>/tmp/   — service-side sockets (grpc.sock, http.sock, prometheus.sock)

## Observability

### Traces
- list_traces(limit=20) — see recent request traces across all services
- get_trace(trace_id=...) — see all spans in a trace (waterfall view)
- Each span shows: operation, service, duration, tags, log fields

### Logs
- get_logs(service="api-gateway", lines=50) — recent stdout/stderr from a service
- Logs include stream type (stdout/stderr/build) and timestamps

### Metrics
- get_metrics(service="api-gateway") — latest Prometheus metrics snapshot
- Includes counters, gauges, summaries with labels

## Debugging Workflow

1. Check service status: list_services — is the service running? Any errors?
2. Read logs: get_logs(service="...") — look for startup errors or panics
3. Check traces: list_traces — find the failing request
4. Inspect trace: get_trace(trace_id=...) — which service/operation failed?
5. If service won't start: get_service_command to get the manual run command,
   then the developer can run it with a debugger

## Managing Services at Runtime

- start_service(name="...") — start a stopped service
- stop_service(name="...") — stop a running service (SIGINT, then SIGKILL after 5s)
- restart_service(name="...") — stop then start

## Config Management

All config changes are saved to the INI file automatically:
- add_database / remove_database
- add_bucket / remove_bucket
- add_mailbox / remove_mailbox
- add_env / remove_env
- add_service / remove_service / update_service

## Available MCP Tools Reference

### Observability
- get_config — project configuration
- list_services — all services with status
- list_traces — recent traces (params: offset, limit)
- get_trace — spans for a trace (params: trace_id)
- get_logs — recent logs (params: service, lines)
- get_metrics — latest metrics (params: service)

### Service Management
- start_service — start a service (params: name)
- stop_service — stop a service (params: name)
- restart_service — restart a service (params: name)
- get_service_command — get manual run command (params: name)

### Config Management
- add_service — add and optionally start a service
- remove_service — stop and remove a service
- update_service — update service configuration
- add_database / remove_database — manage MySQL databases
- add_bucket / remove_bucket — manage DFS buckets
- add_mailbox / remove_mailbox — manage email mailboxes
- add_env / remove_env — manage environment variables
`
