package instance

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.qsoa.cloud/qdevrunner/dfs/dfspb"
	"gopkg.qsoa.cloud/qdevrunner/email/emailpb"
	"gopkg.qsoa.cloud/qdevrunner/tracer"
)

func perm(t *testing.T, path string) os.FileMode {
	t.Helper()
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Mode().Perm()
}

func TestPrivateDir(t *testing.T) {
	uid := os.Getuid()
	root := t.TempDir()

	fresh := filepath.Join(root, "fresh")
	if err := PrivateDir(fresh, uid); err != nil {
		t.Fatal(err)
	}
	if m := perm(t, fresh); m != 0o700 {
		t.Errorf("new dir mode %o, want 700", m)
	}

	for _, mode := range []os.FileMode{0o755, 0o777, 0o711} {
		dir := filepath.Join(root, "open"+mode.String())
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		os.Chmod(dir, mode)
		if err := PrivateDir(dir, uid); err != nil {
			t.Fatal(err)
		}
		if m := perm(t, dir); m != 0o700 {
			t.Errorf("existing %o dir mode %o, want 700", mode, m)
		}
	}

	foreign := filepath.Join(root, "foreign")
	os.Mkdir(foreign, 0o755)
	os.Chmod(foreign, 0o755)
	err := PrivateDir(foreign, uid+1)
	if err == nil || !strings.Contains(err.Error(), "owned by uid") || !strings.Contains(err.Error(), foreign) {
		t.Errorf("foreign owner: %v", err)
	}
	if m := perm(t, foreign); m != 0o755 {
		t.Errorf("foreign dir mode changed to %o", m)
	}

	file := filepath.Join(root, "file")
	os.WriteFile(file, nil, 0o600)
	if err := PrivateDir(file, uid); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("file: %v", err)
	}

	link := filepath.Join(root, "link")
	os.Symlink(fresh, link)
	if err := PrivateDir(link, uid); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("symlink: %v", err)
	}
}

var allSockets = []string{"grpc_runner.sock", "mysql.sock", "clickhouse.sock", "dfs.sock", "email.sock", "secrets.sock"}

func privateInstance(base string) *Instance {
	cfg := Config{
		Name: "svc", Project: "p", Env: "dev", BasePath: base,
		Databases:         map[string]string{"db": "dsn"},
		GetSecret:         func(string, string) (string, bool) { return "", false },
		GetClickHouseDsns: func() map[string]string { return map[string]string{"events": "clickhouse://u:p@127.0.0.1:9000/events"} },
	}
	inst := New(cfg, nil, tracer.NewStore(10), nil, nil, &dfspb.UnimplementedDfsServer{}, &emailpb.UnimplementedQEmailServer{})
	inst.transports = []string{"grpc", "mysql", "clickhouse", "dfs", "email", "secrets"}
	return inst
}

func TestStartTransportsPrivate(t *testing.T) {
	base := filepath.Join(t.TempDir(), "qdevrunner")
	misc := filepath.Join(base, "p", "svc", "misc")
	tmp := filepath.Join(base, "p", "svc", "tmp")
	for _, d := range []string{misc, tmp} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, d := range []string{base, filepath.Join(base, "p"), filepath.Join(base, "p", "svc"), misc, tmp} {
		os.Chmod(d, 0o755)
	}

	inst := privateInstance(base)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := inst.StartTransports(ctx); err != nil {
		t.Fatal(err)
	}

	for _, d := range []string{base, filepath.Join(base, "p"), filepath.Join(base, "p", "svc"), misc, tmp} {
		if m := perm(t, d); m != 0o700 {
			t.Errorf("%s mode %o, want 700", d, m)
		}
	}

	var paths []string
	for _, name := range allSockets {
		sock := filepath.Join(misc, name)
		paths = append(paths, sock)
		var fi os.FileInfo
		var err error
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			if fi, err = os.Stat(sock); err == nil && fi.Mode().Perm() == 0o600 {
				break
			}
		}
		if err != nil {
			t.Fatalf("%s not created: %v", name, err)
		}
		if fi.Mode()&os.ModeSocket == 0 || fi.Mode().Perm() != 0o600 {
			t.Errorf("%s mode %v, want socket 600", name, fi.Mode())
		}
		c, err := net.Dial("unix", sock)
		if err != nil {
			t.Errorf("%s: same-user dial: %v", name, err)
		} else {
			c.Close()
		}
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperDial$")
	cmd.Env = append(os.Environ(), "QDR_HELPER_DIAL="+strings.Join(paths, string(os.PathListSeparator)))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("child process dial: %v: %s", err, out)
	}
}

func TestHelperDial(t *testing.T) {
	paths := os.Getenv("QDR_HELPER_DIAL")
	if paths == "" {
		t.Skip("helper process")
	}
	for _, p := range filepath.SplitList(paths) {
		c, err := net.Dial("unix", p)
		if err != nil {
			t.Fatal(err)
		}
		c.Close()
	}
}

func TestStartTransportsRefusesForeignOwner(t *testing.T) {
	base := filepath.Join(t.TempDir(), "qdevrunner")
	os.Mkdir(base, 0o700)
	inst := privateInstance(base)
	inst.uid = os.Getuid() + 1
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := inst.StartTransports(ctx)
	if err == nil || !strings.Contains(err.Error(), "owned by uid") {
		t.Fatalf("got %v, want owner error", err)
	}
	if _, err := os.Stat(filepath.Join(base, "p")); !os.IsNotExist(err) {
		t.Errorf("project dir created under a foreign base: %v", err)
	}
}
