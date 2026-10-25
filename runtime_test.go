package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	want := filepath.Join(tmp, "qdevrunner")

	for _, pre := range []func(){
		func() {},
		func() { os.Mkdir(want, 0o755); os.Chmod(want, 0o755) },
	} {
		os.RemoveAll(want)
		pre()
		dir, err := runtimeDir()
		if err != nil || dir != want {
			t.Fatalf("runtimeDir() = %q, %v", dir, err)
		}
		fi, err := os.Lstat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if !fi.IsDir() || fi.Mode().Perm() != 0o700 {
			t.Errorf("runtime dir mode %v, want 700 dir", fi.Mode())
		}
	}

	os.RemoveAll(want)
	os.WriteFile(want, nil, 0o600)
	if _, err := runtimeDir(); err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("runtimeDir() over a file: %v", err)
	}
}
