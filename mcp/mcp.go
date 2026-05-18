package mcp

import (
	"net/http"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"gopkg.qsoa.cloud/qdevrunner/instance"
	"gopkg.qsoa.cloud/qdevrunner/logstore"
	"gopkg.qsoa.cloud/qdevrunner/metricsstore"
	"gopkg.qsoa.cloud/qdevrunner/tracer"
)

// BucketManager manages DFS buckets at runtime.
type BucketManager interface {
	AddBucket(name, path string)
	RemoveBucket(name string)
}

// DatabaseManager manages MySQL DSNs at runtime.
type DatabaseManager interface {
	AddDsn(name, dsn string)
	RemoveDsn(name string)
}

// Deps holds all dependencies the MCP handlers need.
type Deps struct {
	Instances       map[string]*instance.Instance
	TracerStore     *tracer.Store
	LogStore        *logstore.Store
	MetricsStore    *metricsstore.Store
	Config          ConfigAccessor
	ConfigPath      string
	BucketManager   BucketManager   // nil if DFS not configured
	DatabaseManager DatabaseManager // nil if no MySQL transports running
}

// DatabaseInfo describes a configured database.
type DatabaseInfo struct {
	Type string `json:"type"`
	Dsn  string `json:"dsn"`
}

// ConfigAccessor provides access to config for MCP handlers.
// Implemented by *config in main package.
type ConfigAccessor interface {
	Save(path string) error
	GetProjectName() string
	GetProjectEnv() string
	GetDatabases() map[string]DatabaseInfo
	GetBuckets() map[string]string
	GetMailboxes() []string
	GetEnvVars() map[string]string
	AddDatabase(name, dbType, dsn string)
	RemoveDatabase(name string)
	AddBucket(name, path string)
	RemoveBucket(name string)
	AddMailbox(address, smtp, smtpPassword, imap_, imapPassword string)
	RemoveMailbox(address string)
	AddEnv(name, value string)
	RemoveEnv(name string)
	AddService(name string, svc interface{})
	RemoveService(name string)
}

// NewMCPServer creates the shared MCP server with all tools and resources.
func NewMCPServer(deps *Deps) *server.MCPServer {
	s := server.NewMCPServer(
		"qdevrunner",
		"1.0.0",
		server.WithToolCapabilities(true),
		server.WithResourceCapabilities(true, false),
	)

	registerTools(s, deps)
	registerResources(s, deps)

	return s
}

// NewHandler creates the MCP StreamableHTTP handler to mount at /mcp/.
func NewHandler(deps *Deps) http.Handler {
	s := NewMCPServer(deps)

	httpServer := server.NewStreamableHTTPServer(s,
		server.WithEndpointPath("/mcp"),
	)

	return httpServer
}

func registerTools(s *server.MCPServer, deps *Deps) {
	// Observability tools.
	s.AddTool(mcp.NewTool("get_config",
		mcp.WithDescription("Get project configuration: databases, DFS buckets, mailboxes, environment variables, and services"),
	), makeGetConfig(deps))

	s.AddTool(mcp.NewTool("list_services",
		mcp.WithDescription("List all services with their status, mode, PID, transports, and HTTP port"),
	), makeListServices(deps))

	s.AddTool(mcp.NewTool("list_traces",
		mcp.WithDescription("List recent distributed traces across all services"),
		mcp.WithNumber("offset", mcp.Description("Number of traces to skip (default 0)")),
		mcp.WithNumber("limit", mcp.Description("Maximum traces to return (default 50)")),
	), makeListTraces(deps))

	s.AddTool(mcp.NewTool("get_trace",
		mcp.WithDescription("Get all spans for a specific trace by trace ID"),
		mcp.WithNumber("trace_id", mcp.Required(), mcp.Description("The trace ID")),
	), makeGetTrace(deps))

	s.AddTool(mcp.NewTool("get_logs",
		mcp.WithDescription("Get recent log lines from services (stdout/stderr)"),
		mcp.WithString("service", mcp.Description("Filter by service name (empty for all)")),
		mcp.WithNumber("lines", mcp.Description("Number of lines to return (default 100)")),
	), makeGetLogs(deps))

	s.AddTool(mcp.NewTool("get_metrics",
		mcp.WithDescription("Get latest Prometheus metrics snapshot"),
		mcp.WithString("service", mcp.Description("Filter by service name (empty for all)")),
	), makeGetMetrics(deps))

	// Service management tools.
	s.AddTool(mcp.NewTool("start_service",
		mcp.WithDescription("Start a stopped service"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Service name")),
	), makeStartService(deps))

	s.AddTool(mcp.NewTool("stop_service",
		mcp.WithDescription("Stop a running service"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Service name")),
	), makeStopService(deps))

	s.AddTool(mcp.NewTool("restart_service",
		mcp.WithDescription("Restart a service (stop then start)"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Service name")),
	), makeRestartService(deps))

	s.AddTool(mcp.NewTool("get_service_command",
		mcp.WithDescription("Get the shell command to run a service manually (for debugging with IDE/delve)"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Service name")),
	), makeGetServiceCommand(deps))

	// Config management tools.
	registerConfigTools(s, deps)
}

func registerResources(s *server.MCPServer, deps *Deps) {
	s.AddResource(mcp.NewResource(
		"qdevrunner://guide",
		"qdevrunner Guide",
		mcp.WithResourceDescription("Comprehensive guide on how to use qdevrunner: setup from scratch, manage services, configure resources, read traces/logs/metrics, debug services"),
		mcp.WithMIMEType("text/markdown"),
	), makeGuideResource())

	s.AddResource(mcp.NewResource(
		"qdevrunner://config",
		"Live Configuration",
		mcp.WithResourceDescription("Current project configuration with databases, buckets, mailboxes, and services"),
		mcp.WithMIMEType("application/json"),
	), makeConfigResource(deps))

	s.AddResource(mcp.NewResource(
		"qdevrunner://services",
		"Live Service Status",
		mcp.WithResourceDescription("Current status of all configured services"),
		mcp.WithMIMEType("application/json"),
	), makeServicesResource(deps))
}
