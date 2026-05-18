package instance

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/qsoa-cloud/qdevrunner/dfs/dfspb"
	"github.com/qsoa-cloud/qdevrunner/email/emailpb"
	"github.com/qsoa-cloud/qdevrunner/logstore"
	"github.com/qsoa-cloud/qdevrunner/metricsstore"
	"github.com/qsoa-cloud/qdevrunner/registry"
	"github.com/qsoa-cloud/qdevrunner/tracer"
	transportDfs "github.com/qsoa-cloud/qdevrunner/transport/dfs"
	transportEmail "github.com/qsoa-cloud/qdevrunner/transport/email"
	"github.com/qsoa-cloud/qdevrunner/transport/grpcproxy"
	"github.com/qsoa-cloud/qdevrunner/transport/httpproxy"
	"github.com/qsoa-cloud/qdevrunner/transport/mysql"
	"github.com/qsoa-cloud/qdevrunner/transport/opentracing"
	"github.com/qsoa-cloud/qdevrunner/transport/prometheus"
)

type Status string

const (
	StatusStopped  Status = "stopped"
	StatusStarting Status = "starting"
	StatusRunning  Status = "running"
	StatusError    Status = "error"
	StatusManual   Status = "manual"
)

type Config struct {
	Name      string
	Workdir   string
	Autostart bool
	Watch     bool
	Httpport  int
	Project   string
	Env       string
	GetEnvVars func() map[string]string
	Databases  map[string]string
	BasePath  string // Base temp directory for sockets
}

type Instance struct {
	cfg           Config
	mu            sync.Mutex
	status        Status
	process       *os.Process
	processState  *os.ProcessState
	started       time.Time
	lastError     string
	cancel        context.CancelFunc
	registry      *registry.Registry
	tracerStore   *tracer.Store
	metricsStore  *metricsstore.Store
	logStore      *logstore.Store
	dfsServer     dfspb.DfsServer
	emailServer   emailpb.QEmailServer
	miscDir       string
	tmpDir        string
	binPath       string   // built binary path
	transports    []string // auto-discovered transports
	stopping      bool     // true when Stop() was called

	// Transports with live-update support.
	mysqlTransport *mysql.MySql

	// ToService transports.
	grpcToService   *grpcproxy.ToService
	httpProxy       *httpproxy.Proxy

	// Listeners.
	statusListeners []func(Status)
}

func New(cfg Config, reg *registry.Registry, ts *tracer.Store, ms *metricsstore.Store, ls *logstore.Store,
	dfsServer dfspb.DfsServer, emailServer emailpb.QEmailServer) *Instance {

	baseDir := filepath.Join(cfg.BasePath, cfg.Project, cfg.Name)
	return &Instance{
		cfg:          cfg,
		status:       StatusStopped,
		registry:     reg,
		tracerStore:  ts,
		metricsStore: ms,
		logStore:     ls,
		dfsServer:    dfsServer,
		emailServer:  emailServer,
		miscDir:      filepath.Join(baseDir, "misc"),
		tmpDir:       filepath.Join(baseDir, "tmp"),
	}
}

// GetName implements registry.Instance.
func (i *Instance) GetName() string { return i.cfg.Name }

func (i *Instance) GetGrpcToServiceAddr() string {
	return filepath.Join(i.tmpDir, "grpc.sock")
}

func (i *Instance) IsGrpcReady() bool {
	if i.grpcToService == nil {
		return false
	}
	return i.grpcToService.IsReady()
}

func (i *Instance) GetHttpToServiceAddr() string {
	return filepath.Join(i.tmpDir, "http.sock")
}

func (i *Instance) IsHttpReady() bool {
	return false
}

func (i *Instance) GetStatus() Status {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.status
}

func (i *Instance) GetPid() int {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.process != nil {
		return i.process.Pid
	}
	return 0
}

func (i *Instance) GetStarted() time.Time {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.started
}

func (i *Instance) GetLastError() string {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.lastError
}

func (i *Instance) GetConfig() Config {
	return i.cfg
}

func (i *Instance) OnStatusChange(fn func(Status)) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.statusListeners = append(i.statusListeners, fn)
}

func (i *Instance) setStatus(s Status) {
	i.status = s
	for _, fn := range i.statusListeners {
		go fn(s)
	}
}

