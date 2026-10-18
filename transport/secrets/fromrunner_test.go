package secrets

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"gopkg.qsoa.cloud/qdevrunner/secrets/secretspb"
)

func TestGet(t *testing.T) {
	store := map[string]map[string]string{
		"api": {"db/password": "api-value"},
		"web": {"token": "web-value"},
	}
	get := func(service, path string) (string, bool) {
		v, ok := store[service][path]
		return v, ok
	}
	fr := NewFromRunner("", "api", get)

	resp, err := fr.Get(context.Background(), &secretspb.GetSecretReq{Path: "db/password"})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if resp.Value != "api-value" {
		t.Errorf("got %q, want %q", resp.Value, "api-value")
	}

	_, err = fr.Get(context.Background(), &secretspb.GetSecretReq{Path: "token"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("got %v, want NotFound", err)
	}
	if status.Convert(err).Message() != "secret not found" {
		t.Errorf("message %q", status.Convert(err).Message())
	}
}

func TestServeSocketMode(t *testing.T) {
	addr := filepath.Join(t.TempDir(), "secrets.sock")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- NewFromRunner(addr, "api", func(string, string) (string, bool) { return "", false }).Serve(ctx)
	}()
	defer func() {
		cancel()
		<-done
	}()

	var fi os.FileInfo
	var err error
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if fi, err = os.Stat(addr); err == nil && fi.Mode().Perm() == 0o600 {
			break
		}
	}
	if err != nil {
		t.Fatalf("socket not created: %v", err)
	}
	if m := fi.Mode().Perm(); m != 0o600 {
		t.Errorf("socket mode %o, want 600", m)
	}
}
