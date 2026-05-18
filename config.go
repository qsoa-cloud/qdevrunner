package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"gopkg.qsoa.cloud/qdevrunner/mcp"
)

var configPath = flag.String("config", "qdevrunner.ini", "Path to config file")
var addrFlag = flag.String("addr", "", "Web UI listen address (overrides config)")
var stdioFlag = flag.Bool("stdio", false, "Serve MCP over stdin/stdout (for Claude Code integration)")

type config struct {
	mu sync.RWMutex

	Project struct {
		Name string
		Env  string
	}

	Webui struct {
		Addr            string
		TraceBufferSize int
		LogBufferSize   int
	}

	Dfs struct {
		Addr string
	}

	Bucket map[string]*struct {
		Path string
	}

	Email struct {
		Addr string
	}

	Mailbox map[string]*struct {
		Smtp         string
		SmtpPassword string
		Imap         string
		ImapPassword string
	}

	Database map[string]*struct {
		Type string
		Dsn  string
	}

	Service map[string]*serviceConfig

	// envs is populated by parseEnvs(), not by gcfg (gcfg doesn't support arbitrary keys in a section).
	envs map[string]string
}

type serviceConfig struct {
	Workdir   string
	Autostart bool
	Watch     bool
	Httpport  int
}

func readConfig() *config {
	cfg := &config{}
	cfg.Project.Name = "default"
	cfg.Project.Env = "dev"
	cfg.Webui.Addr = "127.0.0.1:8090"
	cfg.Webui.TraceBufferSize = 10000
	cfg.Webui.LogBufferSize = 50000

	data, err := os.ReadFile(*configPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("Config file %s not found, starting with defaults", *configPath)
			if err := cfg.Save(*configPath); err != nil {
				log.Printf("Warning: could not save default config: %v", err)
			}
			return cfg
		}
		log.Fatal(err)
	}

	if err := cfg.parseINI(data); err != nil {
		log.Fatal(err)
	}

	return cfg
}

// parseINI parses INI data into the config struct.
func (c *config) parseINI(data []byte) error {
	var section, subsection string

	// Lazy-init targets for subsections.
	var curBucket *struct{ Path string }
	var curMailbox *struct {
		Smtp         string
		SmtpPassword string
		Imap         string
		ImapPassword string
	}
	var curDatabase *struct {
		Type string
		Dsn  string
	}
	var curService *serviceConfig

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}

		// Section header.
		if line[0] == '[' {
			end := strings.IndexByte(line, ']')
			if end < 0 {
				continue
			}
			header := line[1:end]

			// Reset subsection targets.
			curBucket = nil
			curMailbox = nil
			curDatabase = nil
			curService = nil

			// Check for [section "name"] format.
			if qi := strings.IndexByte(header, '"'); qi >= 0 {
				section = strings.TrimSpace(header[:qi])
				qe := strings.LastIndexByte(header, '"')
				if qe > qi {
					subsection = header[qi+1 : qe]
				}
			} else {
				section = strings.TrimSpace(header)
				subsection = ""
			}
			section = strings.ToLower(section)

			// Allocate subsection targets.
			switch section {
			case "bucket":
				if c.Bucket == nil {
					c.Bucket = make(map[string]*struct{ Path string })
				}
				curBucket = &struct{ Path string }{}
				c.Bucket[subsection] = curBucket
			case "mailbox":
				if c.Mailbox == nil {
					c.Mailbox = make(map[string]*struct {
						Smtp         string
						SmtpPassword string
						Imap         string
						ImapPassword string
					})
				}
				curMailbox = &struct {
					Smtp         string
					SmtpPassword string
					Imap         string
					ImapPassword string
				}{}
				c.Mailbox[subsection] = curMailbox
			case "database":
				if c.Database == nil {
					c.Database = make(map[string]*struct {
						Type string
						Dsn  string
					})
				}
				curDatabase = &struct {
					Type string
					Dsn  string
				}{}
				c.Database[subsection] = curDatabase
			case "service":
				if c.Service == nil {
					c.Service = make(map[string]*serviceConfig)
				}
				curService = &serviceConfig{}
				c.Service[subsection] = curService
			case "envs":
				if c.envs == nil {
					c.envs = make(map[string]string)
				}
			}
			continue
		}

		// Key = value.
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:idx]))
		value := strings.TrimSpace(line[idx+1:])

		switch section {
		case "project":
			switch key {
			case "name":
				c.Project.Name = value
			case "env":
				c.Project.Env = value
			}
		case "webui":
			switch key {
			case "addr":
				c.Webui.Addr = value
			case "tracebuffersize":
				c.Webui.TraceBufferSize, _ = strconv.Atoi(value)
			case "logbuffersize":
				c.Webui.LogBufferSize, _ = strconv.Atoi(value)
			}
		case "dfs":
			if key == "addr" {
				c.Dfs.Addr = value
			}
		case "email":
			if key == "addr" {
				c.Email.Addr = value
			}
		case "bucket":
			if curBucket != nil && key == "path" {
				curBucket.Path = value
			}
		case "mailbox":
			if curMailbox != nil {
				switch key {
				case "smtp":
					curMailbox.Smtp = value
				case "smtppassword":
					curMailbox.SmtpPassword = value
				case "imap":
					curMailbox.Imap = value
				case "imappassword":
					curMailbox.ImapPassword = value
				}
			}
		case "database":
			if curDatabase != nil {
				switch key {
				case "type":
					curDatabase.Type = value
				case "dsn":
					curDatabase.Dsn = value
				}
			}
		case "service":
			if curService != nil {
				switch key {
				case "workdir":
					curService.Workdir = value
				case "autostart":
					curService.Autostart, _ = strconv.ParseBool(value)
				case "watch":
					curService.Watch, _ = strconv.ParseBool(value)
				case "httpport":
					curService.Httpport, _ = strconv.Atoi(value)
				}
			}
		case "envs":
			// Envs uses original key casing (not lowercased).
			origKey := strings.TrimSpace(line[:idx])
			c.envs[origKey] = value
		}
	}

	return nil
}

