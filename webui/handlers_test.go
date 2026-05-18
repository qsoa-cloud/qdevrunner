package webui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gopkg.qsoa.cloud/qdevrunner/instance"
	"gopkg.qsoa.cloud/qdevrunner/logstore"
	mcppkg "gopkg.qsoa.cloud/qdevrunner/mcp"
	"gopkg.qsoa.cloud/qdevrunner/metricsstore"
	"gopkg.qsoa.cloud/qdevrunner/tracer"
)

type mockConfigAccessor struct {
	databases map[string]string
	buckets   map[string]string
	mailboxes map[string]bool
	envVars   map[string]string
	saved     bool
}

func newMockConfigAccessor() *mockConfigAccessor {
	return &mockConfigAccessor{
		databases: make(map[string]string),
		buckets:   make(map[string]string),
		mailboxes: make(map[string]bool),
		envVars:   make(map[string]string),
	}
}

func (m *mockConfigAccessor) Save(path string) error          { m.saved = true; return nil }
func (m *mockConfigAccessor) GetProjectName() string           { return "test" }
func (m *mockConfigAccessor) GetProjectEnv() string            { return "dev" }
func (m *mockConfigAccessor) GetDatabases() map[string]mcppkg.DatabaseInfo {
	result := make(map[string]mcppkg.DatabaseInfo, len(m.databases))
	for k, v := range m.databases {
		result[k] = mcppkg.DatabaseInfo{Type: "mysql", Dsn: v}
	}
	return result
}
func (m *mockConfigAccessor) GetBuckets() map[string]string    { return m.buckets }
func (m *mockConfigAccessor) GetEnvVars() map[string]string    { return m.envVars }
func (m *mockConfigAccessor) AddDatabase(n, t, d string)       { m.databases[n] = d }
func (m *mockConfigAccessor) RemoveDatabase(n string)          { delete(m.databases, n) }
func (m *mockConfigAccessor) AddBucket(n, p string)            { m.buckets[n] = p }
func (m *mockConfigAccessor) RemoveBucket(n string)            { delete(m.buckets, n) }
func (m *mockConfigAccessor) AddEnv(n, v string)               { m.envVars[n] = v }
func (m *mockConfigAccessor) RemoveEnv(n string)               { delete(m.envVars, n) }
func (m *mockConfigAccessor) AddService(n string, s interface{}) {}
func (m *mockConfigAccessor) RemoveService(n string)              {}
func (m *mockConfigAccessor) GetMailboxes() []string {
	r := make([]string, 0)
	for k := range m.mailboxes {
		r = append(r, k)
	}
	return r
}
func (m *mockConfigAccessor) AddMailbox(a, s, sp, i, ip string) { m.mailboxes[a] = true }
func (m *mockConfigAccessor) RemoveMailbox(a string)            { delete(m.mailboxes, a) }

func testServer() *Server {
	ts := tracer.NewStore(100)
	ls := logstore.NewStore(100)
	ms := metricsstore.NewStore(100)
	mockCfg := newMockConfigAccessor()

	return &Server{
		addr:         "127.0.0.1:0",
		hub:          NewHub(ts, ls, ms),
		instances:    make(map[string]*instance.Instance),
		tracerStore:  ts,
		logStore:     ls,
		metricsStore: ms,
		cfg: &UIConfig{
			ProjectName: "test",
			ProjectEnv:  "dev",
			Databases:   map[string]mcppkg.DatabaseInfo{},
			Buckets:     map[string]string{},
			Mailboxes:   []string{},
		},
		mcpDeps: &mcppkg.Deps{
			Config:     mockCfg,
			ConfigPath: "/tmp/test.ini",
		},
	}
}

func TestHandleConfig(t *testing.T) {
	s := testServer()

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	w := httptest.NewRecorder()
	s.handleConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result map[string]interface{}
	json.NewDecoder(w.Body).Decode(&result)
	if result["project_name"] != "test" {
		t.Errorf("expected test, got %v", result["project_name"])
	}
}

