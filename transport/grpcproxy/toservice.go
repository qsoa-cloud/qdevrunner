package grpcproxy

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// ToService is the gRPC transport that accepts incoming gRPC calls from outside
// and proxies them to the local service's grpc.sock.
type ToService struct {
	addr        string
	serviceConn *grpc.ClientConn
	mu          sync.Mutex
	ready       bool
	readyErr    error
}

func NewToService(addr string) *ToService {
	return &ToService{addr: addr}
}

func (t *ToService) Prepare() error {
	return nil
}

func (t *ToService) Run(ctx context.Context) {
	for ctx.Err() == nil {
		cc, err := grpc.NewClient("unix://"+t.addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			t.mu.Lock()
			t.readyErr = err
			t.mu.Unlock()
			time.Sleep(100 * time.Millisecond)
			continue
		}
		t.mu.Lock()
		t.serviceConn = cc
		t.ready = true
		t.readyErr = nil
		t.mu.Unlock()
		break
	}
	<-ctx.Done()
	t.mu.Lock()
	if t.serviceConn != nil {
		t.serviceConn.Close()
		t.serviceConn = nil
	}
	t.ready = false
	t.mu.Unlock()
}

func (t *ToService) IsReady() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.ready && t.serviceConn != nil
}

func (t *ToService) IsReadyError() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.readyErr
}

func (t *ToService) ProxyToService(stream grpc.ServerStream) error {
	t.mu.Lock()
	conn := t.serviceConn
	t.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("gRPC is not ready")
	}

	method, ok := grpc.MethodFromServerStream(stream)
	if !ok {
		return fmt.Errorf("no method")
	}

	md, _ := metadata.FromIncomingContext(stream.Context())
	ctx := metadata.NewOutgoingContext(stream.Context(), md)

	from := ""
	if f := md.Get("x-qcloud-from"); len(f) > 0 {
		from = f[0]
	}
	_ = from

	targetStream, err := conn.NewStream(ctx, &grpc.StreamDesc{
		ServerStreams: true,
		ClientStreams: true,
	}, method)
	if err != nil {
		log.Printf("Cannot create stream to service: %v", err)
		return fmt.Errorf("cannot create stream: %v", err)
	}

	return transfer(stream, targetStream)
}
