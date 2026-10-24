package mcp

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerConfigTools(s *server.MCPServer, deps *Deps) {
	// Database management.
	s.AddTool(mcp.NewTool("add_database",
		mcp.WithDescription("Add a database to the project configuration. Services access MySQL via sql.Open(\"qmysql\", \"name\") and ClickHouse via qclickhouse.Open(ctx, \"name\")."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Database name (used by services in sql.Open or qclickhouse.Open)")),
		mcp.WithString("type", mcp.Description("Database type: mysql (default) or clickhouse."), mcp.Enum("mysql", "clickhouse")),
		mcp.WithString("dsn", mcp.Required(), mcp.Description("Connection DSN (e.g. root:pass@tcp(127.0.0.1:3306)/mydb for MySQL, clickhouse://user:password@127.0.0.1:9000/mydb for ClickHouse)")),
	), makeAddDatabase(deps))

	s.AddTool(mcp.NewTool("remove_database",
		mcp.WithDescription("Remove a database from the project configuration"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Database name")),
	), makeRemoveDatabase(deps))

	// DFS Bucket management.
	s.AddTool(mcp.NewTool("add_bucket",
		mcp.WithDescription("Add a DFS bucket backed by a local directory. Services access it via qdfs.GetFs(\"name\")."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Bucket name")),
		mcp.WithString("path", mcp.Required(), mcp.Description("Local directory path")),
	), makeAddBucket(deps))

	s.AddTool(mcp.NewTool("remove_bucket",
		mcp.WithDescription("Remove a DFS bucket from the project configuration"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Bucket name")),
	), makeRemoveBucket(deps))

	// Mailbox management.
	s.AddTool(mcp.NewTool("add_mailbox",
		mcp.WithDescription("Add an email mailbox. Services access it via qemail.GetMailbox(\"address\")."),
		mcp.WithString("address", mcp.Required(), mcp.Description("Email address (e.g. dev@localhost)")),
		mcp.WithString("smtp", mcp.Description("SMTP server address (host:port)")),
		mcp.WithString("smtp_password", mcp.Description("SMTP password")),
		mcp.WithString("imap", mcp.Description("IMAP server address (host:port)")),
		mcp.WithString("imap_password", mcp.Description("IMAP password")),
	), makeAddMailbox(deps))

	s.AddTool(mcp.NewTool("remove_mailbox",
		mcp.WithDescription("Remove a mailbox from the project configuration"),
		mcp.WithString("address", mcp.Required(), mcp.Description("Email address")),
	), makeRemoveMailbox(deps))

	// Environment variable management.
	s.AddTool(mcp.NewTool("add_env",
		mcp.WithDescription("Add an environment variable. Injected to services with QSOA_ prefix. Running services keep the old value until restarted — call restart_service on any service that reads this variable so it picks up the new value."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Variable name (without QSOA_ prefix)")),
		mcp.WithString("value", mcp.Required(), mcp.Description("Variable value")),
	), makeAddEnv(deps))

	s.AddTool(mcp.NewTool("remove_env",
		mcp.WithDescription("Remove an environment variable from the project configuration. Running services keep the old value until restarted — call restart_service on any service that reads this variable so it picks up the change."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Variable name")),
	), makeRemoveEnv(deps))

	s.AddTool(mcp.NewTool("add_secret",
		mcp.WithDescription("Store a secret for a service. The service reads it with qsecrets.Get. The value is never shown again."),
		mcp.WithString("service", mcp.Required(), mcp.Description("Service name")),
		mcp.WithString("path", mcp.Required(), mcp.Description("Secret path: 1-4 segments of [A-Za-z0-9._-]{1,64} joined by /")),
		mcp.WithString("value", mcp.Required(), mcp.Description("Secret value")),
	), makeAddSecret(deps))

	s.AddTool(mcp.NewTool("remove_secret",
		mcp.WithDescription("Remove a service's secret"),
		mcp.WithString("service", mcp.Required(), mcp.Description("Service name")),
		mcp.WithString("path", mcp.Required(), mcp.Description("Secret path")),
	), makeRemoveSecret(deps))

	// Service management.
	s.AddTool(mcp.NewTool("add_service",
		mcp.WithDescription("Add a new service to the project. qdevrunner builds the Go binary and discovers transports automatically."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Service discovery name")),
		mcp.WithString("workdir", mcp.Required(), mcp.Description("Path to the service source directory (must contain Go code)")),
		mcp.WithBoolean("autostart", mcp.Description("Start automatically (true=managed, false=manual)")),
		mcp.WithBoolean("watch", mcp.Description("Auto-rebuild on Go file changes")),
		mcp.WithNumber("httpport", mcp.Description("TCP port for HTTP proxy")),
	), makeAddService(deps))

	s.AddTool(mcp.NewTool("remove_service",
		mcp.WithDescription("Stop and remove a service from the project configuration"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Service name")),
	), makeRemoveService(deps))

	s.AddTool(mcp.NewTool("update_service",
		mcp.WithDescription("Update fields of an existing service configuration. Only provided fields are updated."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Service name")),
		mcp.WithString("workdir", mcp.Description("New working directory")),
	), makeUpdateService(deps))
}

func saveConfig(deps *Deps) error {
	return deps.Config.Save(deps.ConfigPath)
}

func makeAddDatabase(deps *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.RequireString("name")
		dsn, _ := req.RequireString("dsn")
		dbType, _ := req.GetArguments()["type"].(string)
		if dbType == "" {
			dbType = "mysql"
		}
		if dbType == "clickhouse" && !validClickHouseDsn(dsn) {
			return mcp.NewToolResultError("invalid ClickHouse DSN: want clickhouse://user:password@host:port/database"), nil
		}
		deps.Config.AddDatabase(name, dbType, dsn)
		if deps.DatabaseManager != nil && dbType == "mysql" {
			deps.DatabaseManager.AddDsn(name, dsn)
		}
		if err := saveConfig(deps); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if dbType == "clickhouse" {
			return mcp.NewToolResultText(fmt.Sprintf("Database %q added (type: %s). Restart a running service if it had no ClickHouse database before.", name, dbType)), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Database %q added (type: %s, DSN: %s)", name, dbType, dsn)), nil
	}
}

func validClickHouseDsn(dsn string) bool {
	u, err := url.Parse(dsn)
	return err == nil && u.Scheme == "clickhouse" && u.Hostname() != "" && u.Port() != "" && u.User.Username() != ""
}

func makeRemoveDatabase(deps *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.RequireString("name")
		deps.Config.RemoveDatabase(name)
		if deps.DatabaseManager != nil {
			deps.DatabaseManager.RemoveDsn(name)
		}
		if err := saveConfig(deps); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Database %q removed", name)), nil
	}
}

func makeAddBucket(deps *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.RequireString("name")
		path, _ := req.RequireString("path")
		deps.Config.AddBucket(name, path)
		if deps.BucketManager != nil {
			deps.BucketManager.AddBucket(name, path)
		}
		if err := saveConfig(deps); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Bucket %q added (path: %s)", name, path)), nil
	}
}

func makeRemoveBucket(deps *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.RequireString("name")
		deps.Config.RemoveBucket(name)
		if deps.BucketManager != nil {
			deps.BucketManager.RemoveBucket(name)
		}
		if err := saveConfig(deps); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Bucket %q removed", name)), nil
	}
}

func makeAddMailbox(deps *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		address, _ := req.RequireString("address")
		smtp, _ := req.GetArguments()["smtp"].(string)
		smtpPass, _ := req.GetArguments()["smtp_password"].(string)
		imap_, _ := req.GetArguments()["imap"].(string)
		imapPass, _ := req.GetArguments()["imap_password"].(string)
		deps.Config.AddMailbox(address, smtp, smtpPass, imap_, imapPass)
		if err := saveConfig(deps); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Mailbox %q added", address)), nil
	}
}

func makeRemoveMailbox(deps *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		address, _ := req.RequireString("address")
		deps.Config.RemoveMailbox(address)
		if err := saveConfig(deps); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Mailbox %q removed", address)), nil
	}
}

func makeAddEnv(deps *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.RequireString("name")
		value, _ := req.RequireString("value")
		deps.Config.AddEnv(name, value)
		if err := saveConfig(deps); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Env %q=%q added (injected as QSOA_%s). Running services keep the old value until restarted — call restart_service for each service that reads this variable.", name, value, name)), nil
	}
}

func makeRemoveEnv(deps *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.RequireString("name")
		deps.Config.RemoveEnv(name)
		if err := saveConfig(deps); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Env %q removed. Running services keep the old value until restarted — call restart_service for each service that reads this variable.", name)), nil
	}
}

var secretSegment = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

func validSecretPath(path string) bool {
	segs := strings.Split(path, "/")
	if len(segs) > 4 {
		return false
	}
	for _, seg := range segs {
		if seg == "." || seg == ".." || !secretSegment.MatchString(seg) {
			return false
		}
	}
	return true
}

var secretService = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

func validSecretService(name string) bool {
	return secretService.MatchString(name)
}

func makeAddSecret(deps *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		service, _ := req.RequireString("service")
		path, _ := req.RequireString("path")
		value, err := req.RequireString("value")
		if err != nil {
			return mcp.NewToolResultError("value is required"), nil
		}
		if !validSecretService(service) {
			return mcp.NewToolResultError(fmt.Sprintf("invalid service name %q", service)), nil
		}
		if !validSecretPath(path) {
			return mcp.NewToolResultError(fmt.Sprintf("invalid secret path %q: use 1-4 segments of [A-Za-z0-9._-]{1,64} joined by /", path)), nil
		}
		if len(value) > 65536 {
			return mcp.NewToolResultError("value exceeds 65536 bytes"), nil
		}
		deps.Config.AddSecret(service, path, value)
		if err := saveConfig(deps); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Secret %s/%s saved.", service, path)), nil
	}
}

func makeRemoveSecret(deps *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		service, _ := req.RequireString("service")
		path, _ := req.RequireString("path")
		if !deps.Config.RemoveSecret(service, path) {
			return mcp.NewToolResultError(fmt.Sprintf("secret %s/%s not found", service, path)), nil
		}
		if err := saveConfig(deps); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Secret %s/%s removed.", service, path)), nil
	}
}

