package transport

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCreateSocket(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sock")

	l, err := CreateSocket(path)
	if err != nil {
		t.Fatalf("CreateSocket: %v", err)
	}
	defer l.Close()

	// Verify socket exists.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		t.Error("expected socket file")
	}

	// Verify we can connect.
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	conn.Close()
}

func TestCreateSocketRemovesOld(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sock")

	// Create first socket.
	l1, err := CreateSocket(path)
	if err != nil {
		t.Fatalf("first CreateSocket: %v", err)
	}
	l1.Close()

	// Create second socket at same path — should remove old.
	l2, err := CreateSocket(path)
	if err != nil {
		t.Fatalf("second CreateSocket: %v", err)
	}
	defer l2.Close()

	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	conn.Close()
}

func TestCreatePipe(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.fifo")

	f, err := CreatePipe(path)
	if err != nil {
		t.Fatalf("CreatePipe: %v", err)
	}
	defer f.Close()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode()&os.ModeNamedPipe == 0 {
		t.Error("expected named pipe")
	}
}

func TestCreatePipeBlocking(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.fifo")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Open in goroutine since blocking open waits for writer.
	done := make(chan error, 1)
	go func() {
		f, err := CreatePipeBlocking(ctx, path)
		if err == nil {
			f.Close()
		}
		done <- err
	}()

	// Give it time to create the FIFO.
	time.Sleep(100 * time.Millisecond)

	// Open writer to unblock.
	w, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	w.Close()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("CreatePipeBlocking: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
}

func TestCreatePipeBlockingCancellation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.fifo")

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		_, err := CreatePipeBlocking(ctx, path)
		done <- err
	}()

	// Cancel before anyone opens the writer.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout — cancel didn't work")
	}
}
