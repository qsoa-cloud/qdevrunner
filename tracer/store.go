package tracer

import (
	"sync"
	"time"
)

// SpanRecord wraps a span with metadata about which service produced it.
type SpanRecord struct {
	Service    string    `json:"service"`
	ReceivedAt time.Time `json:"received_at"`
	Span       Span      `json:"span"`
}

// TraceInfo is a summary of a trace for listing.
type TraceInfo struct {
	TraceID       uint64    `json:"trace_id"`
	RootOperation string    `json:"root_operation"`
	Services      []string  `json:"services"`
	SpanCount     int       `json:"span_count"`
	StartTime     time.Time `json:"start_time"`
	Duration      time.Duration `json:"duration"`
}

// Store is an in-memory ring buffer for trace spans.
type Store struct {
	mu          sync.RWMutex
	spans       []SpanRecord
	head        int
	count       int
	maxSize     int
	byTraceID   map[uint64][]int // traceID -> indices in spans
	subscribers map[int]chan SpanRecord
	nextSubID   int
}

func NewStore(maxSize int) *Store {
	if maxSize <= 0 {
		maxSize = 10000
	}
	return &Store{
		spans:       make([]SpanRecord, maxSize),
		maxSize:     maxSize,
		byTraceID:   make(map[uint64][]int),
		subscribers: make(map[int]chan SpanRecord),
	}
}

// Add inserts a span and broadcasts it to subscribers.
func (s *Store) Add(record SpanRecord) {
	s.mu.Lock()

	idx := s.head
	s.spans[idx] = record
	s.head = (s.head + 1) % s.maxSize
	if s.count < s.maxSize {
		s.count++
	}

	traceID := record.Span.Ctx.TraceID
	s.byTraceID[traceID] = append(s.byTraceID[traceID], idx)

	// Copy subscribers for broadcast outside lock.
	subs := make([]chan SpanRecord, 0, len(s.subscribers))
	for _, ch := range s.subscribers {
		subs = append(subs, ch)
	}
	s.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- record:
		default:
			// Drop if subscriber is slow.
		}
	}
}

// GetTrace returns all spans for a given traceID.
func (s *Store) GetTrace(traceID uint64) []SpanRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	indices, ok := s.byTraceID[traceID]
	if !ok {
		return nil
	}

	result := make([]SpanRecord, 0, len(indices))
	for _, idx := range indices {
		if idx < s.count {
			result = append(result, s.spans[idx])
		}
	}
	return result
}

// ListTraces returns recent trace summaries.
func (s *Store) ListTraces(offset, limit int) []TraceInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Collect unique traces.
	seen := map[uint64]bool{}
	var traceIDs []uint64

	// Walk backwards from most recent.
	for i := 0; i < s.count; i++ {
		idx := (s.head - 1 - i + s.maxSize) % s.maxSize
		tid := s.spans[idx].Span.Ctx.TraceID
		if !seen[tid] {
			seen[tid] = true
			traceIDs = append(traceIDs, tid)
		}
	}

	// Apply offset/limit.
	if offset >= len(traceIDs) {
		return nil
	}
	end := offset + limit
	if end > len(traceIDs) {
		end = len(traceIDs)
	}
	traceIDs = traceIDs[offset:end]

	// Build trace info for each.
	result := make([]TraceInfo, 0, len(traceIDs))
	for _, tid := range traceIDs {
		indices := s.byTraceID[tid]
		if len(indices) == 0 {
			continue
		}

		serviceSet := map[string]bool{}
		var rootOp string
		var earliest, latest time.Time

		for _, idx := range indices {
			if idx >= s.count {
				continue
			}
			rec := s.spans[idx]
			serviceSet[rec.Service] = true

			if rec.Span.Ctx.ParentSpanId == 0 && rootOp == "" {
				rootOp = rec.Span.Operation
			}

			if earliest.IsZero() || rec.Span.StartTime.Before(earliest) {
				earliest = rec.Span.StartTime
			}
			if rec.Span.FinishTime.After(latest) {
				latest = rec.Span.FinishTime
			}
		}

		services := make([]string, 0, len(serviceSet))
		for svc := range serviceSet {
			services = append(services, svc)
		}

		result = append(result, TraceInfo{
			TraceID:       tid,
			RootOperation: rootOp,
			Services:      services,
			SpanCount:     len(indices),
			StartTime:     earliest,
			Duration:      latest.Sub(earliest),
		})
	}

	return result
}

// RecentSpans returns the last n span records in chronological order.
func (s *Store) RecentSpans(n int) []SpanRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if n > s.count {
		n = s.count
	}
	result := make([]SpanRecord, n)
	for i := 0; i < n; i++ {
		idx := (s.head - n + i + s.maxSize) % s.maxSize
		result[i] = s.spans[idx]
	}
	return result
}

// Subscribe returns a channel that receives new spans. Call Unsubscribe to stop.
func (s *Store) Subscribe(bufSize int) (int, <-chan SpanRecord) {
	if bufSize <= 0 {
		bufSize = 100
	}
	ch := make(chan SpanRecord, bufSize)

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
