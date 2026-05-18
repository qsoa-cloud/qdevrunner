package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"gopkg.qsoa.cloud/qdevrunner/instance"
	"gopkg.qsoa.cloud/qdevrunner/logstore"
	"gopkg.qsoa.cloud/qdevrunner/metricsstore"
	"gopkg.qsoa.cloud/qdevrunner/tracer"
)

// mockConfig implements ConfigAccessor for testing.
type mockConfig struct {
	projectName string
	projectEnv  string
	databases   map[string]string
	buckets     map[string]string
	mailboxes   map[string]bool
	envVars     map[string]string
	savePath    string
}

func newMockConfig() *mockConfig {
	return &mockConfig{
		projectName: "testproject",
		projectEnv:  "dev",
		databases:   make(map[string]string),
		buckets:     make(map[string]string),
		mailboxes:   make(map[string]bool),
		envVars:     make(map[string]string),
	}
}

func (m *mockConfig) Save(path string) error            { m.savePath = path; return nil }
func (m *mockConfig) GetProjectName() string             { return m.projectName }
func (m *mockConfig) GetProjectEnv() string              { return m.projectEnv }
func (m *mockConfig) GetDatabases() map[string]DatabaseInfo {
	result := make(map[string]DatabaseInfo, len(m.databases))
	for k, v := range m.databases {
		result[k] = DatabaseInfo{Type: "mysql", Dsn: v}
	}
	return result
}
func (m *mockConfig) GetBuckets() map[string]string      { return m.buckets }
func (m *mockConfig) GetEnvVars() map[string]string      { return m.envVars }
func (m *mockConfig) AddDatabase(name, dbType, dsn string) { m.databases[name] = dbType + ":" + dsn }
func (m *mockConfig) RemoveDatabase(name string)         { delete(m.databases, name) }
func (m *mockConfig) AddBucket(name, path string)        { m.buckets[name] = path }
func (m *mockConfig) RemoveBucket(name string)           { delete(m.buckets, name) }
func (m *mockConfig) AddEnv(name, value string)          { m.envVars[name] = value }
func (m *mockConfig) RemoveEnv(name string)              { delete(m.envVars, name) }
func (m *mockConfig) AddService(name string, svc interface{}) {}
func (m *mockConfig) RemoveService(name string)              {}
func (m *mockConfig) GetMailboxes() []string {
	result := make([]string, 0, len(m.mailboxes))
	for k := range m.mailboxes {
		result = append(result, k)
	}
	return result
}
func (m *mockConfig) AddMailbox(addr, smtp, sp, imap_, ip string) { m.mailboxes[addr] = true }
func (m *mockConfig) RemoveMailbox(addr string)                   { delete(m.mailboxes, addr) }

func testDeps() *Deps {
	ts := tracer.NewStore(100)
	ls := logstore.NewStore(100)
	ms := metricsstore.NewStore(100)

	return &Deps{
		Instances:    make(map[string]*instance.Instance),
		TracerStore:  ts,
		LogStore:     ls,
		MetricsStore: ms,
		Config:       newMockConfig(),
		ConfigPath:   "/tmp/test.ini",
	}
}

func callTool(t *testing.T, handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error), args map[string]interface{}) *mcp.CallToolResult {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("tool error: %v", err)
	}
	return result
}

func getTextContent(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("no content")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	return tc.Text
}

func TestGetConfig(t *testing.T) {
	deps := testDeps()
	handler := makeGetConfig(deps)

	result := callTool(t, handler, nil)
	text := getTextContent(t, result)

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if data["project_name"] != "testproject" {
		t.Errorf("expected testproject, got %v", data["project_name"])
	}
}

func TestListServicesEmpty(t *testing.T) {
	deps := testDeps()
	handler := makeListServices(deps)

	result := callTool(t, handler, nil)
	text := getTextContent(t, result)

	if text != "null" && text != "[]" {
		var services []serviceInfo
		if err := json.Unmarshal([]byte(text), &services); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if len(services) != 0 {
			t.Errorf("expected 0 services, got %d", len(services))
		}
	}
}

