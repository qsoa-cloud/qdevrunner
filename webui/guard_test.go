package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

const (
	uiHost        = "127.0.0.1:8090"
	foreignOrigin = "http://evil.example"
	envAddBody    = `{"name":"K","value":"V"}`
	mcpInitBody   = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`
)

func guardServer(addr string) (*mockConfigAccessor, http.Handler) {
	s := testServer()
	s.addr = addr
	return s.mcpDeps.Config.(*mockConfigAccessor), s.handler()
}

func serve(h http.Handler, method, target, host, origin, body string, hdr ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Host = host
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestNoCORSHeaders(t *testing.T) {
	_, h := guardServer(uiHost)
	for _, tc := range []struct{ method, path, origin string }{
		{http.MethodGet, "/api/config", foreignOrigin},
		{http.MethodGet, "/api/services", foreignOrigin},
		{http.MethodGet, "/api/services/none", foreignOrigin},
		{http.MethodGet, "/api/traces", foreignOrigin},
		{http.MethodGet, "/api/traces/1", foreignOrigin},
		{http.MethodOptions, "/api/config", ""},
		{http.MethodOptions, "/api/services", ""},
		{http.MethodOptions, "/api/services/none", ""},
		{http.MethodOptions, "/api/config/envs/add", ""},
		{http.MethodOptions, "/api/traces", ""},
		{http.MethodOptions, "/api/traces/1", ""},
		{http.MethodOptions, "/api/config/envs/add", foreignOrigin},
	} {
		w := serve(h, tc.method, tc.path, uiHost, tc.origin, "")
		for k := range w.Header() {
			if strings.HasPrefix(k, "Access-Control-") {
				t.Errorf("%s %s origin %q: %s set", tc.method, tc.path, tc.origin, k)
			}
		}
	}
}

func TestForeignOriginWriteRefused(t *testing.T) {
	for _, origin := range []string{foreignOrigin, "http://127.0.0.1:9999", "http://localhost:8090", "https://evil.example:8090", "null"} {
		cfg, h := guardServer(uiHost)
		for _, tc := range []struct{ method, path, body string }{
			{http.MethodPost, "/api/config/envs/add", envAddBody},
			{http.MethodPost, "/api/config/databases/add", `{"name":"db","dsn":"u:p@tcp(127.0.0.1:3306)/db"}`},
			{http.MethodPut, "/api/config/envs/add", envAddBody},
			{http.MethodDelete, "/api/config/envs/remove", `{"name":"K"}`},
			{http.MethodPost, "/mcp", mcpInitBody},
		} {
			w := serve(h, tc.method, tc.path, uiHost, origin, tc.body, "Content-Type", "application/json", "Accept", "application/json, text/event-stream")
			if w.Code != http.StatusForbidden {
				t.Errorf("origin %q %s %s: %d, want 403", origin, tc.method, tc.path, w.Code)
			}
		}
		if len(cfg.envVars) != 0 || len(cfg.databases) != 0 || cfg.saved {
			t.Errorf("origin %q changed config", origin)
		}
	}
}

func TestForeignOriginUpgradeRefused(t *testing.T) {
	_, h := guardServer(uiHost)
	for _, tc := range []struct {
		origin string
		code   int
	}{
		{foreignOrigin, http.StatusForbidden},
		{"null", http.StatusForbidden},
		{"http://" + uiHost, http.StatusOK},
		{"", http.StatusOK},
	} {
		w := serve(h, http.MethodGet, "/api/config", uiHost, tc.origin, "", "Connection", "Upgrade", "Upgrade", "websocket")
		if w.Code != tc.code {
			t.Errorf("origin %q: %d, want %d", tc.origin, w.Code, tc.code)
		}
	}
}

func TestForeignHostRefused(t *testing.T) {
	for _, tc := range []struct{ addr, host string }{
		{uiHost, "evil.example"},
		{uiHost, "evil.example:8090"},
		{uiHost, "localhost.evil.example:8090"},
		{uiHost, "127.0.0.1.evil.example"},
		{uiHost, "0.0.0.0:8090"},
		{uiHost, ""},
		{":8090", ""},
		{":8090", "192.168.1.5:8090"},
		{"0.0.0.0:8090", "192.168.1.5:8090"},
	} {
		cfg, h := guardServer(tc.addr)
		for _, r := range []struct{ method, path, body string }{
			{http.MethodGet, "/api/config", ""},
			{http.MethodGet, "/api/services", ""},
			{http.MethodGet, "/", ""},
			{http.MethodGet, "/ws", ""},
			{http.MethodPost, "/api/config/envs/add", envAddBody},
			{http.MethodPost, "/mcp", mcpInitBody},
		} {
			w := serve(h, r.method, r.path, tc.host, "", r.body, "Content-Type", "application/json", "Accept", "application/json, text/event-stream")
			if w.Code != http.StatusForbidden {
				t.Errorf("addr %q host %q %s %s: %d, want 403", tc.addr, tc.host, r.method, r.path, w.Code)
			}
			if strings.Contains(w.Body.String(), "project_name") {
				t.Errorf("addr %q host %q %s: config revealed", tc.addr, tc.host, r.path)
			}
		}
		if len(cfg.envVars) != 0 || cfg.saved {
			t.Errorf("addr %q host %q changed config", tc.addr, tc.host)
		}
	}
}

func TestOwnOriginAllowed(t *testing.T) {
	for _, tc := range []struct{ addr, host, origin string }{
		{uiHost, uiHost, ""},
		{uiHost, uiHost, "http://" + uiHost},
		{uiHost, "localhost:8090", "http://localhost:8090"},
		{uiHost, "LocalHost:8090", ""},
		{uiHost, "localhost", ""},
		{uiHost, "[::1]:8090", "http://[::1]:8090"},
		{uiHost, "127.0.0.2:8090", ""},
		{uiHost, "localhost:5173", "http://localhost:5173"},
		{"devbox.lan:8090", "devbox.lan:8090", "http://devbox.lan:8090"},
		{"192.168.1.5:8090", "192.168.1.5:8090", ""},
	} {
		cfg, h := guardServer(tc.addr)
		if w := serve(h, http.MethodGet, "/api/config", tc.host, tc.origin, ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "project_name") {
			t.Errorf("%+v: GET config %d", tc, w.Code)
		}
		if w := serve(h, http.MethodPost, "/api/config/envs/add", tc.host, tc.origin, envAddBody); w.Code != http.StatusOK || cfg.envVars["K"] != "V" {
			t.Errorf("%+v: POST env %d", tc, w.Code)
		}
		if w := serve(h, http.MethodPost, "/mcp", tc.host, tc.origin, mcpInitBody, "Content-Type", "application/json", "Accept", "application/json, text/event-stream"); w.Code != http.StatusOK {
			t.Errorf("%+v: POST mcp %d: %s", tc, w.Code, w.Body.String())
		}
	}
}

func dialWS(t *testing.T, h http.Handler) func(origin string) (int, error) {
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	u := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	return func(origin string) (int, error) {
		hdr := http.Header{}
		if origin == "self" {
			origin = srv.URL
		}
		if origin != "" {
			hdr.Set("Origin", origin)
		}
		c, resp, err := websocket.DefaultDialer.Dial(u, hdr)
		if c != nil {
			c.Close()
		}
		if resp == nil {
			return 0, err
		}
		return resp.StatusCode, err
	}
}

func checkWSOrigins(t *testing.T, dial func(string) (int, error)) {
	for _, tc := range []struct {
		origin string
		code   int
	}{
		{"", http.StatusSwitchingProtocols},
		{"self", http.StatusSwitchingProtocols},
		{foreignOrigin, http.StatusForbidden},
		{"null", http.StatusForbidden},
	} {
		if code, err := dial(tc.origin); code != tc.code {
			t.Errorf("origin %q: %d %v, want %d", tc.origin, code, err, tc.code)
		}
	}
}

func TestWebSocketOrigin(t *testing.T) {
	_, h := guardServer(uiHost)
	checkWSOrigins(t, dialWS(t, h))
}

func TestUpgraderCheckOrigin(t *testing.T) {
	s := testServer()
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.hub.ServeWS)
	checkWSOrigins(t, dialWS(t, mux))
}
