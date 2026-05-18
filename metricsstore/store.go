package metricsstore

import (
	"sync"
	"time"
)

// MetricType identifies the kind of metric.
type MetricType string

const (
	MetricCounter MetricType = "counter"
	MetricSummary MetricType = "summary"
	MetricGauge   MetricType = "gauge"
)

// MetricValue is a single metric data point.
type MetricValue struct {
	Name   string            `json:"name"`
	Type   MetricType        `json:"type"`
	Labels map[string]string `json:"labels,omitempty"`
	Value  float64           `json:"value"`
	// For summary type.
	Sum   float64 `json:"sum,omitempty"`
	Count uint64  `json:"count,omitempty"`
}

// Snapshot represents a collection of metrics at a point in time.
type Snapshot struct {
	Service   string        `json:"service"`
	Timestamp time.Time     `json:"timestamp"`
	Metrics   []MetricValue `json:"metrics"`
}

// Store is an in-memory ring buffer for metrics snapshots.
type Store struct {
	mu          sync.RWMutex
	entries     []Snapshot
	head        int
	count       int
	maxSize     int
	subscribers map[int]chan Snapshot
	nextSubID   int
}

func NewStore(maxSize int) *Store {
	if maxSize <= 0 {
		maxSize = 1000
	}
	return &Store{
		entries:     make([]Snapshot, maxSize),
		maxSize:     maxSize,
		subscribers: make(map[int]chan Snapshot),
	}
}

// Add inserts a snapshot and broadcasts it.
func (s *Store) Add(snapshot Snapshot) {
	s.mu.Lock()
	s.entries[s.head] = snapshot
	s.head = (s.head + 1) % s.maxSize
	if s.count < s.maxSize {
		s.count++
	}

	subs := make([]chan Snapshot, 0, len(s.subscribers))
	for _, ch := range s.subscribers {
		subs = append(subs, ch)
	}
	s.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- snapshot:
		default:
		}
	}
}

// Recent returns the last n snapshots for a service.
func (s *Store) Recent(n int, service string) []Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]Snapshot, 0, n)
	for i := 0; i < s.count && len(result) < n; i++ {
		idx := (s.head - 1 - i + s.maxSize) % s.maxSize
		entry := s.entries[idx]
		if service == "" || entry.Service == service {
			result = append(result, entry)
		}
	}

	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result
}

// Subscribe returns a channel that receives new snapshots.
func (s *Store) Subscribe(bufSize int) (int, <-chan Snapshot) {
	if bufSize <= 0 {
		bufSize = 64
	}
	ch := make(chan Snapshot, bufSize)

	s.mu.Lock()
	id := s.nextSubID
	s.nextSubID++
	s.subscribers[id] = ch
	s.mu.Unlock()

	return id, ch
}

// Unsubscribe removes a subscriber.
func (s *Store) Unsubscribe(id int) {
	s.mu.Lock()
	if ch, ok := s.subscribers[id]; ok {
		close(ch)
		delete(s.subscribers, id)
	}
	s.mu.Unlock()
}
