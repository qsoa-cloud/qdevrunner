package transport

import (
	"context"
	"fmt"
	"net"
	"os"
	"syscall"
)

type FromRunner interface {
	Serve(ctx context.Context) error
}

type ToService interface {
	Prepare() error
	Run(ctx context.Context)
	IsReady() bool
	IsReadyError() error
}

func CreateSocket(filename string) (net.Listener, error) {
	if err := os.Remove(filename); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("cannot remove old unix socket: %v", err)
	}

	conn, err := net.Listen("unix", filename)
	if err != nil {
		return nil, fmt.Errorf("cannot listen unix socket: %v", err)
	}

	if err := os.Chmod(filename, os.ModeSocket|0666); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("cannot chmod unix socket: %v", err)
	}

	return conn, nil
}

func CreatePipe(filename string) (*os.File, error) {
	if err := os.Remove(filename); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("cannot remove old pipe: %v", err)
	}

	if err := syscall.Mkfifo(filename, 0666); err != nil {
		return nil, fmt.Errorf("cannot create pipe: %v", err)
	}

	fi, err := os.Stat(filename)
	if err != nil {
		return nil, fmt.Errorf("cannot stat pipe: %v", err)
	}

	if err := os.Chmod(filename, fi.Mode()|0666); err != nil {
		return nil, fmt.Errorf("cannot chmod pipe: %v", err)
	}

	file, err := os.OpenFile(filename, os.O_RDONLY|syscall.O_NONBLOCK, 0666)
	if err != nil {
		return nil, fmt.Errorf("cannot open pipe: %v", err)
	}

	return file, nil
}

func CreatePipeBlocking(ctx context.Context, filename string) (*os.File, error) {
	if err := os.Remove(filename); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("cannot remove old pipe: %v", err)
	}

	if err := syscall.Mkfifo(filename, 0666); err != nil {
		return nil, fmt.Errorf("cannot create pipe: %v", err)
	}

	fi, err := os.Stat(filename)
	if err != nil {
		return nil, fmt.Errorf("cannot stat pipe: %v", err)
	}

	if err := os.Chmod(filename, fi.Mode()|0666); err != nil {
		return nil, fmt.Errorf("cannot chmod pipe: %v", err)
	}

	// Open in a goroutine so we can cancel via context.
	type result struct {
		f   *os.File
		err error
	}
	ch := make(chan result, 1)
	go func() {
		f, err := os.OpenFile(filename, os.O_RDONLY, 0666)
		ch <- result{f, err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		return r.f, r.err
	}
}
