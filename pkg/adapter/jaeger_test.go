package adapter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/calvinpuram/latencybisect/pkg/trace"
)

const jaegerPayload = `{
  "data": [
    {
      "traceID": "abc123",
      "spans": [
        {
          "traceID": "abc123",
          "spanID": "s1",
          "operationName": "checkout",
          "startTime": 1000000,
          "duration": 21000,
          "processID": "p1",
          "references": []
        },
        {
          "traceID": "abc123",
          "spanID": "s2",
          "operationName": "db.query",
          "startTime": 1001000,
          "duration": 8000,
          "processID": "p2",
          "references": [{"refType": "CHILD_OF", "traceID": "abc123", "spanID": "s1"}]
        }
      ],
      "processes": {
        "p1": {"serviceName": "checkout-api"},
        "p2": {"serviceName": "inventory-service"}
      }
    }
  ]
}`

func newTestJaeger(t *testing.T, handler http.HandlerFunc) *Jaeger {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewJaeger(srv.URL)
}

func TestFetchConvertsSpans(t *testing.T) {
	j := newTestJaeger(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(jaegerPayload))
	})

	traces, err := j.Fetch(context.Background(), "checkout-api", time.Now().Add(-time.Hour), time.Now(), 100)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(traces) != 1 {
		t.Fatalf("got %d traces, want 1", len(traces))
	}

	root, err := trace.BuildTree(traces[0])
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	if root.Span.Name != "checkout-api:checkout" {
		t.Errorf("root name = %q, want service-qualified checkout-api:checkout", root.Span.Name)
	}
	if got := root.Span.Duration(); got != 21_000_000 {
		t.Errorf("root duration = %d nanos, want 21000000 (21ms from 21000 micros)", got)
	}
	if got := root.Span.StartNano; got != 1_000_000_000 {
		t.Errorf("root start = %d nanos, want 1000000000", got)
	}

	if len(root.Children) != 1 {
		t.Fatalf("root has %d children, want 1", len(root.Children))
	}
	child := root.Children[0]
	if child.Span.Name != "inventory-service:db.query" {
		t.Errorf("child name = %q, want inventory-service:db.query", child.Span.Name)
	}
	if got := child.Span.Duration(); got != 8_000_000 {
		t.Errorf("child duration = %d nanos, want 8000000", got)
	}
	if got := root.SelfTime(); got != 13_000_000 {
		t.Errorf("root self time = %d nanos, want 13000000", got)
	}
}

func TestFetchSendsQueryParams(t *testing.T) {
	var got string
	j := newTestJaeger(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.String()
		w.Write([]byte(`{"data":[]}`))
	})

	start := time.Unix(1700000000, 0)
	end := start.Add(time.Hour)
	if _, err := j.Fetch(context.Background(), "checkout-api", start, end, 50); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	for _, want := range []string{
		"/api/traces?",
		"service=checkout-api",
		"start=1700000000000000",
		"end=1700003600000000",
		"limit=50",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("request %q missing %q", got, want)
		}
	}
}

func TestFetchUnqualifiedWhenProcessUnknown(t *testing.T) {
	payload := `{"data":[{"traceID":"t1","spans":[
      {"traceID":"t1","spanID":"s1","operationName":"orphan.op","startTime":1000,"duration":500,"processID":"missing","references":[]}
    ],"processes":{}}]}`

	j := newTestJaeger(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(payload))
	})

	traces, err := j.Fetch(context.Background(), "svc", time.Now(), time.Now(), 10)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got := traces[0].Spans[0].Name; got != "orphan.op" {
		t.Errorf("name = %q, want bare operation name", got)
	}
}

func TestFetchIgnoresFollowsFromReference(t *testing.T) {
	payload := `{"data":[{"traceID":"t1","spans":[
      {"traceID":"t1","spanID":"s1","operationName":"root","startTime":1000,"duration":500,"processID":"p1","references":[]},
      {"traceID":"t1","spanID":"s2","operationName":"async","startTime":1100,"duration":100,"processID":"p1",
       "references":[{"refType":"FOLLOWS_FROM","traceID":"t1","spanID":"s1"}]}
    ],"processes":{"p1":{"serviceName":"svc"}}}]}`

	j := newTestJaeger(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(payload))
	})

	traces, err := j.Fetch(context.Background(), "svc", time.Now(), time.Now(), 10)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	for _, s := range traces[0].Spans {
		if s.SpanID == "s2" && s.ParentSpanID != "" {
			t.Errorf("FOLLOWS_FROM treated as parent: %q", s.ParentSpanID)
		}
	}
}

func TestFetchErrors(t *testing.T) {
	t.Run("http error", func(t *testing.T) {
		j := newTestJaeger(t, func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		})
		_, err := j.Fetch(context.Background(), "svc", time.Now(), time.Now(), 10)
		if err == nil || !strings.Contains(err.Error(), "status 500") {
			t.Fatalf("err = %v, want status 500", err)
		}
	})

	t.Run("bad json", func(t *testing.T) {
		j := newTestJaeger(t, func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("not json"))
		})
		_, err := j.Fetch(context.Background(), "svc", time.Now(), time.Now(), 10)
		if err == nil || !strings.Contains(err.Error(), "decode") {
			t.Fatalf("err = %v, want decode error", err)
		}
	})

	t.Run("missing service", func(t *testing.T) {
		j := NewJaeger("http://example.invalid")
		_, err := j.Fetch(context.Background(), "", time.Now(), time.Now(), 10)
		if err == nil || !strings.Contains(err.Error(), "service is required") {
			t.Fatalf("err = %v, want service required", err)
		}
	})
}

var _ Source = (*Jaeger)(nil)