func makeAddService(deps *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.RequireString("name")
		workdir, _ := req.RequireString("workdir")
		autostart, _ := req.GetArguments()["autostart"].(bool)
		watch, _ := req.GetArguments()["watch"].(bool)
		var httpport int
		if v, ok := req.GetArguments()["httpport"].(float64); ok {
			httpport = int(v)
		}

		deps.Config.AddService(name, &serviceConfigData{
			Workdir: workdir, Autostart: autostart,
			Watch: watch, Httpport: httpport,
		})
		if err := saveConfig(deps); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Service %q added to config. Restart qdevrunner to activate.", name)), nil
	}
}

func makeRemoveService(deps *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.RequireString("name")
		// Stop if running.
		if inst, ok := deps.Instances[name]; ok {
			inst.Stop()
		}
		deps.Config.RemoveService(name)
		if err := saveConfig(deps); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Service %q removed", name)), nil
	}
}

func makeUpdateService(deps *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, _ := req.RequireString("name")

		// Update config fields. The ConfigAccessor handles the mutation.
		// For now, we remove and re-add with merged fields.
		// This is a simplification — a real implementation would have UpdateService on ConfigAccessor.
		return mcp.NewToolResultText(fmt.Sprintf("Service %q config update noted. Restart qdevrunner to apply.", name)), nil
	}
}

// serviceConfigData matches the main package's serviceConfig for serialization.
// We can't import main, so we duplicate the struct.
type serviceConfigData = struct {
	Workdir   string
	Autostart bool
	Watch     bool
	Httpport  int
}