// Save serializes the config to INI format and writes it to the given path.
func (c *config) Save(path string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var b strings.Builder

	// [project]
	b.WriteString("[project]\n")
	writeField(&b, "name", c.Project.Name)
	writeField(&b, "env", c.Project.Env)
	b.WriteByte('\n')

	// [webui]
	b.WriteString("[webui]\n")
	writeField(&b, "addr", c.Webui.Addr)
	if c.Webui.TraceBufferSize != 0 {
		writeField(&b, "tracebuffersize", fmt.Sprint(c.Webui.TraceBufferSize))
	}
	if c.Webui.LogBufferSize != 0 {
		writeField(&b, "logbuffersize", fmt.Sprint(c.Webui.LogBufferSize))
	}
	b.WriteByte('\n')

	// [dfs]
	if c.Dfs.Addr != "" {
		b.WriteString("[dfs]\n")
		writeField(&b, "addr", c.Dfs.Addr)
		b.WriteByte('\n')
	}

	// [bucket "name"]
	for _, name := range sortedKeys(c.Bucket) {
		bucket := c.Bucket[name]
		fmt.Fprintf(&b, "[bucket %q]\n", name)
		writeField(&b, "path", bucket.Path)
		b.WriteByte('\n')
	}

	// [email]
	if c.Email.Addr != "" {
		b.WriteString("[email]\n")
		writeField(&b, "addr", c.Email.Addr)
		b.WriteByte('\n')
	}

	// [mailbox "addr"]
	for _, name := range sortedKeys(c.Mailbox) {
		mb := c.Mailbox[name]
		fmt.Fprintf(&b, "[mailbox %q]\n", name)
		writeField(&b, "smtp", mb.Smtp)
		writeField(&b, "smtppassword", mb.SmtpPassword)
		writeField(&b, "imap", mb.Imap)
		writeField(&b, "imappassword", mb.ImapPassword)
		b.WriteByte('\n')
	}

	// [database "name"]
	for _, name := range sortedKeys(c.Database) {
		db := c.Database[name]
		fmt.Fprintf(&b, "[database %q]\n", name)
		writeField(&b, "type", db.Type)
		writeField(&b, "dsn", db.Dsn)
		b.WriteByte('\n')
	}

	// [envs]
	if len(c.envs) > 0 {
		b.WriteString("[envs]\n")
		for _, name := range sortedStringMapKeys(c.envs) {
			writeField(&b, name, c.envs[name])
		}
		b.WriteByte('\n')
	}

	// [service "name"]
	for _, name := range sortedKeys(c.Service) {
		svc := c.Service[name]
		fmt.Fprintf(&b, "[service %q]\n", name)
		writeField(&b, "workdir", svc.Workdir)
		if svc.Autostart {
			writeField(&b, "autostart", "true")
		}
		if svc.Watch {
			writeField(&b, "watch", "true")
		}
		if svc.Httpport != 0 {
			writeField(&b, "httpport", fmt.Sprint(svc.Httpport))
		}
		b.WriteByte('\n')
	}

	return os.WriteFile(path, []byte(b.String()), 0644)
}

