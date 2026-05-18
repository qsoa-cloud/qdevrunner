//go:generate protoc -I pb --gogofaster_out=plugins=grpc:pb pb/qmysql.proto

package mysql

import (
	"context"
	"fmt"
	"sync"

	"google.golang.org/grpc"

	"github.com/qsoa-cloud/qdevrunner/transport"
	"github.com/qsoa-cloud/qdevrunner/transport/mysql/pb"
)

type MySql struct {
	mu   sync.RWMutex
	dsns map[string]string
	addr string
}

func New(addr string, dsns map[string]string) *MySql {
	return &MySql{
		dsns: dsns,
		addr: addr,
	}
}

func (t *MySql) AddDsn(name, dsn string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.dsns == nil {
		t.dsns = make(map[string]string)
	}
	t.dsns[name] = dsn
}

func (t *MySql) RemoveDsn(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.dsns, name)
}

func (t *MySql) Serve(ctx context.Context) error {
	l, err := transport.CreateSocket(t.addr)
	if err != nil {
		return err
	}
	defer l.Close()

	s := grpc.NewServer()
	pb.RegisterMySqlServer(s, t)

	go func() {
		<-ctx.Done()
		s.GracefulStop()
	}()

	return s.Serve(l)
}

func (t *MySql) GetDsn(_ context.Context, req *pb.GetDsnReq) (*pb.GetDsnResp, error) {
	t.mu.RLock()
	dsn, ok := t.dsns[req.Name]
	t.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown database %q", req.Name)
	}
	return &pb.GetDsnResp{Dsn: dsn}, nil
}
