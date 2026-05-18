package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"

	mcpgoserver "github.com/mark3labs/mcp-go/server"

	"github.com/qsoa-cloud/qdevrunner/dfs"
	"github.com/qsoa-cloud/qdevrunner/email"
	emailpkg "github.com/qsoa-cloud/qdevrunner/email/emailpb"
	"github.com/qsoa-cloud/qdevrunner/instance"
	"github.com/qsoa-cloud/qdevrunner/logstore"
	mcppkg "github.com/qsoa-cloud/qdevrunner/mcp"
	"github.com/qsoa-cloud/qdevrunner/metricsstore"
	"github.com/qsoa-cloud/qdevrunner/registry"
	"github.com/qsoa-cloud/qdevrunner/tracer"
	"github.com/qsoa-cloud/qdevrunner/watcher"
	"github.com/qsoa-cloud/qdevrunner/webui"
)

func main() {
	flag.Parse()

	cfg := readConfig()
	if *addrFlag != "" {
		cfg.Webui.Addr = *addrFlag
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Create in-memory stores.
	tracerStore := tracer.NewStore(cfg.Webui.TraceBufferSize)
	logStore := logstore.NewStore(cfg.Webui.LogBufferSize)
	metricsStore := metricsstore.NewStore(0)

	// Create service registry.
	reg := registry.New()

	// Create shared DFS server (always, so buckets can be added at runtime).
	buckets := make(map[string]string, len(cfg.Bucket))
	for name, b := range cfg.Bucket {
		buckets[name] = b.Path
	}
	dfsServer := dfs.New(buckets)

	// Create shared email server.
	var emailServer emailpkg.QEmailServer
	if cfg.Email.Addr != "" || len(cfg.Mailbox) > 0 {
		mailboxes := make(map[string]*email.MailboxConfig, len(cfg.Mailbox))
		for addr, mb := range cfg.Mailbox {
			mailboxes[addr] = &email.MailboxConfig{
				Address:      addr,
				Smtp:         mb.Smtp,
				SmtpPassword: mb.SmtpPassword,
				Imap:         mb.Imap,
				ImapPassword: mb.ImapPassword,
			}
		}
		emailServer = email.New(mailboxes)
	}

	// Build MySQL DSN map for service instances.
	databases := cfg.GetMySQLDsns()

	// Build bucket paths for UI.
	bucketPaths := make(map[string]string, len(cfg.Bucket))
	for name, b := range cfg.Bucket {
		bucketPaths[name] = b.Path
	}

	// Build mailbox list for UI.
	var mailboxList []string
	for addr := range cfg.Mailbox {
		mailboxList = append(mailboxList, addr)
	}

	// Base path for per-instance temp dirs.
	basePath := filepath.Join(os.TempDir(), "qdevrunner")

	// Create instances for each configured service.
	instances := make(map[string]*instance.Instance, len(cfg.Service))
	for name, svcCfg := range cfg.Service {
		instCfg := instance.Config{
			Name:       name,
			Workdir:    svcCfg.Workdir,
			Autostart:  svcCfg.Autostart,
			Watch:      svcCfg.Watch,
			Httpport:   svcCfg.Httpport,
			Project:    cfg.Project.Name,
			Env:        cfg.Project.Env,
			GetEnvVars: cfg.GetEnvVars,
			Databases:  databases,
			BasePath:   basePath,
		}

		inst := instance.New(instCfg, reg, tracerStore, metricsStore, logStore, dfsServer, emailServer)
		instances[name] = inst
	}

	// Start Web UI + MCP server.
	uiCfg := &webui.UIConfig{
		ProjectName: cfg.Project.Name,
		ProjectEnv:  cfg.Project.Env,
		Databases:   cfg.GetDatabases(),
		Buckets:     bucketPaths,
		Mailboxes:   mailboxList,
	}
	// Database broadcaster: forwards add/remove to all instances' MySQL transports.
	dbBroadcaster := &databaseBroadcaster{instances: instances}

	mcpDeps := &mcppkg.Deps{
		Instances:       instances,
		TracerStore:     tracerStore,
		LogStore:        logStore,
		MetricsStore:    metricsStore,
		Config:          cfg,
		ConfigPath:      *configPath,
		BucketManager:   dfsServer,
		DatabaseManager: dbBroadcaster,
	}
	server := webui.NewServer(cfg.Webui.Addr, instances, tracerStore, logStore, metricsStore, uiCfg, mcpDeps)
	go func() {
		if err := server.Run(); err != nil {
			log.Printf("Web UI error: %v", err)
		}
	}()

	// Start instances.
	var watchers []*watcher.Watcher
	for name, inst := range instances {
		svcCfg := cfg.Service[name]
		if svcCfg.Autostart {
			if err := inst.Start(); err != nil {
				log.Printf("[%s] Failed to start: %v", name, err)
			}
		} else {
			if err := inst.StartManual(); err != nil {
				log.Printf("[%s] Failed to start manual mode: %v", name, err)
			}
		}

		// Start file watcher for auto-rebuild.
		if svcCfg.Watch && svcCfg.Workdir != "" && svcCfg.Autostart {
			w := watcher.New(svcCfg.Workdir, name, inst)
			watchers = append(watchers, w)
			go w.Start()
		}
	}

	// Start MCP stdio server if requested.
	if *stdioFlag {
		mcpServer := mcppkg.NewMCPServer(mcpDeps)
		stdioServer := mcpgoserver.NewStdioServer(mcpServer)
		go func() {
			if err := stdioServer.Listen(ctx, os.Stdin, os.Stdout); err != nil {
				log.Printf("MCP stdio error: %v", err)
			}
		}()
		log.Println("MCP stdio server started")
	}

	// Wait for shutdown signal.
	<-ctx.Done()
	log.Println("Shutting down...")

	// Stop file watchers.
	for _, w := range watchers {
		w.Stop()
	}

	// Stop all instances.
	var wg sync.WaitGroup
	for _, inst := range instances {
		inst := inst
		wg.Add(1)
		go func() {
			defer wg.Done()
			inst.Stop()
		}()
	}
	wg.Wait()

	log.Println("Shutdown complete.")
}

// databaseBroadcaster forwards DSN changes to all instances' MySQL transports.
type databaseBroadcaster struct {
	instances map[string]*instance.Instance
}

func (b *databaseBroadcaster) AddDsn(name, dsn string) {
	for _, inst := range b.instances {
		if t := inst.GetMySQLTransport(); t != nil {
			t.AddDsn(name, dsn)
		}
	}
}

func (b *databaseBroadcaster) RemoveDsn(name string) {
	for _, inst := range b.instances {
		if t := inst.GetMySQLTransport(); t != nil {
			t.RemoveDsn(name)
		}
	}
}
