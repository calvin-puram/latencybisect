package sample

import (
	"strings"
	"testing"

	"github.com/calvinpuram/latencybisect/pkg/trace"
)

func span(id, parent, name string, start, end int64) trace.Span {
	return trace.Span{SpanID: id, ParentSpanID: parent, Name: name, StartNano: start, EndNano: end}
}

func twoSpanTrace(id string, dbEnd int64) trace.Trace {
	return trace.Trace{
		TraceID: id,
		Spans: []trace.Span{
			span("s1", "", "checkout", 0, 100),
			span("s2", "s1", "db.query", 10, dbEnd),
		},
	}
}

func TestCollectBucketsByPath(t *testing.T) {
	s, err := Collect([]trace.Trace{
		twoSpanTrace("t1", 50),
		twoSpanTrace("t2", 60),
		twoSpanTrace("t3", 70),
	})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if s.Traces != 3 {
		t.Errorf("Traces = %d, want 3", s.Traces)
	}
	if len(s.ByPath) != 2 {
		t.Fatalf("got %d paths, want 2: %v", len(s.ByPath), keys(s))
	}

	db, ok := s.ByPath["checkout>db.query"]
	if !ok {
		t.Fatalf("missing path checkout>db.query, have %v", keys(s))
	}
	if db.Count() != 3 {
		t.Errorf("db.query observations = %d, want 3", db.Count())
	}
	if got := db.SelfTimes; got[0] != 40 || got[1] != 50 || got[2] != 60 {
		t.Errorf("db.query self times = %v, want [40 50 60]", got)
	}
	if got := db.Depth; got != 1 {
		t.Errorf("db.query depth = %d, want 1", got)
	}
}

func TestCollectSeparatesSameNameUnderDifferentParents(t *testing.T) {
	tr := trace.Trace{
		TraceID: "t1",
		Spans: []trace.Span{
			span("s1", "", "checkout", 0, 100),
			span("s2", "s1", "inventory", 0, 40),
			span("s3", "s2", "cache.get", 5, 15),
			span("s4", "s1", "pricing", 40, 90),
			span("s5", "s4", "cache.get", 45, 85),
		},
	}

	s, err := Collect([]trace.Trace{tr})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	inv, ok := s.ByPath["checkout>inventory>cache.get"]
	if !ok {
		t.Fatalf("missing inventory cache.get, have %v", keys(s))
	}
	pri, ok := s.ByPath["checkout>pricing>cache.get"]
	if !ok {
		t.Fatalf("missing pricing cache.get, have %v", keys(s))
	}
	if inv.SelfTimes[0] == pri.SelfTimes[0] {
		t.Error("two different cache.get spans collapsed into one bucket")
	}
	if inv.SelfTimes[0] != 10 || pri.SelfTimes[0] != 40 {
		t.Errorf("self times = %d and %d, want 10 and 40", inv.SelfTimes[0], pri.SelfTimes[0])
	}
}

func TestCollectRecordsBothSelfAndTotal(t *testing.T) {
	s, err := Collect([]trace.Trace{twoSpanTrace("t1", 50)})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	root := s.ByPath["checkout"]
	if root.Durations[0] != 100 {
		t.Errorf("root duration = %d, want 100", root.Durations[0])
	}
	if root.SelfTimes[0] != 60 {
		t.Errorf("root self time = %d, want 60", root.SelfTimes[0])
	}
}

func TestCollectHandlesRaggedTraces(t *testing.T) {
	withCache := trace.Trace{
		TraceID: "t2",
		Spans: []trace.Span{
			span("s1", "", "checkout", 0, 100),
			span("s2", "s1", "db.query", 10, 50),
			span("s3", "s1", "cache.get", 60, 70),
		},
	}

	s, err := Collect([]trace.Trace{twoSpanTrace("t1", 50), withCache})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if got := s.ByPath["checkout>db.query"].Count(); got != 2 {
		t.Errorf("db.query seen %d times, want 2", got)
	}
	if got := s.ByPath["checkout>cache.get"].Count(); got != 1 {
		t.Errorf("cache.get seen %d times, want 1", got)
	}
	if s.Traces != 2 {
		t.Errorf("Traces = %d, want 2", s.Traces)
	}
}

func TestCollectPropagatesTreeErrors(t *testing.T) {
	bad := trace.Trace{
		TraceID: "t1",
		Spans:   []trace.Span{span("s1", "nope", "orphan", 0, 10)},
	}

	_, err := Collect([]trace.Trace{bad})
	if err == nil || !strings.Contains(err.Error(), "missing parent") {
		t.Fatalf("err = %v, want missing parent", err)
	}
}

func TestCollectEmptyInput(t *testing.T) {
	s, err := Collect(nil)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if s.Traces != 0 || len(s.ByPath) != 0 {
		t.Errorf("got %d traces and %d paths, want empty sample", s.Traces, len(s.ByPath))
	}
}

func TestParentPath(t *testing.T) {
	cases := []struct{ in, want string }{
		{"checkout>inventory.check>db.query", "checkout>inventory.check"},
		{"checkout>inventory.check", "checkout"},
		{"checkout", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := ParentPath(tc.in); got != tc.want {
			t.Errorf("ParentPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func keys(s *Sample) []string {
	out := make([]string, 0, len(s.ByPath))
	for k := range s.ByPath {
		out = append(out, k)
	}
	return out
}
