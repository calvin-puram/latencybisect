package trace

import (
	"encoding/json"
	"fmt"
	"io"
)

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

type Trace struct {
	TraceID string `json:"traceId"`
	Spans   []Span `json:"spans"`
}

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
