package adapter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/calvinpuram/latencybisect/internal/synth"
	"github.com/calvinpuram/latencybisect/pkg/bisect"
	"github.com/calvinpuram/latencybisect/pkg/sample"
	"github.com/calvinpuram/latencybisect/pkg/stats"
	"github.com/calvinpuram/latencybisect/pkg/trace"
)

type wireSpan struct {
	TraceID       string    `json:"traceID"`
	SpanID        string    `json:"spanID"`
	OperationName string    `json:"operationName"`
	StartTime     int64     `json:"startTime"`
	Duration      int64     `json:"duration"`
	ProcessID     string    `json:"processID"`
	References    []wireRef `json:"references"`
}

type wireRef struct {
	RefType string `json:"refType"`
	TraceID string `json:"traceID"`
	SpanID  string `json:"spanID"`
}

type wireProcess struct {
	ServiceName string `json:"serviceName"`
}

type wireTrace struct {
	TraceID   string                 `json:"traceID"`
	Spans     []wireSpan             `json:"spans"`
	Processes map[string]wireProcess `json:"processes"`
}

type wireResponse struct {
	Data []wireTrace `json:"data"`
}

var serviceOf = map[string]string{
	"checkout":          "checkout-api",
	"inventory.check":   "inventory-service",
	"db.query":          "inventory-service",
	"pricing.calculate": "pricing-service",
}

func toJaegerWire(traces []trace.Trace) wireResponse {
	var resp wireResponse
	for _, t := range traces {
		wt := wireTrace{TraceID: t.TraceID, Processes: map[string]wireProcess{}}
		for _, s := range t.Spans {
			svc := serviceOf[s.Name]
			pid := "p_" + svc
			wt.Processes[pid] = wireProcess{ServiceName: svc}

			ws := wireSpan{
				TraceID:       t.TraceID,
				SpanID:        s.SpanID,
				OperationName: s.Name,
				StartTime:     s.StartNano / 1000,
				Duration:      s.Duration() / 1000,
				ProcessID:     pid,
				References:    []wireRef{},
			}
			if s.ParentSpanID != "" {
				ws.References = append(ws.References, wireRef{
					RefType: "CHILD_OF", TraceID: t.TraceID, SpanID: s.ParentSpanID,
				})
			}
			wt.Spans = append(wt.Spans, ws)
		}
		resp.Data = append(resp.Data, wt)
	}
	return resp
}

func spec(dbMean, dbStdDev float64) synth.SpanSpec {
	return synth.SpanSpec{
		Name: "checkout", SelfMean: 2e6, SelfStdDev: 5e5,
		Children: []synth.SpanSpec{
			{
				Name: "inventory.check", SelfMean: 1e6, SelfStdDev: 3e5,
				Children: []synth.SpanSpec{
					{Name: "db.query", SelfMean: dbMean, SelfStdDev: dbStdDev},
				},
			},
			{Name: "pricing.calculate", SelfMean: 6e6, SelfStdDev: 1e6},
		},
	}
}

func TestPipelineOverJaegerWire(t *testing.T) {
	deploy := time.Unix(1700000000, 0).UTC()
	beforeWire := toJaegerWire(synth.Generate(spec(8e6, 2e6), 300, 1))
	afterWire := toJaegerWire(synth.Generate(spec(190e6, 40e6), 300, 2))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		end := r.URL.Query().Get("end")
		resp := afterWire
		if end == "1700000000000000" {
			resp = beforeWire
		}
		w.Header().Set("content-type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("encode: %v", err)
		}
	}))
	defer srv.Close()

	src := NewJaeger(srv.URL)
	ctx := context.Background()

	beforeTraces, err := src.Fetch(ctx, "checkout-api", deploy.Add(-time.Hour), deploy, 500)
	if err != nil {
		t.Fatalf("fetch before: %v", err)
	}
	afterTraces, err := src.Fetch(ctx, "checkout-api", deploy, deploy.Add(time.Hour), 500)
	if err != nil {
		t.Fatalf("fetch after: %v", err)
	}

	beforeSample, err := sample.Collect(beforeTraces)
	if err != nil {
		t.Fatalf("collect before: %v", err)
	}
	afterSample, err := sample.Collect(afterTraces)
	if err != nil {
		t.Fatalf("collect after: %v", err)
	}

	rep := bisect.Run(beforeSample, afterSample, stats.DefaultConfig())

	if len(rep.Findings) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(rep.Findings), rep.Findings)
	}
	want := "checkout-api:checkout>inventory-service:inventory.check>inventory-service:db.query"
	if rep.Findings[0].PathKey != want {
		t.Fatalf("culprit = %q, want %q", rep.Findings[0].PathKey, want)
	}
	if d := rep.Findings[0].Self.Delta; d < 170e6 || d > 195e6 {
		t.Errorf("self delta = %v, want roughly 182ms", d)
	}

	explained := map[string]bool{}
	for _, a := range rep.Findings[0].Explains {
		explained[a] = true
	}
	if !explained["checkout-api:checkout"] {
		t.Errorf("root not listed as collateral: %v", rep.Findings[0].Explains)
	}
}

func TestWireRoundTripPreservesSelfTimeWithinMicrosecondResolution(t *testing.T) {
	traces := synth.Generate(spec(8e6, 2e6), 1, 5)
	wire := toJaegerWire(traces)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(wire)
	}))
	defer srv.Close()

	got, err := NewJaeger(srv.URL).Fetch(context.Background(), "checkout-api", time.Now(), time.Now(), 10)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	origRoot, err := trace.BuildTree(traces[0])
	if err != nil {
		t.Fatalf("BuildTree original: %v", err)
	}
	gotRoot, err := trace.BuildTree(got[0])
	if err != nil {
		t.Fatalf("BuildTree fetched: %v", err)
	}

	const tolerance = 3000

	drift := origRoot.SelfTime() - gotRoot.SelfTime()
	if drift < 0 {
		drift = -drift
	}
	if drift > tolerance {
		t.Errorf("root self time %d -> %d, drift %dns exceeds microsecond truncation tolerance",
			origRoot.SelfTime(), gotRoot.SelfTime(), drift)
	}
	if len(origRoot.Children) != len(gotRoot.Children) {
		t.Fatalf("child count %d -> %d", len(origRoot.Children), len(gotRoot.Children))
	}
}
