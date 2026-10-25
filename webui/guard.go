package webui

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// localOnly refuses foreign Host headers, and writes or upgrades from a foreign Origin.
func localOnly(addr string, next http.Handler) http.Handler {
	listenHost, _, _ := net.SplitHostPort(addr)
	if ip := net.ParseIP(listenHost); ip != nil && ip.IsUnspecified() {
		listenHost = ""
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		safe := (r.Method == http.MethodGet || r.Method == http.MethodHead) && r.Header.Get("Upgrade") == ""
		if !allowedHost(r.Host, listenHost) || !safe && !sameOrigin(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func allowedHost(hostport, listenHost string) bool {
	h := (&url.URL{Host: hostport}).Hostname()
	if h == "" {
		return false
	}
	if strings.EqualFold(h, "localhost") || strings.EqualFold(h, listenHost) {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && strings.EqualFold(u.Host, r.Host)
}
