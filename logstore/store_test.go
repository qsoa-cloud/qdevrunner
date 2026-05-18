package logstore

import (
	"sync"
	"testing"
	"time"
)

func entry(service, stream, text string) LogEntry {
	return LogEntry{
		Service:   service,
		Stream:    stream,
		Text:      text,
		Timestamp: time.Now(),
	}
}

func TestNewStore(t *testing.T) {
	s := NewStore(100)
	if s.maxSize != 100 {
		t.Errorf("expected maxSize 100, got %d", s.maxSize)
	}
}

func TestNewStoreDefault(t *testing.T) {
	s := NewStore(0)
	if s.maxSize != 50000 {
		t.Errorf("expected default 50000, got %d", s.maxSize)
	}
}

func TestAddAndRecent(t *testing.T) {
	s := NewStore(100)

	s.Add(entry("svc-a", "stdout", "line 1"))
	s.Add(entry("svc-b", "stderr", "line 2"))
	s.Add(entry("svc-a", "stdout", "line 3"))

	entries := s.Recent(10, "")
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	// Chronological order.
	if entries[0].Text != "line 1" {
		t.Errorf("expected first entry 'line 1', got %q", entries[0].Text)
	}
	if entries[2].Text != "line 3" {
		t.Errorf("expected last entry 'line 3', got %q", entries[2].Text)
	}
}

func TestRecentFilterByService(t *testing.T) {
	s := NewStore(100)

	s.Add(entry("svc-a", "stdout", "a1"))
	s.Add(entry("svc-b", "stdout", "b1"))
	s.Add(entry("svc-a", "stdout", "a2"))

	entries := s.Recent(10, "svc-a")
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries for svc-a, got %d", len(entries))
	}
	for _, e := range entries {
		if e.Service != "svc-a" {
			t.Errorf("expected service svc-a, got %q", e.Service)
		}
	}
}

func TestRecentLimit(t *testing.T) {
	s := NewStore(100)

	for i := 0; i < 20; i++ {
		s.Add(entry("svc", "stdout", "line"))
	}

	entries := s.Recent(5, "")
	if len(entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(entries))
	}
}

func TestRingBufferWrap(t *testing.T) {
	s := NewStore(5)

	for i := 0; i < 10; i++ {
		s.Add(entry("svc", "stdout", "line"))
	}

	if s.count != 5 {
		t.Errorf("expected count 5, got %d", s.count)
	}
}

func TestSubscribe(t *testing.T) {
	s := NewStore(100)
	id, ch := s.Subscribe(10)

	s.Add(entry("svc", "stdout", "hello"))

	select {
	case e := <-ch:
		if e.Text != "hello" {
			t.Errorf("expected 'hello', got %q", e.Text)
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
				s.Add(entry("svc", "stdout", "line"))
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