func TestListTraces(t *testing.T) {
	deps := testDeps()
	now := time.Now()
	deps.TracerStore.Add(tracer.SpanRecord{
		Service:    "svc-a",
		ReceivedAt: now,
		Span: tracer.Span{
			Operation:  "GET /users",
			StartTime:  now,
			FinishTime: now.Add(5 * time.Millisecond),
			Ctx:        tracer.SpanContext{TraceID: 123, SpanID: 1},
		},
	})

	handler := makeListTraces(deps)
	result := callTool(t, handler, map[string]interface{}{"limit": float64(10)})
	text := getTextContent(t, result)

	var traces []tracer.TraceInfo
	if err := json.Unmarshal([]byte(text), &traces); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(traces) != 1 {
		t.Fatalf("expected 1 trace, got %d", len(traces))
	}
	if traces[0].TraceID != 123 {
		t.Errorf("expected trace 123, got %d", traces[0].TraceID)
	}
}

func TestGetTrace(t *testing.T) {
	deps := testDeps()
	now := time.Now()
	deps.TracerStore.Add(tracer.SpanRecord{
		Service:    "svc-a",
		ReceivedAt: now,
		Span: tracer.Span{
			Operation: "op",
			Ctx:       tracer.SpanContext{TraceID: 42, SpanID: 1},
		},
	})

	handler := makeGetTrace(deps)
	result := callTool(t, handler, map[string]interface{}{"trace_id": float64(42)})
	text := getTextContent(t, result)

	var spans []tracer.SpanRecord
	if err := json.Unmarshal([]byte(text), &spans); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
}

func TestGetTraceNotFound(t *testing.T) {
	deps := testDeps()
	handler := makeGetTrace(deps)

	result := callTool(t, handler, map[string]interface{}{"trace_id": float64(999)})
	if !result.IsError {
		t.Error("expected error for missing trace")
	}
}

func TestGetLogs(t *testing.T) {
	deps := testDeps()
	deps.LogStore.Add(logstore.LogEntry{Service: "svc-a", Stream: "stdout", Text: "hello", Timestamp: time.Now()})
	deps.LogStore.Add(logstore.LogEntry{Service: "svc-b", Stream: "stderr", Text: "world", Timestamp: time.Now()})

	handler := makeGetLogs(deps)

	// All logs.
	result := callTool(t, handler, map[string]interface{}{"lines": float64(10)})
	text := getTextContent(t, result)

	var entries []logstore.LogEntry
	json.Unmarshal([]byte(text), &entries)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	// Filtered by service.
	result = callTool(t, handler, map[string]interface{}{"service": "svc-a", "lines": float64(10)})
	text = getTextContent(t, result)
	json.Unmarshal([]byte(text), &entries)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry for svc-a, got %d", len(entries))
	}
}

func TestGetMetrics(t *testing.T) {
	deps := testDeps()
	deps.MetricsStore.Add(metricsstore.Snapshot{
		Service:   "svc-a",
		Timestamp: time.Now(),
		Metrics:   []metricsstore.MetricValue{{Name: "req_total", Type: metricsstore.MetricCounter, Value: 42}},
	})

	handler := makeGetMetrics(deps)
	result := callTool(t, handler, map[string]interface{}{"service": "svc-a"})
	text := getTextContent(t, result)

	var snapshots []metricsstore.Snapshot
	json.Unmarshal([]byte(text), &snapshots)
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snapshots))
	}
}

func TestAddRemoveDatabaseTool(t *testing.T) {
	deps := testDeps()
	cfg := deps.Config.(*mockConfig)
	deps.ConfigPath = filepath.Join(t.TempDir(), "test.ini")

	addHandler := makeAddDatabase(deps)
	callTool(t, addHandler, map[string]interface{}{"name": "mydb", "dsn": "root@localhost/mydb"})

	if cfg.databases["mydb"] == "" {
		t.Error("database not added")
	}

	removeHandler := makeRemoveDatabase(deps)
	callTool(t, removeHandler, map[string]interface{}{"name": "mydb"})

	if _, ok := cfg.databases["mydb"]; ok {
		t.Error("database not removed")
	}
}

func TestAddRemoveBucketTool(t *testing.T) {
	deps := testDeps()
	cfg := deps.Config.(*mockConfig)
	deps.ConfigPath = filepath.Join(t.TempDir(), "test.ini")

	addHandler := makeAddBucket(deps)
	callTool(t, addHandler, map[string]interface{}{"name": "uploads", "path": "/tmp/uploads"})

	if cfg.buckets["uploads"] != "/tmp/uploads" {
		t.Error("bucket not added")
	}

	removeHandler := makeRemoveBucket(deps)
	callTool(t, removeHandler, map[string]interface{}{"name": "uploads"})

	if _, ok := cfg.buckets["uploads"]; ok {
		t.Error("bucket not removed")
	}
}

