package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestConfig() *config {
	cfg := &config{}
	cfg.Project.Name = "testproject"
	cfg.Project.Env = "dev"
	cfg.Webui.Addr = "127.0.0.1:9090"
	return cfg
}

func TestSaveAndReadBack(t *testing.T) {
	cfg := newTestConfig()
	cfg.AddDatabase("mydb", "mysql", "root:pass@tcp(localhost)/mydb")
	cfg.AddBucket("uploads", "/tmp/uploads")
	cfg.AddMailbox("dev@test.com", "smtp:587", "pass", "imap:993", "ipass")
	cfg.AddEnv("API_KEY", "secret")
	cfg.AddService("api", &serviceConfig{
		Workdir:   "/home/user/api",
		Autostart: true,
		Watch:     true,
		Httpport:  8080,
	})

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.ini")

	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	content := string(data)

	// Verify sections exist.
	for _, expected := range []string{
		"[project]",
		"name = testproject",
		"env = dev",
		`[database "mydb"]`,
		"type = mysql",
		"dsn = root:pass@tcp(localhost)/mydb",
		`[bucket "uploads"]`,
		"path = /tmp/uploads",
		`[mailbox "dev@test.com"]`,
		"smtp = smtp:587",
		"[envs]",
		"API_KEY = secret",
		`[service "api"]`,
		"workdir = /home/user/api",
		"autostart = true",
		"watch = true",
		"httpport = 8080",
	} {
		if !strings.Contains(content, expected) {
			t.Errorf("expected %q in output, got:\n%s", expected, content)
		}
	}
}

func TestAddRemoveDatabase(t *testing.T) {
	cfg := newTestConfig()

	cfg.AddDatabase("db1", "mysql", "dsn1")
	cfg.AddDatabase("db2", "mysql", "dsn2")

	dbs := cfg.GetDatabases()
	if len(dbs) != 2 {
		t.Fatalf("expected 2 databases, got %d", len(dbs))
	}
	if dbs["db1"].Dsn != "dsn1" {
		t.Errorf("expected dsn1, got %q", dbs["db1"].Dsn)
	}
	if dbs["db1"].Type != "mysql" {
		t.Errorf("expected mysql, got %q", dbs["db1"].Type)
	}

	cfg.RemoveDatabase("db1")
	dbs = cfg.GetDatabases()
	if len(dbs) != 1 {
		t.Fatalf("expected 1 database after remove, got %d", len(dbs))
	}
}

func TestAddRemoveBucket(t *testing.T) {
	cfg := newTestConfig()

	cfg.AddBucket("b1", "/tmp/b1")
	cfg.AddBucket("b2", "/tmp/b2")

	buckets := cfg.GetBuckets()
	if len(buckets) != 2 {
		t.Fatalf("expected 2 buckets, got %d", len(buckets))
	}

	cfg.RemoveBucket("b1")
	buckets = cfg.GetBuckets()
	if len(buckets) != 1 {
		t.Fatalf("expected 1 bucket after remove, got %d", len(buckets))
	}
}

func TestAddRemoveMailbox(t *testing.T) {
	cfg := newTestConfig()

	cfg.AddMailbox("a@b.com", "smtp", "sp", "imap", "ip")
	mailboxes := cfg.GetMailboxes()
	if len(mailboxes) != 1 {
		t.Fatalf("expected 1 mailbox, got %d", len(mailboxes))
	}

	cfg.RemoveMailbox("a@b.com")
	mailboxes = cfg.GetMailboxes()
	if len(mailboxes) != 0 {
		t.Fatalf("expected 0 mailboxes after remove, got %d", len(mailboxes))
	}
}

func TestAddRemoveEnv(t *testing.T) {
	cfg := newTestConfig()

	cfg.AddEnv("KEY", "VAL")
	envs := cfg.GetEnvVars()
	if envs["KEY"] != "VAL" {
		t.Errorf("expected VAL, got %q", envs["KEY"])
	}

	cfg.RemoveEnv("KEY")
	envs = cfg.GetEnvVars()
	if len(envs) != 0 {
		t.Fatalf("expected 0 envs after remove, got %d", len(envs))
	}
}

