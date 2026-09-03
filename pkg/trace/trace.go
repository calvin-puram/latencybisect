package trace

import (
	"encoding/json"
	"fmt"
	"io"
)

// Span is one unit of work in a trace. Times are unix nanos, matching the
// OTel wire format.
type Span struct {
	TraceID      string `json:"traceId"`
	SpanID       string `json:"spanId"`
	ParentSpanID string `json:"parentSpanId"`
	Name         string `json:"name"`
	StartNano    int64  `json:"startNano"`
	EndNano      int64  `json:"endNano"`
}

func (s Span) Duration() int64 {
	return s.EndNano - s.StartNano
}

// Trace is a single request. Spans form a tree via ParentSpanID; the root
// has an empty one.
type Trace struct {
	TraceID string `json:"traceId"`
	Spans   []Span `json:"spans"`
}

// Parse decodes a JSON array of traces.
func Parse(r io.Reader) ([]Trace, error) {
	var traces []Trace
	if err := json.NewDecoder(r).Decode(&traces); err != nil {
		return nil, fmt.Errorf("decode traces: %w", err)
	}
	for i, t := range traces {
		if t.TraceID == "" {
			return nil, fmt.Errorf("trace %d: missing traceId", i)
		}
		for j := range traces[i].Spans {
			traces[i].Spans[j].TraceID = t.TraceID
		}
	}
	return traces, nil
}
