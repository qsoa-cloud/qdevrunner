package mcp

import (
	"context"
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"
)

func makeGuideResource() func(context.Context, mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	return func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      "qdevrunner://guide",
				MIMEType: "text/markdown",
				Text:     guideText,
			},
		}, nil
	}
}

func makeConfigResource(deps *Deps) func(context.Context, mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	return func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
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

		data, _ := json.MarshalIndent(result, "", "  ")
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      "qdevrunner://config",
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	}
}

func makeServicesResource(deps *Deps) func(context.Context, mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	return func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
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

		data, _ := json.MarshalIndent(infos, "", "  ")
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      "qdevrunner://services",
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	}
}
