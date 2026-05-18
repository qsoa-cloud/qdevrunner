package webui

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"gopkg.qsoa.cloud/qdevrunner/instance"
	mcppkg "gopkg.qsoa.cloud/qdevrunner/mcp"
)

func (s *Server) writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(v)
}

func (s *Server) writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

type serviceInfo struct {
	Name       string                `json:"name"`
	Status     instance.Status       `json:"status"`
	Mode       string                `json:"mode"` // "managed" or "manual"
	Pid        int                   `json:"pid,omitempty"`
	Started    string                `json:"started,omitempty"`
	Error      string                `json:"error,omitempty"`
	Transports []string              `json:"transports"`
	Httpport   int                   `json:"httpport,omitempty"`
	RunCommand string                `json:"run_command,omitempty"`
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		return
	}

	type configResp struct {
		ProjectName string                          `json:"project_name"`
		ProjectEnv  string                          `json:"project_env"`
		Databases   map[string]mcppkg.DatabaseInfo   `json:"databases"`
		Buckets     map[string]string               `json:"buckets"`
		Mailboxes   []string                        `json:"mailboxes"`
		EnvVars     map[string]string               `json:"env_vars"`
		Services    []string                        `json:"services"`
	}

	services := make([]string, 0, len(s.instances))
	for name := range s.instances {
		services = append(services, name)
	}

	s.writeJSON(w, configResp{
		ProjectName: s.mcpDeps.Config.GetProjectName(),
		ProjectEnv:  s.mcpDeps.Config.GetProjectEnv(),
		Databases:   s.mcpDeps.Config.GetDatabases(),
		Buckets:     s.mcpDeps.Config.GetBuckets(),
		Mailboxes:   s.mcpDeps.Config.GetMailboxes(),
		EnvVars:     s.mcpDeps.Config.GetEnvVars(),
		Services:    services,
	})
}

