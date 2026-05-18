package grpcproxy

import (
	"context"
	"fmt"
	"net/url"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"gopkg.qsoa.cloud/qdevrunner/registry"
	"gopkg.qsoa.cloud/qdevrunner/transport"
)

// FromRunner is the gRPC proxy that services use to call other services.
// It listens on grpc_runner.sock and routes calls based on x-qcloud-target header.
type FromRunner struct {
	project  string
	env      string
	service  string
	addr     string
	registry *registry.Registry
}

func NewFromRunner(project, env, service, addr string, reg *registry.Registry) *FromRunner {
	return &FromRunner{
		project:  project,
		env:      env,
		service:  service,
		addr:     addr,
		registry: reg,
	}
}

func (t *FromRunner) Serve(ctx context.Context) error {
	l, err := transport.CreateSocket(t.addr)
	if err != nil {
		return err
	}
	defer l.Close()

	s := grpc.NewServer(grpc.UnknownServiceHandler(t.proxyToService))

	go func() {
		<-ctx.Done()
		s.GracefulStop()
	}()

	return s.Serve(l)
}

func (t *FromRunner) proxyToService(srv interface{}, clientStream grpc.ServerStream) error {
	md, ok := metadata.FromIncomingContext(clientStream.Context())
	if !ok {
		return fmt.Errorf("no metadata")
	}

	target := md.Get("x-qcloud-target")
	if len(target) == 0 {
		return fmt.Errorf("no target header 'x-qcloud-target' was provided in the metadata")
	}

	u, err := url.Parse(target[0])
	if err != nil {
		return fmt.Errorf("cannot parse target: %v", err)
	}

	targetService := u.Host

	method, ok := grpc.MethodFromServerStream(clientStream)
	if !ok {
		return fmt.Errorf("no method")
	}

	inst, err := t.registry.GetGrpcInstance(targetService)
	if err != nil {
		return fmt.Errorf("cannot find service %q: %v", targetService, err)
	}

	grpcAddr := inst.GetGrpcToServiceAddr()
	conn, err := grpc.NewClient("unix://"+grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("cannot connect to service %q: %v", targetService, err)
	}
	defer conn.Close()

	md.Set("x-qcloud-project", t.project)
	md.Set("x-qcloud-env", t.env)
	md.Set("x-qcloud-service", targetService)
	md.Set("x-qcloud-from", t.service)

	ctx := metadata.NewOutgoingContext(clientStream.Context(), md)

	targetStream, err := conn.NewStream(ctx, &grpc.StreamDesc{
		ServerStreams: true,
		ClientStreams: true,
	}, method)
	if err != nil {
		return fmt.Errorf("cannot create stream: %v", err)
	}

	return transfer(clientStream, targetStream)
}
