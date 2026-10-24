package clickhouse

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"maps"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.qsoa.cloud/qdevrunner/tracer"
)

const (
	dsnEvents = "clickhouse://owner:pw-events@127.0.0.1:9000/real_events"
	dsnLogs   = "clickhouse://owner:pw-logs@127.0.0.1:9000/real_logs"
)

func TestLookup(t *testing.T) {
	one := map[string]string{"events": dsnEvents}
	two := map[string]string{"events": dsnEvents, "logs": dsnLogs}
	for _, tc := range []struct {
		dsns map[string]string
		db   string
		want string
	}{
		{one, "events", dsnEvents},
		{one, "real_events", ""},
		{one, "other", ""},
		{one, "", dsnEvents},
		{two, "logs", dsnLogs},
		{two, "", ""},
		{map[string]string{"": dsnLogs, "events": dsnEvents}, "", ""},
		{nil, "", ""},
		{nil, "events", ""},
	} {
		tr := NewFromRunner("svc", "", func() map[string]string { return tc.dsns }, nil)
		got, err := tr.lookup(context.Background(), tc.db)
		if err != nil {
			t.Fatalf("lookup(%q): %v", tc.db, err)
		}
		if got != tc.want {
			t.Errorf("lookup(%q) with %d databases: wrong DSN", tc.db, len(tc.dsns))
		}
	}
}

func TestObserveSpan(t *testing.T) {
	store := tracer.NewStore(10)
	tr := NewFromRunner("svc", "", func() map[string]string { return map[string]string{"events": dsnEvents} }, store)
	tr.observe("events", "ok", 5*time.Millisecond)

	spans := store.RecentSpans(10)
	if len(spans) != 1 {
		t.Fatalf("got %d spans", len(spans))
	}
	rec := spans[0]
	if rec.Service != "svc" {
		t.Errorf("service %q", rec.Service)
	}
	if rec.Span.Operation != "clickhouse connect events" {
		t.Errorf("operation %q", rec.Span.Operation)
	}
	if len(rec.Span.Tags) != 2 || rec.Span.Tags["db"] != "events" || rec.Span.Tags["result"] != "ok" {
		t.Errorf("tags %v", rec.Span.Tags)
	}
	if d := rec.Span.FinishTime.Sub(rec.Span.StartTime); d != 5*time.Millisecond {
		t.Errorf("duration %v", d)
	}
	if rec.Span.Ctx.TraceID == 0 || rec.Span.Ctx.SpanID == 0 {
		t.Errorf("ctx %+v", rec.Span.Ctx)
	}
	data, _ := json.Marshal(rec)
	for _, s := range []string{"pw-events", "owner", "real_events", "clickhouse://"} {
		if strings.Contains(string(data), s) {
			t.Errorf("span contains %q", s)
		}
	}

	NewFromRunner("svc", "", nil, nil).observe("events", "ok", time.Millisecond)
}

func TestObserveHidesUnknownNames(t *testing.T) {
	store := tracer.NewStore(10)
	reads := 0
	tr := NewFromRunner("svc", "", func() map[string]string { reads++; return map[string]string{"events": dsnEvents} }, store)
	long := strings.Repeat("x", 4096)
	quoted := `ev"ents`
	tr.observe(long, "unknown_db", time.Millisecond)
	tr.observe(quoted, "unknown_db", time.Millisecond)
	tr.observe("", "handshake", time.Millisecond)
	if reads != 2 {
		t.Errorf("config read %d times, want 2", reads)
	}

	spans := store.RecentSpans(10)
	if len(spans) != 3 {
		t.Fatalf("got %d spans", len(spans))
	}
	ops := map[string]int{}
	for _, rec := range spans {
		ops[rec.Span.Operation+"|"+rec.Span.Tags["db"].(string)]++
		data, _ := json.Marshal(rec)
		if strings.Contains(string(data), long[:64]) || strings.Contains(string(data), `ev\"ents`) {
			t.Error("span contains the raw database name")
		}
	}
	if want := map[string]int{"clickhouse connect <invalid>|<invalid>": 2, "clickhouse connect |": 1}; !maps.Equal(ops, want) {
		for k, n := range ops {
			t.Errorf("span %.60q x%d", k, n)
		}
	}
}

func appendString(b []byte, s string) []byte {
	return append(binary.AppendUvarint(b, uint64(len(s))), s...)
}

func helloBytes(db string) []byte {
	b := appendString([]byte{0}, "test")
	b = binary.AppendUvarint(b, 24)
	b = binary.AppendUvarint(b, 8)
	b = binary.AppendUvarint(b, 54460)
	return appendString(appendString(appendString(b, db), ""), "")
}

func readString(r *bufio.Reader) string {
	n, _ := binary.ReadUvarint(r)
	b := make([]byte, min(n, 4096))
	io.ReadFull(r, b)
	return string(b)
}

func dial(t *testing.T, sock string) net.Conn {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		c, err := net.Dial("unix", sock)
		if err == nil {
			t.Cleanup(func() { c.Close() })
			c.SetDeadline(time.Now().Add(5 * time.Second))
			return c
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestServe(t *testing.T) {
	up, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer up.Close()
	got := make(chan [3]string, 1)
	go func() {
		c, err := up.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		r := bufio.NewReader(c)
		if _, err := binary.ReadUvarint(r); err != nil {
			return
		}
		readString(r)
		for range 3 {
			binary.ReadUvarint(r)
		}
		got <- [3]string{readString(r), readString(r), readString(r)}
	}()

	sock := filepath.Join(t.TempDir(), "clickhouse.sock")
	store := tracer.NewStore(10)
	dsns := map[string]string{"events": "clickhouse://owner:pw-events@" + up.Addr().String() + "/real_events"}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- NewFromRunner("svc", sock, func() map[string]string { return dsns }, store).Serve(ctx) }()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("Serve: %v", err)
		}
	}()

	c := dial(t, sock)
	fi, err := os.Stat(sock)
	if err != nil {
		t.Fatal(err)
	}
	if m := fi.Mode().Perm(); m != 0o600 {
		t.Errorf("socket mode %o, want 600", m)
	}
	if _, err := c.Write(helloBytes("events")); err != nil {
		t.Fatal(err)
	}
	select {
	case h := <-got:
		if h != [3]string{"real_events", "owner", "pw-events"} {
			t.Errorf("upstream hello database %q", h[0])
		}
	case <-time.After(5 * time.Second):
		t.Fatal("upstream got no hello")
	}

	c = dial(t, sock)
	if _, err := c.Write(helloBytes("nope")); err != nil {
		t.Fatal(err)
	}
	r := bufio.NewReader(c)
	if p, err := binary.ReadUvarint(r); err != nil || p != 2 {
		t.Fatalf("packet %d, %v", p, err)
	}
	var code int32
	if err := binary.Read(r, binary.LittleEndian, &code); err != nil || code != 81 {
		t.Fatalf("code %d, %v", code, err)
	}

	results := map[string]string{}
	for range 50 {
		for _, s := range store.RecentSpans(10) {
			results[s.Span.Tags["db"].(string)] = s.Span.Tags["result"].(string)
		}
		if len(results) == 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if results["events"] != "ok" || results["<invalid>"] != "unknown_db" {
		t.Errorf("span results %v", results)
	}
}