func (s *Server) handleServices(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		return
	}

	names := make([]string, 0, len(s.instances))
	for name := range s.instances {
		names = append(names, name)
	}
	sort.Strings(names)

	infos := make([]serviceInfo, 0, len(s.instances))
	for _, name := range names {
		inst := s.instances[name]
		cfg := inst.GetConfig()
		info := serviceInfo{
			Name:       cfg.Name,
			Status:     inst.GetStatus(),
			Transports: inst.GetTransports(),
			Httpport:   cfg.Httpport,
		}

		if cfg.Autostart {
			info.Mode = "managed"
		} else {
			info.Mode = "manual"
			info.RunCommand = inst.GetRunCommand()
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

	s.writeJSON(w, infos)
}

func (s *Server) handleServiceAction(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		return
	}

	// /api/services/{name}/{action}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/services/"), "/")
	if len(parts) < 1 {
		s.writeError(w, http.StatusBadRequest, "missing service name")
		return
	}

	name := parts[0]
	inst, ok := s.instances[name]
	if !ok {
		s.writeError(w, http.StatusNotFound, "service not found")
		return
	}

	if len(parts) == 1 {
		// GET /api/services/{name} — single service info.
		cfg := inst.GetConfig()
		info := serviceInfo{
			Name:       cfg.Name,
			Status:     inst.GetStatus(),
			Transports: inst.GetTransports(),
			Httpport:   cfg.Httpport,
		}
		if !cfg.Autostart {
			info.Mode = "manual"
			info.RunCommand = inst.GetRunCommand()
		} else {
			info.Mode = "managed"
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
		s.writeJSON(w, info)
		return
	}

	action := parts[1]
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "POST required for actions")
		return
	}

	switch action {
	case "start":
		if err := inst.Start(); err != nil {
			s.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.writeJSON(w, map[string]string{"status": "started"})

	case "stop":
		inst.Stop()
		s.writeJSON(w, map[string]string{"status": "stopped"})

	case "restart":
		if err := inst.Restart(); err != nil {
			s.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.writeJSON(w, map[string]string{"status": "restarted"})

	case "command":
		s.writeJSON(w, map[string]string{"command": inst.GetRunCommand()})

	default:
		s.writeError(w, http.StatusBadRequest, "unknown action")
	}
}

// Config management endpoints.

func (s *Server) handleConfigManagement(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		return
	}
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	// /api/config/{resource}/{action}
	path := strings.TrimPrefix(r.URL.Path, "/api/config/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 {
		s.writeError(w, http.StatusBadRequest, "missing action")
		return
	}

	resource, action := parts[0], parts[1]
	var body map[string]interface{}
	json.NewDecoder(r.Body).Decode(&body)

	getString := func(key string) string {
		v, _ := body[key].(string)
		return v
	}

	switch resource {
	case "databases":
		switch action {
		case "add":
			name, dsn := getString("name"), getString("dsn")
			if name == "" || dsn == "" {
				s.writeError(w, http.StatusBadRequest, "name and dsn required")
				return
			}
			dbType := getString("type")
			if dbType == "" {
				dbType = "mysql"
			}
			s.mcpDeps.Config.AddDatabase(name, dbType, dsn)
			if s.mcpDeps.DatabaseManager != nil && dbType == "mysql" {
				s.mcpDeps.DatabaseManager.AddDsn(name, dsn)
			}
		case "remove":
			name := getString("name")
			s.mcpDeps.Config.RemoveDatabase(name)
			if s.mcpDeps.DatabaseManager != nil {
				s.mcpDeps.DatabaseManager.RemoveDsn(name)
			}
		default:
			s.writeError(w, http.StatusBadRequest, "unknown action")
			return
		}

	case "buckets":
		switch action {
		case "add":
			name, path := getString("name"), getString("path")
			if name == "" || path == "" {
				s.writeError(w, http.StatusBadRequest, "name and path required")
				return
			}
			s.mcpDeps.Config.AddBucket(name, path)
			if s.mcpDeps.BucketManager != nil {
				s.mcpDeps.BucketManager.AddBucket(name, path)
			}
		case "remove":
			bname := getString("name")
			s.mcpDeps.Config.RemoveBucket(bname)
			if s.mcpDeps.BucketManager != nil {
				s.mcpDeps.BucketManager.RemoveBucket(bname)
			}
		default:
			s.writeError(w, http.StatusBadRequest, "unknown action")
			return
		}

	case "mailboxes":
		switch action {
		case "add":
			addr := getString("address")
			if addr == "" {
				s.writeError(w, http.StatusBadRequest, "address required")
				return
			}
			s.mcpDeps.Config.AddMailbox(addr, getString("smtp"), getString("smtp_password"), getString("imap"), getString("imap_password"))
		case "remove":
			s.mcpDeps.Config.RemoveMailbox(getString("address"))
		default:
			s.writeError(w, http.StatusBadRequest, "unknown action")
			return
		}

	case "envs":
		switch action {
		case "add":
			name, value := getString("name"), getString("value")
			if name == "" {
				s.writeError(w, http.StatusBadRequest, "name required")
				return
			}
			s.mcpDeps.Config.AddEnv(name, value)
		case "remove":
			s.mcpDeps.Config.RemoveEnv(getString("name"))
		default:
			s.writeError(w, http.StatusBadRequest, "unknown action")
			return
		}

	case "services":
		switch action {
		case "add":
			name := getString("name")
			workdir := getString("workdir")
			if name == "" || workdir == "" {
				s.writeError(w, http.StatusBadRequest, "name and workdir required")
				return
			}
			getBool := func(key string) bool {
				v, _ := body[key].(bool)
				return v
			}
			getInt := func(key string) int {
				v, _ := body[key].(float64)
				return int(v)
			}
			svc := &struct {
				Workdir   string
				Autostart bool
				Watch     bool
				Httpport  int
			}{
				Workdir:   workdir,
				Autostart: getBool("autostart"),
				Watch:     getBool("watch"),
				Httpport:  getInt("httpport"),
			}
			s.mcpDeps.Config.AddService(name, svc)
		case "remove":
			name := getString("name")
			if inst, ok := s.instances[name]; ok {
				inst.Stop()
			}
			s.mcpDeps.Config.RemoveService(name)
		default:
			s.writeError(w, http.StatusBadRequest, "unknown action")
			return
		}

	default:
		s.writeError(w, http.StatusBadRequest, "unknown resource")
		return
	}

	if err := s.mcpDeps.Config.Save(s.mcpDeps.ConfigPath); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, map[string]string{"status": "ok"})
}

func (s *Server) handleTraces(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		return
	}

	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}

	traces := s.tracerStore.ListTraces(offset, limit)
	s.writeJSON(w, traces)
}

func (s *Server) handleTrace(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		return
	}

	// /api/traces/{traceID}
	idStr := strings.TrimPrefix(r.URL.Path, "/api/traces/")
	traceID, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid trace ID")
		return
	}

	spans := s.tracerStore.GetTrace(traceID)
	if spans == nil {
		s.writeError(w, http.StatusNotFound, "trace not found")
		return
	}

	s.writeJSON(w, spans)
}
