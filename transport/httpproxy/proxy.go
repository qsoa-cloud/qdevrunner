package httpproxy

import (
	"context"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"time"
)

// Proxy listens on a TCP port and proxies HTTP requests to a service's Unix socket.
type Proxy struct {
	tcpAddr    string
	unixSocket string
	server     *http.Server
}

func New(tcpAddr, unixSocket string) *Proxy {
	return &Proxy{
		tcpAddr:    tcpAddr,
		unixSocket: unixSocket,
	}
}

func (p *Proxy) Run(ctx context.Context) {
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = "http"
			req.URL.Host = "localhost"
		},
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.DialTimeout("unix", p.unixSocket, 5*time.Second)
			},
		},
	}

	p.server = &http.Server{
		Addr:    p.tcpAddr,
		Handler: proxy,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		p.server.Shutdown(shutdownCtx)
	}()

	log.Printf("HTTP proxy %s -> unix://%s", p.tcpAddr, p.unixSocket)
	if err := p.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("HTTP proxy error: %v", err)
	}
}