func TestEnvsRoundTrip(t *testing.T) {
	cfg := newTestConfig()
	cfg.AddEnv("API_KEY", "secret")
	cfg.AddEnv("DB_HOST", "localhost")

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.ini")

	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	parsed := &config{}
	if err := parsed.parseINI(data); err != nil {
		t.Fatalf("parseINI failed: %v", err)
	}

	if parsed.envs["API_KEY"] != "secret" {
		t.Errorf("expected secret, got %q", parsed.envs["API_KEY"])
	}
	if parsed.envs["DB_HOST"] != "localhost" {
		t.Errorf("expected localhost, got %q", parsed.envs["DB_HOST"])
	}
}

func TestAddRemoveService(t *testing.T) {
	cfg := newTestConfig()

	cfg.AddService("svc", &serviceConfig{Workdir: "/home/user/svc"})

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.ini")
	cfg.Save(path)

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `[service "svc"]`) {
		t.Error("expected service in saved config")
	}

	cfg.RemoveService("svc")
	cfg.Save(path)

	data, _ = os.ReadFile(path)
	if strings.Contains(string(data), `[service "svc"]`) {
		t.Error("service should be removed from saved config")
	}
}

func TestParseINI(t *testing.T) {
	ini := `
[project]
name = myproject
env = staging

[webui]
addr = 0.0.0.0:9090
tracebuffersize = 5000
logbuffersize = 20000

[dfs]
addr = /tmp/dfs.sock

[bucket "uploads"]
path = /tmp/uploads

[bucket "media"]
path = /tmp/media

[email]
addr = /tmp/email.sock

[mailbox "dev@test.com"]
smtp = localhost:587
smtppassword = s3cret
imap = localhost:993
imappassword = im4p

[database "maindb"]
type = mysql
dsn = root:pass@tcp(localhost)/maindb

[database "analytics"]
dsn = root:@tcp(localhost)/analytics

[envs]
API_KEY = secret123
DB_HOST = localhost

[service "api"]
workdir = /home/user/api
autostart = true
watch = true
httpport = 8080

[service "worker"]
workdir = /home/user/worker
autostart = false
`

	cfg := &config{}
	if err := cfg.parseINI([]byte(ini)); err != nil {
		t.Fatalf("parseINI failed: %v", err)
	}

	// Project.
	if cfg.Project.Name != "myproject" {
		t.Errorf("project name: expected myproject, got %q", cfg.Project.Name)
	}
	if cfg.Project.Env != "staging" {
		t.Errorf("project env: expected staging, got %q", cfg.Project.Env)
	}

	// Webui.
	if cfg.Webui.Addr != "0.0.0.0:9090" {
		t.Errorf("webui addr: expected 0.0.0.0:9090, got %q", cfg.Webui.Addr)
	}
	if cfg.Webui.TraceBufferSize != 5000 {
		t.Errorf("tracebuffersize: expected 5000, got %d", cfg.Webui.TraceBufferSize)
	}
	if cfg.Webui.LogBufferSize != 20000 {
		t.Errorf("logbuffersize: expected 20000, got %d", cfg.Webui.LogBufferSize)
	}

	// DFS.
	if cfg.Dfs.Addr != "/tmp/dfs.sock" {
		t.Errorf("dfs addr: expected /tmp/dfs.sock, got %q", cfg.Dfs.Addr)
	}

	// Buckets.
	if len(cfg.Bucket) != 2 {
		t.Fatalf("expected 2 buckets, got %d", len(cfg.Bucket))
	}
	if cfg.Bucket["uploads"].Path != "/tmp/uploads" {
		t.Errorf("bucket uploads: expected /tmp/uploads, got %q", cfg.Bucket["uploads"].Path)
	}
	if cfg.Bucket["media"].Path != "/tmp/media" {
		t.Errorf("bucket media: expected /tmp/media, got %q", cfg.Bucket["media"].Path)
	}

	// Email.
	if cfg.Email.Addr != "/tmp/email.sock" {
		t.Errorf("email addr: expected /tmp/email.sock, got %q", cfg.Email.Addr)
	}

	// Mailbox.
	if len(cfg.Mailbox) != 1 {
		t.Fatalf("expected 1 mailbox, got %d", len(cfg.Mailbox))
	}
	mb := cfg.Mailbox["dev@test.com"]
	if mb == nil {
		t.Fatal("mailbox dev@test.com not found")
	}
	if mb.Smtp != "localhost:587" {
		t.Errorf("mailbox smtp: expected localhost:587, got %q", mb.Smtp)
	}
	if mb.SmtpPassword != "s3cret" {
		t.Errorf("mailbox smtppassword: expected s3cret, got %q", mb.SmtpPassword)
	}
	if mb.Imap != "localhost:993" {
		t.Errorf("mailbox imap: expected localhost:993, got %q", mb.Imap)
	}
	if mb.ImapPassword != "im4p" {
		t.Errorf("mailbox imappassword: expected im4p, got %q", mb.ImapPassword)
	}

	// Databases.
	if len(cfg.Database) != 2 {
		t.Fatalf("expected 2 databases, got %d", len(cfg.Database))
	}
	if cfg.Database["maindb"].Type != "mysql" {
		t.Errorf("database maindb type: expected mysql, got %q", cfg.Database["maindb"].Type)
	}
	if cfg.Database["maindb"].Dsn != "root:pass@tcp(localhost)/maindb" {
		t.Errorf("database maindb dsn: expected root:pass@tcp(localhost)/maindb, got %q", cfg.Database["maindb"].Dsn)
	}
	if cfg.Database["analytics"].Dsn != "root:@tcp(localhost)/analytics" {
		t.Errorf("database analytics dsn: expected root:@tcp(localhost)/analytics, got %q", cfg.Database["analytics"].Dsn)
	}

	// Envs.
	if len(cfg.envs) != 2 {
		t.Fatalf("expected 2 envs, got %d", len(cfg.envs))
	}
	if cfg.envs["API_KEY"] != "secret123" {
		t.Errorf("env API_KEY: expected secret123, got %q", cfg.envs["API_KEY"])
	}
	if cfg.envs["DB_HOST"] != "localhost" {
		t.Errorf("env DB_HOST: expected localhost, got %q", cfg.envs["DB_HOST"])
	}

	// Services.
	if len(cfg.Service) != 2 {
		t.Fatalf("expected 2 services, got %d", len(cfg.Service))
	}
	api := cfg.Service["api"]
	if api.Workdir != "/home/user/api" {
		t.Errorf("service api workdir: expected /home/user/api, got %q", api.Workdir)
	}
	if !api.Autostart {
		t.Error("service api autostart: expected true")
	}
	if !api.Watch {
		t.Error("service api watch: expected true")
	}
	if api.Httpport != 8080 {
		t.Errorf("service api httpport: expected 8080, got %d", api.Httpport)
	}

	worker := cfg.Service["worker"]
	if worker.Workdir != "/home/user/worker" {
		t.Errorf("service worker workdir: expected /home/user/worker, got %q", worker.Workdir)
	}
	if worker.Autostart {
		t.Error("service worker autostart: expected false")
	}
	if worker.Watch {
		t.Error("service worker watch: expected false")
	}
	if worker.Httpport != 0 {
		t.Errorf("service worker httpport: expected 0, got %d", worker.Httpport)
	}
}

