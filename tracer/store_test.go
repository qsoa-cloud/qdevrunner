package tracer

import (
	"sync"
	"testing"
	"time"
)

func makeSpan(traceID, spanID uint64, service, operation string) SpanRecord {
	now := time.Now()
	return SpanRecord{
		Service:    service,
		ReceivedAt: now,
		Span: Span{
			Operation:  operation,
			StartTime:  now,
			FinishTime: now.Add(10 * time.Millisecond),
			Ctx:        SpanContext{TraceID: traceID, SpanID: spanID},
		},
	}
}

func makeChildSpan(traceID, spanID, parentID uint64, service, operation string) SpanRecord {
	r := makeSpan(traceID, spanID, service, operation)
	r.Span.Ctx.ParentSpanId = parentID
	return r
}

func TestNewStore(t *testing.T) {
	s := NewStore(100)
	if s == nil {
		t.Fatal("NewStore returned nil")
	}
	if s.maxSize != 100 {
		t.Errorf("expected maxSize 100, got %d", s.maxSize)
	}
}

func TestNewStoreDefaultSize(t *testing.T) {
	s := NewStore(0)
	if s.maxSize != 10000 {
		t.Errorf("expected default maxSize 10000, got %d", s.maxSize)
	}
}

func TestAddAndGetTrace(t *testing.T) {
	s := NewStore(100)

	s.Add(makeSpan(1, 10, "svc-a", "GET /users"))
	s.Add(makeSpan(1, 11, "svc-b", "SELECT users"))
	s.Add(makeSpan(2, 20, "svc-a", "GET /items"))

	spans := s.GetTrace(1)
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans for trace 1, got %d", len(spans))
	}

	spans = s.GetTrace(2)
	if len(spans) != 1 {
		t.Fatalf("expected 1 span for trace 2, got %d", len(spans))
	}

	spans = s.GetTrace(999)
	if spans != nil {
		t.Fatalf("expected nil for unknown trace, got %d spans", len(spans))
	}
}

func TestRingBufferWrap(t *testing.T) {
	s := NewStore(5)

	for i := uint64(1); i <= 10; i++ {
		s.Add(makeSpan(i, i*10, "svc", "op"))
	}

	if s.count != 5 {
		t.Errorf("expected count 5, got %d", s.count)
	}
}

func TestListTraces(t *testing.T) {
	s := NewStore(100)

	s.Add(makeSpan(1, 10, "svc-a", "root-op-1"))
	s.Add(makeChildSpan(1, 11, 10, "svc-b", "child-op"))
	s.Add(makeSpan(2, 20, "svc-a", "root-op-2"))
	s.Add(makeSpan(3, 30, "svc-c", "root-op-3"))

	traces := s.ListTraces(0, 10)
	if len(traces) != 3 {
		t.Fatalf("expected 3 traces, got %d", len(traces))
	}

	// Most recent first.
	if traces[0].TraceID != 3 {
		t.Errorf("expected first trace to be 3, got %d", traces[0].TraceID)
	}

	// Trace 1 has 2 spans.
	for _, tr := range traces {
		if tr.TraceID == 1 {
			if tr.SpanCount != 2 {
				t.Errorf("trace 1: expected 2 spans, got %d", tr.SpanCount)
			}
			if tr.RootOperation != "root-op-1" {
				t.Errorf("trace 1: expected root op 'root-op-1', got %q", tr.RootOperation)
			}
		}
	}
}

func TestListTracesPagination(t *testing.T) {
	s := NewStore(100)

	for i := uint64(1); i <= 5; i++ {
		s.Add(makeSpan(i, i*10, "svc", "op"))
	}

	traces := s.ListTraces(0, 2)
	if len(traces) != 2 {
		t.Fatalf("expected 2 traces, got %d", len(traces))
	}

	traces = s.ListTraces(2, 2)
	if len(traces) != 2 {
		t.Fatalf("expected 2 traces from offset 2, got %d", len(traces))
	}

	traces = s.ListTraces(4, 10)
	if len(traces) != 1 {
		t.Fatalf("expected 1 trace from offset 4, got %d", len(traces))
	}

	traces = s.ListTraces(10, 10)
	if len(traces) != 0 {
		t.Fatalf("expected 0 traces from offset 10, got %d", len(traces))
	}
}

func TestSubscribeUnsubscribe(t *testing.T) {
	s := NewStore(100)

	id, ch := s.Subscribe(10)

	s.Add(makeSpan(1, 10, "svc", "op"))

	select {
	case record := <-ch:
		if record.Span.Ctx.TraceID != 1 {
			t.Errorf("expected traceID 1, got %d", record.Span.Ctx.TraceID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for subscriber")
	}

	s.Unsubscribe(id)

	// Channel should be closed.
	_, ok := <-ch
	if ok {
		t.Error("expected channel to be closed")
	}
}

func TestMultipleSubscribers(t *testing.T) {
	s := NewStore(100)

	_, ch1 := s.Subscribe(10)
	_, ch2 := s.Subscribe(10)

	s.Add(makeSpan(1, 10, "svc", "op"))

	for i, ch := range []<-chan SpanRecord{ch1, ch2} {
		select {
		case r := <-ch:
			if r.Span.Ctx.TraceID != 1 {
				t.Errorf("subscriber %d: expected traceID 1", i)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timeout", i)
		}
	}
}

func TestConcurrentAccess(t *testing.T) {
	s := NewStore(1000)
	var wg sync.WaitGroup

	// Concurrent writers.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				s.Add(makeSpan(uint64(id*1000+j), uint64(j), "svc", "op"))
			}
		}(i)
	}

	// Concurrent readers.
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				s.ListTraces(0, 10)
				s.GetTrace(1)
			}
		}()
	}

	wg.Wait()
}
