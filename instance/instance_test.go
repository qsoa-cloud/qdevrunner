package instance

import (
	"strings"
	"testing"
)

func testConfig() Config {
	return Config{
		Name:       "test-svc",
		Workdir:    "/home/user/test-svc",
		Autostart:  true,
		Watch:      true,
		Httpport:   8080,
		Project:    "myproject",
		Env:        "dev",
		GetEnvVars: func() map[string]string { return map[string]string{"KEY": "VAL"} },
		Databases:  map[string]string{"db": "dsn"},
		BasePath:   "/tmp/qdevrunner",
	}
}

func TestNewInstance(t *testing.T) {
	cfg := testConfig()
	inst := New(cfg, nil, nil, nil, nil, nil, nil)

	if inst.GetName() != "test-svc" {
		t.Errorf("expected test-svc, got %s", inst.GetName())
	}
	if inst.GetStatus() != StatusStopped {
		t.Errorf("expected stopped, got %s", inst.GetStatus())
	}
	if inst.GetPid() != 0 {
		t.Errorf("expected pid 0, got %d", inst.GetPid())
	}
	if inst.GetLastError() != "" {
		t.Errorf("expected no error, got %q", inst.GetLastError())
	}
}

func TestHasTransport(t *testing.T) {
	cfg := testConfig()
	inst := New(cfg, nil, nil, nil, nil, nil, nil)
	// Simulate discovered transports.
	inst.transports = []string{"http", "grpc", "mysql"}

	if !inst.hasTransport("http") {
		t.Error("expected http transport")
	}
	if !inst.hasTransport("grpc") {
		t.Error("expected grpc transport")
	}
	if inst.hasTransport("nonexistent") {
		t.Error("unexpected nonexistent transport")
	}
}

func TestBuildArgs(t *testing.T) {
	cfg := testConfig()
	inst := New(cfg, nil, nil, nil, nil, nil, nil)
	// Simulate discovered transports.
	inst.transports = []string{"http", "grpc", "mysql", "dfs", "email", "tracer", "prometheus"}

	args := inst.buildArgs()

	// Check essential args.
	found := map[string]bool{}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-q_project=") {
			found["project"] = true
			if !strings.Contains(arg, "myproject") {
				t.Errorf("expected myproject in %s", arg)
			}
		}
		if strings.HasPrefix(arg, "-q_env=") {
			found["env"] = true
		}
		if strings.HasPrefix(arg, "-q_service=") {
			found["service"] = true
		}
		if strings.HasPrefix(arg, "-q_dfs_sock=") {
			found["dfs"] = true
			if !strings.HasPrefix(arg, "-q_dfs_sock=unix://") {
				t.Errorf("dfs sock should start with unix://, got %s", arg)
			}
		}
		if strings.HasPrefix(arg, "-q_grpc_proxy=") {
			found["grpc_proxy"] = true
		}
		if strings.HasPrefix(arg, "-q_grpc_addr=") {
			found["grpc_addr"] = true
		}
		if strings.HasPrefix(arg, "-q_mysql_addr=") {
			found["mysql"] = true
		}
		if strings.HasPrefix(arg, "-q_http_addr=") {
			found["http"] = true
		}
		if strings.HasPrefix(arg, "-q_tracer_file=") {
			found["tracer"] = true
		}
		if strings.HasPrefix(arg, "-q_metrics_addr=") {
			found["metrics"] = true
		}
	}

	for _, key := range []string{"project", "env", "service", "dfs", "grpc_proxy", "grpc_addr", "mysql", "http", "tracer", "metrics"} {
		if !found[key] {
			t.Errorf("missing arg %q in buildArgs output: %v", key, args)
		}
	}
}

func TestBuildArgsPartialTransports(t *testing.T) {
	cfg := Config{
		Name:     "minimal",
		Workdir:  "/home/user/minimal",
		Project:  "proj",
		Env:      "dev",
		BasePath: "/tmp/qdevrunner",
	}
	inst := New(cfg, nil, nil, nil, nil, nil, nil)
	// Simulate only grpc discovered.
	inst.transports = []string{"grpc"}

	args := inst.buildArgs()

	for _, arg := range args {
		if strings.HasPrefix(arg, "-q_http_addr=") {
			t.Error("http transport should not be in args")
		}
		if strings.HasPrefix(arg, "-q_dfs_sock=") {
			t.Error("dfs transport should not be in args")
		}
	}

	hasGrpc := false
	for _, arg := range args {
		if strings.HasPrefix(arg, "-q_grpc_proxy=") {
			hasGrpc = true
		}
	}
	if !hasGrpc {
		t.Error("expected grpc_proxy in args")
	}
}

func TestGetRunCommand(t *testing.T) {
	cfg := testConfig()
	inst := New(cfg, nil, nil, nil, nil, nil, nil)
	inst.transports = []string{"grpc"}

	cmd := inst.GetRunCommand()

	if !strings.HasPrefix(cmd, "cd /home/user/test-svc && go run .") {
		t.Errorf("unexpected command prefix: %s", cmd)
	}
	if !strings.Contains(cmd, "-q_project=myproject") {
		t.Error("expected -q_project in command")
	}
}

func TestGetConfig(t *testing.T) {
	cfg := testConfig()
	inst := New(cfg, nil, nil, nil, nil, nil, nil)

	got := inst.GetConfig()
	if got.Name != "test-svc" {
		t.Errorf("expected test-svc, got %s", got.Name)
	}
	if got.Httpport != 8080 {
		t.Errorf("expected 8080, got %d", got.Httpport)
	}
}

func TestGetGrpcToServiceAddr(t *testing.T) {
	cfg := testConfig()
	inst := New(cfg, nil, nil, nil, nil, nil, nil)

	addr := inst.GetGrpcToServiceAddr()
	if !strings.Contains(addr, "grpc.sock") {
		t.Errorf("expected grpc.sock in addr, got %s", addr)
	}
}

func TestGetHttpToServiceAddr(t *testing.T) {
	cfg := testConfig()
	inst := New(cfg, nil, nil, nil, nil, nil, nil)

	addr := inst.GetHttpToServiceAddr()
	if !strings.Contains(addr, "http.sock") {
		t.Errorf("expected http.sock in addr, got %s", addr)
	}
}