func TestParseINIDefaults(t *testing.T) {
	cfg := &config{}
	if err := cfg.parseINI([]byte("")); err != nil {
		t.Fatalf("parseINI failed on empty input: %v", err)
	}

	// Struct fields should be zero-valued (defaults are set by readConfig, not parseINI).
	if cfg.Project.Name != "" {
		t.Errorf("expected empty project name, got %q", cfg.Project.Name)
	}
	if len(cfg.Service) != 0 {
		t.Errorf("expected 0 services, got %d", len(cfg.Service))
	}
	if len(cfg.envs) != 0 {
		t.Errorf("expected 0 envs, got %d", len(cfg.envs))
	}
}

func TestParseINIComments(t *testing.T) {
	ini := `
# This is a comment
; This is also a comment
[project]
# comment inside section
name = myproject
; another comment
env = dev
`
	cfg := &config{}
	if err := cfg.parseINI([]byte(ini)); err != nil {
		t.Fatalf("parseINI failed: %v", err)
	}
	if cfg.Project.Name != "myproject" {
		t.Errorf("expected myproject, got %q", cfg.Project.Name)
	}
	if cfg.Project.Env != "dev" {
		t.Errorf("expected dev, got %q", cfg.Project.Env)
	}
}

func TestParseINIUnknownSection(t *testing.T) {
	ini := `
[project]
name = test

[unknown]
foo = bar

[webui]
addr = 127.0.0.1:8080
`
	cfg := &config{}
	// Should not crash or return error on unknown sections.
	if err := cfg.parseINI([]byte(ini)); err != nil {
		t.Fatalf("parseINI should not fail on unknown section: %v", err)
	}
	if cfg.Project.Name != "test" {
		t.Errorf("expected test, got %q", cfg.Project.Name)
	}
	if cfg.Webui.Addr != "127.0.0.1:8080" {
		t.Errorf("expected 127.0.0.1:8080, got %q", cfg.Webui.Addr)
	}
}

