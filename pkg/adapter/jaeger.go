package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/calvinpuram/latencybisect/pkg/trace"
)

type Jaeger struct {
	BaseURL string
	Client  *http.Client
}

func NewJaeger(baseURL string) *Jaeger {
	return &Jaeger{
		BaseURL: baseURL,
		Client:  &http.Client{Timeout: 30 * time.Second},
	}
}

type jaegerResponse struct {
	Data []struct {
		TraceID   string `json:"traceID"`
		Spans     []jaegerSpan
		Processes map[string]struct {
			ServiceName string `json:"serviceName"`
		} `json:"processes"`
	} `json:"data"`
}

type jaegerSpan struct {
	TraceID       string `json:"traceID"`
	SpanID        string `json:"spanID"`
	OperationName string `json:"operationName"`
	StartTime     int64  `json:"startTime"`
	Duration      int64  `json:"duration"`
	ProcessID     string `json:"processID"`
	References    []struct {
		RefType string `json:"refType"`
		SpanID  string `json:"spanID"`
		TraceID string `json:"traceID"`
	} `json:"references"`
}

func (j *Jaeger) Fetch(ctx context.Context, service string, start, end time.Time, limit int) ([]trace.Trace, error) {
	if j.BaseURL == "" {
		return nil, fmt.Errorf("jaeger: no base url")
	}
	if service == "" {
		return nil, fmt.Errorf("jaeger: service is required")
	}

	u, err := url.Parse(j.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("jaeger: bad base url: %w", err)
	}
	u.Path = "/api/traces"
	q := url.Values{}
	q.Set("service", service)
	q.Set("start", strconv.FormatInt(start.UnixMicro(), 10))
	q.Set("end", strconv.FormatInt(end.UnixMicro(), 10))
	q.Set("limit", strconv.Itoa(limit))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("accept", "application/json")

	client := j.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jaeger: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("jaeger: status %d: %s", resp.StatusCode, body)
	}

	var jr jaegerResponse
	if err := json.NewDecoder(resp.Body).Decode(&jr); err != nil {
		return nil, fmt.Errorf("jaeger: decode: %w", err)
	}

	traces := make([]trace.Trace, 0, len(jr.Data))
	for _, d := range jr.Data {
		spans := make([]trace.Span, 0, len(d.Spans))
		for _, s := range d.Spans {
			name := s.OperationName
			if p, ok := d.Processes[s.ProcessID]; ok && p.ServiceName != "" {
				name = p.ServiceName + ":" + s.OperationName
			}
			spans = append(spans, trace.Span{
				TraceID:      d.TraceID,
				SpanID:       s.SpanID,
				ParentSpanID: parentOf(s),
				Name:         name,
				StartNano:    s.StartTime * 1000,
				EndNano:      (s.StartTime + s.Duration) * 1000,
			})
		}
		traces = append(traces, trace.Trace{TraceID: d.TraceID, Spans: spans})
	}

	return traces, nil
}

func parentOf(s jaegerSpan) string {
	for _, r := range s.References {
		if r.RefType == "CHILD_OF" && r.TraceID == s.TraceID {
			return r.SpanID
		}
	}
	return ""
}