func writeField(b *strings.Builder, key, value string) {
	if value == "" {
		return
	}
	fmt.Fprintf(b, "%s = %s\n", key, value)
}

func sortedKeys[V any](m map[string]*V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedStringMapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}


// Getter methods for ConfigAccessor interface.

func (c *config) GetProjectName() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Project.Name
}

func (c *config) GetProjectEnv() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Project.Env
}

func (c *config) GetDatabases() map[string]mcp.DatabaseInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make(map[string]mcp.DatabaseInfo, len(c.Database))
	for name, db := range c.Database {
		dbType := db.Type
		if dbType == "" {
			dbType = "mysql"
		}
		result[name] = mcp.DatabaseInfo{Type: dbType, Dsn: db.Dsn}
	}
	return result
}

// GetMySQLDsns returns only MySQL database DSNs (for the mysql transport).
func (c *config) GetMySQLDsns() map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make(map[string]string)
	for name, db := range c.Database {
		if db.Type == "" || db.Type == "mysql" {
			result[name] = db.Dsn
		}
	}
	return result
}

func (c *config) GetBuckets() map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make(map[string]string, len(c.Bucket))
	for name, b := range c.Bucket {
		result[name] = b.Path
	}
	return result
}

func (c *config) GetMailboxes() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]string, 0, len(c.Mailbox))
	for addr := range c.Mailbox {
		result = append(result, addr)
	}
	return result
}

func (c *config) GetEnvVars() map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make(map[string]string, len(c.envs))
	for k, v := range c.envs {
		result[k] = v
	}
	return result
}

// Mutation helpers.

func (c *config) AddDatabase(name, dbType, dsn string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if dbType == "" {
		dbType = "mysql"
	}
	if c.Database == nil {
		c.Database = make(map[string]*struct {
			Type string
			Dsn  string
		})
	}
	c.Database[name] = &struct {
		Type string
		Dsn  string
	}{Type: dbType, Dsn: dsn}
}

func (c *config) RemoveDatabase(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.Database, name)
}

func (c *config) AddBucket(name, path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Bucket == nil {
		c.Bucket = make(map[string]*struct{ Path string })
	}
	c.Bucket[name] = &struct{ Path string }{Path: path}
}

func (c *config) RemoveBucket(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.Bucket, name)
}

func (c *config) AddMailbox(address, smtp, smtpPassword, imap_, imapPassword string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Mailbox == nil {
		c.Mailbox = make(map[string]*struct {
			Smtp         string
			SmtpPassword string
			Imap         string
			ImapPassword string
		})
	}
	c.Mailbox[address] = &struct {
		Smtp         string
		SmtpPassword string
		Imap         string
		ImapPassword string
	}{Smtp: smtp, SmtpPassword: smtpPassword, Imap: imap_, ImapPassword: imapPassword}
}

func (c *config) RemoveMailbox(address string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.Mailbox, address)
}

func (c *config) AddEnv(name, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.envs == nil {
		c.envs = make(map[string]string)
	}
	c.envs[name] = value
}

func (c *config) RemoveEnv(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.envs, name)
}

func (c *config) AddService(name string, svc interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Service == nil {
		c.Service = make(map[string]*serviceConfig)
	}
	switch v := svc.(type) {
	case *serviceConfig:
		c.Service[name] = v
	case *struct {
		Workdir   string
		Autostart bool
		Watch     bool
		Httpport  int
	}:
		c.Service[name] = &serviceConfig{
			Workdir: v.Workdir, Autostart: v.Autostart,
			Watch: v.Watch, Httpport: v.Httpport,
		}
	}
}

func (c *config) RemoveService(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.Service, name)
}
