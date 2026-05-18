package email

import (
	"context"

	"google.golang.org/grpc"

	"gopkg.qsoa.cloud/qdevrunner/email/emailpb"
	"gopkg.qsoa.cloud/qdevrunner/transport"
)

// FromRunner creates a per-instance email gRPC server that proxies to the shared email server.
type FromRunner struct {
	addr        string
	emailServer emailpb.QEmailServer
}

func NewFromRunner(addr string, emailServer emailpb.QEmailServer) *FromRunner {
	return &FromRunner{
		addr:        addr,
		emailServer: emailServer,
	}
}

func (t *FromRunner) Serve(ctx context.Context) error {
	l, err := transport.CreateSocket(t.addr)
	if err != nil {
		return err
	}
	defer l.Close()

	s := grpc.NewServer()
	emailpb.RegisterQEmailServer(s, t.emailServer)

	go func() {
		<-ctx.Done()
		s.GracefulStop()
	}()

	return s.Serve(l)
}
