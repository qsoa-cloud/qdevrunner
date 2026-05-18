package tracer

import "time"

// Span matches the JSON format written by gopkg.qsoa.cloud/tracer.
// Services write these as JSON to the tracer FIFO.
type Span struct {
	Operation  string                 `json:"o"`
	StartTime  time.Time              `json:"s"`
	FinishTime time.Time              `json:"f"`
	Ctx        SpanContext            `json:"c"`
	Tags       map[string]interface{} `json:"t"`
	LogFields  []LogField             `json:"lf"`
}

type SpanContext struct {
	TraceID      uint64 `json:"t"`
	SpanID       uint64 `json:"s"`
	ParentSpanId uint64 `json:"p,omitempty"`
}

type LogField struct {
	K string `json:"k"`
	T string `json:"t"`
	V string `json:"v"`
}
