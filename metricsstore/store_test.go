package metricsstore

import (
	"sync"
	"testing"
	"time"
)

func snapshot(service string, metrics ...MetricValue) Snapshot {
	return Snapshot{
		Service:   service,
		Timestamp: time.Now(),
		Metrics:   metrics,
	}
}

func counter(name string, value float64) MetricValue {
	return MetricValue{Name: name, Type: MetricCounter, Value: value}
}

func TestNewStore(t *testing.T) {
	s := NewStore(50)
	if s.maxSize != 50 {
		t.Errorf("expected maxSize 50, got %d", s.maxSize)
	}
}

func TestNewStoreDefault(t *testing.T) {
	s := NewStore(0)
	if s.maxSize != 1000 {
		t.Errorf("expected default 1000, got %d", s.maxSize)
	}
}

func TestAddAndRecent(t *testing.T) {
	s := NewStore(100)

	s.Add(snapshot("svc-a", counter("req_total", 10)))
	s.Add(snapshot("svc-b", counter("req_total", 20)))
	s.Add(snapshot("svc-a", counter("req_total", 30)))

	results := s.Recent(10, "")
	if len(results) != 3 {
		t.Fatalf("expected 3, got %d", len(results))
	}
}

func TestRecentFilterByService(t *testing.T) {
	s := NewStore(100)

	s.Add(snapshot("svc-a", counter("m", 1)))
	s.Add(snapshot("svc-b", counter("m", 2)))
	s.Add(snapshot("svc-a", counter("m", 3)))

	results := s.Recent(10, "svc-b")
	if len(results) != 1 {
		t.Fatalf("expected 1, got %d", len(results))
	}
	if results[0].Metrics[0].Value != 2 {
		t.Errorf("expected value 2, got %f", results[0].Metrics[0].Value)
	}
}

func TestRecentLimit(t *testing.T) {
	s := NewStore(100)
	for i := 0; i < 20; i++ {
		s.Add(snapshot("svc", counter("m", float64(i))))
	}

	results := s.Recent(3, "")
	if len(results) != 3 {
		t.Fatalf("expected 3, got %d", len(results))
	}
}

func TestRingBufferWrap(t *testing.T) {
	s := NewStore(5)
	for i := 0; i < 10; i++ {
		s.Add(snapshot("svc", counter("m", float64(i))))
	}
	if s.count != 5 {
		t.Errorf("expected count 5, got %d", s.count)
	}
}

func TestSubscribe(t *testing.T) {
	s := NewStore(100)
	id, ch := s.Subscribe(10)

	s.Add(snapshot("svc", counter("m", 42)))

	select {
	case snap := <-ch:
		if snap.Metrics[0].Value != 42 {
			t.Errorf("expected 42, got %f", snap.Metrics[0].Value)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}

	s.Unsubscribe(id)
	_, ok := <-ch
	if ok {
		t.Error("channel should be closed")
	}
}

func TestConcurrentAccess(t *testing.T) {
	s := NewStore(1000)
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				s.Add(snapshot("svc", counter("m", 1)))
			}
		}()
	}

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				s.Recent(10, "")
			}
		}()
	}

	wg.Wait()
}
