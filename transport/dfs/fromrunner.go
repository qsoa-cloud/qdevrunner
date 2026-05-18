package dfs

import (
	"context"

	"google.golang.org/grpc"

	"gopkg.qsoa.cloud/qdevrunner/dfs/dfspb"
	"gopkg.qsoa.cloud/qdevrunner/transport"
)

// FromRunner creates a per-instance DFS gRPC server that proxies to the shared DFS server.
type FromRunner struct {
	addr      string
	dfsServer dfspb.DfsServer
}

func NewFromRunner(addr string, dfsServer dfspb.DfsServer) *FromRunner {
	return &FromRunner{
		addr:      addr,
		dfsServer: dfsServer,
	}
}

func (t *FromRunner) Serve(ctx context.Context) error {
	l, err := transport.CreateSocket(t.addr)
	if err != nil {
		return err
	}
	defer l.Close()

	s := grpc.NewServer()
	dfspb.RegisterDfsServer(s, t.dfsServer)

	go func() {
		<-ctx.Done()
		s.GracefulStop()
	}()

	return s.Serve(l)
}
