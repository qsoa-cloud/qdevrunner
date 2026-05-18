package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

type serviceInfo struct {
	Name       string   `json:"name"`
	Status     string   `json:"status"`
	Mode       string   `json:"mode"`
	Pid        int      `json:"pid,omitempty"`
	Started    string   `json:"started,omitempty"`
	Error      string   `json:"error,omitempty"`
	Transports []string `json:"transports"`
	Httpport   int      `json:"httpport,omitempty"`
}

func jsonResult(v interface{}) (*mcp.CallToolResult, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return mcp.NewToolResultText(string(data)), nil
}

func makeGetConfig(deps *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result := map[string]interface{}{
			"project_name": deps.Config.GetProjectName(),
			"project_env":  deps.Config.GetProjectEnv(),
			"databases":    deps.Config.GetDatabases(),
			"buckets":      deps.Config.GetBuckets(),
			"mailboxes":    deps.Config.GetMailboxes(),
			"env_vars":     deps.Config.GetEnvVars(),
		}

		services := make([]string, 0, len(deps.Instances))
		for name := range deps.Instances {
			services = append(services, name)
		}
		result["services"] = services

		return jsonResult(result)
	}
}

func makeListServices(deps *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var infos []serviceInfo
		for _, inst := range deps.Instances {
			cfg := inst.GetConfig()
			info := serviceInfo{
				Name:       cfg.Name,
				Status:     string(inst.GetStatus()),
				Transports: inst.GetTransports(),
				Httpport:   cfg.Httpport,
			}
			if cfg.Autostart {
				info.Mode = "managed"
			} else {
				info.Mode = "manual"
			}
			if pid := inst.GetPid(); pid > 0 {
				info.Pid = pid
			}
			if started := inst.GetStarted(); !started.IsZero() {
				info.Started = started.Format("2006-01-02T15:04:05Z07:00")
			}
			if err := inst.GetLastError(); err != "" {
				info.Error = err
			}
			infos = append(infos, info)
		}
		return jsonResult(infos)
	}
}

func makeListTraces(deps *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		offset := 0
		limit := 50
		if v, err := req.RequireFloat("offset"); err == nil {
			offset = int(v)
		}
		if v, err := req.RequireFloat("limit"); err == nil {
			limit = int(v)
		}

		traces := deps.TracerStore.ListTraces(offset, limit)
		return jsonResult(traces)
	}
}

func makeGetTrace(deps *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		traceID, err := req.RequireFloat("trace_id")
		if err != nil {
			return mcp.NewToolResultError("trace_id is required"), nil
		}

		spans := deps.TracerStore.GetTrace(uint64(traceID))
		if spans == nil {
			return mcp.NewToolResultError(fmt.Sprintf("trace %d not found", uint64(traceID))), nil
		}
		return jsonResult(spans)
	}
}

func makeGetLogs(deps *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		service := ""
		if v, ok := req.GetArguments()["service"].(string); ok {
			service = v
		}
		lines := 100
		if v, err := req.RequireFloat("lines"); err == nil {
			lines = int(v)
		}

		entries := deps.LogStore.Recent(lines, service)
		return jsonResult(entries)
	}
}

func makeGetMetrics(deps *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		service := ""
		if v, ok := req.GetArguments()["service"].(string); ok {
			service = v
		}

		snapshots := deps.MetricsStore.Recent(10, service)
		return jsonResult(snapshots)
	}
}

func makeStartService(deps *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := req.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError("name is required"), nil
		}
		inst, ok := deps.Instances[name]
		if !ok {
			return mcp.NewToolResultError(fmt.Sprintf("service %q not found", name)), nil
		}
		if err := inst.Start(); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Service %q started", name)), nil
	}
}

func makeStopService(deps *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := req.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError("name is required"), nil
		}
		inst, ok := deps.Instances[name]
		if !ok {
			return mcp.NewToolResultError(fmt.Sprintf("service %q not found", name)), nil
		}
		inst.Stop()
		return mcp.NewToolResultText(fmt.Sprintf("Service %q stopped", name)), nil
	}
}

func makeRestartService(deps *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := req.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError("name is required"), nil
		}
		inst, ok := deps.Instances[name]
		if !ok {
			return mcp.NewToolResultError(fmt.Sprintf("service %q not found", name)), nil
		}
		if err := inst.Restart(); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Service %q restarted", name)), nil
	}
}

func makeGetServiceCommand(deps *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := req.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError("name is required"), nil
		}
		inst, ok := deps.Instances[name]
		if !ok {
			return mcp.NewToolResultError(fmt.Sprintf("service %q not found", name)), nil
		}
		return mcp.NewToolResultText(inst.GetRunCommand()), nil
	}
}
