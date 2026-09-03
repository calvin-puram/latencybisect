package trace

import (
	"strings"
	"testing"
)

func span(id, parent, name string, start, end int64) Span {
	return Span{SpanID: id, ParentSpanID: parent, Name: name, StartNano: start, EndNano: end}
}

func sequentialTrace() Trace {
	return Trace{
		TraceID: "t1",
		Spans: []Span{
			span("s1", "", "checkout", 0, 100),
			span("s2", "s1", "inventory.check", 10, 60),
			span("s3", "s2", "db.query", 15, 55),
			span("s4", "s1", "pricing.calculate", 60, 90),
		},
	}
}

func TestBuildTree(t *testing.T) {
	root, err := BuildTree(sequentialTrace())
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}
	if root.Span.Name != "checkout" {
		t.Fatalf("root = %q, want checkout", root.Span.Name)
	}
	if got := len(root.Children); got != 2 {
		t.Fatalf("root children = %d, want 2", got)
	}
	inv := root.Children[0]
	if inv.Parent != root {
		t.Error("child parent pointer not set")
	}
	if got := inv.Children[0].Span.Name; got != "db.query" {
		t.Errorf("grandchild = %q, want db.query", got)
	}
}

func TestSelfTime(t *testing.T) {
	root, err := BuildTree(sequentialTrace())
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	want := map[string]int64{
		"checkout":          20,
		"inventory.check":   10,
		"db.query":          40,
		"pricing.calculate": 30,
	}
	root.Walk(func(n *Node) {
		if got := n.SelfTime(); got != want[n.Span.Name] {
			t.Errorf("%s self time = %d, want %d", n.Span.Name, got, want[n.Span.Name])
		}
	})
}

func TestSelfTimeConcurrentChildrenClampsToZero(t *testing.T) {
	root, err := BuildTree(Trace{
		TraceID: "t1",
		Spans: []Span{
			span("s1", "", "fanout", 0, 35),
			span("s2", "s1", "a", 0, 30),
			span("s3", "s1", "b", 0, 30),
		},
	})
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}
	if got := root.SelfTime(); got != 0 {
		t.Errorf("self time = %d, want 0", got)
	}
}

func TestPathKeyDisambiguatesSameName(t *testing.T) {
	root, err := BuildTree(Trace{
		TraceID: "t1",
		Spans: []Span{
			span("s1", "", "checkout", 0, 100),
			span("s2", "s1", "inventory", 0, 40),
			span("s3", "s2", "cache.get", 5, 15),
			span("s4", "s1", "pricing", 40, 90),
			span("s5", "s4", "cache.get", 45, 55),
		},
	})
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	var keys []string
	root.Walk(func(n *Node) {
		if n.Span.Name == "cache.get" {
			keys = append(keys, n.PathKey())
		}
	})
	if len(keys) != 2 {
		t.Fatalf("found %d cache.get spans, want 2", len(keys))
	}
	if keys[0] == keys[1] {
		t.Fatalf("both cache.get spans share path key %q", keys[0])
	}
	if keys[0] != "checkout>inventory>cache.get" {
		t.Errorf("key = %q, want checkout>inventory>cache.get", keys[0])
	}
}

func TestDepth(t *testing.T) {
	root, err := BuildTree(sequentialTrace())
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}
	want := map[string]int{"checkout": 0, "inventory.check": 1, "db.query": 2, "pricing.calculate": 1}
	root.Walk(func(n *Node) {
		if got := n.Depth(); got != want[n.Span.Name] {
			t.Errorf("%s depth = %d, want %d", n.Span.Name, got, want[n.Span.Name])
		}
	})
}

func TestBuildTreeErrors(t *testing.T) {
	cases := []struct {
		name  string
		trace Trace
		want  string
	}{
		{"empty", Trace{TraceID: "t1"}, "no spans"},
		{"orphan", Trace{TraceID: "t1", Spans: []Span{
			span("s1", "", "root", 0, 10),
			span("s2", "missing", "orphan", 1, 5),
		}}, "missing parent"},
		{"multi root", Trace{TraceID: "t1", Spans: []Span{
			span("s1", "", "a", 0, 10),
			span("s2", "", "b", 0, 10),
		}}, "multiple roots"},
		{"duplicate id", Trace{TraceID: "t1", Spans: []Span{
			span("s1", "", "a", 0, 10),
			span("s1", "", "b", 0, 10),
		}}, "duplicate span id"},
		{"no root", Trace{TraceID: "t1", Spans: []Span{
			span("s1", "s2", "a", 0, 10),
			span("s2", "s1", "b", 0, 10),
		}}, "no root"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildTree(tc.trace)
			if err == nil {
				t.Fatalf("BuildTree succeeded, want error containing %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestParse(t *testing.T) {
	in := `[{"traceId":"t1","spans":[{"spanId":"s1","parentSpanId":"","name":"checkout","startNano":0,"endNano":100}]}]`
	traces, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(traces) != 1 {
		t.Fatalf("got %d traces, want 1", len(traces))
	}
	if got := traces[0].Spans[0].TraceID; got != "t1" {
		t.Errorf("span trace id = %q, want t1 backfilled from trace", got)
	}
}

func TestParseMissingTraceID(t *testing.T) {
	_, err := Parse(strings.NewReader(`[{"spans":[]}]`))
	if err == nil || !strings.Contains(err.Error(), "missing traceId") {
		t.Fatalf("err = %v, want missing traceId", err)
	}
}
