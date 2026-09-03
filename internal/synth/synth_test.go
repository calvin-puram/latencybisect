package synth

import (
	"math"
	"testing"

	"github.com/calvinpuram/latencybisect/pkg/trace"
)

func TestGeneratedSelfTimesRoundTrip(t *testing.T) {
	spec := SpanSpec{
		Name: "checkout", SelfMean: 2e6, SelfStdDev: 0,
		Children: []SpanSpec{
			{Name: "inventory.check", SelfMean: 1e6, SelfStdDev: 0,
				Children: []SpanSpec{{Name: "db.query", SelfMean: 8e6, SelfStdDev: 0}}},
			{Name: "pricing.calculate", SelfMean: 6e6, SelfStdDev: 0},
		},
	}

	traces := Generate(spec, 1, 1)
	root, err := trace.BuildTree(traces[0])
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	want := map[string]int64{
		"checkout":          2e6,
		"inventory.check":   1e6,
		"db.query":          8e6,
		"pricing.calculate": 6e6,
	}
	root.Walk(func(n *trace.Node) {
		if got := n.SelfTime(); got != want[n.Span.Name] {
			t.Errorf("%s self time = %d, want %d", n.Span.Name, got, want[n.Span.Name])
		}
	})

	if got := root.Span.Duration(); got != 17e6 {
		t.Errorf("root duration = %d, want 17000000", got)
	}
}

func TestGeneratedChildrenNestWithinParent(t *testing.T) {
	spec := SpanSpec{
		Name: "root", SelfMean: 5e6, SelfStdDev: 1e6,
		Children: []SpanSpec{
			{Name: "a", SelfMean: 10e6, SelfStdDev: 2e6},
			{Name: "b", SelfMean: 20e6, SelfStdDev: 3e6},
		},
	}

	for _, tr := range Generate(spec, 50, 7) {
		root, err := trace.BuildTree(tr)
		if err != nil {
			t.Fatalf("BuildTree: %v", err)
		}
		var prevEnd int64
		for _, c := range root.Children {
			if c.Span.StartNano < root.Span.StartNano || c.Span.EndNano > root.Span.EndNano {
				t.Fatalf("child %s escapes parent bounds", c.Span.Name)
			}
			if c.Span.StartNano < prevEnd {
				t.Fatalf("child %s overlaps previous sibling", c.Span.Name)
			}
			prevEnd = c.Span.EndNano
		}
	}
}

func TestGenerateIsDeterministic(t *testing.T) {
	spec := SpanSpec{Name: "root", SelfMean: 10e6, SelfStdDev: 2e6}
	a := Generate(spec, 10, 42)
	b := Generate(spec, 10, 42)

	for i := range a {
		if a[i].Spans[0].EndNano != b[i].Spans[0].EndNano {
			t.Fatalf("trace %d differs between runs with same seed", i)
		}
	}
}

func TestGeneratedDistributionMatchesSpec(t *testing.T) {
	spec := SpanSpec{Name: "root", SelfMean: 100e6, SelfStdDev: 10e6}
	traces := Generate(spec, 2000, 3)

	var sum float64
	for _, tr := range traces {
		sum += float64(tr.Spans[0].Duration())
	}
	mean := sum / float64(len(traces))

	if math.Abs(mean-100e6) > 2e6 {
		t.Errorf("mean = %v, want within 2ms of 100ms", mean)
	}
}