// GetRunCommand returns the shell command for manual mode.
func (i *Instance) GetRunCommand() string {
	args := i.buildArgs()
	return fmt.Sprintf("cd %s && go run . %s", i.cfg.Workdir, strings.Join(args, " \\\n  "))
}

func (i *Instance) hasTransport(name string) bool {
	for _, t := range i.transports {
		if t == name {
			return true
		}
	}
	return false
}

// GetMySQLTransport returns the MySQL transport for live DSN updates, or nil.
func (i *Instance) GetMySQLTransport() *mysql.MySql {
	return i.mysqlTransport
}

// GetTransports returns the auto-discovered transports.
func (i *Instance) GetTransports() []string {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.transports
}

func (i *Instance) buildArgs() []string {
	var args []string

	args = append(args, fmt.Sprintf("-q_project=%s", i.cfg.Project))
	args = append(args, fmt.Sprintf("-q_env=%s", i.cfg.Env))
	args = append(args, fmt.Sprintf("-q_service=%s", i.cfg.Name))
	args = append(args, "-q_version=local")

	if i.hasTransport("dfs") {
		args = append(args, fmt.Sprintf("-q_dfs_sock=unix://%s", filepath.Join(i.miscDir, "dfs.sock")))
	}
	if i.hasTransport("email") {
		args = append(args, fmt.Sprintf("-q_email_sock=unix://%s", filepath.Join(i.miscDir, "email.sock")))
	}
	if i.hasTransport("grpc") {
		args = append(args, fmt.Sprintf("-q_grpc_proxy=unix://%s", filepath.Join(i.miscDir, "grpc_runner.sock")))
		args = append(args, fmt.Sprintf("-q_grpc_addr=unix://%s", filepath.Join(i.tmpDir, "grpc.sock")))
	}
	if i.hasTransport("mysql") {
		args = append(args, fmt.Sprintf("-q_mysql_addr=unix://%s", filepath.Join(i.miscDir, "mysql.sock")))
	}
	if i.hasTransport("http") {
		args = append(args, fmt.Sprintf("-q_http_addr=unix://%s", filepath.Join(i.tmpDir, "http.sock")))
	}
	if i.hasTransport("tracer") {
		args = append(args, fmt.Sprintf("-q_tracer_file=%s", filepath.Join(i.miscDir, "tracer.fifo")))
	}
	if i.hasTransport("prometheus") {
		args = append(args, fmt.Sprintf("-q_metrics_addr=unix://%s", filepath.Join(i.tmpDir, "prometheus.sock")))
	}

	return args
}

// StartTransports creates directories and starts all FromRunner transports.
// This is called for both managed and manual modes.
func (i *Instance) StartTransports(ctx context.Context) error {
	if err := os.MkdirAll(i.miscDir, 0777); err != nil {
		return fmt.Errorf("cannot create misc dir: %v", err)
	}
	// Clean stale service-side sockets from a previous run.
	os.RemoveAll(i.tmpDir)
	if err := os.MkdirAll(i.tmpDir, 0777); err != nil {
		return fmt.Errorf("cannot create tmp dir: %v", err)
	}

	// FromRunner transports (qdevrunner listens, service connects).
	if i.hasTransport("grpc") {
		fr := grpcproxy.NewFromRunner(i.cfg.Project, i.cfg.Env, i.cfg.Name,
			filepath.Join(i.miscDir, "grpc_runner.sock"), i.registry)
		go func() {
			if err := fr.Serve(ctx); err != nil {
				log.Printf("[%s] gRPC proxy error: %v", i.cfg.Name, err)
			}
		}()
	}

	if i.hasTransport("mysql") {
		i.mysqlTransport = mysql.New(filepath.Join(i.miscDir, "mysql.sock"), i.cfg.Databases)
		go func() {
			if err := i.mysqlTransport.Serve(ctx); err != nil {
				log.Printf("[%s] MySQL transport error: %v", i.cfg.Name, err)
			}
		}()
	}

	if i.hasTransport("dfs") && i.dfsServer != nil {
		d := transportDfs.NewFromRunner(filepath.Join(i.miscDir, "dfs.sock"), i.dfsServer)
		go func() {
			if err := d.Serve(ctx); err != nil {
				log.Printf("[%s] DFS transport error: %v", i.cfg.Name, err)
			}
		}()
	}

	if i.hasTransport("email") && i.emailServer != nil {
		e := transportEmail.NewFromRunner(filepath.Join(i.miscDir, "email.sock"), i.emailServer)
		go func() {
			if err := e.Serve(ctx); err != nil {
				log.Printf("[%s] Email transport error: %v", i.cfg.Name, err)
			}
		}()
	}

	if i.hasTransport("tracer") {
		ot := opentracing.New(i.cfg.Name, filepath.Join(i.miscDir, "tracer.fifo"), i.tracerStore)
		go func() {
			if err := ot.Serve(ctx); err != nil {
				log.Printf("[%s] OpenTracing error: %v", i.cfg.Name, err)
			}
		}()
	}

	return nil
}

