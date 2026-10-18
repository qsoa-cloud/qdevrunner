package secrets

import (
	"context"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"gopkg.qsoa.cloud/qdevrunner/secrets/secretspb"
	"gopkg.qsoa.cloud/qdevrunner/transport"
)

// FromRunner serves one service's secrets on a per-instance socket.
type FromRunner struct {
	addr    string
	service string
	get     func(service, path string) (string, bool)
}

func NewFromRunner(addr, service string, get func(service, path string) (string, bool)) *FromRunner {
	return &FromRunner{addr: addr, service: service, get: get}
}

func (t *FromRunner) Serve(ctx context.Context) error {
	l, err := transport.CreateSocket(t.addr)
	if err != nil {
		return err
	}
	defer l.Close()
	if err := os.Chmod(t.addr, 0o600); err != nil {
		return err
	}

	s := grpc.NewServer()
	secretspb.RegisterQSecretsServer(s, t)

	go func() {
		<-ctx.Done()
		s.GracefulStop()
	}()

	return s.Serve(l)
}

func (t *FromRunner) Get(_ context.Context, req *secretspb.GetSecretReq) (*secretspb.GetSecretResp, error) {
	v, ok := t.get(t.service, req.Path)
	if !ok {
		return nil, status.Error(codes.NotFound, "secret not found")
	}
	return &secretspb.GetSecretResp{Value: v}, nil
}
