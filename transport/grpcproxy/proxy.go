package grpcproxy

import (
	"log"
	"sync"

	"google.golang.org/grpc"
)

func transfer(src grpc.ServerStream, dst grpc.ClientStream) error {
	wg := &sync.WaitGroup{}
	wg.Add(1)

	// Src -> Dst (client to target service)
	go func() {
		defer wg.Done()
		for {
			var msg proxyMessage
			if err := src.RecvMsg(&msg); err != nil {
				if err.Error() == "EOF" {
					if err := dst.CloseSend(); err != nil {
						log.Printf("Cannot close target stream: %v", err)
					}
				}
				break
			}
			if err := dst.SendMsg(&msg); err != nil {
				break
			}
		}
	}()

	// Dst -> Src (target service to client)
	var resErr error
	for {
		var msg proxyMessage
		if err := dst.RecvMsg(&msg); err != nil {
			if err.Error() != "EOF" {
				resErr = err
			}
			break
		}
		if err := src.SendMsg(&msg); err != nil {
			break
		}
	}

	wg.Wait()
	return resErr
}