// Start starts the service process (managed mode).
func (i *Instance) Start() error {
	i.mu.Lock()
	if i.process != nil {
		i.mu.Unlock()
		return fmt.Errorf("instance %s is already running", i.cfg.Name)
	}
	i.processState = nil
	i.lastError = ""
	i.stopping = false
	i.setStatus(StatusStarting)
	i.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	i.cancel = cancel

	// Build the service binary.
	log.Printf("[%s] Building: go build -o qservice .", i.cfg.Name)
	if err := i.runBuild(ctx); err != nil {
		cancel()
		i.mu.Lock()
		i.setStatus(StatusError)
		i.lastError = fmt.Sprintf("build failed: %v", err)
		i.mu.Unlock()
		return fmt.Errorf("build failed: %v", err)
	}

	// Discover transports from the built binary.
	i.discoverTransports()

	if err := i.StartTransports(ctx); err != nil {
		cancel()
		i.mu.Lock()
		i.setStatus(StatusError)
		i.lastError = err.Error()
		i.mu.Unlock()
		return err
	}

	// Start the process.
	args := i.buildArgs()

	cmd := exec.CommandContext(ctx, i.binPath, args...)
	cmd.Dir = i.cfg.Workdir

	// Environment variables. Fetch fresh from the live config on every spawn so
	// changes made via add_env/remove_env take effect on restart_service.
	cmd.Env = os.Environ()
	var envVars map[string]string
	if i.cfg.GetEnvVars != nil {
		envVars = i.cfg.GetEnvVars()
	}
	for k, v := range envVars {
		cmd.Env = append(cmd.Env, fmt.Sprintf("QSOA_%s=%s", k, v))
	}

	// Capture stdout/stderr.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("stdout pipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		i.mu.Lock()
		i.setStatus(StatusError)
		i.lastError = err.Error()
		i.mu.Unlock()
		return err
	}

	i.mu.Lock()
	i.process = cmd.Process
	i.started = time.Now()
	i.setStatus(StatusRunning)
	i.mu.Unlock()

	// Register in service mesh.
	if i.hasTransport("grpc") {
		i.grpcToService = grpcproxy.NewToService(filepath.Join(i.tmpDir, "grpc.sock"))
		go i.grpcToService.Run(ctx)
	}

	// Register in registry for service mesh.
	i.registry.Register(i)

	// Start prometheus scraper.
	if i.hasTransport("prometheus") {
		prom := prometheus.New(i.cfg.Name, filepath.Join(i.tmpDir, "prometheus.sock"), i.metricsStore)
		go prom.Run(ctx)
	}

	// Start HTTP TCP proxy.
	if i.hasTransport("http") && i.cfg.Httpport > 0 {
		i.httpProxy = httpproxy.New(
			fmt.Sprintf("127.0.0.1:%d", i.cfg.Httpport),
			filepath.Join(i.tmpDir, "http.sock"),
		)
		go i.httpProxy.Run(ctx)
	}

	// Log capture goroutines.
	go i.captureOutput(stdout, "stdout")
	go i.captureOutput(stderr, "stderr")

	// Wait for process exit.
	go func() {
		_ = cmd.Wait()
		i.registry.Unregister(i)
		i.mu.Lock()
		i.process = nil
		i.processState = cmd.ProcessState
		i.started = time.Time{}
		if cmd.ProcessState.ExitCode() != 0 && !i.stopping {
			i.lastError = fmt.Sprintf("exited with code %d", cmd.ProcessState.ExitCode())
			i.setStatus(StatusError)
		} else {
			i.setStatus(StatusStopped)
		}
		i.mu.Unlock()
		cancel()
	}()

	log.Printf("[%s] Started (PID %d)", i.cfg.Name, cmd.Process.Pid)
	return nil
}