func TestAddRemoveMailboxTool(t *testing.T) {
	deps := testDeps()
	cfg := deps.Config.(*mockConfig)
	deps.ConfigPath = filepath.Join(t.TempDir(), "test.ini")

	addHandler := makeAddMailbox(deps)
	callTool(t, addHandler, map[string]interface{}{"address": "dev@test.com", "smtp": "localhost:587"})

	if !cfg.mailboxes["dev@test.com"] {
		t.Error("mailbox not added")
	}

	removeHandler := makeRemoveMailbox(deps)
	callTool(t, removeHandler, map[string]interface{}{"address": "dev@test.com"})

	if cfg.mailboxes["dev@test.com"] {
		t.Error("mailbox not removed")
	}
}

func TestAddRemoveEnvTool(t *testing.T) {
	deps := testDeps()
	cfg := deps.Config.(*mockConfig)
	deps.ConfigPath = filepath.Join(t.TempDir(), "test.ini")

	addHandler := makeAddEnv(deps)
	callTool(t, addHandler, map[string]interface{}{"name": "KEY", "value": "VAL"})

	if cfg.envVars["KEY"] != "VAL" {
		t.Error("env not added")
	}

	removeHandler := makeRemoveEnv(deps)
	callTool(t, removeHandler, map[string]interface{}{"name": "KEY"})

	if _, ok := cfg.envVars["KEY"]; ok {
		t.Error("env not removed")
	}
}

func TestGuideResource(t *testing.T) {
	handler := makeGuideResource()
	result, err := handler(context.Background(), mcp.ReadResourceRequest{})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 content, got %d", len(result))
	}
	tc := result[0].(mcp.TextResourceContents)
	if tc.URI != "qdevrunner://guide" {
		t.Errorf("expected qdevrunner://guide, got %s", tc.URI)
	}
	if len(tc.Text) < 100 {
		t.Error("guide text too short")
	}
}

func TestConfigResource(t *testing.T) {
	deps := testDeps()
	handler := makeConfigResource(deps)
	result, err := handler(context.Background(), mcp.ReadResourceRequest{})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	tc := result[0].(mcp.TextResourceContents)

	var data map[string]interface{}
	json.Unmarshal([]byte(tc.Text), &data)
	if data["project_name"] != "testproject" {
		t.Errorf("expected testproject, got %v", data["project_name"])
	}
}

func TestServicesResource(t *testing.T) {
	deps := testDeps()
	handler := makeServicesResource(deps)
	result, err := handler(context.Background(), mcp.ReadResourceRequest{})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	tc := result[0].(mcp.TextResourceContents)
	if tc.URI != "qdevrunner://services" {
		t.Errorf("expected qdevrunner://services, got %s", tc.URI)
	}
}

func TestNewHandler(t *testing.T) {
	deps := testDeps()
	handler := NewHandler(deps)
	if handler == nil {
		t.Fatal("NewHandler returned nil")
	}
}

func TestStartServiceNotFound(t *testing.T) {
	deps := testDeps()
	handler := makeStartService(deps)

	result := callTool(t, handler, map[string]interface{}{"name": "nonexistent"})
	if !result.IsError {
		t.Error("expected error for missing service")
	}
}

func TestStopServiceNotFound(t *testing.T) {
	deps := testDeps()
	handler := makeStopService(deps)

	result := callTool(t, handler, map[string]interface{}{"name": "nonexistent"})
	if !result.IsError {
		t.Error("expected error for missing service")
	}
}

func TestGetServiceCommandNotFound(t *testing.T) {
	deps := testDeps()
	handler := makeGetServiceCommand(deps)

	result := callTool(t, handler, map[string]interface{}{"name": "nonexistent"})
	if !result.IsError {
		t.Error("expected error for missing service")
	}
}

func TestSaveCalledOnConfigChange(t *testing.T) {
	deps := testDeps()
	cfg := deps.Config.(*mockConfig)
	tmpDir := t.TempDir()
	deps.ConfigPath = filepath.Join(tmpDir, "test.ini")

	// Create the file so Save can write to it.
	os.WriteFile(deps.ConfigPath, []byte{}, 0644)

	addHandler := makeAddDatabase(deps)
	callTool(t, addHandler, map[string]interface{}{"name": "db", "dsn": "dsn"})

	if cfg.savePath != deps.ConfigPath {
		t.Errorf("expected Save called with %q, got %q", deps.ConfigPath, cfg.savePath)
	}
}