func TestSaveParseRoundTrip(t *testing.T) {
	// Populate config programmatically.
	orig := newTestConfig()
	orig.AddDatabase("mydb", "mysql", "root:pass@tcp(localhost)/mydb")
	orig.AddBucket("uploads", "/tmp/uploads")
	orig.AddMailbox("dev@test.com", "smtp:587", "pass", "imap:993", "ipass")
	orig.AddEnv("API_KEY", "secret")
	orig.AddEnv("MODE", "debug")
	orig.AddService("api", &serviceConfig{
		Workdir:   "/home/user/api",
		Autostart: true,
		Watch:     true,
		Httpport:  8080,
	})

	// Save to file.
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.ini")
	if err := orig.Save(path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Parse back.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	parsed := &config{}
	if err := parsed.parseINI(data); err != nil {
		t.Fatalf("parseINI failed: %v", err)
	}

	// Compare fields.
	if parsed.Project.Name != orig.Project.Name {
		t.Errorf("project name: expected %q, got %q", orig.Project.Name, parsed.Project.Name)
	}
	if parsed.Project.Env != orig.Project.Env {
		t.Errorf("project env: expected %q, got %q", orig.Project.Env, parsed.Project.Env)
	}
	if parsed.Webui.Addr != orig.Webui.Addr {
		t.Errorf("webui addr: expected %q, got %q", orig.Webui.Addr, parsed.Webui.Addr)
	}

	if len(parsed.Database) != 1 {
		t.Fatalf("expected 1 database, got %d", len(parsed.Database))
	}
	if parsed.Database["mydb"].Dsn != "root:pass@tcp(localhost)/mydb" {
		t.Errorf("database dsn mismatch: %q", parsed.Database["mydb"].Dsn)
	}

	if len(parsed.Bucket) != 1 {
		t.Fatalf("expected 1 bucket, got %d", len(parsed.Bucket))
	}
	if parsed.Bucket["uploads"].Path != "/tmp/uploads" {
		t.Errorf("bucket path mismatch: %q", parsed.Bucket["uploads"].Path)
	}

	if len(parsed.Mailbox) != 1 {
		t.Fatalf("expected 1 mailbox, got %d", len(parsed.Mailbox))
	}
	if parsed.Mailbox["dev@test.com"].Smtp != "smtp:587" {
		t.Errorf("mailbox smtp mismatch: %q", parsed.Mailbox["dev@test.com"].Smtp)
	}

	if len(parsed.envs) != 2 {
		t.Fatalf("expected 2 envs, got %d", len(parsed.envs))
	}
	if parsed.envs["API_KEY"] != "secret" {
		t.Errorf("env API_KEY: expected secret, got %q", parsed.envs["API_KEY"])
	}
	if parsed.envs["MODE"] != "debug" {
		t.Errorf("env MODE: expected debug, got %q", parsed.envs["MODE"])
	}

	api := parsed.Service["api"]
	if api == nil {
		t.Fatal("service api not found")
	}
	if api.Workdir != "/home/user/api" {
		t.Errorf("service workdir: expected /home/user/api, got %q", api.Workdir)
	}
	if !api.Autostart {
		t.Error("service autostart: expected true")
	}
	if !api.Watch {
		t.Error("service watch: expected true")
	}
	if api.Httpport != 8080 {
		t.Errorf("service httpport: expected 8080, got %d", api.Httpport)
	}
}

func TestGetters(t *testing.T) {
	cfg := newTestConfig()
	if cfg.GetProjectName() != "testproject" {
		t.Errorf("expected testproject, got %q", cfg.GetProjectName())
	}
	if cfg.GetProjectEnv() != "dev" {
		t.Errorf("expected dev, got %q", cfg.GetProjectEnv())
	}
}

