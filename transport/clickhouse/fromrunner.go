package clickhouse

import (
	"context"
	"math/rand/v2"
	"time"

	"gopkg.qsoa.cloud/chproxy"

	"gopkg.qsoa.cloud/qdevrunner/tracer"
	"gopkg.qsoa.cloud/qdevrunner/transport"
)

const invalidDb = "<invalid>"

// FromRunner serves configured ClickHouse databases to one service by config name.
type FromRunner struct {
	service string
	addr    string
	dsns    func() map[string]string
	store   *tracer.Store
}

func NewFromRunner(service, addr string, dsns func() map[string]string, store *tracer.Store) *FromRunner {
	return &FromRunner{service: service, addr: addr, dsns: dsns, store: store}
}

func (t *FromRunner) Serve(ctx context.Context) error {
	l, err := transport.CreateSocket(t.addr)
	if err != nil {
		return err
	}
	defer l.Close()
	return chproxy.Serve(ctx, l, t.lookup, t.observe)
}

func (t *FromRunner) lookup(_ context.Context, db string) (string, error) {
	dsns := t.dsns()
	if db != "" {
		return dsns[db], nil
	}
	if len(dsns) == 1 {
		for _, dsn := range dsns {
			return dsn, nil
		}
	}
	return "", nil
}

func (t *FromRunner) observe(db, result string, d time.Duration) {
	if t.store == nil {
		return
	}
	if db != "" {
		if _, ok := t.dsns()[db]; !ok {
			db = invalidDb
		}
	}
	name := "clickhouse connect"
	if db != "" {
		name += " " + db
	}
	now := time.Now()
	t.store.Add(tracer.SpanRecord{
		Service:    t.service,
		ReceivedAt: now,
		Span: tracer.Span{
			Operation:  name,
			StartTime:  now.Add(-d),
			FinishTime: now,
			Ctx:        tracer.SpanContext{TraceID: rand.Uint64() | 1, SpanID: rand.Uint64() | 1},
			Tags:       map[string]interface{}{"db": db, "result": result},
		},
	})
}