// StartManual starts only transports (manual mode).
func (i *Instance) StartManual() error {
	ctx, cancel := context.WithCancel(context.Background())
	i.cancel = cancel

	// Build to discover transports.
	log.Printf("[%s] Building for transport discovery...", i.cfg.Name)
	if err := i.runBuild(ctx); err != nil {
		cancel()
		return fmt.Errorf("build failed: %v", err)
	}
	i.discoverTransports()

	if err := i.StartTransports(ctx); err != nil {
		cancel()
		return err
	}

	// Register in registry so other services can call this one once it connects.
	if i.hasTransport("grpc") {
		i.grpcToService = grpcproxy.NewToService(filepath.Join(i.tmpDir, "grpc.sock"))
		go i.grpcToService.Run(ctx)
		i.registry.Register(i)
	}

	i.mu.Lock()
	i.setStatus(StatusManual)
	i.mu.Unlock()

	log.Printf("[%s] Manual mode - transports ready", i.cfg.Name)
	log.Printf("[%s] Run command:\n  %s", i.cfg.Name, i.GetRunCommand())
	return nil
}

// Stop stops the service process.
func (i *Instance) Stop() {
	i.mu.Lock()
	proc := i.process
	cancel := i.cancel
	i.stopping = true
	i.mu.Unlock()

	i.registry.Unregister(i)

	if proc != nil {
		deadline := time.Now().Add(5 * time.Second)
		for {
			i.mu.Lock()
			p := i.process
			i.mu.Unlock()
			if p == nil {
				break
			}
			sig := os.Interrupt
			if time.Now().After(deadline) {
				sig = os.Kill
			}
			_ = p.Signal(sig)
			time.Sleep(100 * time.Millisecond)
		}
	}

	if cancel != nil {
		cancel()
	}

	i.mu.Lock()
	i.setStatus(StatusStopped)
	i.mu.Unlock()

	log.Printf("[%s] Stopped", i.cfg.Name)
}

// Restart stops then starts the service.
func (i *Instance) Restart() error {
	i.Stop()
	return i.Start()
}

func (i *Instance) runBuild(ctx context.Context) error {
	i.binPath = filepath.Join(filepath.Dir(i.miscDir), "qservice")

	cmd := exec.CommandContext(ctx, "go", "build", "-o", i.binPath, ".")
	cmd.Dir = i.cfg.Workdir

	output, err := cmd.CombinedOutput()
	if len(output) > 0 {
		i.logStore.Add(logstore.LogEntry{
			Service:   i.cfg.Name,
			Stream:    "build",
			Text:      string(output),
			Timestamp: time.Now(),
		})
	}
	return err
}

// discoverTransports runs the built binary with -help and parses the output
// to detect which -q_* flags it accepts, mapping them to transport names.
func (i *Instance) discoverTransports() {
	cmd := exec.Command(i.binPath, "-help")
	output, _ := cmd.CombinedOutput() // -help exits with code 2, expected

	flagToTransport := map[string]string{
		"-q_grpc_proxy":  "grpc",
		"-q_grpc_addr":   "grpc",
		"-q_http_addr":   "http",
		"-q_mysql_addr":  "mysql",
		"-q_dfs_sock":    "dfs",
		"-q_email_sock":  "email",
		"-q_tracer_file": "tracer",
		"-q_metrics_addr": "prometheus",
	}

	seen := map[string]bool{}
	var transports []string
	for _, line := range strings.Split(string(output), "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		if !strings.HasPrefix(trimmed, "-q_") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) == 0 {
			continue
		}
		flag := fields[0]
		if transport, ok := flagToTransport[flag]; ok && !seen[transport] {
			seen[transport] = true
			transports = append(transports, transport)
		}
	}

	i.transports = transports
	log.Printf("[%s] Discovered transports: %v", i.cfg.Name, i.transports)
}

func (i *Instance) captureOutput(r interface{ Read([]byte) (int, error) }, stream string) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		i.logStore.Add(logstore.LogEntry{
			Service:   i.cfg.Name,
			Stream:    stream,
			Text:      line,
			Timestamp: time.Now(),
		})
		// Also print to console.
		log.Printf("[%s:%s] %s", i.cfg.Name, stream, line)
	}
}

// Ensure Instance implements registry.Instance.
var _ registry.Instance = (*Instance)(nil)