func TestHandleServices(t *testing.T) {
	s := testServer()

	req := httptest.NewRequest(http.MethodGet, "/api/services", nil)
	w := httptest.NewRecorder()
	s.handleServices(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHandleTraces(t *testing.T) {
	s := testServer()

	req := httptest.NewRequest(http.MethodGet, "/api/traces?limit=10", nil)
	w := httptest.NewRecorder()
	s.handleTraces(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHandleTraceNotFound(t *testing.T) {
	s := testServer()

	req := httptest.NewRequest(http.MethodGet, "/api/traces/12345", nil)
	w := httptest.NewRecorder()
	s.handleTrace(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleTraceInvalidID(t *testing.T) {
	s := testServer()

	req := httptest.NewRequest(http.MethodGet, "/api/traces/abc", nil)
	w := httptest.NewRecorder()
	s.handleTrace(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleServiceActionNotFound(t *testing.T) {
	s := testServer()

	req := httptest.NewRequest(http.MethodGet, "/api/services/nonexistent", nil)
	w := httptest.NewRecorder()
	s.handleServiceAction(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleConfigManagementAddDatabase(t *testing.T) {
	s := testServer()

	body := `{"name":"testdb","dsn":"root@localhost/testdb"}`
	req := httptest.NewRequest(http.MethodPost, "/api/config/databases/add", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleConfigManagement(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	cfg := s.mcpDeps.Config.(*mockConfigAccessor)
	if cfg.databases["testdb"] != "root@localhost/testdb" {
		t.Error("database not added")
	}
	if !cfg.saved {
		t.Error("config not saved")
	}
}

func TestHandleConfigManagementRemoveDatabase(t *testing.T) {
	s := testServer()
	cfg := s.mcpDeps.Config.(*mockConfigAccessor)
	cfg.databases["old"] = "dsn"

	body := `{"name":"old"}`
	req := httptest.NewRequest(http.MethodPost, "/api/config/databases/remove", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleConfigManagement(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if _, ok := cfg.databases["old"]; ok {
		t.Error("database should be removed")
	}
}

func TestHandleConfigManagementAddBucket(t *testing.T) {
	s := testServer()

	body := `{"name":"uploads","path":"/tmp/uploads"}`
	req := httptest.NewRequest(http.MethodPost, "/api/config/buckets/add", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleConfigManagement(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	cfg := s.mcpDeps.Config.(*mockConfigAccessor)
	if cfg.buckets["uploads"] != "/tmp/uploads" {
		t.Error("bucket not added")
	}
}

func TestHandleConfigManagementAddMailbox(t *testing.T) {
	s := testServer()

	body := `{"address":"dev@test.com","smtp":"localhost:587"}`
	req := httptest.NewRequest(http.MethodPost, "/api/config/mailboxes/add", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleConfigManagement(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHandleConfigManagementAddEnv(t *testing.T) {
	s := testServer()

	body := `{"name":"KEY","value":"VAL"}`
	req := httptest.NewRequest(http.MethodPost, "/api/config/envs/add", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleConfigManagement(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	cfg := s.mcpDeps.Config.(*mockConfigAccessor)
	if cfg.envVars["KEY"] != "VAL" {
		t.Error("env not added")
	}
}

func TestHandleConfigManagementBadMethod(t *testing.T) {
	s := testServer()

	req := httptest.NewRequest(http.MethodGet, "/api/config/databases/add", nil)
	w := httptest.NewRecorder()
	s.handleConfigManagement(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestHandleConfigManagementBadResource(t *testing.T) {
	s := testServer()

	req := httptest.NewRequest(http.MethodPost, "/api/config/unknown/add", nil)
	w := httptest.NewRecorder()
	s.handleConfigManagement(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleConfigManagementMissingAction(t *testing.T) {
	s := testServer()

	req := httptest.NewRequest(http.MethodPost, "/api/config/databases", nil)
	w := httptest.NewRecorder()
	s.handleConfigManagement(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
