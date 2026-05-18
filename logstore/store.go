package logstore

import (
	"sync"
	"time"
)

// LogEntry represents a single log line from a service.
type LogEntry struct {
	Service   string    `json:"service"`
	Stream    string    `json:"stream"` // "stdout" or "stderr"
	Text      string    `json:"text"`
	Timestamp time.Time `json:"timestamp"`
}

// Store is an in-memory ring buffer for service log lines.
type Store struct {
	mu          sync.RWMutex
	entries     []LogEntry
	head        int
	count       int
	maxSize     int
	subscribers map[int]chan LogEntry
	nextSubID   int
}

func NewStore(maxSize int) *Store {
	if maxSize <= 0 {
		maxSize = 50000
	}
	return &Store{
		entries:     make([]LogEntry, maxSize),
		maxSize:     maxSize,
		subscribers: make(map[int]chan LogEntry),
	}
}

// Add inserts a log entry and broadcasts it.
func (s *Store) Add(entry LogEntry) {
	s.mu.Lock()
	s.entries[s.head] = entry
	s.head = (s.head + 1) % s.maxSize
	if s.count < s.maxSize {
		s.count++
	}

	subs := make([]chan LogEntry, 0, len(s.subscribers))
	for _, ch := range s.subscribers {
		subs = append(subs, ch)
	}
	s.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- entry:
		default:
		}
	}
}

// Recent returns the last n log entries, optionally filtered by service.
func (s *Store) Recent(n int, service string) []LogEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]LogEntry, 0, n)
	for i := 0; i < s.count && len(result) < n; i++ {
		idx := (s.head - 1 - i + s.maxSize) % s.maxSize
		entry := s.entries[idx]
		if service == "" || entry.Service == service {
			result = append(result, entry)
		}
	}

	// Reverse to chronological order.
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result
}

// Subscribe returns a channel that receives new log entries.
func (s *Store) Subscribe(bufSize int) (int, <-chan LogEntry) {
	if bufSize <= 0 {
		bufSize = 256
	}
	ch := make(chan LogEntry, bufSize)

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
